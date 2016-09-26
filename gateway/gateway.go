package gateway

import (
	"io"
	"log"
	"net"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/funny/link"
	"github.com/funny/slab"
)

const connBuckets = 32

type Gateway struct {
	protocol
	servers [2]*link.Server

	physicalConnID uint32
	physicalConns  [connBuckets][2]*link.Uint32Channel

	virtualConnID      uint32
	virtualConns       [connBuckets]map[uint32][2]*link.Session
	virtualConnMutexes [connBuckets]sync.RWMutex
}

func NewGateway(pool *slab.AtomPool, maxPacketSize int) *Gateway {
	var gateway Gateway

	gateway.pool = pool
	gateway.maxPacketSize = maxPacketSize

	for i := 0; i < connBuckets; i++ {
		gateway.virtualConns[i] = make(map[uint32][2]*link.Session)
	}

	for i := 0; i < connBuckets; i++ {
		gateway.physicalConns[i][0] = link.NewUint32Channel()
		gateway.physicalConns[i][1] = link.NewUint32Channel()
	}

	return &gateway
}

func (g *Gateway) addVirtualConn(connID uint32, pair [2]*link.Session) {
	bucket := connID % connBuckets
	g.virtualConnMutexes[bucket].Lock()
	defer g.virtualConnMutexes[bucket].Unlock()
	if _, exists := g.virtualConns[bucket][connID]; exists {
		panic("Virtual Connection Already Exists")
	}
	g.virtualConns[bucket][connID] = pair
}

func (g *Gateway) getVirtualConn(connID uint32) [2]*link.Session {
	bucket := connID % connBuckets
	g.virtualConnMutexes[bucket].RLock()
	defer g.virtualConnMutexes[bucket].RUnlock()
	return g.virtualConns[bucket][connID]
}

func (g *Gateway) delVirtualConn(connID uint32) ([2]*link.Session, bool) {
	bucket := connID % connBuckets
	g.virtualConnMutexes[bucket].Lock()
	pair, exists := g.virtualConns[bucket][connID]
	if exists {
		delete(g.virtualConns[bucket], connID)
	}
	g.virtualConnMutexes[bucket].Unlock()
	return pair, exists
}

func (g *Gateway) addPhysicalConn(connID uint32, side int, session *link.Session) {
	g.physicalConns[connID%connBuckets][side].Put(connID, session)
}

func (g *Gateway) getPhysicalConn(connID uint32, side int) *link.Session {
	return g.physicalConns[connID%connBuckets][side].Get(connID)
}

type gwState struct {
	sync.Mutex
	Disposed       bool
	PhysicalConnID uint32
	VirtualConns   map[uint32]struct{}
}

func (s *gwState) Dispose() {
	s.Lock()
	s.Disposed = true
	s.Unlock()
}

func (g *Gateway) ServeClients(lsn net.Listener, maxConn, bufferSize, sendChanSize, pingInterval int) {
	g.servers[0] = link.NewServer(lsn, link.ProtocolFunc(func(rw io.ReadWriter) (link.Codec, link.Context, error) {
		return g.newCodec(rw.(net.Conn), bufferSize), nil, nil
	}), sendChanSize)

	g.servers[0].Serve(link.HandlerFunc(func(session *link.Session, ctx link.Context, err error) {
		if err != nil {
			return
		}
		g.handleSession(session, atomic.AddUint32(&g.physicalConnID, 1), 0, pingInterval, maxConn)
	}))
}

func (g *Gateway) ServeServers(lsn net.Listener, key string, authTimeout, bufferSize, sendChanSize, pingInterval int) {
	g.servers[1] = link.NewServer(lsn, link.ProtocolFunc(func(rw io.ReadWriter) (link.Codec, link.Context, error) {
		serverID, err := g.serverAuth(rw.(net.Conn), []byte(key), time.Duration(authTimeout)*time.Second)
		if err != nil {
			return nil, nil, err
		}
		return g.newCodec(rw.(net.Conn), bufferSize), serverID, nil
	}), sendChanSize)

	g.servers[1].Serve(link.HandlerFunc(func(session *link.Session, ctx link.Context, err error) {
		if err != nil {
			return
		}
		g.handleSession(session, ctx.(uint32), 1, pingInterval, 0)
	}))
}

func (g *Gateway) Stop() {
	g.servers[0].Stop()
	g.servers[1].Stop()
}

func (g *Gateway) handleSession(session *link.Session, id uint32, side, pingInteval, maxConn int) {
	g.addPhysicalConn(id, side, session)

	var (
		health    = 2
		closeChan = make(chan int)
		pingTimer = time.NewTimer(time.Duration(pingInteval) * time.Second)
		otherSide = (side + 1) % 2
	)

	var state = gwState{
		PhysicalConnID: id,
		VirtualConns:   make(map[uint32]struct{}),
	}
	session.State = &state

	defer func() {
		close(closeChan)
		pingTimer.Stop()
		session.Close()
		state.Dispose()

		// Close releated virtual connections
		for connID := range state.VirtualConns {
			g.closeVirtualConn(connID)
		}

		// Free message buffers in send chan.
		close(session.SendChan())
		for msg := range session.SendChan() {
			g.free(*(msg.(*[]byte)))
		}

		if err := recover(); err != nil {
			log.Printf("Panic - %v", err)
			debug.PrintStack()
		}
	}()

	go func() {
		for {
			select {
			case <-pingTimer.C:
				if health--; health <= 0 || g.send(session, g.encodePingCmd()) != nil {
					session.Close()
					return
				}
			case <-closeChan:
				return
			}
		}
	}()

	for {
		buf, err := session.Receive()
		if err != nil {
			return
		}

		pingTimer.Reset(time.Duration(pingInteval) * time.Second)

		msg := *(buf.(*[]byte))
		connID := g.decodePacket(msg)

		if connID == 0 {
			switch g.decodeCmd(msg) {
			case newCmd:
				g.free(msg)
				var connID uint32
				for connID == 0 {
					connID = atomic.AddUint32(&g.virtualConnID, 1)
				}
				g.send(session, g.encodeOpenCmd(connID))
			case dialCmd:
				connID, remoteID := g.decodeDialCmd(msg)
				g.free(msg)
				var pair [2]*link.Session
				pair[side] = session
				pair[otherSide] = g.getPhysicalConn(remoteID, otherSide)
				if pair[otherSide] == nil || !g.acceptVirtualConn(connID, pair, session, maxConn) {
					g.send(session, g.encodeRefuseCmd(connID))
				}
			case closeCmd:
				connID := g.decodeCloseCmd(msg)
				g.free(msg)
				g.closeVirtualConn(connID)
			case pingCmd:
				g.free(msg)
				health = (health + 1) % 3
			default:
				g.free(msg)
				panic("Unsupported Gateway Command")
			}
			continue
		}

		pair := g.getVirtualConn(connID)

		if pair[side] != session {
			g.free(msg)
			panic("Virtual Endpoing Not Match")
		}

		if pair[otherSide] == nil {
			g.free(msg)
			continue
		}

		g.send(pair[otherSide], msg)
	}
}

func (g *Gateway) acceptVirtualConn(connID uint32, pair [2]*link.Session, session *link.Session, maxConn int) bool {
	for i := 0; i < 2; i++ {
		state := pair[i].State.(*gwState)

		state.Lock()
		defer state.Unlock()
		if state.Disposed {
			return false
		}

		if pair[i] == session && maxConn != 0 && len(state.VirtualConns) >= maxConn {
			return false
		}

		if _, exists := state.VirtualConns[connID]; exists {
			panic("Virtual Connection Already Exists")
		}

		state.VirtualConns[connID] = struct{}{}
	}

	g.addVirtualConn(connID, pair)

	for i := 0; i < 2; i++ {
		remoteID := pair[(i+1)%2].State.(*gwState).PhysicalConnID
		g.send(pair[i], g.encodeAcceptCmd(connID, remoteID))
	}
	return true
}

func (g *Gateway) closeVirtualConn(connID uint32) {
	pair, ok := g.delVirtualConn(connID)
	if !ok {
		return
	}

	for i := 0; i < 2; i++ {
		state := pair[i].State.(*gwState)
		state.Lock()
		defer state.Unlock()
		if state.Disposed {
			return
		}
		delete(state.VirtualConns, connID)
	}

	for i := 0; i < 2; i++ {
		g.send(pair[i], g.encodeCloseCmd(connID))
	}
}

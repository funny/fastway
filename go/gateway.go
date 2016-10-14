package fastway

import (
	"fmt"
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

type GatewayCfg struct {
	MaxConn      int
	BufferSize   int
	SendChanSize int
	IdleTimeout  time.Duration
	AuthKey      string
}

// Gateway implements gateway protocol.
type Gateway struct {
	protocol
	timer   *timingWheel
	servers [2]*link.Server

	physicalConnID uint32
	physicalConns  [connBuckets][2]*link.Uint32Channel

	virtualConnID      uint32
	virtualConns       [connBuckets]map[uint32][2]*link.Session
	virtualConnMutexes [connBuckets]sync.RWMutex
}

// NewGateway create a gateway.
func NewGateway(pool slab.Pool, maxPacketSize int) *Gateway {
	var gateway Gateway

	gateway.pool = pool
	gateway.maxPacketSize = maxPacketSize
	gateway.timer = newTimingWheel(time.Second, 1800)

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

// ServeClients serve client connections.
func (g *Gateway) ServeClients(lsn net.Listener, cfg GatewayCfg) {
	g.servers[0] = link.NewServer(lsn, link.ProtocolFunc(func(rw io.ReadWriter) (link.Codec, link.Context, error) {
		return g.newCodec(rw.(net.Conn), cfg.BufferSize), nil, nil
	}), cfg.SendChanSize)

	g.servers[0].Serve(link.HandlerFunc(func(session *link.Session, ctx link.Context, err error) {
		g.handleSession(atomic.AddUint32(&g.physicalConnID, 1), session, 0, cfg.MaxConn, cfg.IdleTimeout)
	}))
}

// ServeServers serve server connections.
func (g *Gateway) ServeServers(lsn net.Listener, cfg GatewayCfg) {
	g.servers[1] = link.NewServer(lsn, link.ProtocolFunc(func(rw io.ReadWriter) (link.Codec, link.Context, error) {
		serverID, err := g.serverAuth(rw.(net.Conn), []byte(cfg.AuthKey))
		if err != nil {
			log.Printf("error happends when accept server from %s: %s", rw.(net.Conn).RemoteAddr(), err)
			return nil, nil, err
		}
		log.Printf("accept server %d from %s", serverID, rw.(net.Conn).RemoteAddr())
		return g.newCodec(rw.(net.Conn), cfg.BufferSize), serverID, nil
	}), cfg.SendChanSize)

	g.servers[1].Serve(link.HandlerFunc(func(session *link.Session, ctx link.Context, err error) {
		g.handleSession(ctx.(uint32), session, 1, 0, cfg.IdleTimeout)
	}))
}

// Stop gateway.
func (g *Gateway) Stop() {
	g.servers[0].Stop()
	g.servers[1].Stop()
	g.timer.Stop()
}

type gwState struct {
	sync.Mutex
	id           uint32
	gateway      *Gateway
	session      *link.Session
	lastActive   int64
	pingChan     chan struct{}
	watchChan    chan struct{}
	disposeChan  chan struct{}
	disposeOnce  sync.Once
	disposed     bool
	virtualConns map[uint32]struct{}
}

func (g *Gateway) newSessionState(id uint32, session *link.Session, idleTimeout time.Duration) *gwState {
	gs := &gwState{
		id:           id,
		session:      session,
		gateway:      g,
		watchChan:    make(chan struct{}),
		pingChan:     make(chan struct{}),
		disposeChan:  make(chan struct{}),
		virtualConns: make(map[uint32]struct{}),
	}
	go gs.watcher(session, idleTimeout)
	return gs
}

func (gs *gwState) Dispose() {
	gs.disposeOnce.Do(func() {
		close(gs.disposeChan)
		gs.session.Close()

		gs.Lock()
		gs.disposed = true
		gs.Unlock()

		// Close releated virtual connections
		for connID := range gs.virtualConns {
			gs.gateway.closeVirtualConn(connID)
		}

		// Free message buffers in send chan.
		close(gs.session.SendChan())
		for msg := range gs.session.SendChan() {
			gs.gateway.free(*(msg.(*[]byte)))
		}
	})
}

func (gs *gwState) watcher(session *link.Session, idleTimeout time.Duration) {
L:
	for {
		select {
		case <-gs.pingChan:
		case <-gs.gateway.timer.After(idleTimeout):
			if time.Since(time.Unix(atomic.LoadInt64(&gs.lastActive), 0)) >= idleTimeout {
				break L
			}
		case <-gs.disposeChan:
			break L
		}
	}
	gs.Dispose()
}

func (g *Gateway) handleSession(id uint32, session *link.Session, side, maxConn int, idleTimeout time.Duration) {
	state := g.newSessionState(id, session, idleTimeout)
	session.State = state
	g.addPhysicalConn(id, side, session)

	defer func() {
		state.Dispose()

		if err := recover(); err != nil {
			log.Printf("fast/gateway.Gateway panic: %v\n%s", err, debug.Stack())
		}
	}()

	otherSide := (side + 1) % 2

	for {
		atomic.StoreInt64(&state.lastActive, time.Now().Unix())

		buf, err := session.Receive()
		if err != nil {
			return
		}

		msg := *(buf.(*[]byte))
		connID := g.decodePacket(msg)
		if connID == 0 {
			g.processCmd(msg, session, state, side, otherSide, maxConn)
			continue
		}

		pair := g.getVirtualConn(connID)
		if pair[side] == nil || pair[otherSide] == nil {
			g.free(msg)
			g.send(session, g.encodeCloseCmd(connID))
			continue
		}
		if pair[side] != session {
			g.free(msg)
			panic("endpoint not match")
		}
		g.send(pair[otherSide], msg)
	}
}

func (g *Gateway) processCmd(msg []byte, session *link.Session, state *gwState, side, otherSide, maxConn int) {
	switch g.decodeCmd(msg) {
	case dialCmd:
		remoteID := g.decodeDialCmd(msg)
		g.free(msg)

		var pair [2]*link.Session
		pair[side] = session
		pair[otherSide] = g.getPhysicalConn(remoteID, otherSide)
		if pair[otherSide] == nil || !g.acceptVirtualConn(pair, session, maxConn) {
			g.send(session, g.encodeRefuseCmd(remoteID))
		}

	case closeCmd:
		connID := g.decodeCloseCmd(msg)
		g.free(msg)
		g.closeVirtualConn(connID)

	case pingCmd:
		state.pingChan <- struct{}{}
		g.free(msg)
		g.send(session, g.encodePingCmd())

	default:
		g.free(msg)
		panic(fmt.Sprintf("Unsupported Gateway Command: %d", g.decodeCmd(msg)))
	}
}

func (g *Gateway) acceptVirtualConn(pair [2]*link.Session, session *link.Session, maxConn int) bool {
	var connID uint32
	for connID == 0 {
		connID = atomic.AddUint32(&g.virtualConnID, 1)
	}

	for i := 0; i < 2; i++ {
		state := pair[i].State.(*gwState)

		state.Lock()
		defer state.Unlock()
		if state.disposed {
			return false
		}

		if pair[i] == session && maxConn != 0 && len(state.virtualConns) >= maxConn {
			return false
		}

		if _, exists := state.virtualConns[connID]; exists {
			panic("Virtual Connection Already Exists")
		}

		state.virtualConns[connID] = struct{}{}
	}

	g.addVirtualConn(connID, pair)

	for i := 0; i < 2; i++ {
		remoteID := pair[(i+1)%2].State.(*gwState).id
		if pair[i] == session {
			g.send(pair[i], g.encodeAcceptCmd(connID, remoteID))
		} else {
			g.send(pair[i], g.encodeConnectCmd(connID, remoteID))
		}
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
		if state.disposed {
			continue
		}
		delete(state.virtualConns, connID)
		g.send(pair[i], g.encodeCloseCmd(connID))
	}
}

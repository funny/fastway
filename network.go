package main

import (
	"log"
	"net"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fastgo/gateway/gateway"
	"github.com/fastgo/reuseport"
	"github.com/funny/link"
)

// Listen setup a listener.
func Listen(addr string) net.Listener {
	var lsn net.Listener
	var err error

	if *ReusePort {
		lsn, err = reuseport.NewReusablePortListener("tcp", *ClientAddr)
	} else {
		lsn, err = net.Listen("tcp", *ClientAddr)
	}

	if err != nil {
		log.Fatalf("Setup listener at %s failed - %s", addr, err)
	}
	return lsn
}

// ServeClients setup a server to serve client connections.
func ServeClients() {
	server := link.NewServer(Listen(*ClientAddr), link.ProtocolFunc(Protocol.NewClientCodec), *ClientSendChanSize)

	server.Serve(link.HandlerFunc(
		func(session *link.Session, ctx link.Context, err error) {
			if err != nil {
				return
			}
			HandleSession(session, atomic.AddUint32(&PhysicalConnID, 1), 0, *ClientPingInterval)
		},
	))
}

// ServeServers setup a server to serve server connections.
func ServeServers() {
	server := link.NewServer(Listen(*ServerAddr), link.ProtocolFunc(Protocol.NewServerCodec), *ServerSendChanSize)

	server.Serve(link.HandlerFunc(
		func(session *link.Session, ctx link.Context, err error) {
			if err != nil {
				return
			}
			HandleSession(session, ctx.(uint32), 1, *ServerPingInterval)
		},
	))
}

// State is gateway session state.
type State struct {
	sync.Mutex
	Disposed       bool
	PhysicalConnID uint32
	VirtualConns   map[uint32]struct{}
}

// Dispose marks session state disposed.
func (s *State) Dispose() {
	s.Lock()
	s.Disposed = true
	s.Unlock()
}

// HandleSession handle gateway sessions.
func HandleSession(session *link.Session, id uint32, side, pingInteval int) {
	AddPhysicalConn(id, side, session)

	var (
		health    = 2
		closeChan = make(chan int)
		pingTimer = time.NewTimer(time.Duration(pingInteval) * time.Second)
		otherSide = (side + 1) % 2
	)

	var state = State{
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
			CloseVirtualConn(connID)
		}

		// Free message buffers in send chan.
		close(session.SendChan())
		for msg := range session.SendChan() {
			Protocol.Pool.Free(*(msg.(*[]byte)))
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
				if health--; health <= 0 || Protocol.SendPingCmd(session) != nil {
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
		connID := Protocol.DecodePacket(msg)

		if connID == 0 {
			switch Protocol.DecodeCmd(msg) {
			case gateway.DialCmd:
				HandleDialCmd(session, side, otherSide, msg)
			case gateway.CloseCmd:
				HandleCloseCmd(msg)
			case gateway.PingCmd:
				health = (health + 1) % 3
				Protocol.Pool.Free(msg)
			default:
				Protocol.Pool.Free(msg)
				panic("Unsupported Gateway Command")
			}
			continue
		}

		remote := GetVirtualConn(connID, otherSide)
		if remote == nil {
			continue
		}
		Protocol.Send(remote, msg)
	}
}

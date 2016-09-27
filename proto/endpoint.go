package proto

import (
	"errors"
	"log"
	"net"
	"runtime/debug"
	"sync"
	"time"

	"github.com/funny/link"
	"github.com/funny/slab"
)

var ErrRefused = errors.New("virtual connection refused")

func DialClient(addr string, pool *slab.AtomPool, maxPacketSize, bufferSize, sendChanSize int) (*Endpoint, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	ep := newEndpoint(pool, maxPacketSize)
	ep.session = link.NewSession(ep.newCodec(conn, bufferSize), sendChanSize)
	go ep.loop()
	return ep, nil
}

func DialServer(addr string, pool *slab.AtomPool, serverID uint32, key string, authTimeout, maxPacketSize, bufferSize, sendChanSize int) (*Endpoint, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	ep := newEndpoint(pool, maxPacketSize)
	if err := ep.serverInit(conn, serverID, []byte(key), time.Duration(authTimeout)*time.Second); err != nil {
		return nil, err
	}
	ep.session = link.NewSession(ep.newCodec(conn, bufferSize), sendChanSize)
	go ep.loop()
	return ep, nil
}

type vconn struct {
	Session  *link.Session
	ConnID   uint32
	RemoteID uint32
}

type Endpoint struct {
	protocol
	session      *link.Session
	newConnMutex sync.Mutex
	newConnChan  chan uint32
	dialMutex    sync.Mutex
	dialChan     chan vconn
	acceptChan   chan vconn
	virtualConns *link.Uint32Channel
}

func newEndpoint(pool *slab.AtomPool, maxPacketSize int) *Endpoint {
	return &Endpoint{
		protocol: protocol{
			pool:          pool,
			maxPacketSize: maxPacketSize,
		},
		newConnChan:  make(chan uint32),
		acceptChan:   make(chan vconn),
		virtualConns: link.NewUint32Channel(),
	}
}

func (p *Endpoint) Accept() (session *link.Session, connID, remoteID uint32, err error) {
	conn := <-p.acceptChan
	return conn.Session, conn.ConnID, conn.RemoteID, nil
}

func (p *Endpoint) Dial(remoteID uint32) (*link.Session, uint32, error) {
	p.dialMutex.Lock()
	defer p.dialMutex.Unlock()

	if err := p.send(p.session, p.encodeNewCmd()); err != nil {
		return nil, 0, err
	}

	connID := <-p.newConnChan
	if connID == 0 {
		return nil, 0, ErrRefused
	}

	p.dialChan = make(chan vconn, 1)
	if err := p.send(p.session, p.encodeDialCmd(connID, remoteID)); err != nil {
		return nil, 0, err
	}
	conn := <-p.dialChan
	p.dialChan = nil

	if conn.Session == nil {
		return nil, 0, ErrRefused
	}
	return conn.Session, conn.ConnID, nil
}

func (p *Endpoint) addVirtualConn(connID, remoteID uint32) {
	codec := p.newVirtualCodec(p.session, connID)
	session := link.NewSession(codec, 0)
	p.virtualConns.Put(connID, session)

	if p.dialChan != nil {
		p.dialChan <- vconn{session, connID, remoteID}
		return
	}

	select {
	case p.acceptChan <- vconn{session, connID, remoteID}:
	default:
		p.send(p.session, p.encodeCloseCmd(connID))
	}
}

func (p *Endpoint) loop() {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("fast/gateway.Endpoint panic - %v", err)
			debug.PrintStack()
		}
	}()
	for {
		msg, err := p.session.Receive()
		if err != nil {
			return
		}

		buf := *(msg.(*[]byte))
		connID := p.decodePacket(buf)

		if connID == 0 {
			cmd := p.decodeCmd(buf)
			switch cmd {
			case openCmd:
				connID := p.decodeOpenCmd(buf)
				p.free(buf)
				p.newConnChan <- connID

			case refuseCmd:
				connID := p.decodeRefuseCmd(buf)
				p.free(buf)
				p.dialChan <- vconn{nil, connID, 0}

			case acceptCmd:
				connID, remoteID := p.decodeAcceptCmd(buf)
				p.free(buf)
				p.addVirtualConn(connID, remoteID)

			case closeCmd:
				connID := p.decodeCloseCmd(buf)
				p.free(buf)
				vconn := p.virtualConns.Get(connID)
				if vconn != nil {
					vconn.Close()
				}

			case pingCmd:
				p.free(buf)
				p.send(p.session, p.encodePingCmd())

			default:
				p.free(buf)
				panic("unsupported command")
			}
			continue
		}

		vconn := p.virtualConns.Get(connID)
		if vconn != nil {
			select {
			case vconn.Codec().(*virtualCodec).recvChan <- buf:
				continue
			default:
				vconn.Close()
			}
		}
		p.free(buf)
		p.send(p.session, p.encodeCloseCmd(connID))
	}
}

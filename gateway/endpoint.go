package gateway

import (
	"errors"
	"net"
	"sync"
	"time"

	"github.com/funny/link"
	"github.com/funny/slab"
)

var ErrRefused = errors.New("virtual connection refused")

func DialClient(addr string, format MsgFormat, pool *slab.AtomPool, maxPacketSize, bufferSize, sendChanSize int) (*Endpoint, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	ep := newEndpoint(pool, maxPacketSize, format)
	ep.session = link.NewSession(ep.newCodec(conn, bufferSize), sendChanSize)
	go ep.loop()
	return ep, nil
}

func DialServer(addr string, format MsgFormat, pool *slab.AtomPool, serverID uint32, key string, authTimeout, maxPacketSize, bufferSize, sendChanSize int) (*Endpoint, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	ep := newEndpoint(pool, maxPacketSize, format)
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
	format       MsgFormat
	session      *link.Session
	newConnMutex sync.Mutex
	newConnChan  chan uint32
	dialChans    map[uint32]chan vconn
	acceptChan   chan vconn
	virtualConns *link.Uint32Channel
}

func newEndpoint(pool *slab.AtomPool, maxPacketSize int, format MsgFormat) *Endpoint {
	return &Endpoint{
		protocol: protocol{
			pool:          pool,
			maxPacketSize: maxPacketSize,
		},
		format:       format,
		newConnChan:  make(chan uint32),
		dialChans:    make(map[uint32]chan vconn),
		acceptChan:   make(chan vconn),
		virtualConns: link.NewUint32Channel(),
	}
}

func (p *Endpoint) Accept() (session *link.Session, connID, remoteID uint32, err error) {
	conn := <-p.acceptChan
	return conn.Session, conn.ConnID, conn.RemoteID, nil
}

func (p *Endpoint) Dial(remoteID uint32) (*link.Session, uint32, error) {
	c, connID, err := p.newConn()
	if err != nil {
		return nil, 0, err
	}

	if err := p.send(p.session, p.encodeDialCmd(connID, remoteID)); err != nil {
		p.newConnMutex.Lock()
		defer p.newConnMutex.Unlock()
		delete(p.dialChans, connID)
		return nil, 0, err
	}

	conn := <-c
	if conn.ConnID == 0 {
		return nil, 0, ErrRefused
	}
	return conn.Session, conn.ConnID, nil
}

func (p *Endpoint) newConn() (c chan vconn, connID uint32, err error) {
	p.newConnMutex.Lock()
	defer p.newConnMutex.Unlock()

	if err = p.send(p.session, p.encodeNewCmd()); err != nil {
		return
	}

	connID = <-p.newConnChan
	if connID == 0 {
		return nil, 0, ErrRefused
	}

	c = make(chan vconn)
	p.dialChans[connID] = c
	return
}

func (p *Endpoint) addVirtualConn(connID, remoteID uint32) *link.Session {
	codec := p.newVirtualCodec(p.session, connID, p.format)
	session := link.NewSession(codec, 0)
	p.virtualConns.Put(connID, session)
	p.newConnMutex.Lock()
	defer p.newConnMutex.Unlock()
	if c, exists := p.dialChans[connID]; exists {
		delete(p.dialChans, connID)
		c <- vconn{session, connID, remoteID}
		return nil
	}
	return session
}

func (p *Endpoint) loop() {
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
				p.dialChans[connID] <- vconn{nil, 0, 0}
			case acceptCmd:
				connID, remoteID := p.decodeAcceptCmd(buf)
				p.free(buf)
				if session := p.addVirtualConn(connID, remoteID); session != nil {
					select {
					case p.acceptChan <- vconn{session, connID, remoteID}:
					default:
						p.send(p.session, p.encodeCloseCmd(connID))
					}
				}
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
			default:
				vconn.Close()
				p.send(p.session, p.encodeCloseCmd(connID))
			}
		} else {
			p.send(p.session, p.encodeCloseCmd(connID))
		}
	}
}

package proto

import (
	"errors"
	"io"
	"log"
	"net"
	"runtime/debug"
	"sync"
	"time"

	"github.com/funny/link"
	"github.com/funny/slab"
)

// ErrRefused happens when virtual connection couldn't dial to remote endpoint.
var ErrRefused = errors.New("virtual connection refused")

// DialClient dial to gateway and return a client endpoint.
// addr is the target gateway's client address.
// pool used to pooling message buffers.
// maxPacketSize limits max packet size.
// bufferSize settings bufio.Reader memory usage.
// sendChanSize settings async sending behavior.
func DialClient(addr string, pool slab.Pool, maxPacketSize, bufferSize, sendChanSize int) (*Endpoint, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	ep := newEndpoint(pool, maxPacketSize)
	ep.session = link.NewSession(ep.newCodec(conn, bufferSize), sendChanSize)
	go ep.loop()
	return ep, nil
}

// DialServer dial to gateway and return a server endpoint.
// addr is the target gateway's server address.
// pool used to pooling message buffers.
// serverID is the server ID of current server.
// key is the auth key used in server handshake.
// authTimeout is the IO waiting timeout when server handshake.
// maxPacketSize limits max packet size.
// bufferSize settings bufio.Reader memory usage.
// sendChanSize settings async sending behavior.
func DialServer(addr string, pool slab.Pool, serverID uint32, key string, authTimeout, maxPacketSize, bufferSize, sendChanSize int) (*Endpoint, error) {
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

// Endpoint is can be a client or a server.
type Endpoint struct {
	protocol
	session      *link.Session
	newConnMutex sync.Mutex
	newConnChan  chan uint32
	dialMutex    sync.Mutex
	dialChan     chan vconn
	acceptChan   chan vconn
	virtualConns *link.Uint32Channel
	closeChan    chan struct{}
	closeOnce    sync.Once
}

func newEndpoint(pool slab.Pool, maxPacketSize int) *Endpoint {
	return &Endpoint{
		protocol: protocol{
			pool:          pool,
			maxPacketSize: maxPacketSize,
		},
		newConnChan:  make(chan uint32),
		acceptChan:   make(chan vconn),
		virtualConns: link.NewUint32Channel(),
		closeChan:    make(chan struct{}),
	}
}

// Accept accept a virtual connection.
func (p *Endpoint) Accept() (session *link.Session, connID, remoteID uint32, err error) {
	select {
	case conn := <-p.acceptChan:
		return conn.Session, conn.ConnID, conn.RemoteID, nil
	case <-p.closeChan:
		return nil, 0, 0, io.EOF
	}
}

// Dial create a virtual connection and dial to a remote endpoint.
func (p *Endpoint) Dial(remoteID uint32) (*link.Session, uint32, error) {
	p.dialMutex.Lock()
	defer p.dialMutex.Unlock()

	if err := p.send(p.session, p.encodeNewCmd()); err != nil {
		return nil, 0, err
	}

	var connID uint32
	select {
	case connID = <-p.newConnChan:
	case <-p.closeChan:
		return nil, 0, io.EOF
	}
	if connID == 0 {
		return nil, 0, ErrRefused
	}

	var conn vconn
	p.dialChan = make(chan vconn, 1)
	if err := p.send(p.session, p.encodeDialCmd(connID, remoteID)); err != nil {
		return nil, 0, err
	}
	select {
	case conn = <-p.dialChan:
	case <-p.closeChan:
		return nil, 0, io.EOF
	}
	p.dialChan = nil

	if conn.Session == nil {
		return nil, 0, ErrRefused
	}
	return conn.Session, conn.ConnID, nil
}

// Close endpoint.
func (p *Endpoint) Close() {
	p.closeOnce.Do(func() {
		p.session.Close()
		close(p.closeChan)
	})
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
	case <-p.closeChan:
	default:
		p.send(p.session, p.encodeCloseCmd(connID))
	}
}

func (p *Endpoint) loop() {
	defer func() {
		p.Close()
		if err := recover(); err != nil {
			log.Printf("fast/gateway.Endpoint panic: %v\n%s", err, debug.Stack())
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
			p.processCmd(buf)
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

func (p *Endpoint) processCmd(buf []byte) {
	cmd := p.decodeCmd(buf)
	switch cmd {
	case openCmd:
		connID := p.decodeOpenCmd(buf)
		p.free(buf)
		select {
		case p.newConnChan <- connID:
		case <-p.closeChan:
			return
		}

	case refuseCmd:
		connID := p.decodeRefuseCmd(buf)
		p.free(buf)
		select {
		case p.dialChan <- vconn{nil, connID, 0}:
		case <-p.closeChan:
			return
		}

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
}

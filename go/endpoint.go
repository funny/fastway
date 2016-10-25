package fastway

import (
	"errors"
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

var EndPointTimer = newTimingWheel(time.Second, 1800)

// ErrRefused happens when virtual connection couldn't dial to remote EndPoint.
var ErrRefused = errors.New("virtual connection refused")

// EndPointCfg used to config EndPoint.
type EndPointCfg struct {
	MemPool         slab.Pool
	MaxPacket       int
	BufferSize      int
	SendChanSize    int
	RecvChanSize    int
	PingInterval    time.Duration
	PingTimeout     time.Duration
	TimeoutCallback func()
	ServerID        uint32
	AuthKey         string
}

// DialClient dial to gateway and return a client EndPoint.
// addr is the gateway address.
func DialClient(network, addr string, cfg EndPointCfg) (*EndPoint, error) {
	conn, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	return NewClient(conn, cfg)
}

// DialServer dial to gateway and return a server EndPoint.
// addr is the gateway address.
func DialServer(network, addr string, cfg EndPointCfg) (*EndPoint, error) {
	conn, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	return NewServer(conn, cfg)
}

// NewClient dial to gateway and return a client EndPoint.
// conn is the physical connection.
func NewClient(conn net.Conn, cfg EndPointCfg) (*EndPoint, error) {
	ep := newEndPoint(cfg.MemPool, cfg.MaxPacket, cfg.RecvChanSize)
	ep.session = link.NewSession(ep.newCodec(0, conn, cfg.BufferSize), cfg.SendChanSize)
	go ep.loop()
	go ep.keepalive(cfg.PingInterval, cfg.PingTimeout, cfg.TimeoutCallback)
	return ep, nil
}

// NewServer dial to gateway and return a server EndPoint.
// conn is the physical connection.
func NewServer(conn net.Conn, cfg EndPointCfg) (*EndPoint, error) {
	ep := newEndPoint(cfg.MemPool, cfg.MaxPacket, cfg.RecvChanSize)
	if err := ep.serverInit(conn, cfg.ServerID, []byte(cfg.AuthKey)); err != nil {
		return nil, err
	}
	ep.session = link.NewSession(ep.newCodec(0, conn, cfg.BufferSize), cfg.SendChanSize)
	go ep.loop()
	go ep.keepalive(cfg.PingInterval, cfg.PingTimeout, cfg.TimeoutCallback)
	return ep, nil
}

type Session struct {
	base     *link.Session
	connID   uint32
	remoteID uint32

	State interface{}
}

func (s *Session) ID() uint64 {
	return s.base.ID()
}

func (s *Session) ConnID() uint32 {
	return s.connID
}

func (s *Session) RemoteID() uint32 {
	return s.remoteID
}

func (s *Session) Receive() ([]byte, error) {
	msg, err := s.base.Receive()
	if err != nil {
		return nil, err
	}
	return *(msg.(*[]byte)), nil
}

func (s *Session) Send(msg []byte) error {
	return s.base.Send(&msg)
}

func (s *Session) IsClosed() bool {
	return s.base.IsClosed()
}

func (s *Session) Close() error {
	return s.base.Close()
}

func (s *Session) AddCloseCallback(handler, key interface{}, callback func()) {
	s.base.AddCloseCallback(handler, key, callback)
}

func (s *Session) RemoveCloseCallback(handler, key interface{}) {
	s.base.RemoveCloseCallback(handler, key)
}

// EndPoint is can be a client or a server.
type EndPoint struct {
	protocol
	sessions     map[uint64]*Session
	recvChanSize int
	session      *link.Session
	lastActive   int64
	newConnMutex sync.Mutex
	newConnChan  chan uint32
	dialMutex    sync.Mutex
	acceptChan   chan *Session
	connectChan  chan *Session
	virtualConns *link.Uint32Channel
	closeChan    chan struct{}
	closeFlag    int32
}

func newEndPoint(pool slab.Pool, maxPacketSize, recvChanSize int) *EndPoint {
	return &EndPoint{
		protocol: protocol{
			pool:          pool,
			maxPacketSize: maxPacketSize,
		},
		sessions:     make(map[uint64]*Session),
		recvChanSize: recvChanSize,
		newConnChan:  make(chan uint32),
		acceptChan:   make(chan *Session, 1),
		connectChan:  make(chan *Session, 1000),
		virtualConns: link.NewUint32Channel(),
		closeChan:    make(chan struct{}),
	}
}

// Accept accept a virtual connection.
func (p *EndPoint) Accept() (*Session, error) {
	select {
	case session := <-p.connectChan:
		return session, nil
	case <-p.closeChan:
		return nil, io.EOF
	}
}

// Dial create a virtual connection and dial to a remote EndPoint.
func (p *EndPoint) Dial(remoteID uint32) (*Session, error) {
	p.dialMutex.Lock()
	defer p.dialMutex.Unlock()

	if err := p.send(p.session, p.encodeDialCmd(remoteID)); err != nil {
		return nil, err
	}

	select {
	case session := <-p.acceptChan:
		if session == nil {
			return nil, ErrRefused
		}
		return session, nil
	case <-p.closeChan:
		return nil, io.EOF
	}
}

// GetSession get a virtual connection session by session ID.
func (p *EndPoint) GetSession(sessionID uint64) *Session {
	return p.sessions[sessionID]
}

// Close EndPoint.
func (p *EndPoint) Close() {
	if atomic.CompareAndSwapInt32(&p.closeFlag, 0, 1) {
		for _, session := range p.sessions {
			session.RemoveCloseCallback(p, nil)
			session.Close()
		}
		p.session.Close()
		close(p.closeChan)
	}
}

func (p *EndPoint) keepalive(pingInterval, pingTimeout time.Duration, timeoutCallback func()) {
	for {
		select {
		case <-EndPointTimer.After(pingInterval):
			if time.Duration(atomic.LoadInt64(&p.lastActive)) >= pingInterval {
				if p.send(p.session, p.encodePingCmd()) != nil {
					return
				}
			}
			if timeoutCallback != nil {
				select {
				case <-EndPointTimer.After(pingTimeout):
					if time.Duration(atomic.LoadInt64(&p.lastActive)) >= pingTimeout {
						timeoutCallback()
					}
				case <-p.closeChan:
					return
				}
			}
		case <-p.closeChan:
			return
		}
	}
}

func (p *EndPoint) addVirtualConn(connID, remoteID uint32, c chan *Session) {
	codec := p.newVirtualCodec(p.session, connID, p.recvChanSize, &p.lastActive)
	session := &Session{link.NewSession(codec, 0), connID, remoteID}
	p.sessions[session.ID()] = session
	session.AddCloseCallback(p, nil, func() {
		delete(p.sessions, session.ID())
	})
	p.virtualConns.Put(connID, session.base)
	select {
	case c <- session:
	case <-p.closeChan:
	default:
		p.send(p.session, p.encodeCloseCmd(connID))
	}
}

func (p *EndPoint) loop() {
	defer func() {
		p.Close()
		if err := recover(); err != nil {
			log.Printf("fast/gateway.EndPoint panic: %v\n%s", err, debug.Stack())
		}
	}()
	for {
		atomic.StoreInt64(&p.lastActive, time.Now().UnixNano())

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

func (p *EndPoint) processCmd(buf []byte) {
	cmd := p.decodeCmd(buf)
	switch cmd {
	case acceptCmd:
		connID, remoteID := p.decodeAcceptCmd(buf)
		p.free(buf)
		p.addVirtualConn(connID, remoteID, p.acceptChan)

	case refuseCmd:
		p.free(buf)
		select {
		case p.acceptChan <- nil:
		case <-p.closeChan:
			return
		}

	case connectCmd:
		connID, remoteID := p.decodeConnectCmd(buf)
		p.free(buf)
		p.addVirtualConn(connID, remoteID, p.connectChan)

	case closeCmd:
		connID := p.decodeCloseCmd(buf)
		p.free(buf)
		vconn := p.virtualConns.Get(connID)
		if vconn != nil {
			vconn.Close()
		}

	case pingCmd:
		p.free(buf)

	default:
		p.free(buf)
		panic("unsupported command")
	}
}

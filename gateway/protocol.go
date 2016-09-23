package gateway

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"io"
	"math/rand"
	"net"
	"time"

	"github.com/funny/link"
	"github.com/funny/slab"
)

// ErrServerAuthFailed happens when server connection auth failed.
var ErrServerAuthFailed = errors.New("server auth failed")

// Protocol implements gateway communication protocol.
type Protocol struct {
	// Rand used to generate server auth challenge code.
	Rand *rand.Rand

	// Pool used to pooling message buffers.
	Pool *slab.AtomPool

	// MaxPacketSize limit incomming message size.
	MaxPacketSize int

	// ClientBufferSize settings memory usage of bufio.Reader for client connections.
	ClientBufferSize int

	// ClientBufferSize settings memory usage of bufio.Reader for server connections.
	ServerBufferSize int

	// ServerAuthPassword used to auth server connection.
	ServerAuthPassword []byte

	// ServerAuthTimeout used to limit server auth IO waiting.
	ServerAuthTimeout time.Duration
}

// NewClientCodec create a link.Codec for client connection.
func (p *Protocol) NewClientCodec(rw io.ReadWriter) (link.Codec, link.Context, error) {
	return NewCodec(p, rw.(net.Conn), p.ClientBufferSize), nil, nil
}

// NewServerCodec create a link.Codec for server connection. The server ID will returns by link.Context.
func (p *Protocol) NewServerCodec(rw io.ReadWriter) (link.Codec, link.Context, error) {
	serverID, err := p.AcceptServer(rw.(net.Conn))
	if err != nil {
		return nil, nil, err
	}
	return NewCodec(p, rw.(net.Conn), p.ServerBufferSize), serverID, nil
}

// Send send a message to session.
func (p *Protocol) Send(session *link.Session, buffer []byte) error {
	if err := session.Send(&buffer); err != nil {
		p.Pool.Free(buffer)
		session.Close()
	}
	return nil
}

// DialServer initialize a server connection for server.
func (p *Protocol) DialServer(conn net.Conn, serverID uint32) error {
	var buf [md5.Size + cmdIDSize]byte

	conn.SetDeadline(time.Now().Add(p.ServerAuthTimeout))
	if _, err := io.ReadFull(conn, buf[:8]); err != nil {
		conn.Close()
		return err
	}

	hash := md5.New()
	hash.Write(buf[:8])
	hash.Write(p.ServerAuthPassword)
	verify := hash.Sum(nil)
	copy(buf[:md5.Size], verify)
	binary.LittleEndian.PutUint32(buf[md5.Size:], serverID)
	if _, err := conn.Write(buf[:]); err != nil {
		conn.Close()
		return err
	}
	conn.SetDeadline(time.Time{})

	return nil
}

// AcceptServer initialize a new server connection for gateway.
func (p *Protocol) AcceptServer(conn net.Conn) (uint32, error) {
	var buf [md5.Size + cmdIDSize]byte

	conn.SetDeadline(time.Now().Add(p.ServerAuthTimeout))
	challenge := uint64(p.Rand.Int63())
	binary.LittleEndian.PutUint64(buf[:8], challenge)
	if _, err := conn.Write(buf[:8]); err != nil {
		conn.Close()
		return 0, err
	}

	hash := md5.New()
	hash.Write(buf[:8])
	hash.Write(p.ServerAuthPassword)
	verify := hash.Sum(nil)
	if _, err := io.ReadFull(conn, buf[:]); err != nil {
		conn.Close()
		return 0, err
	}
	if !bytes.Equal(verify, buf[:md5.Size]) {
		conn.Close()
		return 0, ErrServerAuthFailed
	}
	conn.SetDeadline(time.Time{})

	return binary.LittleEndian.Uint32(buf[md5.Size:]), nil
}

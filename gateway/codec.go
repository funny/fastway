package gateway

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"

	"github.com/funny/link"
	"github.com/funny/slab"
)

// SizeofLen is the size of `Length` field.
const SizeofLen = 4

// ErrTooLargePacket happens when gateway receive a packet length greater than `MaxPacketSize` setting.
var ErrTooLargePacket = errors.New("too large packet")

type codec struct {
	*protocol
	conn    net.Conn
	reader  *bufio.Reader
	headBuf []byte
	headDat [SizeofLen]byte
	pool    *slab.AtomPool
}

func (p *protocol) newCodec(conn net.Conn, bufferSize int) *codec {
	c := &codec{
		protocol: p,
		conn:     conn,
		reader:   bufio.NewReaderSize(conn, bufferSize),
	}
	c.headBuf = c.headDat[:]
	return c
}

// Receive implements link/Codec.Receive() method.
func (c *codec) Receive() (interface{}, error) {
	if _, err := io.ReadFull(c.reader, c.headBuf); err != nil {
		return nil, err
	}
	length := int(binary.LittleEndian.Uint32(c.headBuf))
	if length > c.maxPacketSize {
		return nil, ErrTooLargePacket
	}
	buffer := c.alloc(SizeofLen + length)
	copy(buffer, c.headBuf)
	if _, err := io.ReadFull(c.reader, buffer[SizeofLen:]); err != nil {
		c.free(buffer)
		return nil, err
	}
	return &buffer, nil
}

// Send implements link/Codec.Send() method.
func (c *codec) Send(msg interface{}) error {
	buffer := *(msg.(*[]byte))
	_, err := c.conn.Write(buffer)
	c.free(buffer)
	return err
}

// Close implements link/Codec.Close() method.
func (c *codec) Close() error {
	return c.conn.Close()
}

// ===========================================================================

type MsgFormat interface {
	SizeofMsg(msg interface{}) int
	MarshalMsg(msg interface{}, buf []byte) error
	UnmarshalMsg(buf []byte) (interface{}, error)
}

type virtualCodec struct {
	*protocol
	physicalConn *link.Session
	connID       uint32
	format       MsgFormat
	recvChan     chan []byte
	closeOnce    sync.Once
}

func (p *protocol) newVirtualCodec(physicalConn *link.Session, connID uint32, format MsgFormat) *virtualCodec {
	return &virtualCodec{
		protocol:     p,
		connID:       connID,
		format:       format,
		physicalConn: physicalConn,
		recvChan:     make(chan []byte, 1024),
	}
}

func (c *virtualCodec) Receive() (interface{}, error) {
	buf, ok := <-c.recvChan
	if !ok {
		return nil, io.EOF
	}
	msg, err := c.format.UnmarshalMsg(buf[cmdConnID+cmdIDSize:])
	c.free(buf)
	return msg, err
}

func (c *virtualCodec) Send(msg interface{}) error {
	msgSize := c.format.SizeofMsg(msg)
	if msgSize > c.maxPacketSize {
		return ErrTooLargePacket
	}
	buf := c.alloc(SizeofLen + cmdIDSize + msgSize)
	err := c.format.MarshalMsg(msg, buf[cmdConnID+cmdIDSize:])
	if err != nil {
		c.free(buf)
		return err
	}
	binary.LittleEndian.PutUint32(buf, uint32(cmdIDSize+msgSize))
	binary.LittleEndian.PutUint32(buf[cmdConnID:], c.connID)
	return c.send(c.physicalConn, buf)
}

func (c *virtualCodec) Close() error {
	c.closeOnce.Do(func() {
		close(c.recvChan)
		c.send(c.physicalConn, c.encodeCloseCmd(c.connID))
	})
	for buf := range c.recvChan {
		c.free(buf)
	}
	return nil
}

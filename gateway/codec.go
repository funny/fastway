package gateway

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"net"
)

// SizeofLen is the size of `Length` field.
const SizeofLen = 4

// ErrTooLargePacket happens when gateway receive a packet length greater than `MaxPacketSize` setting.
var ErrTooLargePacket = errors.New("too large packet")

// Codec implements basic gateway protocol.
type Codec struct {
	proto   *Protocol
	conn    net.Conn
	reader  *bufio.Reader
	headBuf []byte
	headDat [SizeofLen]byte
}

// NewCodec create a new gateway codec from conn.
func NewCodec(proto *Protocol, conn net.Conn, bufferSize int) *Codec {
	codec := &Codec{
		proto:  proto,
		conn:   conn,
		reader: bufio.NewReaderSize(conn, bufferSize),
	}
	codec.headBuf = codec.headDat[:]
	return codec
}

// Receive implements link/Codec.Receive() method.
func (c *Codec) Receive() (interface{}, error) {
	if _, err := io.ReadFull(c.reader, c.headBuf); err != nil {
		return nil, err
	}
	length := int(binary.LittleEndian.Uint32(c.headBuf))
	if length > c.proto.MaxPacketSize {
		return nil, ErrTooLargePacket
	}
	buffer := c.proto.Pool.Alloc(SizeofLen + length)
	copy(buffer, c.headBuf)
	if _, err := io.ReadFull(c.reader, buffer[SizeofLen:]); err != nil {
		c.proto.Pool.Free(buffer)
		return nil, err
	}
	return &buffer, nil
}

// Send implements link/Codec.Send() method.
func (c *Codec) Send(msg interface{}) error {
	buffer := *(msg.(*[]byte))
	_, err := c.conn.Write(buffer)
	c.proto.Pool.Free(buffer)
	return err
}

// Close implements link/Codec.Close() method.
func (c *Codec) Close() error {
	return c.conn.Close()
}

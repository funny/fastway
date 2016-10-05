package proto

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"time"

	"github.com/funny/link"
	"github.com/funny/slab"
)

type protocol struct {
	pool          slab.Pool
	maxPacketSize int
}

func (p *protocol) alloc(size int) []byte {
	return p.pool.Alloc(size)
}

func (p *protocol) free(msg []byte) {
	p.pool.Free(msg)
}

func (p *protocol) send(session *link.Session, msg []byte) error {
	err := session.Send(&msg)
	if err != nil {
		p.free(msg)
		session.Close()
	}
	return err
}

const (
	// cmdHeadSize is the size of command header.
	cmdHeadSize = cmdIDSize + cmdTypeSize

	// SizeofCmd is the size of `CMD`field.
	cmdTypeSize = 1

	// cmdIDSize is the size of `Conn ID`, `Remote ID` field.
	cmdIDSize = 4

	// cmdConnID is the beginning index of `Conn ID` field in command header.
	cmdConnID = SizeofLen

	// cmdType is the beginning index of `Type` field in command header.
	cmdType = cmdConnID + cmdIDSize

	// cmdArgs is the beginning index of `Args` part in command.
	cmdArgs = cmdType + cmdTypeSize
)

func (p *protocol) allocCmd(t byte, size int) []byte {
	buffer := p.alloc(SizeofLen + size)
	binary.LittleEndian.PutUint32(buffer, uint32(size))
	binary.LittleEndian.PutUint32(buffer[cmdConnID:], 0)
	buffer[cmdType] = t
	return buffer
}

// decodePacket decodes gateway message and returns virtual connection ID.
func (p *protocol) decodePacket(msg []byte) (connID uint32) {
	return binary.LittleEndian.Uint32(msg[cmdConnID:])
}

// DecodeCmd decodes gateway command and returns command type.
func (p *protocol) decodeCmd(msg []byte) (CMD byte) {
	return msg[cmdType]
}

// ==================================================

const (
	dialCmd         = 0
	dialCmdSize     = cmdHeadSize + cmdIDSize
	dialCmdRemoteID = cmdArgs
)

func (p *protocol) decodeDialCmd(msg []byte) (remoteID uint32) {
	return binary.LittleEndian.Uint32(msg[dialCmdRemoteID:])
}

func (p *protocol) encodeDialCmd(remoteID uint32) []byte {
	buffer := p.allocCmd(dialCmd, dialCmdSize)
	binary.LittleEndian.PutUint32(buffer[dialCmdRemoteID:], remoteID)
	return buffer
}

// ==================================================

const (
	acceptCmd         = 1
	acceptCmdSize     = cmdHeadSize + cmdIDSize*2
	acceptCmdConnID   = cmdArgs
	acceptCmdRemoteID = acceptCmdConnID + cmdIDSize
)

func (p *protocol) decodeAcceptCmd(msg []byte) (connID, remoteID uint32) {
	connID = binary.LittleEndian.Uint32(msg[acceptCmdConnID:])
	remoteID = binary.LittleEndian.Uint32(msg[acceptCmdRemoteID:])
	return
}

func (p *protocol) encodeAcceptCmd(connID, remoteID uint32) []byte {
	buffer := p.allocCmd(acceptCmd, acceptCmdSize)
	binary.LittleEndian.PutUint32(buffer[acceptCmdConnID:], connID)
	binary.LittleEndian.PutUint32(buffer[acceptCmdRemoteID:], remoteID)
	return buffer
}

// ==================================================

const (
	connectCmd         = 2
	connectCmdSize     = cmdHeadSize + cmdIDSize*2
	connectCmdConnID   = cmdArgs
	connectCmdRemoteID = connectCmdConnID + cmdIDSize
)

func (p *protocol) decodeConnectCmd(msg []byte) (connID, remoteID uint32) {
	connID = binary.LittleEndian.Uint32(msg[connectCmdConnID:])
	remoteID = binary.LittleEndian.Uint32(msg[connectCmdRemoteID:])
	return
}

func (p *protocol) encodeConnectCmd(connID, remoteID uint32) []byte {
	buffer := p.allocCmd(connectCmd, connectCmdSize)
	binary.LittleEndian.PutUint32(buffer[connectCmdConnID:], connID)
	binary.LittleEndian.PutUint32(buffer[connectCmdRemoteID:], remoteID)
	return buffer
}

// ==================================================

const (
	refuseCmd     = 3
	refuseCmdSize = cmdHeadSize
)

func (p *protocol) encodeRefuseCmd() []byte {
	return p.allocCmd(refuseCmd, refuseCmdSize)
}

// ==================================================

const (
	closeCmd       = 4
	closeCmdSize   = cmdHeadSize + cmdIDSize
	closeCmdConnID = cmdArgs
)

func (p *protocol) decodeCloseCmd(msg []byte) uint32 {
	return binary.LittleEndian.Uint32(msg[closeCmdConnID:])
}

func (p *protocol) encodeCloseCmd(connID uint32) []byte {
	buffer := p.allocCmd(closeCmd, closeCmdSize)
	binary.LittleEndian.PutUint32(buffer[closeCmdConnID:], connID)
	return buffer
}

// ==================================================

const (
	pingCmd     = 5
	pingCmdSize = cmdHeadSize
)

func (p *protocol) encodePingCmd() []byte {
	return p.allocCmd(pingCmd, pingCmdSize)
}

// ==================================================

// ErrServerAuthFailed happens when server connection auth failed.
var ErrServerAuthFailed = errors.New("server auth failed")

func (p *protocol) serverInit(conn net.Conn, serverID uint32, key []byte, timeout time.Duration) error {
	var buf [md5.Size + cmdIDSize]byte

	conn.SetDeadline(time.Now().Add(timeout))
	if _, err := io.ReadFull(conn, buf[:8]); err != nil {
		conn.Close()
		return err
	}

	hash := md5.New()
	hash.Write(buf[:8])
	hash.Write(key)
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

func (p *protocol) serverAuth(conn net.Conn, key []byte, timeout time.Duration) (uint32, error) {
	var buf [md5.Size + cmdIDSize]byte

	conn.SetDeadline(time.Now().Add(timeout))
	rand.Read(buf[:8])
	if _, err := conn.Write(buf[:8]); err != nil {
		conn.Close()
		return 0, err
	}

	hash := md5.New()
	hash.Write(buf[:8])
	hash.Write(key)
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

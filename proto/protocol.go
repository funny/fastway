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
	pool          *slab.AtomPool
	maxPacketSize int
}

func (p *protocol) alloc(size int) []byte {
	return p.pool.Alloc(size)
}

func (p *protocol) free(msg []byte) {
	p.pool.Free(msg)
}

func (p *protocol) send(session *link.Session, msg []byte) error {
	if err := session.Send(&msg); err != nil {
		p.pool.Free(msg)
		session.Close()
	}
	return nil
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
	buffer := p.pool.Alloc(SizeofLen + size)
	binary.LittleEndian.PutUint32(buffer, uint32(size))
	binary.LittleEndian.PutUint32(buffer[cmdConnID:], 0)
	buffer[cmdType] = t
	return buffer
}

// decodePacket decodes gateway message and returns virtual connection ID.
func (_ *protocol) decodePacket(msg []byte) (connID uint32) {
	return binary.LittleEndian.Uint32(msg[cmdConnID:])
}

// DecodeCmd decodes gateway command and returns command type.
func (p *protocol) decodeCmd(msg []byte) (CMD byte) {
	return msg[cmdType]
}

// ==================================================

const (
	newCmd     = 0
	newCmdSize = cmdHeadSize
)

func (p *protocol) encodeNewCmd() []byte {
	return p.allocCmd(newCmd, newCmdSize)
}

const (
	openCmd       = 1
	openCmdSize   = cmdHeadSize + cmdIDSize
	openCmdConnID = cmdArgs
)

func (_ *protocol) decodeOpenCmd(msg []byte) (connID uint32) {
	return binary.LittleEndian.Uint32(msg[openCmdConnID:])
}

func (p *protocol) encodeOpenCmd(connID uint32) []byte {
	buffer := p.allocCmd(openCmd, openCmdSize)
	binary.LittleEndian.PutUint32(buffer[openCmdConnID:], connID)
	return buffer
}

// ==================================================

const (
	dialCmd         = 2
	dialCmdSize     = cmdHeadSize + cmdIDSize*2
	dialCmdConnID   = cmdArgs
	dialCmdRemoteID = dialCmdConnID + cmdIDSize
)

func (_ *protocol) decodeDialCmd(msg []byte) (connID, remoteID uint32) {
	connID = binary.LittleEndian.Uint32(msg[dialCmdConnID:])
	remoteID = binary.LittleEndian.Uint32(msg[dialCmdRemoteID:])
	return
}

func (p *protocol) encodeDialCmd(connID, remoteID uint32) []byte {
	buffer := p.allocCmd(dialCmd, dialCmdSize)
	binary.LittleEndian.PutUint32(buffer[dialCmdConnID:], connID)
	binary.LittleEndian.PutUint32(buffer[dialCmdRemoteID:], remoteID)
	return buffer
}

const (
	refuseCmd       = 3
	refuseCmdSize   = cmdHeadSize + cmdIDSize
	refuseCmdConnID = cmdArgs
)

func (p *protocol) decodeRefuseCmd(msg []byte) (connID uint32) {
	return binary.LittleEndian.Uint32(msg[refuseCmdConnID:])
}

func (p *protocol) encodeRefuseCmd(connID uint32) []byte {
	buffer := p.allocCmd(refuseCmd, refuseCmdSize)
	binary.LittleEndian.PutUint32(buffer[refuseCmdConnID:], connID)
	return buffer
}

const (
	acceptCmd         = 4
	acceptCmdSize     = cmdHeadSize + cmdIDSize*2
	acceptCmdConnID   = cmdArgs
	acceptCmdRemoteID = acceptCmdConnID + cmdIDSize
)

func (_ *protocol) decodeAcceptCmd(msg []byte) (connID, remoteID uint32) {
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
	closeCmd       = 5
	closeCmdSize   = cmdHeadSize + cmdIDSize
	closeCmdConnID = cmdArgs
)

func (_ *protocol) decodeCloseCmd(msg []byte) uint32 {
	return binary.LittleEndian.Uint32(msg[closeCmdConnID:])
}

func (p *protocol) encodeCloseCmd(connID uint32) []byte {
	buffer := p.allocCmd(closeCmd, closeCmdSize)
	binary.LittleEndian.PutUint32(buffer[closeCmdConnID:], connID)
	return buffer
}

// ==================================================

const (
	pingCmd     = 6
	pingCmdSize = cmdHeadSize
)

func (p *protocol) encodePingCmd() []byte {
	return p.allocCmd(pingCmd, pingCmdSize)
}

// ==================================================

// ErrServerAuthFailed happens when server connection auth failed.
var ErrServerAuthFailed = errors.New("server auth failed")

func (_ *protocol) serverInit(conn net.Conn, serverID uint32, key []byte, timeout time.Duration) error {
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

func (_ *protocol) serverAuth(conn net.Conn, key []byte, timeout time.Duration) (uint32, error) {
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

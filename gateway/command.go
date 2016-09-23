package gateway

import (
	"encoding/binary"

	"github.com/funny/link"
)

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

func (p *Protocol) allocCmd(t byte, size int) []byte {
	buffer := p.Pool.Alloc(SizeofLen + size)
	binary.LittleEndian.PutUint32(buffer, uint32(size))
	binary.LittleEndian.PutUint32(buffer[cmdConnID:], 0)
	buffer[cmdType] = t
	return buffer
}

// DecodePacket decodes gateway message and returns virtual connection ID.
func (p *Protocol) DecodePacket(msg []byte) (connID uint32) {
	return binary.LittleEndian.Uint32(msg[cmdConnID:])
}

// DecodeCmd decodes gateway command and returns command type.
func (p *Protocol) DecodeCmd(msg []byte) (CMD byte) {
	return msg[cmdType]
}

const (
	// DialCmd is the command that means create a virtual connection.
	DialCmd = 1

	// DialCmdSize is the size of `Dial` command.
	DialCmdSize = cmdHeadSize + cmdIDSize

	// DialCmdRemoteID is the index of `RemoteID` field in `Dial` command.
	DialCmdRemoteID = cmdArgs
)

// DecodeDialCmd decodes `Dial` command and returns `RemoteID`.
func (p *Protocol) DecodeDialCmd(msg []byte) (remoteID uint32) {
	return binary.LittleEndian.Uint32(msg[DialCmdRemoteID:])
}

// EncodeDialCmd make a `Dial` command.
func (p *Protocol) EncodeDialCmd(remoteID uint32) []byte {
	buffer := p.allocCmd(DialCmd, DialCmdSize)
	binary.LittleEndian.PutUint32(buffer[DialCmdRemoteID:], remoteID)
	return buffer
}

// SendDialCmd send a `Dial` command to session.
func (p *Protocol) SendDialCmd(session *link.Session, remoteID uint32) error {
	buffer := p.EncodeDialCmd(remoteID)
	return p.Send(session, buffer)
}

const (
	// AcceptCmd is the command that means virtual connection created.
	AcceptCmd = 3

	// AcceptCmdSize is the size of `Accept` command.
	AcceptCmdSize = cmdHeadSize + cmdIDSize*2

	// AcceptCmdRemoteID is the index of `RemoteID` field in `Accept` command.
	AcceptCmdRemoteID = cmdArgs

	// AcceptCmdConnID is the index of `ConnID` field in `Accept` command.
	AcceptCmdConnID = AcceptCmdRemoteID + cmdIDSize
)

// DecodeAcceptCmd decode `Accept` command and returns `RemoteID` and `ConnID`.
func (p *Protocol) DecodeAcceptCmd(msg []byte) (remoteID, connID uint32) {
	remoteID = binary.LittleEndian.Uint32(msg[AcceptCmdRemoteID:])
	connID = binary.LittleEndian.Uint32(msg[AcceptCmdConnID:])
	return
}

// EncodeAcceptCmd make a `Accept` command.
func (p *Protocol) EncodeAcceptCmd(remoteID, connID uint32) []byte {
	buffer := p.allocCmd(AcceptCmd, AcceptCmdSize)
	binary.LittleEndian.PutUint32(buffer[AcceptCmdRemoteID:], remoteID)
	binary.LittleEndian.PutUint32(buffer[AcceptCmdConnID:], connID)
	return buffer
}

// SendAcceptCmd send a `Accept` command to session.
func (p *Protocol) SendAcceptCmd(session *link.Session, remoteID, connID uint32) {
	buffer := p.EncodeAcceptCmd(remoteID, connID)
	p.Send(session, buffer)
}

const (
	// RefuseCmd is the command that means virtual connection refused.
	RefuseCmd = 2

	// RefuseCmdSize is the size of `Refuse` command.
	RefuseCmdSize = cmdHeadSize + cmdIDSize

	// RefuseCmdRemoteID is the index of `RemoteID` field in `Refuse` command.
	RefuseCmdRemoteID = cmdArgs
)

// DecodeRefuseCmd decode `Refuse` command and returns `RemoteID`.
func (p *Protocol) DecodeRefuseCmd(msg []byte) (remoteID uint32) {
	return binary.LittleEndian.Uint32(msg[RefuseCmdRemoteID:])
}

// EncodeRefuseCmd make a `Refuse` command.
func (p *Protocol) EncodeRefuseCmd(remoteID uint32) []byte {
	buffer := p.allocCmd(RefuseCmd, RefuseCmdSize)
	binary.LittleEndian.PutUint32(buffer[RefuseCmdRemoteID:], remoteID)
	return buffer
}

// SendRefuseCmd send `Refuse` command to session.
func (p *Protocol) SendRefuseCmd(session *link.Session, remoteID uint32) {
	buffer := p.EncodeRefuseCmd(remoteID)
	p.Send(session, buffer)
}

const (
	// CloseCmd is the command that means virtual connection closed.
	CloseCmd = 4

	// CloseCmdSize is the size of `Close` command.
	CloseCmdSize = cmdHeadSize + cmdIDSize

	// CloseCmdConnID is the index of `ConnID` field in `Close` command.
	CloseCmdConnID = cmdArgs
)

// DecodeCloseCmd decode `Close` command and returns virtual connection ID.
func (p *Protocol) DecodeCloseCmd(msg []byte) uint32 {
	return binary.LittleEndian.Uint32(msg[CloseCmdConnID:])
}

// EncodeCloseCmd make a `Close` command.
func (p *Protocol) EncodeCloseCmd(connID uint32) []byte {
	buffer := p.allocCmd(CloseCmd, CloseCmdSize)
	binary.LittleEndian.PutUint32(buffer[CloseCmdConnID:], connID)
	return buffer
}

// SendCloseCmd send `Close` command to session.
func (p *Protocol) SendCloseCmd(session *link.Session, connID uint32) {
	buffer := p.EncodeCloseCmd(connID)
	p.Send(session, buffer)
}

const (
	// PingCmd is the command that used to check physical connection is alive.
	PingCmd = 5

	// PingCmdSize is the size of `Ping` command.
	PingCmdSize = cmdHeadSize
)

// EncodePingCmd make a `Ping` command.
func (p *Protocol) EncodePingCmd() []byte {
	return p.allocCmd(PingCmd, PingCmdSize)
}

// SendPingCmd send `Ping` command to session.
func (p *Protocol) SendPingCmd(session *link.Session) error {
	buffer := p.EncodePingCmd()
	return p.Send(session, buffer)
}

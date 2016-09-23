package main

import (
	"sync/atomic"

	"github.com/funny/link"
)

// HandleDialCmd process `Dial` command.
func HandleDialCmd(session *link.Session, side, otherSide int, msg []byte) {
	remoteID := Protocol.DecodeDialCmd(msg)
	remote := GetPhysicalConn(remoteID, otherSide)

	var pair [2]*link.Session
	pair[side] = session
	pair[otherSide] = remote

	if remote == nil || !AcceptVirtualConn(pair) {
		Protocol.SendRefuseCmd(session, remoteID)
	}

	Protocol.Pool.Free(msg)
}

// AcceptVirtualConn create a virtual connection and sending notification to both side.
func AcceptVirtualConn(pair [2]*link.Session) bool {
	var connID uint32
	for connID == 0 {
		connID = atomic.AddUint32(&VirtualConnID, 1)
	}

	for i := 0; i < 2; i++ {
		state := pair[i].State.(*State)
		state.Lock()
		defer state.Unlock()
		if state.Disposed {
			return false
		}
		state.VirtualConns[connID] = struct{}{}
	}

	AddVirtualConn(connID, pair)

	for i := 0; i < 2; i++ {
		remoteID := pair[(i+1)%2].State.(*State).PhysicalConnID
		Protocol.SendAcceptCmd(pair[i], remoteID, connID)
	}
	return true
}

// HandleCloseCmd process `Close` command.
func HandleCloseCmd(msg []byte) {
	connID := Protocol.DecodeCloseCmd(msg)
	CloseVirtualConn(connID)
	Protocol.Pool.Free(msg)
}

// CloseVirtualConn close the virtual connection and sending notification to both side.
func CloseVirtualConn(connID uint32) {
	pair, ok := DelVirtualConn(connID)
	if !ok {
		return
	}

	for i := 0; i < 2; i++ {
		state := pair[i].State.(*State)
		state.Lock()
		defer state.Unlock()
		if state.Disposed {
			return
		}
		delete(state.VirtualConns, connID)
	}

	for i := 0; i < 2; i++ {
		Protocol.SendCloseCmd(pair[i], connID)
	}
}

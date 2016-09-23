package main

import (
	"sync"

	"github.com/funny/link"
)

// ConnBuckets used to reduce lock granularity.
const ConnBuckets = 32

var (
	// PhysicalConnID used to generate physical connection ID.
	PhysicalConnID uint32

	// PhysicalConns index physical connection by `Server ID` or `Client ID`.
	// The first one used to index client connections.
	// The second one used to index server connections.
	// The ConnBuckets used to reduce lock granularity.
	PhysicalConns [ConnBuckets][2]*link.Uint32Channel

	// VirtualConnID used to generate virtual connection ID.
	VirtualConnID uint32

	// VirtualConns index virtual connections by `Conn ID`.
	// The first one is client session.
	// The second one is server session.
	// The ConnBuckets used to reduce lock granularity.
	VirtualConns [ConnBuckets]map[uint32][2]*link.Session

	// VirtualConnMutexes used to protect VirtualConns in concurrent access.
	VirtualConnMutexes [ConnBuckets]sync.RWMutex
)

func init() {
	for i := 0; i < ConnBuckets; i++ {
		VirtualConns[i] = make(map[uint32][2]*link.Session)
	}

	for i := 0; i < ConnBuckets; i++ {
		PhysicalConns[i][0] = link.NewUint32Channel()
		PhysicalConns[i][1] = link.NewUint32Channel()
	}
}

// AddVirtualConn add a virtual connection into index.
func AddVirtualConn(connID uint32, pair [2]*link.Session) {
	bucket := connID % ConnBuckets
	VirtualConnMutexes[bucket].Lock()
	VirtualConns[bucket][connID] = pair
	VirtualConnMutexes[bucket].Unlock()
}

// GetVirtualConn get a virtual connection by connID and side.
func GetVirtualConn(connID uint32, side int) *link.Session {
	bucket := connID % ConnBuckets
	VirtualConnMutexes[bucket].RLock()
	conn := VirtualConns[bucket][connID][side]
	VirtualConnMutexes[bucket].RUnlock()
	return conn
}

// DelVirtualConn remove a virtual connection from index.
func DelVirtualConn(connID uint32) ([2]*link.Session, bool) {
	bucket := connID % ConnBuckets
	VirtualConnMutexes[bucket].Lock()
	pair, exists := VirtualConns[bucket][connID]
	if exists {
		delete(VirtualConns[bucket], connID)
	}
	VirtualConnMutexes[bucket].Unlock()
	return pair, exists
}

// AddPhysicalConn add a physical connection into index.
func AddPhysicalConn(connID uint32, side int, session *link.Session) {
	PhysicalConns[connID%ConnBuckets][side].Put(connID, session)
}

// GetPhysicalConn get a physical connection by connID and side.
func GetPhysicalConn(connID uint32, side int) *link.Session {
	return PhysicalConns[connID%ConnBuckets][side].Get(connID)
}

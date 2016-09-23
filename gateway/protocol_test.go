package gateway

import (
	"math/rand"
	"net"
	"testing"
	"time"

	"github.com/funny/utest"
)

func Test_ServerHandshake(t *testing.T) {
	proto := &Protocol{
		MaxPacketSize:      2048,
		Pool:               TestPool,
		ServerAuthPassword: []byte("test"),
		ServerAuthTimeout:  time.Second,
		Rand:               rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	lsn, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	lsn.Addr().String()

	go func() {
		conn, err := net.Dial("tcp", lsn.Addr().String())
		utest.IsNilNow(t, err)
		proto.DialServer(conn, 123)
	}()

	conn, err := lsn.Accept()
	utest.IsNilNow(t, err)

	serverID, err := proto.AcceptServer(conn)
	utest.IsNilNow(t, err)
	utest.EqualNow(t, serverID, 123)
}

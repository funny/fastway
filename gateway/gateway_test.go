package gateway

import (
	"math/rand"
	"net"
	"testing"
	"time"

	"github.com/funny/link"
	"github.com/funny/utest"
)

func Test_Gateway(t *testing.T) {
	lsn1, err := net.Listen("tcp", ":0")
	utest.IsNilNow(t, err)

	lsn2, err := net.Listen("tcp", ":0")
	utest.IsNilNow(t, err)

	gw := NewGateway(TestPool, 2048)

	go gw.ServeClients(lsn1, 10, 1024, 1024, 30)
	go gw.ServeServers(lsn2, "123", 3, 1024, 1024, 30)

	time.Sleep(time.Second)

	client, err := DialClient(lsn1.Addr().String(), TestMsgFormat{}, TestPool, 2048, 1024, 1024)
	utest.IsNilNow(t, err)

	server, err := DialServer(lsn2.Addr().String(), TestMsgFormat{}, TestPool, 123, "123", 3, 2048, 1024, 1024)
	utest.IsNilNow(t, err)

	var vconns [2]*link.Session
	var connIDs [2]uint32

	go func() {
		var err error
		vconns[0], connIDs[0], _, err = server.Accept()
		utest.IsNilNow(t, err)
	}()

	vconns[1], connIDs[1], err = client.Dial(123)
	utest.IsNilNow(t, err)

	utest.EqualNow(t, connIDs[0], connIDs[1])

	for i := 0; i < 10000; i++ {
		buffer1 := make([]byte, 1024)
		for i := 0; i < len(buffer1); i++ {
			buffer1[i] = byte(rand.Intn(256))
		}

		x := rand.Intn(2)

		err := vconns[x].Send(buffer1)
		utest.IsNilNow(t, err)

		buffer2, err := vconns[(x+1)%2].Receive()
		utest.IsNilNow(t, err)
		utest.EqualNow(t, buffer1, buffer2)
	}
}

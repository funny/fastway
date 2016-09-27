package proto

import (
	"math/rand"
	"net"
	"testing"
	"time"

	"github.com/funny/utest"
)

func Test_NewCmd(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	utest.IsNilNow(t, err)
	defer conn.Close()

	codec := TestProto.newCodec(conn, 1024)

	for i := 0; i < 10000; i++ {
		msg1 := TestProto.encodeNewCmd()

		err := codec.Send(&msg1)
		utest.IsNilNow(t, err)

		msg2, err := codec.Receive()
		utest.IsNilNow(t, err)

		msg3 := *(msg2.(*[]byte))
		cmd := TestProto.decodeCmd(msg3)
		utest.EqualNow(t, cmd, newCmd)
	}
}

func Test_OpenCmd(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	utest.IsNilNow(t, err)
	defer conn.Close()

	codec := TestProto.newCodec(conn, 1024)

	for i := 0; i < 10000; i++ {
		connID1 := rand.Uint32()
		msg1 := TestProto.encodeOpenCmd(connID1)

		err := codec.Send(&msg1)
		utest.IsNilNow(t, err)

		msg2, err := codec.Receive()
		utest.IsNilNow(t, err)

		msg3 := *(msg2.(*[]byte))
		cmd := TestProto.decodeCmd(msg3)
		utest.EqualNow(t, cmd, openCmd)

		connID2 := TestProto.decodeOpenCmd(msg3)
		utest.EqualNow(t, connID1, connID2)
	}
}

func Test_DialCmd(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	utest.IsNilNow(t, err)
	defer conn.Close()

	codec := TestProto.newCodec(conn, 1024)

	for i := 0; i < 10000; i++ {
		connID1 := rand.Uint32()
		remoteID1 := rand.Uint32()
		msg1 := TestProto.encodeDialCmd(connID1, remoteID1)

		err := codec.Send(&msg1)
		utest.IsNilNow(t, err)

		msg2, err := codec.Receive()
		utest.IsNilNow(t, err)

		msg3 := *(msg2.(*[]byte))
		cmd := TestProto.decodeCmd(msg3)
		utest.EqualNow(t, cmd, dialCmd)

		connID2, remoteID2 := TestProto.decodeDialCmd(msg3)
		utest.EqualNow(t, connID2, connID2)
		utest.EqualNow(t, remoteID1, remoteID2)
	}
}

func Test_AcceptCmd(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	utest.IsNilNow(t, err)
	defer conn.Close()

	codec := TestProto.newCodec(conn, 1024)

	for i := 0; i < 10000; i++ {
		connID1 := rand.Uint32()
		remoteID1 := rand.Uint32()
		msg1 := TestProto.encodeAcceptCmd(connID1, remoteID1)

		err := codec.Send(&msg1)
		utest.IsNilNow(t, err)

		msg2, err := codec.Receive()
		utest.IsNilNow(t, err)

		msg3 := *(msg2.(*[]byte))
		cmd := TestProto.decodeCmd(msg3)
		utest.EqualNow(t, cmd, acceptCmd)

		connID2, remoteID2 := TestProto.decodeAcceptCmd(msg3)
		utest.EqualNow(t, connID1, connID2)
		utest.EqualNow(t, remoteID1, remoteID2)
	}
}

func Test_RefuseCmd(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	utest.IsNilNow(t, err)
	defer conn.Close()

	codec := TestProto.newCodec(conn, 1024)

	for i := 0; i < 10000; i++ {
		remoteID1 := rand.Uint32()
		msg1 := TestProto.encodeRefuseCmd(remoteID1)

		err := codec.Send(&msg1)
		utest.IsNilNow(t, err)

		msg2, err := codec.Receive()
		utest.IsNilNow(t, err)

		msg3 := *(msg2.(*[]byte))
		cmd := TestProto.decodeCmd(msg3)
		utest.EqualNow(t, cmd, refuseCmd)

		remoteID2 := TestProto.decodeRefuseCmd(msg3)
		utest.EqualNow(t, remoteID1, remoteID2)
	}
}

func Test_CloseCmd(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	utest.IsNilNow(t, err)
	defer conn.Close()

	codec := TestProto.newCodec(conn, 1024)

	for i := 0; i < 10000; i++ {
		conndID1 := rand.Uint32()
		msg1 := TestProto.encodeCloseCmd(conndID1)

		err := codec.Send(&msg1)
		utest.IsNilNow(t, err)

		msg2, err := codec.Receive()
		utest.IsNilNow(t, err)

		msg3 := *(msg2.(*[]byte))
		cmd := TestProto.decodeCmd(msg3)
		utest.EqualNow(t, cmd, closeCmd)

		conndID2 := TestProto.decodeCloseCmd(msg3)
		utest.EqualNow(t, conndID1, conndID2)
	}
}

func Test_PingCmd(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	utest.IsNilNow(t, err)
	defer conn.Close()

	codec := TestProto.newCodec(conn, 1024)

	for i := 0; i < 10000; i++ {
		msg1 := TestProto.encodePingCmd()

		err := codec.Send(&msg1)
		utest.IsNilNow(t, err)

		msg2, err := codec.Receive()
		utest.IsNilNow(t, err)

		msg3 := *(msg2.(*[]byte))
		cmd := TestProto.decodeCmd(msg3)
		utest.EqualNow(t, cmd, pingCmd)
	}
}

func Test_ServerHandshake(t *testing.T) {
	lsn, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	lsn.Addr().String()

	go func() {
		conn, err := net.Dial("tcp", lsn.Addr().String())
		utest.IsNilNow(t, err)
		TestProto.serverInit(conn, 123, []byte("test"), time.Second*3)
	}()

	conn, err := lsn.Accept()
	utest.IsNilNow(t, err)

	serverID, err := TestProto.serverAuth(conn, []byte("test"), time.Second*3)
	utest.IsNilNow(t, err)
	utest.EqualNow(t, serverID, 123)
}

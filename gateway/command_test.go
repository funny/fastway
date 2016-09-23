package gateway

import (
	"math/rand"
	"net"
	"testing"

	"github.com/funny/utest"
)

func Test_DialCmd(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	utest.IsNilNow(t, err)
	defer conn.Close()

	proto := &Protocol{
		MaxPacketSize: 2048,
		Pool:          TestPool,
	}
	codec := NewCodec(proto, conn, 1024)

	for i := 0; i < 10000; i++ {
		remoteID1 := rand.Uint32()
		msg1 := proto.EncodeDialCmd(remoteID1)

		err := codec.Send(&msg1)
		utest.IsNilNow(t, err)

		msg2, err := codec.Receive()
		utest.IsNilNow(t, err)

		msg3 := *(msg2.(*[]byte))
		cmd := proto.DecodeCmd(msg3)
		utest.EqualNow(t, cmd, DialCmd)

		remoteID2 := proto.DecodeDialCmd(msg3)
		utest.EqualNow(t, remoteID1, remoteID2)
	}
}

func Test_AcceptCmd(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	utest.IsNilNow(t, err)
	defer conn.Close()

	proto := &Protocol{
		MaxPacketSize: 2048,
		Pool:          TestPool,
	}
	codec := NewCodec(proto, conn, 1024)

	for i := 0; i < 10000; i++ {
		remoteID1 := rand.Uint32()
		connID1 := rand.Uint32()
		msg1 := proto.EncodeAcceptCmd(remoteID1, connID1)

		err := codec.Send(&msg1)
		utest.IsNilNow(t, err)

		msg2, err := codec.Receive()
		utest.IsNilNow(t, err)

		msg3 := *(msg2.(*[]byte))
		cmd := proto.DecodeCmd(msg3)
		utest.EqualNow(t, cmd, AcceptCmd)

		remoteID2, connID2 := proto.DecodeAcceptCmd(msg3)
		utest.EqualNow(t, remoteID1, remoteID2)
		utest.EqualNow(t, connID1, connID2)
	}
}

func Test_RefuseCmd(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	utest.IsNilNow(t, err)
	defer conn.Close()

	proto := &Protocol{
		MaxPacketSize: 2048,
		Pool:          TestPool,
	}
	codec := NewCodec(proto, conn, 1024)

	for i := 0; i < 10000; i++ {
		remoteID1 := rand.Uint32()
		msg1 := proto.EncodeRefuseCmd(remoteID1)

		err := codec.Send(&msg1)
		utest.IsNilNow(t, err)

		msg2, err := codec.Receive()
		utest.IsNilNow(t, err)

		msg3 := *(msg2.(*[]byte))
		cmd := proto.DecodeCmd(msg3)
		utest.EqualNow(t, cmd, RefuseCmd)

		remoteID2 := proto.DecodeRefuseCmd(msg3)
		utest.EqualNow(t, remoteID1, remoteID2)
	}
}

func Test_CloseCmd(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	utest.IsNilNow(t, err)
	defer conn.Close()

	proto := &Protocol{
		MaxPacketSize: 2048,
		Pool:          TestPool,
	}
	codec := NewCodec(proto, conn, 1024)

	for i := 0; i < 10000; i++ {
		conndID1 := rand.Uint32()
		msg1 := proto.EncodeCloseCmd(conndID1)

		err := codec.Send(&msg1)
		utest.IsNilNow(t, err)

		msg2, err := codec.Receive()
		utest.IsNilNow(t, err)

		msg3 := *(msg2.(*[]byte))
		cmd := proto.DecodeCmd(msg3)
		utest.EqualNow(t, cmd, CloseCmd)

		conndID2 := proto.DecodeCloseCmd(msg3)
		utest.EqualNow(t, conndID1, conndID2)
	}
}

func Test_PingCmd(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	utest.IsNilNow(t, err)
	defer conn.Close()

	proto := &Protocol{
		MaxPacketSize: 2048,
		Pool:          TestPool,
	}
	codec := NewCodec(proto, conn, 1024)

	for i := 0; i < 10000; i++ {
		msg1 := proto.EncodePingCmd()

		err := codec.Send(&msg1)
		utest.IsNilNow(t, err)

		msg2, err := codec.Receive()
		utest.IsNilNow(t, err)

		msg3 := *(msg2.(*[]byte))
		cmd := proto.DecodeCmd(msg3)
		utest.EqualNow(t, cmd, PingCmd)
	}
}

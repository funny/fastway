package gateway

import (
	"encoding/binary"
	"io"
	"math/rand"
	"net"
	"testing"

	"github.com/funny/link"
	"github.com/funny/slab"
	"github.com/funny/utest"
)

var TestAddr string
var TestPool = slab.NewAtomPool(64, 64*1024, 2, 1024*1024)
var TestProto = protocol{TestPool, 2048}

func init() {
	lsn, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	TestAddr = lsn.Addr().String()

	go func() {
		for {
			conn, err := lsn.Accept()
			if err != nil {
				return
			}
			go func() {
				io.Copy(conn, conn)
				conn.Close()
			}()
		}
	}()
}

func Test_Codec(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	utest.IsNilNow(t, err)
	defer conn.Close()

	codec := TestProto.newCodec(conn, 1024)

	for i := 0; i < 1000; i++ {
		buffer1 := TestPool.Alloc(SizeofLen + rand.Intn(1024))
		binary.LittleEndian.PutUint32(buffer1, uint32(len(buffer1)-SizeofLen))
		for i := SizeofLen; i < len(buffer1); i++ {
			buffer1[i] = byte(rand.Intn(256))
		}

		buffer2 := make([]byte, len(buffer1))
		copy(buffer2, buffer1)

		codec.Send(&buffer1)

		buffer3, err := codec.Receive()
		utest.IsNilNow(t, err)
		utest.EqualNow(t, buffer2, *(buffer3.(*[]byte)))
	}
}

type TestMsgFormat struct{}

func (_ TestMsgFormat) SizeofMsg(msg interface{}) int { return len(msg.([]byte)) }
func (_ TestMsgFormat) MarshalMsg(msg interface{}, buf []byte) error {
	copy(buf, msg.([]byte))
	return nil
}
func (_ TestMsgFormat) UnmarshalMsg(buf []byte) (interface{}, error) {
	msg := make([]byte, len(buf))
	copy(msg, buf)
	return msg, nil
}

func Test_VirtualCodec(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	utest.IsNilNow(t, err)
	defer conn.Close()

	codec := TestProto.newCodec(conn, 1024)
	pconn := link.NewSession(codec, 1000)
	vcodec := TestProto.newVirtualCodec(pconn, 123, TestMsgFormat{})

	for i := 0; i < 1000; i++ {
		buffer1 := make([]byte, 1024)
		for i := 0; i < len(buffer1); i++ {
			buffer1[i] = byte(rand.Intn(256))
		}

		vcodec.Send(buffer1)

		msg, err := codec.Receive()
		utest.IsNilNow(t, err)

		buffer2 := *(msg.(*[]byte))
		connID := codec.decodePacket(buffer2)
		utest.EqualNow(t, connID, 123)
		vcodec.recvChan <- buffer2

		buffer3, err := vcodec.Receive()
		utest.IsNilNow(t, err)
		utest.EqualNow(t, buffer1, buffer3)
	}
}

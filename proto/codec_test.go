package proto

import (
	"encoding/binary"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"sync"
	"testing"

	"github.com/funny/link"
	"github.com/funny/slab"
	"github.com/funny/utest"
)

var TestAddr string
var TestPool = slab.NewSyncPool(64, 64*1024, 2)
var TestProto = protocol{TestPool, 2048}

func init() {
	lsn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	TestAddr = lsn.Addr().String()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		wg.Done()
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
	wg.Wait()

	logFile, err := os.Create("proto.test.log")
	if err != nil {
		panic(err)
	}
	log.SetOutput(logFile)
}

type NullWriter struct{}

func (w NullWriter) Write(b []byte) (int, error) { return len(b), nil }

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

func Test_VirtualCodec(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	utest.IsNilNow(t, err)
	defer conn.Close()

	codec := TestProto.newCodec(conn, 1024)
	pconn := link.NewSession(codec, 1000)
	vcodec := TestProto.newVirtualCodec(pconn, 123, 1024)

	for i := 0; i < 1000; i++ {
		buffer1 := make([]byte, 1024)
		for i := 0; i < len(buffer1); i++ {
			buffer1[i] = byte(rand.Intn(256))
		}
		vcodec.Send(&buffer1)

		msg, err := codec.Receive()
		utest.IsNilNow(t, err)

		buffer2 := *(msg.(*[]byte))
		connID := codec.decodePacket(buffer2)
		utest.EqualNow(t, connID, 123)
		vcodec.recvChan <- buffer2

		buffer3, err := vcodec.Receive()
		utest.IsNilNow(t, err)
		utest.EqualNow(t, buffer1, *(buffer3.(*[]byte)))
	}
}

func Test_BadCodec(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	utest.IsNilNow(t, err)

	codec := TestProto.newCodec(conn, 1024)
	_, err = conn.Write([]byte{255, 0, 0, 0})
	utest.IsNilNow(t, err)
	codec.reader.ReadByte()
	codec.reader.UnreadByte()
	conn.Close()

	msg, err := codec.Receive()
	utest.IsNilNow(t, msg)
	utest.NotNilNow(t, err)

	conn, err = net.Dial("tcp", TestAddr)
	utest.IsNilNow(t, err)

	codec = TestProto.newCodec(conn, 1024)
	_, err = conn.Write([]byte{255, 255, 255, 255})
	utest.IsNilNow(t, err)

	msg, err = codec.Receive()
	utest.IsNilNow(t, msg)
	utest.NotNilNow(t, err)
	utest.EqualNow(t, err, ErrTooLargePacket)
}

func Test_BadVirtualCodec(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	utest.IsNilNow(t, err)
	defer conn.Close()

	codec := TestProto.newCodec(conn, 1024)
	pconn := link.NewSession(codec, 1000)
	vcodec := TestProto.newVirtualCodec(pconn, 123, 1024)

	bigMsg := make([]byte, TestProto.maxPacketSize+1)
	err = vcodec.Send(&bigMsg)
	utest.NotNilNow(t, err)
	utest.EqualNow(t, err, ErrTooLargePacket)

	vcodec.recvChan <- bigMsg
	vcodec.Close()
}

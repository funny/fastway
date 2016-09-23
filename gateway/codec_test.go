package gateway

import (
	"encoding/binary"
	"io"
	"math/rand"
	"net"
	"testing"

	"github.com/funny/slab"
	"github.com/funny/utest"
)

var TestAddr string
var TestPool = slab.NewAtomPool(64, 64*1024, 2, 1024*1024)

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

	codec := NewCodec(&Protocol{
		MaxPacketSize: 2048,
		Pool:          TestPool,
	}, conn, 1024)

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

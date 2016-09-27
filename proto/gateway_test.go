package proto

import (
	"math/rand"
	"net"
	"runtime"
	"sync"
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

	go gw.ServeClients(lsn1, 10000, 1024, 1024, 30)
	go gw.ServeServers(lsn2, "123", 3, 1024, 1024, 30)

	time.Sleep(time.Second)

	client, err := DialClient(lsn1.Addr().String(), TestPool, 2048, 1024, 1024)
	utest.IsNilNow(t, err)

	server, err := DialServer(lsn2.Addr().String(), TestPool, 123, "123", 3, 2048, 1024, 1024)
	utest.IsNilNow(t, err)

	// make sure connection registered
L:
	for {
		n := 0
		for i := 0; i < len(gw.physicalConns); i++ {
			n += gw.physicalConns[i][0].Len()
			n += gw.physicalConns[i][1].Len()
			if n >= 2 {
				break L
			}
		}
		time.Sleep(time.Second)
	}

	var clientID uint32
	var vconns [2][2]*link.Session
	var connIDs [2][2]uint32
	var acceptChans = [2]chan int{
		make(chan int),
		make(chan int),
	}

	go func() {
		var err error

		vconns[0][0], connIDs[0][0], clientID, err = server.Accept()
		utest.IsNilNow(t, err)
		acceptChans[0] <- 1

		vconns[1][0], connIDs[1][0], _, err = client.Accept()
		utest.IsNilNow(t, err)
		acceptChans[1] <- 1
	}()

	vconns[0][1], connIDs[0][1], err = client.Dial(123)
	utest.IsNilNow(t, err)
	<-acceptChans[0]

	vconns[1][1], connIDs[1][1], err = server.Dial(clientID)
	utest.IsNilNow(t, err)
	<-acceptChans[1]

	utest.EqualNow(t, connIDs[0][0], connIDs[0][1])
	utest.EqualNow(t, connIDs[1][0], connIDs[1][1])

	for i := 0; i < 10000; i++ {
		buffer1 := make([]byte, 1024)
		for i := 0; i < len(buffer1); i++ {
			buffer1[i] = byte(rand.Intn(256))
		}

		x := rand.Intn(2)
		y := rand.Intn(2)

		err := vconns[x][y].Send(&buffer1)
		utest.IsNilNow(t, err)

		buffer2, err := vconns[x][(y+1)%2].Receive()
		utest.IsNilNow(t, err)
		utest.EqualNow(t, buffer1, *(buffer2.(*[]byte)))
	}

	vconns[0][0].Close()
	vconns[0][1].Close()
	vconns[1][0].Close()
	vconns[1][1].Close()

	time.Sleep(time.Second)

	utest.EqualNow(t, 0, client.virtualConns.Len())
	utest.EqualNow(t, 0, server.virtualConns.Len())

	for i := 0; i < len(gw.virtualConns); i++ {
		utest.EqualNow(t, 0, len(gw.virtualConns[i]))
	}

	gw.Stop()
}

func Test_GatewayParallel(t *testing.T) {
	lsn1, err := net.Listen("tcp", ":0")
	utest.IsNilNow(t, err)

	lsn2, err := net.Listen("tcp", ":0")
	utest.IsNilNow(t, err)

	gw := NewGateway(TestPool, 2048)

	go gw.ServeClients(lsn1, 10000, 1024, 1024, 30)
	go gw.ServeServers(lsn2, "123", 3, 1024, 1024, 30)

	time.Sleep(time.Second)

	client, err := DialClient(lsn1.Addr().String(), TestPool, 2048, 1024, 1024)
	utest.IsNilNow(t, err)

	server, err := DialServer(lsn2.Addr().String(), TestPool, 123, "123", 3, 2048, 1024, 1024)
	utest.IsNilNow(t, err)

	// make sure connection registered
L:
	for {
		n := 0
		for i := 0; i < len(gw.physicalConns); i++ {
			n += gw.physicalConns[i][0].Len()
			n += gw.physicalConns[i][1].Len()
			if n >= 2 {
				break L
			}
		}
		time.Sleep(time.Second)
	}

	go func() {
		for {
			vconn, _, _, err := server.Accept()
			utest.IsNilNow(t, err)
			go func() {
				for {
					msg, err := vconn.Receive()
					if err != nil {
						return
					}
					if vconn.Send(msg) != nil {
						return
					}
				}
			}()
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < runtime.GOMAXPROCS(-1); i++ {
		wg.Add(1)
		go func() {
			vconns, _, err := client.Dial(123)
			utest.IsNilNow(t, err)

			n := 5000 + rand.Intn(5000)

			for i := 0; i < n; i++ {
				buffer1 := make([]byte, 1024)
				for i := 0; i < len(buffer1); i++ {
					buffer1[i] = byte(rand.Intn(256))
				}
				err := vconns.Send(&buffer1)
				utest.IsNilNow(t, err)

				buffer2, err := vconns.Receive()
				utest.IsNilNow(t, err)
				utest.EqualNow(t, buffer1, *(buffer2.(*[]byte)))
			}

			vconns.Close()
			wg.Done()
		}()
	}

	wg.Wait()
	time.Sleep(time.Second)

	utest.EqualNow(t, 0, client.virtualConns.Len())
	utest.EqualNow(t, 0, server.virtualConns.Len())

	for i := 0; i < len(gw.virtualConns); i++ {
		utest.EqualNow(t, 0, len(gw.virtualConns[i]))
	}

	gw.Stop()
}

package fastway

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
	lsn1, err := net.Listen("tcp", "127.0.0.1:0")
	utest.IsNilNow(t, err)
	defer lsn1.Close()

	lsn2, err := net.Listen("tcp", "127.0.0.1:0")
	utest.IsNilNow(t, err)
	defer lsn2.Close()

	gw := NewGateway(TestPool, 2048)

	go gw.ServeClients(lsn1, 10000, 1024, 1024, 30*time.Second)
	go gw.ServeServers(lsn2, "123", 3*time.Second, 1024, 1024, 30*time.Second)

	time.Sleep(time.Second)

	client, err := DialClient("tcp", lsn1.Addr().String(), TestPool, 2048, 1024, 10240, 10240)
	utest.IsNilNow(t, err)

	server, err := DialServer("tcp", lsn2.Addr().String(), TestPool, 123, "123", 3*time.Second, 2048, 1024, 10240, 10240)
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
		runtime.Gosched()
	}

	utest.IsNilNow(t, client.Ping())
	utest.IsNilNow(t, server.Ping())
	time.Sleep(time.Second)
	utest.Assert(t, client.LastActive().Unix() != 0)
	utest.Assert(t, server.LastActive().Unix() != 0)

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
		gw.virtualConnMutexes[i].Lock()
		utest.EqualNow(t, 0, len(gw.virtualConns[i]))
		gw.virtualConnMutexes[i].Unlock()
	}

	gw.Stop()
}

func Test_GatewayParallel(t *testing.T) {
	lsn1, err := net.Listen("tcp", "127.0.0.1:0")
	utest.IsNilNow(t, err)
	defer lsn1.Close()

	lsn2, err := net.Listen("tcp", "127.0.0.1:0")
	utest.IsNilNow(t, err)
	defer lsn2.Close()

	gw := NewGateway(TestPool, 2048)

	go gw.ServeClients(lsn1, 10000, 1024, 10240, 30*time.Second)
	go gw.ServeServers(lsn2, "123", 3*time.Second, 1024, 10240, 30*time.Second)

	time.Sleep(time.Second)

	client, err := DialClient("tcp", lsn1.Addr().String(), TestPool, 2048, 1024, 10240, 10240)
	utest.IsNilNow(t, err)

	server, err := DialServer("tcp", lsn2.Addr().String(), TestPool, 123, "123", 3*time.Second, 2048, 1024, 10240, 10240)
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
			if err != nil {
				return
			}
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
	var errors = make([]error, runtime.GOMAXPROCS(-1))
	var errorInfos = make([]string, len(errors))
	for i := 0; i < len(errors); i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()

			var vconns *link.Session
			vconns, _, errors[n] = client.Dial(123)
			if errors[n] != nil {
				errorInfos[n] = "dial"
				return
			}
			defer vconns.Close()

			times := 3000 + rand.Intn(3000)
			for i := 0; i < times; i++ {
				buffer1 := make([]byte, 1024)
				for i := 0; i < len(buffer1); i++ {
					buffer1[i] = byte(rand.Intn(256))
				}

				errors[n] = vconns.Send(&buffer1)
				if errors[n] != nil {
					errorInfos[n] = "send"
					return
				}

				var buffer2 interface{}
				buffer2, errors[n] = vconns.Receive()
				if errors[n] != nil {
					errorInfos[n] = "receive"
					return
				}

				utest.EqualNow(t, buffer1, *(buffer2.(*[]byte)))
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(time.Second)

	var failed bool
	for i := 0; i < len(errors); i++ {
		utest.IsNil(t, errors[i])
		if !failed && errors[i] != nil {
			failed = true
			t.Log(i, errorInfos[i], errors[i])
		}
	}
	utest.Assert(t, !failed)

	utest.EqualNow(t, 0, client.virtualConns.Len())
	utest.EqualNow(t, 0, server.virtualConns.Len())

	for i := 0; i < len(gw.virtualConns); i++ {
		gw.virtualConnMutexes[i].Lock()
		utest.EqualNow(t, 0, len(gw.virtualConns[i]))
		gw.virtualConnMutexes[i].Unlock()
	}

	gw.Stop()
}

func Test_BadClients(t *testing.T) {
	lsn1, err := net.Listen("tcp", "127.0.0.1:0")
	utest.IsNilNow(t, err)

	lsn2, err := net.Listen("tcp", "127.0.0.1:0")
	utest.IsNilNow(t, err)

	gw := NewGateway(TestPool, 2048)

	go gw.ServeClients(lsn1, 10000, 1024, 1024, 30*time.Second)
	go gw.ServeServers(lsn2, "123", 3*time.Second, 1024, 1024, 30*time.Second)

	time.Sleep(time.Second)

	conn, err := net.Dial("tcp", lsn1.Addr().String())
	utest.IsNilNow(t, err)
	conn.Write([]byte{0, 0, 0})
	conn.Close()

	conn, err = net.Dial("tcp", lsn2.Addr().String())
	utest.IsNilNow(t, err)
	conn.Write([]byte{0, 0, 0})
	conn.Close()

	conn, err = net.Dial("tcp", lsn1.Addr().String())
	utest.IsNilNow(t, err)
	conn.Write(TestProto.encodeRefuseCmd(123))
	conn.Close()
}

func Test_BadEndpoint(t *testing.T) {
	lsn1, err := net.Listen("tcp", "127.0.0.1:0")
	utest.IsNilNow(t, err)
	defer lsn1.Close()

	lsn2, err := net.Listen("tcp", "127.0.0.1:0")
	utest.IsNilNow(t, err)
	defer lsn2.Close()

	gw := NewGateway(TestPool, 2048)

	go gw.ServeClients(lsn1, 10000, 1024, 1024, time.Second)
	go gw.ServeServers(lsn2, "123", 3*time.Second, 1024, 1024, time.Second)

	// bad remote ID
	client, err := DialClient("tcp", lsn1.Addr().String(), TestPool, 2048, 1024, 10240, 10240)
	utest.IsNilNow(t, err)
	_, _, err = client.Dial(12345)
	utest.NotNilNow(t, err)
	utest.EqualNow(t, err, ErrRefused)

	// bad address
	_, err = DialClient("tcp", "127.0.0.1:0", TestPool, 2048, 1024, 10240, 10240)
	utest.NotNilNow(t, err)
	_, err = DialServer("tcp", "127.0.0.1:0", TestPool, 123, "bad key", 3*time.Second, 2048, 1024, 10240, 10240)
	utest.NotNilNow(t, err)

	// idle timeout
	conn, err := net.Dial("tcp", lsn1.Addr().String())
	utest.IsNilNow(t, err)
	defer conn.Close()
	_, err = DialClient("tcp", lsn1.Addr().String(), TestPool, 2048, 1024, 10240, 10240)
	utest.IsNilNow(t, err)
	time.Sleep(time.Second * 4)

	// bad key
	server, _ := DialServer("tcp", lsn2.Addr().String(), TestPool, 123, "bad key", 1*time.Second, 2048, 1024, 10240, 10240)
	utest.NotNilNow(t, server)
	var x = []byte{0, 0, 0}
	_, err = server.session.Codec().(*codec).conn.Read(x)
	utest.NotNilNow(t, err)

	// bad gateway 1
	lsn3, err := net.Listen("tcp", "127.0.0.1:0")
	utest.IsNilNow(t, err)
	defer lsn3.Close()
	go func() {
		conn, err := lsn3.Accept()
		if err == nil {
			conn.Close()
		}
	}()
	_, err = DialServer("tcp", lsn3.Addr().String(), TestPool, 123, "bad key", 1*time.Second, 2048, 1024, 10240, 10240)
	utest.NotNilNow(t, err)
}

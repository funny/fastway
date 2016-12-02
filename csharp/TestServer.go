package main

import (
	"log"
	"net"
	"time"

	fastway "github.com/funny/fastway/go"
	"github.com/funny/slab"
)

type TestMsgFormat struct{}

func (f *TestMsgFormat) EncodeMessage(msg interface{}) ([]byte, error) {
	buf := make([]byte, len(msg.([]byte)))
	copy(buf, msg.([]byte))
	return buf, nil
}

func (f *TestMsgFormat) DecodeMessage(msg []byte) (interface{}, error) {
	buf := make([]byte, len(msg))
	copy(buf, msg)
	return buf, nil
}

func main() {
	lsn1, err := net.Listen("tcp", "127.0.0.1:10010")
	if err != nil {
		log.Fatal(err)
	}

	lsn2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}

	pool := slab.NewAtomPool(64, 64*1024, 2, 1024*1024)

	gateway := fastway.NewGateway(pool, 512*1024)

	go gateway.ServeClients(lsn1, fastway.GatewayCfg{
		MaxConn:      10,
		BufferSize:   1024,
		SendChanSize: 10000,
		IdleTimeout:  time.Second * 3,
	})

	go gateway.ServeServers(lsn2, fastway.GatewayCfg{
		AuthKey:      "test key",
		BufferSize:   1024,
		SendChanSize: 10000,
		IdleTimeout:  time.Second * 3,
	})

	server, err := fastway.DialServer("tcp", lsn2.Addr().String(),
		fastway.EndPointCfg{
			ServerID:     10086,
			AuthKey:      "test key",
			MemPool:      pool,
			MaxPacket:    512 * 1024,
			BufferSize:   1024,
			SendChanSize: 10000,
			RecvChanSize: 10000,
			PingInterval: time.Second,
			PingTimeout:  time.Second,
			MsgFormat:    &TestMsgFormat{},
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	defer server.Close()

	for {
		session, err := server.Accept()
		if err != nil {
			log.Fatal(err)
		}
		conn := session.State.(*fastway.ConnInfo)
		log.Printf("new connection: %d, %d", conn.ConnID(), conn.RemoteID())

		go func() {
			defer session.Close()

			for {
				msg, err := session.Receive()
				if err != nil {
					log.Printf("receive failed: %v", err)
					return
				}

				err = session.Send(msg)
				if err != nil {
					log.Printf("send failed: %v", err)
					return
				}
			}
		}()
	}
}

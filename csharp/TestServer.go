package main

import (
	"log"
	"net"
	"time"

	fastway "github.com/funny/fastway/go"
	"github.com/funny/slab"
)

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
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	defer server.Close()

	for {
		conn, connID, remoteID, err := server.Accept()
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("new connection: %d, %d", connID, remoteID)

		go func() {
			defer conn.Close()

			for {
				msg, err := conn.Receive()
				if err != nil {
					log.Printf("receive failed: %v", err)
					return
				}

				err = conn.Send(msg)
				if err != nil {
					log.Printf("receive failed: %v", err)
					return
				}
			}
		}()
	}
}

package main

import (
	"log"
	"net"
	"time"

	fastway "github.com/fast/fastway/go"
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
	go gateway.ServeClients(lsn1, 10, 1024, 10000, time.Second)
	go gateway.ServeServers(lsn2, "test key", time.Second*3, 1024, 10000, time.Second)

	server, err := fastway.DialServer(
		lsn2.Addr().String(), pool,
		10086, "test key", time.Second*3,
		512*1024, 1024, 10000, 10000,
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

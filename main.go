package main

import (
	"flag"
	"log"
	"net"
	"time"

	"github.com/funny/cmd"
	fastway "github.com/funny/fastway/go"
	"github.com/funny/reuseport"
	"github.com/funny/slab"
	snet "github.com/funny/snet/go"
)

var (
	reusePort       = flag.Bool("ReusePort", false, "Enable/Disable the reuseport feature.")
	maxPacketSize   = flag.Int("MaxPacketSize", 512*1024, "Limit max packet size.")
	memPoolType     = flag.String("MemPoolType", "atom", "Type of memory pool ('sync', 'atom' or 'chan').")
	memPoolFactor   = flag.Int("MemPoolFactor", 2, "Growth in chunk size in memory pool.")
	memPoolMinChunk = flag.Int("MemPoolMinChunk", 64, "Smallest chunk size in memory pool.")
	memPoolMaxChunk = flag.Int("MemPoolMaxChunk", 64*1024, "Largest chunk size in memory pool.")
	memPoolPageSize = flag.Int("MemPoolPageSize", 1024*1024, "Size of each slab in memory pool.")

	clientAddr            = flag.String("ClientAddr", ":0", "The gateway address where clients connect to.")
	clientMaxConn         = flag.Int("ClientMaxConn", 16, "Limit max virtual connections for each client.")
	clientBufferSize      = flag.Int("ClientBufferSize", 2*1024, "Setting bufio.Reader's buffer size.")
	clientSendChanSize    = flag.Int("ClientSendChanSize", 1024, "Tunning client session's async behavior.")
	clientPingInterval    = flag.Duration("ClientPingInterval", 30*time.Second, "The interval of that gateway sending PING command to client.")
	clientSnetEnable      = flag.Bool("ClientSnetEnable", false, "Enable/Disable snet protocol for clients.")
	clientSnetEncrypt     = flag.Bool("ClientSnetEncrypt", false, "Enable/Disable client snet protocol encrypt feature.")
	clientSnetBuffer      = flag.Int("ClientSnetBuffer", 64*1024, "Client snet protocol rewriter buffer size.")
	clientSnetInitTimeout = flag.Duration("ClientSnetInitTimeout", 10*time.Second, "Client snet protocol handshake timeout.")
	clientSnetWaitTimeout = flag.Duration("ClientSnetWaitTimeout", 60*time.Second, "Client snet protocol waitting reconnection timeout.")

	serverAddr            = flag.String("ServerAddr", ":0", "The gateway address where servers connect to.")
	serverAuthTimeout     = flag.Duration("ServerAuthTimeout", 3*time.Second, "Server auth IO waiting timeout.")
	serverAuthKey         = flag.String("ServerAuthKey", "", "The private key used to auth server connection.")
	serverBufferSize      = flag.Int("ServerBufferSize", 64*1024, "Buffer size of bufio.Reader for server connections.")
	serverSendChanSize    = flag.Int("ServerSendChanSize", 102400, "Tunning server session's async behavior, this value must be greater than zero.")
	serverPingInterval    = flag.Duration("ServerPingInterval", 30*time.Second, "The interval of that gateway sending PING command to server.")
	serverSnetEnable      = flag.Bool("ServerSnetEnable", false, "Enable/Disable snet protocol for server.")
	serverSnetEncrypt     = flag.Bool("ServerSnetEncrypt", false, "Enable/Disable server snet protocol encrypt feature.")
	serverSnetBuffer      = flag.Int("ServerSnetBuffer", 64*1024, "Server snet protocol rewriter buffer size.")
	serverSnetInitTimeout = flag.Duration("ServerSnetInitTimeout", 10*time.Second, "Server snet protocol handshake timeout.")
	serverSnetWaitTimeout = flag.Duration("ServerSnetWaitTimeout", 60*time.Second, "Server snet protocol waitting reconnection timeout.")
)

func main() {
	flag.Parse()

	if *serverSendChanSize <= 0 {
		println("server send chan size must greater than zero.")
	}

	var pool slab.Pool
	switch *memPoolType {
	case "sync":
		pool = slab.NewSyncPool(*memPoolMinChunk, *memPoolMaxChunk, *memPoolFactor)
	case "atom":
		pool = slab.NewAtomPool(*memPoolMinChunk, *memPoolMaxChunk, *memPoolFactor, *memPoolPageSize)
	case "chan":
		pool = slab.NewChanPool(*memPoolMinChunk, *memPoolMaxChunk, *memPoolFactor, *memPoolPageSize)
	default:
		println(`unsupported memory pool type, must be "sync", "atom" or "chan"`)
	}

	gw := fastway.NewGateway(pool, *maxPacketSize)

	go gw.ServeClients(
		listen("client", *clientAddr, *reusePort,
			*clientSnetEnable,
			*clientSnetEncrypt,
			*clientSnetBuffer,
			*clientSnetInitTimeout,
			*clientSnetWaitTimeout,
		),
		*clientMaxConn,
		*clientBufferSize,
		*clientSendChanSize,
		*clientPingInterval,
	)

	go gw.ServeServers(
		listen("server", *serverAddr, *reusePort,
			*serverSnetEnable,
			*serverSnetEncrypt,
			*serverSnetBuffer,
			*serverSnetInitTimeout,
			*serverSnetWaitTimeout,
		),
		*serverAuthKey,
		*serverAuthTimeout,
		*serverBufferSize,
		*serverSendChanSize,
		*serverPingInterval,
	)

	cmd.Shell("fastway")

	gw.Stop()
}

func listen(who, addr string, reuse, snetEnable, snetEncrypt bool, snetBuffer int, snetInitTimeout, snetWaitTimeout time.Duration) net.Listener {
	var lsn net.Listener
	var err error

	if reuse {
		lsn, err = reuseport.NewReusablePortListener("tcp", addr)
	} else {
		lsn, err = net.Listen("tcp", addr)
	}

	if err != nil {
		log.Fatalf("setup %s listener at %s failed - %s", who, addr, err)
	}

	if snetEnable {
		lsn, _ = snet.Listen(snet.Config{
			EnableCrypt:        snetEncrypt,
			RewriterBufferSize: snetBuffer,
			HandshakeTimeout:   snetInitTimeout,
			ReconnWaitTimeout:  snetWaitTimeout,
		}, func() (net.Listener, error) {
			return lsn, nil
		})
	}

	log.Printf("setup %s listener at - %s", who, lsn.Addr())
	return lsn
}

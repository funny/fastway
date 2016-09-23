package main

import (
	"flag"
	"math/rand"
	"time"

	"github.com/fastgo/gateway/gateway"
	"github.com/funny/cmd"
	"github.com/funny/slab"
)

var (
	// ReusePort enable/disable the reuseport feature.
	ReusePort = flag.Bool("ReusePort", false, "Enable/disable the reuseport feature.")

	// MaxPacketSize limit the incomming packet size.
	MaxPacketSize = flag.Int("MaxPacketSize", 512*1024, "Limit max packet size.")

	// MemPoolSize setting size of memory pool.
	MemPoolSize = flag.Int("MemPoolSize", 10*1024*1024, "Size of memory pool.")

	// MemPoolMinChunk setting smallest chunk size of memory pool.
	MemPoolMinChunk = flag.Int("MemPoolMinChunk", 64, "Smallest chunk size of memory pool.")

	// MemPoolMaxChunk setting largest chunk size of memory pool.
	MemPoolMaxChunk = flag.Int("MemPoolMaxChunk", 64*1024, "Largest chunk size of memory pool.")

	// MemPoolFactor setting growth in chunk size of memory pool.
	MemPoolFactor = flag.Int("MemPoolFactor", 2, "Growth in chunk size of memory pool.")
)

var (
	// ClientAddr is the address where clients connect to.
	ClientAddr = flag.String("ClientAddr", ":0", "Is the address where clients connect to.")

	// ClientMaxConn limit max virtual connection number of each client.
	ClientMaxConn = flag.Int("ClientMaxConn", 8, "Limit max virtual connections for each client.")

	// ClientBufferSize setting bufio.Reader's buffer size.
	ClientBufferSize = flag.Int("ClientBufferSize", 2*1024, "Setting bufio.Reader's buffer size.")

	// ClientSendChanSize tunning server session's async behavior.
	ClientSendChanSize = flag.Int("ClientSendChanSize", 1024, "Tunning client session's async behavior.")

	// ClientPingInterval settings the time interval of sending PING to check connection alive after last time receiving message.
	ClientPingInterval = flag.Int("ClientPingInterval", 30, "The time interval of sending PING to check client connection alive after last time receiving message.")
)

var (
	// ServerAddr is the address where servers connect to.
	ServerAddr = flag.String("ServerAddr", ":0", "Is the address where servers connect to.")

	// ServerAuthTimeout setting server auth IO waiting timeout.
	ServerAuthTimeout = flag.Int("ServerAuthTimeout", 3, "Server auth IO waiting timeout.")

	// ServerAuthKey is the private key used to auth server connection.
	ServerAuthKey = flag.String("ServerAuthKey", "", "The private key used to auth server connection.")

	// ServerBufferSize setting buffer size of bufio.Reader for server connections.
	ServerBufferSize = flag.Int("ServerBufferSize", 64*1024, "Buffer size of bufio.Reader for server connections.")

	// ServerSendChanSize tunning server session's async behavior, this value must greater be then zero.
	ServerSendChanSize = flag.Int("ServerSendChanSize", 102400, "Tunning server session's async behavior, this value must be greater then zero.")

	// ServerPingInterval setting the time interval of sending PING to check connection alive after last time receiving message.
	ServerPingInterval = flag.Int("ServerPingInterval", 30, "The time interval of sending PING to check server connection alive after last time receiving message.")
)

var Protocol gateway.Protocol

func main() {
	flag.Parse()

	Protocol = gateway.Protocol{
		Rand:              rand.New(rand.NewSource(time.Now().UnixNano())),
		Pool:              slab.NewAtomPool(*MemPoolMinChunk, *MemPoolMaxChunk, *MemPoolFactor, *MemPoolSize),
		MaxPacketSize:     *MaxPacketSize,
		ClientBufferSize:  *ClientBufferSize,
		ServerBufferSize:  *ServerBufferSize,
		ServerAuthKey:     []byte(*ServerAuthKey),
		ServerAuthTimeout: time.Duration(*ServerAuthTimeout),
	}

	go ServeClients()
	go ServeServers()

	cmd.Shell("gateway")
}

[![Foundation](https://img.shields.io/badge/Golang-Foundation-green.svg)](http://golangfoundation.org)

说明
====

本网关不是一个通用型的网关，它有一套自己的通讯协议，需要配套的客户端和服务端才能使用。

本网关的实现目的有以下几点：

1. 复用客户端网络连接
2. 减少暴露在公网的服务器
3. 提升网络故障转移的效率

拓扑结构：

![Gateway](https://raw.githubusercontent.com/fastgo/gateway/master/README.png)

基础逻辑：

+ 通讯过程中需要客户端和服务端主动连接到网关
+ 每个客户端只会连接到一个网关，并且只会建立一个物理连接
+ 每个服务端可以连接到多个网关，每个网关建立一个物理连接
+ 每个客户端可以通过网关协议跟多个服务端建立虚拟连接
+ 每个服务端也可以通过网关协议主动跟多个客户端建立虚拟连接

命令参数：

```
-ClientAddr string
	网关暴露给客户端的地址 (默认 ":0")

-ClientBufferSize int
	每个客户端连接使用的 bufio.Reader 缓冲区大小 (默认 2048字节)

-ClientMaxConn int
	每个客户端可以创建的最大虚拟连接数 (默认 8)

-ClientPingInterval int
	网关在多少秒没收到客户端消息后发送PING指令给客户端 (默认 30秒)

-ClientSendChanSize int
	每个客户端连接异步发送消息用的chan缓冲区大小 (默认 1024)

-MaxPacketSize int
	最大的消息包体积 (默认 524288字节)

-MemPoolFactor int
	Slab内存池的Chunk递增指数 (默认 2)

-MemPoolMaxChunk int
	Slab内存池中最大的Chunk大小 (默认 65536字节)

-MemPoolMinChunk int
	Slab内存池中最小的Chunk大小 (默认 64字节)

-MemPoolSize int
	Slab内存池的总内存大小 (默认 10485760字节)

-Password string
	用于验证服务端合法性的秘钥

-ReusePort
	是否开启reuseport特性

-ServerAddr string
	网关暴露给服务端的地址 (默认 ":0")

-ServerAuthTimeout int
	验证服务端连接时的最大IO等待时间 (默认 3秒)

-ServerBufferSize int
	每个服务端连接使用的 bufio.Reader 缓冲区大小 (默认 65536字节)

-ServerPingInterval int
	网关在多少秒没收到客户端消息后发送PING指令给客户端 (默认 30秒)

-ServerSendChanSize int
	每个服务端连接异步发送消息用的chan缓冲区大小 (默认 102400)
```

通讯协议：

[通讯协议文档](https://github.com/fastgo/gateway/tree/master/gateway)

[![Go Foundation](https://img.shields.io/badge/go-foundation-green.svg)](http://golangfoundation.org)
[![Go Report Card](https://goreportcard.com/badge/github.com/fast/fastway)](https://goreportcard.com/report/github.com/fast/fastway)
[![Build Status](https://travis-ci.org/fast/fastway.svg?branch=master)](https://travis-ci.org/fast/fastway)
[![codecov](https://codecov.io/gh/fast/fastway/branch/master/graph/badge.svg)](https://codecov.io/gh/fast/fastway)
[![GoDoc](https://img.shields.io/badge/api-reference-blue.svg)](https://godoc.org/github.com/fast/fastway/proto)

简介
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

使用说明
=======

本网关提供了一个命令行程序用来对外提供网关服务。

命令行程序支持以下参数：

| 参数名 | 说明 | 默认值 |
| --- | --- | --- |
| ReusePort | 是否开启reuseport特性 | false |
| MaxPacketSize | 最大的消息包体积 | 512K |
| MemPoolType | Slab内存池类型 (sync、atom或chan) | atom |
| MemPoolFactor | Slab内存池的Chunk递增指数 | 2 |
| MemPoolMaxChunk | Slab内存池中最大的Chunk大小 | 64K |
| MemPoolMinChunk | Slab内存池中最小的Chunk大小 | 64B |
| MemPoolPageSize | Slab内存池的每个Slab内存大小 | 1M |
| ClientAddr | 网关暴露给客户端的地址 | ":0" |
| ClientMaxConn | 每个客户端可以创建的最大虚拟连接数 | 8 |
| ClientBufferSize | 每个客户端连接使用的 bufio.Reader 缓冲区大小 | 2K |
| ClientPingInterval | 网关在多少秒没收到客户端消息后发送PING指令给客户端 | 30秒 |
| ClientSendChanSize | 每个客户端连接异步发送消息用的chan缓冲区大小 | 1千 |
| ServerAddr | 网关暴露给服务端的地址 | ":0" |
| ServerAuthPassword | 用于验证服务端合法性的秘钥 | 空 |
| ServerAuthTimeout | 验证服务端连接时的最大IO等待时间 | 3秒 |
| ServerBufferSize | 每个服务端连接使用的 bufio.Reader 缓冲区大小 | 64K |
| ServerPingInterval | 网关在多少秒没收到客户端消息后发送PING指令给客户端 | 30秒 |
| ServerSendChanSize | 每个服务端连接异步发送消息用的chan缓冲区大小 | 10万 |

调用示例
=======

本项目提供了配套的调用库，调用库实现了网关配套的通讯协议。

网关通讯协议只规定消息包格式，应用层的消息内容格式没有特殊要求，消息内容可以使用任何形式的序列化格式，比如`Protobuf`、`JSON`等，只要能把消息序列化成`[]byte`并反序列化回来就即可。

应用层的消息类型识别和派发也不在网关的职责范围内，消息类型识别可以参考[`link`](https://github.com/funny/link)项目在[`codec`](https://github.com/funny/link/tree/master/codec)目录下的示例代码。

调用库提供出来的通讯接口是基于`link`封装的，了解`link`的代码有助于理解网关调用库的使用。

**注意：虚拟连接所用的消息类型是`*[]byte`**

调用示例1 - 以客户端身份连接到网关：

```go
client, err := proto.DialClient(
	GatewayAddr,   // 网关地址
	MyMsgFormat,   // 消息格式
	MyMemPool,     // 内存池
	MaxPacketSize, // 包体积限制
	BufferSize,    // 预读所用的缓冲区大小
	SendChanSize,  // 异步发送用的chan缓冲区大小
)
```

调用示例2 - 以服务端身份连接到网关：

```go
server, err := proto.DialServer(
	GatewayAddr,   // 网关地址
	MyMsgFormat,   // 消息格式
	MyMemPool,     // 内存池
	ServerID,      // 服务端ID
	AuthKey,       // 身份验证用的Key
	AuthTimeout,   // 身份验证IO等待超时时间
	MaxPacketSize, // 包体积限制
	BufferSize,    // 预读所用的缓冲区大小
	SendChanSize,  // 异步发送用的chan缓冲区大小
)
```

调用示例3 - 创建一个虚拟连接：

```go
conn, connID, err := client.Dial(ServerID)
```

调用示例4 - 接收一个虚拟连接：

```go
conn, connID, clientID, err := server.Accept()
```

调用示例5 - 以JSON格式发送一个消息：

```go
var msg MyMessage
buf, err := json.Marshal(&msg)
conn.Send(&buf)
```

调用示例6 - 以JSON格式接收一个消息：

```go
var msg MyMessage
buf, err := conn.Receive()
json.Unmarshal(*(buf.(*[]byte)), &msg)
```

通讯协议
=======

客户端和服务端均采用相同的协议格式与网关进行通讯，消息由4个字节包长度信息和变长的包体组成：

```
+--------+--------+
| Length | Packet |
+--------+--------+
  4 byte   Length
```

每个消息的包体可以再分解为4个字节的虚拟连接ID和变长的消息内容两部分：

```
+---------+-------------+
| Conn ID |   Message   |
+---------+-------------+
   4 byte    Length - 4
```

`Conn ID`是虚拟连接的唯一标识，网关通过识别`Conn ID`来转发消息。

当`Conn ID`为0时，`Message`的内容被网关进一步解析并作为指令进行处理。

网关指令固定以一个字节的指令类型字段开头，指令参数根据指令类型而定：

```
+--------+------------------+
|  Type  |       Args       |
+--------+------------------+
  1 byte    Length - 4 - 1
```

目前支持的指令如下：

| **Type** | **用途** | **Args** | **说明** |
| ---- | ---- | ---- | ---- |
| 0 | Dial | Remote ID | 创建虚拟连接 |
| 1 | Accept | Conn ID + Remote ID | 由网关发送给虚拟连接创建者，告知虚拟连接创建成功 |
| 2 | Connect | Conn ID + Remote ID | 由网关发送给虚拟连接的被连接方，告知有新的虚拟连接产生 |
| 3 | Refuse | 无 | 由网关发送给虚拟连接创建者，告知无法连接到远端 |
| 4 | Close | Conn ID | 客户端、服务端、网关都有可能发送次消息 |
| 5 | Ping | 无 | 网关下发给客户端和服务端，收到后应立即响应 |

特殊说明：

+ 协议允许客户端主动连接服务端，也允许服务端主动连接客户端。
+ 新建虚拟连接的时候，网关会把虚拟连接信息发送给两端。
+ 客户端连接服务端时，网关回发的`Accept`指令中`Remote ID`为服务端ID，发送给服务端的`Connect`指令中`Remote ID`为客户端ID。
+ 服务端连接客户端时，网关回发的`Accept`指令中`Remote ID`为客户端ID，发送给客户端的`Connect`指令中`Remote ID`为服务端ID。

握手协议
=======

服务端在连接网关时需要先进行握手来验证服务端的合法性。

握手过程如下：

0. 合法的服务端应持有正确的网关秘钥。
1. 网关在接受到新的服务端连接之后，向新连接发送一个`uint64`范围内的随机数作为挑战码。
2. 服务端收到挑战码后，拿出秘钥，计算 `MD5(挑战码 + 秘钥)`，得到验证码。
3. 服务端将验证码和自身节点ID一起发送给网关。
4. 网关收到消息后同样计算 `MD5(挑战码 + 秘钥)`，跟收到的验证码比对是否一致。
5. 验证码比对一致，网关将新连接登记为对应节点ID的连接。

握手下行数据格式：

```
+----------------+
| Challenge Code |
+----------------+
      8 byte
```

握手上行数据格式：

```
+-----------+-----------+
|    MD5    | Server ID |
+-----------+-----------+
   16 byte      4 byte
```

协议示例
=======

客户端请求网关创建虚拟连接到服务端：

```
+------------+-------------+---------+---------------+
| Length = 9 | Conn ID = 0 | CMD = 0 | Server ID = 1 |
+------------+-------------+---------+---------------+
	4 byte        4 byte      1 byte        4 byte
```

网关响应虚拟连接创建请求：

```
+-------------+-------------+---------+----------------+---------------+
| Length = 13 | Conn ID = 0 | CMD = 1 | Conn ID = 9527 | Server ID = 1 |
+-------------+-------------+---------+----------------+---------------+
	4 byte        4 byte       1 byte       4 byte           4 byte
```

网关告知服务端有新的虚拟连接：

```
+-------------+-------------+---------+----------------+---------------+
| Length = 13 | Conn ID = 0 | CMD = 3 | Conn ID = 9527 | Server ID = 1 |
+-------------+-------------+---------+----------------+---------------+
	4 byte        4 byte       1 byte       4 byte           4 byte
```

客户端通过虚拟连接发送"Hello"到服务端：

```
+------------+----------------+-------------------+
| Length = 9 | Conn ID = 9527 | Message = "Hello" |
+------------+----------------+-------------------+
	4 byte         4 byte            5 byte
```

客户端关闭虚拟连接：

```
+------------+-------------+---------+----------------+
| Length = 9 | Conn ID = 0 | CMD = 5 | Conn ID = 9527 |
+------------+-------------+---------+----------------+
	4 byte        4 byte      1 byte       4 byte
```

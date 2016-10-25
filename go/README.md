简介
====

此项目是和`fastway`配套的Go语言API，实现了`fastway`的通讯协议。

`fastway`通讯协议只规定消息包格式，应用层的消息内容格式没有特殊要求，消息内容可以使用任何形式的序列化格式，比如`Protobuf`、`JSON`等，只要能把消息序列化成`[]byte`并反序列化回来就即可。

应用层的消息类型识别和派发也不在网关的职责范围内，消息类型识别可以参考[`link`](https://github.com/funny/link)项目在[`codec`](https://github.com/funny/link/tree/master/codec)目录下的示例代码。

调用库提供出来的通讯接口是基于`link`封装的，了解`link`的代码有助于理解网关调用库的使用。

调用示例
=======

调用示例1 - 以客户端身份连接到网关：

```go
client := proto.NewClient(
	Conn, // 物理连接
	fastway.EndPointCfg {
		MemPool,         // 内存池
		MaxPacket,       // 包体积限制
		BufferSize,      // 预读所用的缓冲区大小
		SendChanSize,    // 物理连接异步发送用的chan缓冲区大小
		RecvChanSize,    // 虚拟连接异步接收用的chan缓冲区大小
		PingInterval,    // 发送Ping的时间间隔，必须小于网关的ClientIdleTimeout
		PingTimeout,     // 可选参数，当网关响应Ping超过此项设置，将调用TimeoutCallback
		TimeoutCallback, // 此项设置需要跟PingTimeout配套
	},
)
```

调用示例2 - 以服务端身份连接到网关：

```go
server, err := proto.DialServer(
	Conn, // 物理连接
	fastway.EndPointCfg {
		ServerID,        // 服务端ID
		AuthKey,         // 身份验证用的Key
		MemPool,         // 内存池
		MaxPacket,       // 包体积限制
		BufferSize,      // 预读所用的缓冲区大小
		SendChanSize,    // 物理连接异步发送用的chan缓冲区大小
		RecvChanSize,    // 虚拟连接异步接收用的chan缓冲区大小
		PingInterval,    // 发送Ping的时间间隔，必须小于网关的ClientIdleTimeout
		PingTimeout,     // 可选参数，当网关响应Ping超过此项设置，将调用TimeoutCallback
		TimeoutCallback, // 此项设置需要跟PingTimeout配套
	},
)
```

调用示例3 - 创建一个虚拟连接：

```go
conn, err := client.Dial(ServerID)
```

调用示例4 - 接收一个虚拟连接：

```go
conn, err := server.Accept()
```

调用示例5 - 以JSON格式发送一个消息：

```go
var msg MyMessage
buf, err := json.Marshal(&msg)
conn.Send(buf)
```

调用示例6 - 以JSON格式接收一个消息：

```go
var msg MyMessage
buf, err := conn.Receive()
json.Unmarshal(buf, &msg)
```

注意事项：

+ 网关开启[snet协议](https://github.com/funny/snet)时，需要用[snet协议的连接](https://github.com/funny/snet/golang)来创建Endpoint
+ 使用snet协议时，可以利用ping超时机制来做到尽早的尝试重连，依赖于TCP重传失败或者IO超时，都比较难以控制超时时间。

简介
====

此项目是和`fastway`配套的C# API，实现了`fastway`的通讯协议，可以供Unity3D项目使用（建议不要直接用在C#服务端）。

相关的调用API均设计成非阻塞调用，可以直接在Unity3D的Update循环里调用。

调用示例
=======

调用示例1 - 连接到网关：

```csharp
var endPoint = new Fastway.EndPoint (
	Stream,          // 基础的网络流，可以是NetStream或者Snet.SnetStream
	PingInterval,    // Ping网关的时间间隔，必须小于网关的IdleTimeout设置
	PingTimeout,     // 网关回应Ping的超时时间，如果回应超时，将调用TimeoutCallback
	TimeoutCallback, // 
);
```

调用示例2 - 连接到服务端：

```csharp
var conn = endPoint.Dial (ServerID);
```

调用示例3 - 发送一个无意义的随机消息包：

```csharp
var n = random.Next (1000, 2000);
var msg1 = new byte[n];
random.NextBytes(msg1);

if (!conn.Send (msg1)) {
	Console.WriteLine ("connection closed");
}
```

调用示例4 - 接收消息包：

```csharp
var msg2 = conn.Receive ();

if (msg2 == null) {
	Console.WriteLine ("connection closed");
}

if (msg2 == Conn.NoMsg) {
	Console.WriteLine ("no message");
}
```

注意事项：

+ 当虚拟连接或底层物理连接已关闭，`Conn.Send()`将返回`false`。
+ 网关开启[snet协议](https://github.com/funny/snet)时，客户端的网络流需要是[snet协议的流](https://github.com/funny/snet/csharp)
+ 使用snet协议时，可以利用ping超时机制来做到尽早的尝试重连，依赖于TCP重传失败或者IO超时，都比较难以控制超时时间。
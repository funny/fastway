简介
====

此项目是和`fastway`配套的C# API，实现了`fastway`的通讯协议，可以供Unity3D项目使用（建议不要直接用在C#服务端）。

相关的调用API均设计成非阻塞调用，可以直接在Unity3D的Update循环里调用。

调用示例
=======

调用示例1 - 连接到网关：

```csharp
var endPoint = new Fastway.EndPoint (
	new System.Net.Sockets.TcpClient (
		GatewayIP, 
		GatewayPort
	).GetStream ()
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
+ 当虚拟连接或底层物理连接已关闭，`Conn.Receive()`将返回`null`。
+ 当消息队列中没有可用消息，`Conn.Receive()`将返回长`Conn.NoMsg`。
+ 网关开启[snet协议](https://github.com/funny/snet)时，客户端的网络流需要是[snet协议的流](https://github.com/funny/snet/csharp)
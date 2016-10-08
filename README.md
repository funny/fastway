[![Go Foundation](https://img.shields.io/badge/go-foundation-green.svg)](http://golangfoundation.org)
[![Go Report Card](https://goreportcard.com/badge/github.com/fast/fastway)](https://goreportcard.com/report/github.com/fast/fastway)
[![Build Status](https://travis-ci.org/fast/fastway.svg?branch=master)](https://travis-ci.org/fast/fastway)
[![codecov](https://codecov.io/gh/fast/fastway/branch/master/graph/badge.svg)](https://codecov.io/gh/fast/fastway)
[![GoDoc](https://img.shields.io/badge/api-reference-blue.svg)](https://godoc.org/github.com/fast/fastway/proto)

简介
====

本网关是一个游戏用的通讯网关，它负责客户端和服务端之间的消息转发。

通过网关，我们预期达到以下目的：

1. 减少暴露在公网的服务器
2. 提升网络故障转移的效率
3. 复用客户端网络连接
4. 使服务端主动连接客户端成为可能

网关、客户端以及服务端之间的拓扑结构如下：

![Gateway](https://rawgit.com/fast/fastway/master/README.svg)

基本逻辑：

+ 每个系统实际运行时允许部署任意个网关
+ 每个网关启动时守护对外和对内两个网络端口
+ 游戏服务端启动时主动连接到所有网关的对内端口，并告知网关自己的ID
+ 新的客户端连接产生时，任意连接到一个网关的对外端口
+ 客户端可以通过网关协议跟多个服务端建立虚拟连接
+ 服务端也可以通过网关协议主动跟多个客户端建立虚拟连接

命令行
=====

本网关提供了一个命令行程序用来对外提供服务。

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
| ClientMaxConn | 每个客户端可以创建的最大虚拟连接数 | 16 |
| ClientBufferSize | 每个客户端连接使用的 bufio.Reader 缓冲区大小 | 2K |
| ClientPingInterval | 网关在多少秒没收到客户端消息后发送PING指令给客户端 | 30秒 |
| ClientSendChanSize | 每个客户端连接异步发送消息用的chan缓冲区大小 | 1000 |
| ServerAddr | 网关暴露给服务端的地址 | ":0" |
| ServerAuthPassword | 用于验证服务端合法性的秘钥 | 空 |
| ServerAuthTimeout | 验证服务端连接时的最大IO等待时间 | 3秒 |
| ServerBufferSize | 每个服务端连接使用的 bufio.Reader 缓冲区大小 | 64K |
| ServerPingInterval | 网关在多少秒没收到客户端消息后发送PING指令给客户端 | 30秒 |
| ServerSendChanSize | 每个服务端连接异步发送消息用的chan缓冲区大小 | 10万 |

调用库
=======

本项目提供了配套的调用库：

+ [Go版](https://github.com/fast/fastway/tree/master/proto)
+ [C#版](https://github.com/fast/fastway/tree/master/dotnet)

通讯协议细节请阅读Go调用库的文档。

网关通讯协议只规定消息包格式，应用层的消息内容格式没有特殊要求，消息内容可以使用任何形式的序列化格式，比如`Protobuf`、`JSON`等，只要能把消息序列化成`[]byte`并反序列化回来就即可。

应用层的消息类型识别和派发也不在网关的职责范围内，消息类型识别可以参考[`link`](https://github.com/funny/link)项目在[`codec`](https://github.com/funny/link/tree/master/codec)目录下的示例代码。

调用库提供出来的通讯接口是基于`link`封装的，了解`link`的代码有助于理解网关调用库的使用。

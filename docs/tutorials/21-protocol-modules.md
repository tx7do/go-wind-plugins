# Go-Wind 插件教程 · 第 21 章：协议模块文档地图

> 本篇是 [Go-Wind 插件系列教程](./README.md)的组成部分。[第 8 章](./08-transport-middleware.md)给出了协议矩阵总览；本章按功能分组逐个导航——有模块 README 的给文档地图，没有的给源码指针，并在最后把「消息代理承载型」传输模块归位到[第 7 章](./07-broker.md)。

## 1. 分组总览

| 组 | 模块 |
|----|------|
| 定时与任务 | `cron` · `hptimer` · `asynq` · `machinery` |
| 实时与推送 | `websocket` · `socketio` · `signalr` · `sse` · `webrtc` · `webtransport` |
| 网络与 RPC | `tcp` · `kcp` · `http3` · `thrift` · `trpc` · `mcp` · `grpc/client` |
| 消息代理承载 | `activemq` · `azuresb` · `gcpubsub` · `kafka` · `mqtt` · `nats` · `nsq` · `pulsar` · `rabbitmq` · `redis` · `rocketmq` · `sqs`（→ [第 7 章](./07-broker.md)） |
| 已有专属教程 | HTTP 家族（[第 8 章](./08-transport-middleware.md) + [HTTP 实战](../http-server-tutorial.md) + [第 20 章](./20-http-utilities.md)工具族） · gRPC（[实战](../grpc-server-tutorial.md)） · GraphQL（[实战](../graphql-server-tutorial.md)） |

## 2. 定时与任务

### 2.1 Cron —— 周期调度

模块路径 `github.com/tx7do/go-wind-plugins/transport/cron`，模块文档 [`README.md`](../../transport/cron/README.md) 覆盖：配置选项、任务管理、**秒级 6 位 Cron 表达式与描述符语法**（`@every` 等robfig/cron 风格）、生命周期说明、适用场景。另有 [`example_test.go`](../../transport/cron/example_test.go) 可运行示例。

### 2.2 HPTimer —— 高精度定时

模块路径 `github.com/tx7do/go-wind-plugins/transport/hptimer`，模块文档 [`README.md`](../../transport/hptimer/README.md) 覆盖：任务创建与任务选项、**三种触发模式**、**与 Cron 的对比**（精度/开销/语义差异）、基准测试数据。

### 2.3 Asynq —— Redis 任务队列

模块路径 `github.com/tx7do/go-wind-plugins/transport/asynq`，模块文档 [`README.md`](../../transport/asynq/README.md) 覆盖：Redis 连接配置的**四种形态**（单节点 / URI / 集群 / 哨兵）、Asynqmon 管理后台部署、任务投递与消费。另有 [`example_test.go`](../../transport/asynq/example_test.go)。任务语义（重试、优先级队列、死信）由 asynq 引擎定义，文档为准。

### 2.4 Machinery —— 类 Celery 任务队列

模块路径 `github.com/tx7do/go-wind-plugins/transport/machinery`。当前无模块 README，接入面见[模块源码](../../transport/machinery/)。

## 3. 实时与推送

### 3.1 SSE —— 服务端推送

模块路径 `github.com/tx7do/go-wind-plugins/transport/sse`，模块文档 [`README.md`](../../transport/sse/README.md) 覆盖：**SSE 与 WebSocket 的选型对比**、流管理、事件发布、**鉴权与 Token**（默认 Token 提取规则、自定义提取函数）。SSE 是单向推送场景（进度条、通知流）的首选——比 WebSocket 少一半的状态复杂度。

### 3.2 SignalR —— 双向实时 RPC

模块路径 `github.com/tx7do/go-wind-plugins/transport/signalr`，模块文档 [`README.md`](../../transport/signalr/README.md) 覆盖：**Hub 开发模型**（方法注册、分组、连接生命周期）。

### 3.3 WebRTC —— 点对点媒体与数据

模块路径 `github.com/tx7do/go-wind-plugins/transport/webrtc`，模块文档 [`README.md`](../../transport/webrtc/README.md) 覆盖：服务端与客户端两侧的接入、服务端/客户端选项、**媒体流支持**（启用媒体、订阅媒体流）。信令交换经你自选的通道（常配本章 3.1 节的 SSE 或 WebSocket）。

### 3.4 WebSocket / Socket.IO / WebTransport

模块路径 `github.com/tx7do/go-wind-plugins/transport/websocket`、`.../transport/socketio`、`.../transport/webtransport`。三者当前无模块 README，接入面见各[模块源码](../../transport/)。Socket.IO 的会话语义（自动重连、房间、降级轮询）以引擎 SDK 为准。

## 4. 网络与 RPC

### 4.1 Thrift —— Apache Thrift RPC

模块路径 `github.com/tx7do/go-wind-plugins/transport/thrift`，模块文档 [`README.md`](../../transport/thrift/README.md) 覆盖：**Thrift 编译器安装**（Linux/macOS/Windows）、**IDL 定义与代码生成**、服务端接入。Thrift 载荷的编解码走 [第 5 章](./05-encoding.md)的 `thrift` 编解码器（要求 `TStruct` 生成类型）。

### 4.2 gRPC 客户端库

模块路径 `github.com/tx7do/go-wind-plugins/transport/grpc/client`——gRPC 家族的服务器与中间件在[实战教程](../grpc-server-tutorial.md)，本模块是**客户端侧**的调用封装，接入面见[模块源码](../../transport/grpc/client/)。服务端模块为 `transport/grpc/server`（[example_test.go](../../transport/grpc/server/example_test.go)）。

### 4.3 TCP / KCP / HTTP/3 / tRPC / MCP

模块路径 `github.com/tx7do/go-wind-plugins/transport/tcp`、`.../transport/kcp`、`.../transport/http3`、`.../transport/trpc`、`.../transport/mcp`，均无模块 README，接入面见各[模块源码](../../transport/)。其中 MCP（Model Context Protocol）面向把自有能力以标准工具协议暴露给模型端——背景见[第 18 章](./18-ai.md)。`tcp` 服务器的编解码配置示例见 [第 5 章](./05-encoding.md)第 3 节。

## 5. 消息代理承载型模块

以下十二个传输模块把[第 7 章](./07-broker.md)的消息代理消费面建模为服务器——订阅主题即注册 handler，生命周期、指标埋点（[第 4 章](./04-metrics.md)）与 HTTP 服务器完全一致：

```
github.com/tx7do/go-wind-plugins/transport/activemq
github.com/tx7do/go-wind-plugins/transport/azuresb
github.com/tx7do/go-wind-plugins/transport/gcpubsub
github.com/tx7do/go-wind-plugins/transport/kafka
github.com/tx7do/go-wind-plugins/transport/mqtt
github.com/tx7do/go-wind-plugins/transport/nats
github.com/tx7do/go-wind-plugins/transport/nsq
github.com/tx7do/go-wind-plugins/transport/pulsar
github.com/tx7do/go-wind-plugins/transport/rabbitmq
github.com/tx7do/go-wind-plugins/transport/redis
github.com/tx7do/go-wind-plugins/transport/rocketmq
github.com/tx7do/go-wind-plugins/transport/sqs
```

连接参数、QoS、ack 语义等全部继承对应 broker 引擎（配置细节见第 7 章各引擎模块 README）；这些传输模块自身只做「订阅→handler」的服务器化包装。选用判据与第 7 章第 4 节相同：常驻服务用传输层形态，独立工具用库形态。

## 6. 深入阅读

带 README 的七个协议模块（本章第 2、3、4 节的链接）；三篇实战教程与 [第 8 章](./08-transport-middleware.md)矩阵；其余模块源码目录 [`transport/`](../../transport/)。

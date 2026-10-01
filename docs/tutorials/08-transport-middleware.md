# Go-Wind 插件教程 · 第 8 章：传输层与中间件（Transport & Middleware）

> 本篇是 [Go-Wind 插件系列教程](./README.md)的组成部分。传输层是本仓覆盖最广的家族，本章做总览与导航；HTTP 侧的深度实战见本章第 4 节的配套教程。

## 1. 这一层解决什么问题

传输层把「监听端口、解析协议、把字节流变成 handler 调用」这件事标准化。go-wind 框架的应用选项 `WithServer(srv ...transport.Server)` 接收任意数量的服务器实例——一个进程同时挂 HTTP API、gRPC 内部通道、WebSocket 推送，共享同一套生命周期（启动、优雅停机、[第 2 章](./02-registry.md)所述的注册注销时序）与同一套中间件、日志、指标、追踪接线。

| 接口 | 方法 | 说明 |
|------|------|------|
| `Server` | `Start(ctx)` / `Stop(ctx)` / `Endpoint()` | 生命周期管理与端点发现 |
| `Driver`（HTTP） | `Handle / Start / Stop` | 底层框架适配驱动 |

## 2. 协议矩阵

| 插件 | 模块路径 | 引擎 |
|------|---------|------|
| HTTP（标准库） | `github.com/tx7do/go-wind-plugins/transport/http` | net/http |
| HTTP（Chi） | `github.com/tx7do/go-wind-plugins/transport/http/driver/chi` | go-chi/chi |
| HTTP（Gin） | `github.com/tx7do/go-wind-plugins/transport/http/driver/gin` | gin-gonic/gin |
| HTTP（Fiber） | `github.com/tx7do/go-wind-plugins/transport/http/driver/fiber` | gofiber/fiber |
| HTTP/3 | `github.com/tx7do/go-wind-plugins/transport/http3` | quic-go/http3 |
| gRPC | `github.com/tx7do/go-wind-plugins/transport/grpc/server` | google.golang.org/grpc |
| WebSocket | `github.com/tx7do/go-wind-plugins/transport/websocket` | gorilla/websocket |
| Socket.IO | `github.com/tx7do/go-wind-plugins/transport/socketio` | googollee/go-socket.io |
| SignalR | `github.com/tx7do/go-wind-plugins/transport/signalr` | SignalR 协议 |
| SSE | `github.com/tx7do/go-wind-plugins/transport/sse` | Server-Sent Events |
| TCP | `github.com/tx7do/go-wind-plugins/transport/tcp` | net.Listener |
| KCP | `github.com/tx7do/go-wind-plugins/transport/kcp` | xtaci/kcp-go |
| WebRTC | `github.com/tx7do/go-wind-plugins/transport/webrtc` | pion/webrtc v4 |
| WebTransport | `github.com/tx7do/go-wind-plugins/transport/webtransport` | webtransport-go |
| GraphQL | `github.com/tx7do/go-wind-plugins/transport/graphql` | graphql-go |
| Thrift | `github.com/tx7do/go-wind-plugins/transport/thrift` | Apache Thrift |
| tRPC | `github.com/tx7do/go-wind-plugins/transport/trpc` | tRPC 协议 |
| Cron | `github.com/tx7do/go-wind-plugins/transport/cron` | robfig/cron |
| HPTimer | `github.com/tx7do/go-wind-plugins/transport/hptimer` | 高精度定时器 |
| Asynq | `github.com/tx7do/go-wind-plugins/transport/asynq` | hibiken/asynq |
| Machinery | `github.com/tx7do/go-wind-plugins/transport/machinery` | machinery（类 Celery） |
| MCP | `github.com/tx7do/go-wind-plugins/transport/mcp` | Model Context Protocol |

「定时器 / 任务队列也是服务器」是本层的一个设计立场：cron、hptimer、asynq、machinery 把周期触发与后台任务消费建模为 Server，handler 注册与生命周期同 HTTP 完全一致。另外，消息代理的消费面也有对应服务器（`transport/kafka` 等，见[第 7 章](./07-broker.md)）。

**已知问题**：`transport/http/driver/chi` 当前存在驱动接口不一致的构建问题（2026-09-30 基线确认），选用前先验证构建。

## 3. HTTP 驱动系统

HTTP 家族内部再分一层驱动：同一套路由注册与中间件 API，底层可在标准库、gin、chi、fiber 间切换（`transport/http/driver/std` 等）。切换只需改一个构造参数，路由与 handler 原样保留。驱动能力的边界（路径参数、通配符等）以各驱动实现为准——HTTP 实战教程第 4、5 章有完整对照。

驱动选型的官方依据是模块内的 [`transport/http/STATUS.md`](../../transport/http/STATUS.md)：三档接入方式（标准接口 / options 注入框架原生路由 / 裸用底层引擎）、四个驱动的能力与内存实测对比、以及「两套中间件体系并行且互不互通」的边界说明都在其中。延迟与内存的原始数据见 [`transport/http/benchmark/README.md`](../../transport/http/benchmark/README.md)，[第 20 章](./20-http-utilities.md)有报告摘要。

## 4. 配套深度实战教程

| 教程 | 覆盖 |
|------|------|
| [Go-Wind HTTP 服务器从入门到精通](../http-server-tutorial.md) | 服务器与驱动系统、路由注册、中间件机制与工作原理、自定义中间件、Chain 组合、TLS、生产级完整示例 |
| [Go-Wind gRPC 服务器从入门到精通](../grpc-server-tutorial.md) | gRPC 服务器与 gRPC 侧中间件套件 |
| [Go-Wind GraphQL 服务器从入门到精通](../graphql-server-tutorial.md) | GraphQL 服务器搭建 |

## 5. 中间件套件

HTTP 与 gRPC 两侧各有一组**同名成对**的中间件模块（HTTP 侧 17 个、gRPC 侧 16 个——`cors` 仅 HTTP 侧），逐名列举如下：

| 关注点 | 中间件 | HTTP 模块路径 | gRPC 模块路径 |
|------|------|------|------|
| 可观测 | `logging` | `github.com/tx7do/go-wind-plugins/transport/http/middleware/logging` | `github.com/tx7do/go-wind-plugins/transport/grpc/middleware/logging` |
| 可观测 | `metrics` | `github.com/tx7do/go-wind-plugins/transport/http/middleware/metrics` | `github.com/tx7do/go-wind-plugins/transport/grpc/middleware/metrics` |
| 可观测 | `tracing` | `github.com/tx7do/go-wind-plugins/transport/http/middleware/tracing` | `github.com/tx7do/go-wind-plugins/transport/grpc/middleware/tracing` |
| 可观测 | `requestid` | `github.com/tx7do/go-wind-plugins/transport/http/middleware/requestid` | `github.com/tx7do/go-wind-plugins/transport/grpc/middleware/requestid` |
| 流量治理 | `ratelimit` | `github.com/tx7do/go-wind-plugins/transport/http/middleware/ratelimit` | `github.com/tx7do/go-wind-plugins/transport/grpc/middleware/ratelimit` |
| 流量治理 | `circuitbreaker` | `github.com/tx7do/go-wind-plugins/transport/http/middleware/circuitbreaker` | `github.com/tx7do/go-wind-plugins/transport/grpc/middleware/circuitbreaker` |
| 流量治理 | `retry` | `github.com/tx7do/go-wind-plugins/transport/http/middleware/retry` | `github.com/tx7do/go-wind-plugins/transport/grpc/middleware/retry` |
| 流量治理 | `timeout` | `github.com/tx7do/go-wind-plugins/transport/http/middleware/timeout` | `github.com/tx7do/go-wind-plugins/transport/grpc/middleware/timeout` |
| 安全 | `authn` | `github.com/tx7do/go-wind-plugins/transport/http/middleware/authn` | `github.com/tx7do/go-wind-plugins/transport/grpc/middleware/authn` |
| 安全 | `authz` | `github.com/tx7do/go-wind-plugins/transport/http/middleware/authz` | `github.com/tx7do/go-wind-plugins/transport/grpc/middleware/authz` |
| 安全 | `crypto` | `github.com/tx7do/go-wind-plugins/transport/http/middleware/crypto` | `github.com/tx7do/go-wind-plugins/transport/grpc/middleware/crypto` |
| 协议处理 | `codec` | `github.com/tx7do/go-wind-plugins/transport/http/middleware/codec` | `github.com/tx7do/go-wind-plugins/transport/grpc/middleware/codec` |
| 协议处理 | `cors` | `github.com/tx7do/go-wind-plugins/transport/http/middleware/cors` | ——（仅 HTTP） |
| 协议处理 | `metadata` | `github.com/tx7do/go-wind-plugins/transport/http/middleware/metadata` | `github.com/tx7do/go-wind-plugins/transport/grpc/middleware/metadata` |
| 协议处理 | `errors` | `github.com/tx7do/go-wind-plugins/transport/http/middleware/errors` | `github.com/tx7do/go-wind-plugins/transport/grpc/middleware/errors` |
| 协议处理 | `recovery` | `github.com/tx7do/go-wind-plugins/transport/http/middleware/recovery` | `github.com/tx7do/go-wind-plugins/transport/grpc/middleware/recovery` |
| 协议处理 | `validate` | `github.com/tx7do/go-wind-plugins/transport/http/middleware/validate` | `github.com/tx7do/go-wind-plugins/transport/grpc/middleware/validate` |

其中 `authn` / `authz` 的引擎选型见[第 10 章](./10-security.md)，`ratelimit` / `circuitbreaker` / `retry` 的接口与引擎见[第 9 章](./09-resilience.md)，`codec` 的内容协商语义见[第 5 章](./05-encoding.md)。

标准中间件只作用于第 1 档标准接口路由；框架原生路由（`WithRoute` / `Engine()`）走框架各自的中间件，两套体系互不互通、也无法安全地自动适配——这条边界与「为什么不做 adapter」的论证见 [`transport/http/STATUS.md`](../../transport/http/STATUS.md)。中间件的注册方式、执行顺序语义、自定义编写与 Chain 组合，在 HTTP 实战教程第 6–9 章有逐节讲解，此处不重复。

HTTP 家族中不属于协议服务器与中间件的工具模块（文档 UI 挂载、请求绑定库、驱动基准测试）见[第 20 章](./20-http-utilities.md)；各协议模块（含无 README 模块的源码指针与消息代理承载型模块的归位）的文档导航见[第 21 章](./21-protocol-modules.md)。

## 6. 深入阅读

- 三篇实战教程（上表）
- [根 README · 传输层矩阵](../../README.md#传输层transport)
- 各协议模块源码目录：[`transport/`](../../transport/)

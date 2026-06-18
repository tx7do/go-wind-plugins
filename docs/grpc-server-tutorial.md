# Go-Wind gRPC 服务器从入门到精通

> 本教程面向初学者，基于 `transport/grpc/server/server.go`，循序渐进地讲解如何使用 Go-Wind 插件库搭建 gRPC 服务器——从最简单的服务定义开始，逐步深入到拦截器（Interceptor）机制，最终构建一个生产级的 gRPC 微服务。

---

## 目录

- [1. 简介](#1-简介)
- [2. 前置知识：gRPC 基础](#2-前置知识grpc-基础)
- [3. 核心概念](#3-核心概念)
- [4. 快速开始：第一个 gRPC 服务](#4-快速开始第一个-grpc-服务)
- [5. 服务注册](#5-服务注册)
- [6. 拦截器（Interceptor）入门](#6-拦截器interceptor入门)
- [7. 内置拦截器详解](#7-内置拦截器详解)
  - [7.1 recovery —— 异常恢复](#71-recovery--异常恢复)
  - [7.2 logging —— 访问日志](#72-logging--访问日志)
  - [7.3 requestid —— 请求追踪](#73-requestid--请求追踪)
  - [7.4 timeout —— 超时控制](#74-timeout--超时控制)
  - [7.5 ratelimit —— 限流](#75-ratelimit--限流)
  - [7.6 authn —— 身份认证](#76-authn--身份认证)
- [8. 编写自定义拦截器](#8-编写自定义拦截器)
- [9. 拦截器组合：Chain](#9-拦截器组合chain)
- [10. 流式 RPC 与 Stream 拦截器](#10-流式-rpc-与-stream-拦截器)
- [11. gRPC 客户端](#11-grpc-客户端)
- [12. 生产级完整示例](#12-生产级完整示例)
- [13. 常见问题](#13-常见问题)

---

## 1. 简介

`transport/grpc/server` 是 Go-Wind 插件库提供的 **gRPC 服务器封装层**。它在原生 `google.golang.org/grpc` 的基础上，提供了：

| 特性 | 说明 |
|------|------|
| **统一抽象** | 实现 `transport.Server` 接口，与 HTTP 服务器使用方式一致 |
| **拦截器链** | 内置 `Use()` / `UseStream()` 方法，优雅地管理一元和流拦截器 |
| **优雅关闭** | 内置基于 `context.Context` 的 `GracefulStop` 支持 |
| **丰富中间件** | 提供 16 个开箱即用的拦截器（recovery、logging、authn 等） |

如果你已经熟悉 gRPC 的基本用法，那么上手会非常快——因为底层的 `*grpc.Server`、`ServiceDesc`、`UnaryServerInterceptor` 等类型全部是原生 gRPC 类型。

---

## 2. 前置知识：gRPC 基础

在开始之前，你需要了解 gRPC 的几个核心概念：

### 2.1 什么是 gRPC？

gRPC 是 Google 开源的高性能 RPC 框架，使用 **Protocol Buffers**（protobuf）作为接口定义语言和序列化格式。

```
客户端 (gRPC Client)  ──── HTTP/2 + Protobuf ────►  服务端 (gRPC Server)
```

### 2.2 一元 RPC vs 流式 RPC

gRPC 支持四种调用模式：

| 模式 | 说明 | 类比 |
|------|------|------|
| **一元 RPC** (Unary) | 一次请求，一次响应 | 类似 HTTP 请求 |
| **服务端流** | 一次请求，多次响应 | 类似 SSE |
| **客户端流** | 多次请求，一次响应 | 类似文件上传 |
| **双向流** | 双方都可多次收发 | 类似 WebSocket |

> 本教程主要聚焦于 **一元 RPC**，最后会讲解流式 RPC 的拦截器用法。

### 2.3 你需要准备的工具

```bash
# 安装 protoc 编译器
# 参考: https://grpc.io/docs/protoc-installation/

# 安装 Go 插件
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

---

## 3. 核心概念

### 3.1 Server（服务器）

`Server` 是整个封装的入口。它内部持有：
- `addr`：监听地址（如 `:9000`）
- `server`：底层的 `*grpc.Server`（延迟初始化）
- `middlewares`：一元拦截器链
- `streamMiddleware`：流拦截器链

它实现了 `transport.Server` 接口，提供 `Start / Stop / Endpoint` 等方法。

### 3.2 Interceptor（拦截器）

gRPC 的拦截器相当于 HTTP 的中间件——在 RPC 方法调用前后插入自定义逻辑。Go-Wind 将它们定义为类型别名：

```go
// 一元拦截器（处理普通 RPC）
type Middleware = grpc.UnaryServerInterceptor

// 流拦截器（处理流式 RPC）
type StreamMiddleware = grpc.StreamServerInterceptor
```

一元拦截器的函数签名如下：

```go
func(
    ctx context.Context,       // RPC 上下文
    req any,                   // 请求消息
    info *grpc.UnaryServerInfo, // RPC 元信息（方法名等）
    handler grpc.UnaryHandler, // 下一个拦截器或最终 handler
) (resp any, err error)        // 返回响应和错误
```

### 3.3 grpc.Server

Go-Wind 的 Server 封装了原生的 `*grpc.Server`，你可以通过 `srv.Server()` 获取它，然后用原生 gRPC 的方式注册服务。

---

## 4. 快速开始：第一个 gRPC 服务

### 4.1 定义 proto 文件

创建 `proto/hello.proto`：

```protobuf
syntax = "proto3";

package hello;
option go_package = "./proto/hello";

// 请求消息
message HelloRequest {
  string name = 1;
}

// 响应消息
message HelloResponse {
  string message = 1;
}

// 服务定义
service HelloService {
  rpc SayHello(HelloRequest) returns (HelloResponse);
}
```

### 4.2 生成 Go 代码

```bash
protoc --go_out=. --go-grpc_out=. proto/hello.proto
```

### 4.3 实现服务并启动

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	grpcServer "github.com/tx7do/go-wind-plugins/transport/grpc/server"
	"google.golang.org/grpc"
)

// helloServiceImpl 实现 protobuf 生成的 HelloServiceServer 接口
type helloServiceImpl struct {
	pb.UnimplementedHelloServiceServer // 必须嵌入，提供前向兼容
}

// SayHello 实现 RPC 方法
func (s *helloServiceImpl) SayHello(_ context.Context, req *pb.HelloRequest) (*pb.HelloResponse, error) {
	return &pb.HelloResponse{
		Message: "Hello, " + req.GetName() + "!",
	}, nil
}

func main() {
	// 1. 创建 gRPC 服务器
	srv := grpcServer.NewServer(":9000")

	// 2. 注册服务
	pb.RegisterHelloServiceServer(srv.Server(), &helloServiceImpl{})

	// 3. 监听退出信号，实现优雅关闭
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("gRPC server listening on %s\n", srv.Endpoint())
	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("server stopped")
}
```

### 4.4 代码解读

| 代码 | 作用 |
|------|------|
| `NewServer(":9000")` | 创建 gRPC 服务器，监听 9000 端口 |
| `pb.UnimplementedHelloServiceServer` | **必须嵌入**，为新增方法提供默认实现（返回 Unimplemented 错误） |
| `srv.Server()` | 获取底层 `*grpc.Server`，用于注册服务 |
| `pb.RegisterHelloServiceServer(...)` | protobuf 生成的注册函数，将服务实现注册到 gRPC |
| `srv.Start(ctx)` | 启动服务器并阻塞，收到信号时优雅关闭 |
| `srv.Endpoint()` | 返回访问地址，如 `grpc://localhost:9000` |

> **初学者提示**：与 HTTP 不同，gRPC 的 `NewServer` 不需要指定驱动——因为底层就是 `google.golang.org/grpc`。

---

## 5. 服务注册

### 5.1 两种注册方式

**方式一：使用 protobuf 生成的注册函数（推荐）**

```go
// protoc-gen-go-grpc 会自动生成这个函数
pb.RegisterHelloServiceServer(srv.Server(), &helloServiceImpl{})
```

这是最简单的方式，生成的代码会自动处理 `ServiceDesc`。

**方式二：手动注册（了解原理）**

```go
srv.RegisterService(&pb.HelloService_ServiceDesc, &helloServiceImpl{})
```

### 5.2 注册多个服务

一个 gRPC 服务器可以承载多个服务：

```go
srv := grpcServer.NewServer(":9000")

// 注册多个服务到同一个服务器
pb.RegisterUserServiceServer(srv.Server(), &userServiceImpl{})
pb.RegisterOrderServiceServer(srv.Server(), &orderServiceImpl{})
pb.RegisterPaymentServiceServer(srv.Server(), &paymentServiceImpl{})
```

### 5.3 Server() 的延迟初始化

底层 `*grpc.Server` 在首次调用 `RegisterService()` 或 `Start()` 时才被创建：

```go
// ensureServer 的逻辑（简化）
func (s *Server) ensureServer() {
	if s.server != nil {
		return // 已创建，跳过
	}
	var opts []grpc.ServerOption
	if len(s.middlewares) > 0 {
		opts = append(opts, grpc.ChainUnaryInterceptor(s.middlewares...))
	}
	s.server = grpc.NewServer(opts...)
}
```

这意味着：**拦截器必须在注册服务或调用 Start 之前通过 `Use()` 添加**。

---

## 6. 拦截器（Interceptor）入门

### 6.1 什么是拦截器？

拦截器是 gRPC 版的"中间件"——采用洋葱模型，RPC 调用依次穿过每一层：

```
RPC请求 → [recovery] → [requestid] → [logging] → 业务Handler
                                                      ↓
RPC响应 ← [recovery] ← [requestid] ← [logging] ← 业务Handler
```

每一层拦截器都可以：
- 在调用 `handler` **之前** 执行逻辑（如鉴权、限流）
- 在调用 `handler` **之后** 执行逻辑（如记录耗时）
- 决定 **是否** 调用 `handler`（如鉴权失败时直接返回错误）

### 6.2 注册拦截器的两种方式

**方式一：`Use` 方法（推荐）**

```go
srv := grpcServer.NewServer(":9000")

// 在注册服务之前调用 Use
srv.Use(
	recovery.UnaryInterceptor(),
	logging.UnaryInterceptor(),
)

// 然后注册服务
pb.RegisterHelloServiceServer(srv.Server(), &helloServiceImpl{})
```

**方式二：`WithMiddleware` 选项（创建时传入）**

```go
srv := grpcServer.NewServer(":9000",
	grpcServer.WithMiddleware(
		recovery.UnaryInterceptor(),
		logging.UnaryInterceptor(),
	),
)
```

两种方式效果相同。`Use` 更灵活（可在创建后按需添加）。

### 6.3 拦截器顺序很重要！

拦截器按 **注册顺序** 执行：**先注册的最外层**（最先被调用）。

```go
// 正确顺序示例
srv.Use(
	recovery.UnaryInterceptor(),      // 最外层：捕获所有 panic
	requestid.UnaryServerInterceptor(), // 生成请求 ID
	logging.UnaryInterceptor(),        // 记录日志
	authn.UnaryInterceptor(auth),      // 认证（最内层，贴近业务）
)
```

> **经验法则**：`recovery` 永远放最外层，认证/限流类放最内层。

### 6.4 拦截器的工作原理

当你调用 `srv.Use(m1, m2)` 后，在 `ensureServer()` 时它们会被组装成 gRPC 的拦截器链：

```go
// ensureServer 中的关键逻辑
if len(s.middlewares) > 0 {
	opts = append(opts, grpc.ChainUnaryInterceptor(s.middlewares...))
}
s.server = grpc.NewServer(opts...)
```

`grpc.ChainUnaryInterceptor` 会按顺序链接拦截器：第一个注册的就是最外层。

---

## 7. 内置拦截器详解

Go-Wind 在 `transport/grpc/middleware/` 下提供了 16 个开箱即用的拦截器：

| 拦截器 | 作用 |
|--------|------|
| `recovery` | 捕获 panic，返回 Internal 错误 |
| `logging` | 记录 RPC 调用日志 |
| `requestid` | 生成/传播请求 ID |
| `timeout` | 超时控制 |
| `ratelimit` | 请求限流 |
| `authn` | 身份认证 |
| `authz` | 权限授权 |
| `metrics` | 指标采集 |
| `tracing` | 链路追踪 |
| `metadata` | 元数据传递 |
| `errors` | 统一错误处理 |
| `circuitbreaker` | 熔断保护 |
| `retry` | 请求重试 |
| `validate` | 参数校验 |
| `codec` | 编解码扩展 |
| `crypto` | 加解密 |

下面详细介绍最常用的 6 个。

---

### 7.1 recovery —— 异常恢复

**作用**：捕获 handler 中的 panic，记录日志并返回 gRPC `Internal` 错误，防止服务器崩溃。

**应该始终放在拦截器链的最外层。**

```go
import grpcRecovery "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/recovery"

// 基础用法
srv.Use(grpcRecovery.UnaryInterceptor())

// 高级用法
srv.Use(grpcRecovery.UnaryInterceptor(
	grpcRecovery.WithStackTrace(true),  // 记录堆栈信息（默认开启）
	grpcRecovery.WithLogger(myLogger),  // 自定义 logger
))
```

**工作流程**：

```go
// recovery 拦截器的内部逻辑（简化）
return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	defer func() {
		if rvr := recover(); rvr != nil {
			logger.Error(ctx, "panic recovered", "error", rvr, "method", info.FullMethod)
			err = status.Errorf(codes.Internal, "internal server error")
		}
	}()
	return handler(ctx, req) // 如果这里 panic，会被上面的 defer 捕获
}
```

> **与 HTTP 的区别**：gRPC 使用 `codes.Internal`（状态码 13）而非 HTTP 500，错误通过 gRPC status 返回。

---

### 7.2 logging —— 访问日志

**作用**：记录每个 RPC 调用的方法名、状态码、耗时。

```go
import "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/logging"

// 基础用法
srv.Use(logging.UnaryInterceptor())

// 跳过健康检查方法的日志
srv.Use(logging.UnaryInterceptor(
	logging.WithSkipMethods(
		"/grpc.health.v1.Health/Check", // gRPC 健康检查的完整方法名
	),
	logging.WithLogger(myLogger),
))
```

**日志输出示例**：

```
grpc unary rpc  method=/hello.HelloService/SayHello code=OK latency_ms=2
grpc unary rpc  method=/hello.HelloService/SayHello code=Internal error="panic: ..." latency_ms=1
```

**智能日志级别**：
- 状态码 `>= Internal (13)` → `Error` 级别
- 状态码 `>= NotFound (5)` → `Warn` 级别
- 其他 → `Info` 级别

> **提示**：`FullMethod` 的格式是 `/<包名>.<服务名>/<方法名>`，如 `/hello.HelloService/SayHello`。

---

### 7.3 requestid —— 请求追踪

**作用**：为每个 RPC 生成唯一 ID，通过 gRPC metadata 传播，便于日志关联和链路追踪。

```go
import "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/requestid"

// 服务端：提取或生成请求 ID
srv.Use(requestid.UnaryServerInterceptor())
```

**在 handler 中获取请求 ID**：

```go
func (s *helloServiceImpl) SayHello(ctx context.Context, req *pb.HelloRequest) (*pb.HelloResponse, error) {
	id := requestid.FromContext(ctx)
	fmt.Printf("handling request: %s\n", id)
	return &pb.HelloResponse{Message: "Hello, " + req.GetName() + "!"}, nil
}
```

**请求 ID 的传播机制**：

```
客户端                        服务端
  │                             │
  │  metadata:                  │  提取 x-request-id
  │  x-request-id: abc123  ──►  │  ↓
  │                             │  注入到 ctx
  │                             │  ↓
  │                             │  handler 通过 FromContext(ctx) 获取
```

- 客户端通过 metadata 发送 `x-request-id`
- 如果没有，服务端自动生成一个 32 字符的随机 hex ID
- ID 存入 context，下游 handler 可用 `FromContext()` 获取

**客户端也需要安装对应拦截器才能自动传播**（详见第 11 节）。

---

### 7.4 timeout —— 超时控制

**作用**：为没有设置 deadline 的 RPC 请求添加默认超时，防止慢请求拖垮服务器。

```go
import (
	"time"
	"github.com/tx7do/go-wind-plugins/transport/grpc/middleware/timeout"
)

// 所有 RPC 默认 30 秒超时
srv.Use(timeout.UnaryServerInterceptor(30 * time.Second))
```

**行为说明**：
- 如果客户端已通过 `context.WithDeadline` 设置了超时，则 **尊重客户端的设置**
- 如果没有设置，服务端会添加默认超时
- 超时后返回 `codes.DeadlineExceeded` 错误

**跳过特定方法**（如长时间运行的 RPC）：

```go
srv.Use(timeout.UnaryServerInterceptor(30*time.Second,
	timeout.WithSkipFunc(func(method string) bool {
		return method == "/hello.HelloService/LongRunningTask"
	}),
))
```

> **gRPC 超时机制**：gRPC 原生支持通过 context 传递 deadline。这个拦截器的作用是为"忘记设置 deadline"的请求兜底。

---

### 7.5 ratelimit —— 限流

**作用**：控制 RPC 调用速率，防止服务被压垮。超限时返回 `codes.ResourceExhausted`。

```go
import (
	"github.com/tx7do/go-wind-plugins/ratelimit/tokenbucket"
	grpcRatelimit "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/ratelimit"
)

// 创建令牌桶限流器：100 QPS，突发容量 200
limiter, _ := tokenbucket.New(100, 200)

// 拒绝模式（默认）：超限直接返回错误
srv.Use(grpcRatelimit.UnaryInterceptor(limiter))

// 等待模式：超限后排队等待
srv.Use(grpcRatelimit.UnaryInterceptor(limiter, grpcRatelimit.WithWait()))
```

**跳过健康检查方法**：

```go
srv.Use(grpcRatelimit.UnaryInterceptor(limiter,
	grpcRatelimit.WithSkipMethods("/grpc.health.v1.Health/Check"),
))
```

> **与 HTTP ratelimit 的区别**：gRPC 使用方法名（`info.FullMethod`）而非 URL 路径来做跳过判断。

---

### 7.6 authn —— 身份认证

**作用**：验证 RPC 调用者的身份（如 JWT Token），将认证结果注入 context 供下游使用。

需要配合 `security/authn` 模块的具体实现（如 JWT）：

```go
import (
	grpcAuthn "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/authn"
	jwtAuthn "github.com/tx7do/go-wind-plugins/security/authn/jwt"
)

// 1. 创建 JWT 认证器
authenticator, _ := jwtAuthn.NewAuthenticator(jwtAuthn.WithKey([]byte("my-secret")))

// 2. 应用认证拦截器
srv.Use(grpcAuthn.UnaryInterceptor(authenticator))
```

**跳过特定方法**（如健康检查不需要认证）：

```go
srv.Use(grpcAuthn.UnaryInterceptor(authenticator,
	grpcAuthn.WithSkipMethods("/grpc.health.v1.Health/Check"),
))
```

**在 handler 中获取认证信息**：

```go
import engine "github.com/tx7do/go-wind-plugins/security/authn"

func (s *userServiceImpl) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.User, error) {
	claims, ok := engine.AuthClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "no claims")
	}
	// 使用 claims 中的用户信息...
	return &pb.User{Id: req.GetId(), Name: "alice"}, nil
}
```

**行为说明**：
- 从 gRPC metadata 中提取 `authorization` 头
- 验证失败返回 `codes.Unauthenticated`，**不会**调用 handler
- 验证成功将 claims 注入 context

---

## 8. 编写自定义拦截器

### 8.1 最简单的拦截器

拦截器就是 `grpc.UnaryServerInterceptor`。下面写一个记录方法名的拦截器：

```go
// 定义拦截器
func logMethodInterceptor(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	fmt.Printf("calling method: %s\n", info.FullMethod)
	return handler(ctx, req) // 调用下一个拦截器或最终 handler
}

// 使用
srv.Use(logMethodInterceptor)
```

### 8.2 带配置的拦截器（函数式选项风格）

仿照内置拦截器，用 functional options 模式编写：

```go
// myValidator 拦截器

type Option func(*options)

type options struct {
	strict bool
}

func WithStrictMode(s bool) Option {
	return func(o *options) { o.strict = s }
}

func UnaryInterceptor(opts ...Option) grpc.UnaryServerInterceptor {
	cfg := &options{strict: false}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		// 在调用 handler 之前做参数校验
		if validator, ok := req.(interface{ Validate() error }); ok {
			if err := validator.Validate(); err != nil {
				return nil, status.Error(codes.InvalidArgument, err.Error())
			}
		}
		// 调用业务 handler
		return handler(ctx, req)
	}
}
```

### 8.3 短路拦截器（不调用 handler）

鉴权失败时直接返回，不执行后续逻辑：

```go
func requireAPIKey(validKey string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "no metadata")
		}
		keys := md.Get("x-api-key")
		if len(keys) == 0 || keys[0] != validKey {
			return nil, status.Error(codes.Unauthenticated, "invalid API key")
			// ↑ 直接返回，不调用 handler
		}
		return handler(ctx, req) // 验证通过，继续
	}
}
```

---

## 9. 拦截器组合：Chain

`Chain` 函数可以将多个拦截器打包成一个，便于复用：

```go
// 定义一组通用拦截器
commonInterceptors := grpcServer.Chain(
	recovery.UnaryInterceptor(),
	requestid.UnaryServerInterceptor(),
	logging.UnaryInterceptor(),
)

// 应用到服务器
srv.Use(commonInterceptors)
```

`Chain` 的实现逻辑（从后往前包裹，和 HTTP 版的 `Chain` 类似）：

```go
func Chain(middlewares ...Middleware) Middleware {
	switch len(middlewares) {
	case 0:
		return nil
	case 1:
		return middlewares[0]
	}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		h := handler
		for i := len(middlewares) - 1; i >= 0; i-- {
			interceptor, next := middlewares[i], h
			h = func(ctx context.Context, req any) (any, error) {
				return interceptor(ctx, req, info, next)
			}
		}
		return h(ctx, req)
	}
}
```

> **使用场景**：当多个服务器需要共享相同的拦截器集合时，用 `Chain` 避免重复。

---

## 10. 流式 RPC 与 Stream 拦截器

### 10.1 流式 RPC 简介

在 proto 中定义流式 RPC：

```protobuf
service ChatService {
  // 服务端流：客户端发一次，服务端返回多次
  rpc Subscribe(SubscribeRequest) returns (stream Event);

  // 双向流：双方都可多次收发
  rpc Chat(stream ChatMessage) returns (stream ChatMessage);
}
```

### 10.2 Stream 拦截器

流式 RPC 需要使用 `StreamMiddleware`（即 `grpc.StreamServerInterceptor`）：

```go
// 注册流拦截器
srv.UseStream(
	recovery.StreamInterceptor(),
	logging.StreamInterceptor(),
)
```

或通过选项：

```go
srv := grpcServer.NewServer(":9000",
	grpcServer.WithStreamMiddleware(
		recovery.StreamInterceptor(),
		logging.StreamInterceptor(),
	),
)
```

### 10.3 Stream 拦截器的特殊之处

Stream 拦截器的签名与一元拦截器不同：

```go
func(
    srv any,                       // 服务实现
    ss grpc.ServerStream,          // 服务端流（用于收发消息）
    info *grpc.StreamServerInfo,   // 流信息
    handler grpc.StreamHandler,    // 下一个处理器
) error
```

**关键区别**：
- 一元拦截器直接拿到 `req` 和返回 `resp`
- Stream 拦截器操作的是 `grpc.ServerStream`，需要通过它来收发消息
- Stream 拦截器如果要修改 context，需要包装 `ServerStream`（因为 context 是只读的）

```go
// Stream 拦截器修改 context 的通用模式
return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	// 创建带新值的 context
	ctx := ss.Context()
	newCtx := context.WithValue(ctx, myKey{}, "my-value")

	// 包装 ServerStream 以替换 context
	wrapped := &wrappedServerStream{ServerStream: ss, ctx: newCtx}
	return handler(srv, wrapped)
}

// wrappedServerStream 包装类
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}
```

> **初学者提示**：如果你刚开始用 gRPC，可以暂时只关注一元 RPC 和一元拦截器，等需要流式 RPC 时再研究 Stream 拦截器。

---

## 11. gRPC 客户端

Go-Wind 也封装了 gRPC 客户端，位于 `transport/grpc/client`。

### 11.1 基本用法

```go
import (
	"context"
	"fmt"

	grpcClient "github.com/tx7do/go-wind-plugins/transport/grpc/client"
)

func main() {
	// 创建客户端
	c := grpcClient.NewClient("localhost:9000")

	// 建立连接
	if err := c.Dial(context.Background()); err != nil {
		panic(err)
	}
	defer c.Close()

	// 使用 protobuf 生成的客户端调用服务
	helloClient := pb.NewHelloServiceClient(c.Conn())
	resp, err := helloClient.SayHello(context.Background(), &pb.HelloRequest{Name: "World"})
	if err != nil {
		panic(err)
	}
	fmt.Println(resp.GetMessage())
}
```

### 11.2 客户端拦截器

客户端也支持拦截器，常用于 **请求追踪** 和 **超时控制**：

```go
import (
	"time"
	grpcClient "github.com/tx7do/go-wind-plugins/transport/grpc/client"
	"github.com/tx7do/go-wind-plugins/transport/grpc/middleware/requestid"
	grpcTimeout "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/timeout"
)

c := grpcClient.NewClient("localhost:9000",
	grpcClient.WithMiddleware(
		requestid.UnaryClientInterceptor(),              // 自动注入请求 ID
		grpcTimeout.UnaryClientInterceptor(10*time.Second), // 客户端超时
	),
)
```

### 11.3 TLS 配置

```go
import (
	"google.golang.org/grpc/credentials"
)

creds, _ := credentials.NewClientTLSFromFile("cert.pem", "")
c := grpcClient.NewClient("localhost:9000",
	grpcClient.WithTransportCredentials(creds),
)

// 如果不设置任何凭证，默认使用 insecure（明文）
// 也可以显式指定：
c := grpcClient.NewClient("localhost:9000",
	grpcClient.WithInsecure(),
)
```

### 11.4 客户端选项一览

| 选项 | 说明 |
|------|------|
| `WithConn(conn)` | 注入已建立的连接 |
| `WithDialOption(opts...)` | 透传原生 `grpc.DialOption` |
| `WithTransportCredentials(creds)` | 设置 TLS 凭证 |
| `WithInsecure()` | 明文传输 |
| `WithMiddleware(mws...)` | 一元客户端拦截器 |
| `WithStreamMiddleware(mws...)` | 流客户端拦截器 |

---

## 12. 生产级完整示例

下面这个示例整合了前面学到的所有内容，是一个接近生产环境的 gRPC 服务：

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tx7do/go-wind-plugins/ratelimit/tokenbucket"
	grpcAuthn "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/authn"
	"github.com/tx7do/go-wind-plugins/transport/grpc/middleware/logging"
	"github.com/tx7do/go-wind-plugins/transport/grpc/middleware/ratelimit"
	grpcRecovery "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/recovery"
	"github.com/tx7do/go-wind-plugins/transport/grpc/middleware/requestid"
	grpcTimeout "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/timeout"
	grpcServer "github.com/tx7do/go-wind-plugins/transport/grpc/server"
	jwtAuthn "github.com/tx7do/go-wind-plugins/security/authn/jwt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// helloServiceImpl 实现服务
type helloServiceImpl struct {
	pb.UnimplementedHelloServiceServer
}

func (s *helloServiceImpl) SayHello(ctx context.Context, req *pb.HelloRequest) (*pb.HelloResponse, error) {
	// 获取请求 ID（来自 requestid 拦截器）
	requestID := requestid.FromContext(ctx)

	// 获取认证信息（来自 authn 拦截器，可选）
	// claims, _ := engine.AuthClaimsFromContext(ctx)

	return &pb.HelloResponse{
		Message: fmt.Sprintf("Hello, %s! (request: %s)", req.GetName(), requestID),
	}, nil
}

func main() {
	// 创建限流器
	limiter, _ := tokenbucket.New(100, 200)

	// 创建 JWT 认证器
	authenticator, _ := jwtAuthn.NewAuthenticator(
		jwtAuthn.WithKey([]byte("my-secret-key")),
	)

	srv := grpcServer.NewServer(":9000",
		grpcServer.WithTimeout(30*time.Second), // 优雅关闭超时
	)

	// 拦截器链（顺序很重要！）
	srv.Use(
		grpcRecovery.UnaryInterceptor(),                     // 1. 最外层：捕获 panic
		requestid.UnaryServerInterceptor(),                  // 2. 生成请求 ID
		logging.UnaryInterceptor(                            // 3. 访问日志
			logging.WithSkipMethods("/grpc.health.v1.Health/Check"),
		),
		grpcTimeout.UnaryServerInterceptor(30*time.Second),  // 4. 超时控制
		ratelimit.UnaryInterceptor(limiter,                  // 5. 限流
			ratelimit.WithSkipMethods("/grpc.health.v1.Health/Check"),
		),
		authn.UnaryInterceptor(authenticator,                // 6. 认证（最内层）
			authn.WithSkipMethods("/grpc.health.v1.Health/Check"),
		),
	)

	// 注册服务
	pb.RegisterHelloServiceServer(srv.Server(), &helloServiceImpl{})

	// 优雅关闭
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("gRPC server listening on %s\n", srv.Endpoint())
	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("server stopped gracefully")
}
```

**测试**（使用 grpcurl）：

```bash
# 健康检查（跳过认证）
grpcurl -plaintext localhost:9000 list

# 调用服务（需要带 token）
grpcurl -plaintext \
  -H "authorization: Bearer <your-jwt-token>" \
  -d '{"name":"World"}' \
  localhost:9000 hello.HelloService/SayHello
```

---

## 13. 常见问题

### Q1: 服务方法返回 "Unimplemented" 错误？

**原因**：服务实现没有嵌入 `UnimplementedXxxServer`。

**解决**：

```go
type myServiceImpl struct {
	pb.UnimplementedMyServiceServer // ← 必须嵌入！
}
```

这是 protobuf 的前向兼容设计——当 proto 文件新增方法但你的代码还没实现时，会返回 `Unimplemented` 而非编译错误。

### Q2: 拦截器没生效？

**检查清单**：
1. `Use()` 必须在 `RegisterService()` 或 `Start()` **之前**调用
2. 确认拦截器包已正确 import
3. 检查拦截器顺序是否合理（如 recovery 必须在最外层）

```go
// ✅ 正确顺序
srv.Use(recovery.UnaryInterceptor(), logging.UnaryInterceptor())
pb.RegisterHelloServiceServer(srv.Server(), &impl{}) // Use 之后

// ❌ 错误顺序
pb.RegisterHelloServiceServer(srv.Server(), &impl{}) // 此时 grpc.Server 已创建
srv.Use(recovery.UnaryInterceptor())                 // 太晚了！不会生效
```

### Q3: 如何获取实际监听端口（用于 `:0` 随机端口）？

```go
srv := grpcServer.NewServer(":0")
// 启动后
fmt.Println(srv.Endpoint()) // 输出: grpc://localhost:54321
```

### Q4: 如何只对部分方法跳过认证/限流？

所有内置拦截器都支持 `WithSkipMethods` 选项：

```go
srv.Use(authn.UnaryInterceptor(auth,
	authn.WithSkipMethods(
		"/grpc.health.v1.Health/Check",  // 健康检查不需要认证
		"/hello.PublicService/Health",   // 公开接口
	),
))
```

方法的完整格式是 `/<包名>.<服务名>/<方法名>`。

### Q5: gRPC 和 HTTP 的拦截器/中间件有什么区别？

| 方面 | HTTP 中间件 | gRPC 拦截器 |
|------|------------|-------------|
| 类型签名 | `func(http.Handler) http.Handler` | `grpc.UnaryServerInterceptor` |
| 请求数据 | `*http.Request` | `any`（protobuf 消息） |
| 响应方式 | `http.ResponseWriter` | 返回值 `(resp, err)` |
| 错误处理 | HTTP 状态码 | gRPC status codes |
| 路由区分 | URL 路径 | `info.FullMethod` |
| 流支持 | 无 | `StreamServerInterceptor` |

### Q6: 客户端报 "connection refused"？

**排查步骤**：
1. 确认服务端已启动且监听正确端口
2. 确认地址格式正确：`localhost:9000`（不是 `grpc://localhost:9000`）
3. 确认 TLS 设置匹配（服务端用 TLS，客户端也要用 TLS）
4. 如果用 `:0` 随机端口，通过 `srv.Endpoint()` 获取实际端口

---

## 附录：API 速查表

### Server 方法

| 方法 | 说明 |
|------|------|
| `NewServer(addr, opts...)` | 创建服务器 |
| `Use(mws...)` | 注册一元拦截器 |
| `UseStream(mws...)` | 注册流拦截器 |
| `Server()` | 获取底层 `*grpc.Server` |
| `RegisterService(desc, impl)` | 注册服务 |
| `Start(ctx)` | 启动服务器（阻塞） |
| `Stop(ctx)` | 停止服务器（优雅关闭） |
| `Endpoint()` | 获取访问地址 |
| `Addr()` | 获取监听地址 |

### Option 选项

| 选项 | 说明 |
|------|------|
| `WithServer(srv)` | 直接设置底层 `*grpc.Server` |
| `WithMiddleware(mws...)` | 创建时设置一元拦截器 |
| `WithStreamMiddleware(mws...)` | 创建时设置流拦截器 |
| `WithTimeout(d)` | 设置优雅关闭超时 |

### 顶层函数

| 函数 | 说明 |
|------|------|
| `Chain(mws...)` | 将多个拦截器组合为一个 |
| `FormatEndpoint(host, port)` | 格式化 gRPC endpoint 字符串 |

### 常用 gRPC 状态码

| 状态码 | 值 | 含义 |
|--------|-----|------|
| `OK` | 0 | 成功 |
| `NotFound` | 5 | 未找到 |
| `InvalidArgument` | 3 | 参数错误 |
| `Unauthenticated` | 16 | 未认证 |
| `PermissionDenied` | 7 | 无权限 |
| `ResourceExhausted` | 8 | 资源耗尽（限流） |
| `DeadlineExceeded` | 4 | 超时 |
| `Internal` | 13 | 内部错误 |

---

## 附录：与 HTTP 教程的对照

如果你已经阅读过 [HTTP 服务器教程](./http-server-tutorial.md)，下面的对照表可以帮助你快速迁移知识：

| 概念 | HTTP | gRPC |
|------|------|------|
| 服务器创建 | `httpServer.NewServer(addr, WithDriver(...))` | `grpcServer.NewServer(addr)` |
| 中间件类型 | `func(http.Handler) http.Handler` | `grpc.UnaryServerInterceptor` |
| 注册中间件 | `srv.Use(mws...)` | `srv.Use(mws...)` |
| 注册路由/服务 | `srv.GET(path, handler)` | `pb.RegisterXxxServer(srv.Server(), impl)` |
| 启动 | `srv.Start(ctx)` | `srv.Start(ctx)` |
| 错误返回 | HTTP 状态码（`http.Error`） | gRPC status（`status.Error`） |
| 请求标识 | URL 路径 | `info.FullMethod` |
| 上下文传递 | `r.WithContext(ctx)` | `handler(ctx, req)` |

---

> **下一步学习**：尝试运行 `_examples/grpc-basic` 目录下的示例代码，动手实践是掌握这些概念的最佳方式！

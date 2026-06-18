# Go-Wind HTTP 服务器从入门到精通

> 本教程面向初学者，基于 `transport/http/server.go`，循序渐进地讲解如何使用 Go-Wind 插件库搭建 HTTP 服务器——从最简单的 "Hello World" 开始，逐步深入到驱动系统、中间件机制，最终构建一个生产级的服务。

---

## 目录

- [1. 简介](#1-简介)
- [2. 核心概念](#2-核心概念)
- [3. 快速开始：30 秒跑起一个服务器](#3-快速开始30-秒跑起一个服务器)
- [4. 路由注册](#4-路由注册)
- [5. 驱动系统：切换底层框架](#5-驱动系统切换底层框架)
- [6. 中间件入门](#6-中间件入门)
- [7. 内置中间件详解](#7-内置中间件详解)
  - [7.1 recovery —— 异常恢复](#71-recovery--异常恢复)
  - [7.2 requestid —— 请求追踪](#72-requestid--请求追踪)
  - [7.3 logging —— 访问日志](#73-logging--访问日志)
  - [7.4 cors —— 跨域资源共享](#74-cors--跨域资源共享)
  - [7.5 timeout —— 请求超时](#75-timeout--请求超时)
  - [7.6 codec —— 内容协商](#76-codec--内容协商)
  - [7.7 ratelimit —— 限流](#77-ratelimit--限流)
  - [7.8 authn —— 身份认证](#78-authn--身份认证)
- [8. 编写自定义中间件](#8-编写自定义中间件)
- [9. 中间件组合：Chain](#9-中间件组合chain)
- [10. HTTPS / TLS 配置](#10-https--tls-配置)
- [11. 生产级完整示例](#11-生产级完整示例)
- [12. 常见问题](#12-常见问题)

---

## 1. 简介

`transport/http` 是 Go-Wind 插件库提供的 **HTTP 服务器抽象层**。它的核心价值在于：

| 特性 | 说明 |
|------|------|
| **多驱动架构** | 底层可在标准库 `net/http`、`gin`、`chi`、`fiber` 之间无缝切换，上层代码无需改动 |
| **统一中间件** | 所有中间件都遵循 `func(http.Handler) http.Handler` 标准，与具体驱动无关 |
| **优雅关闭** | 内置基于 `context.Context` 的优雅停机支持 |
| **TLS 友好** | 自动处理 HTTPS 监听器，自动推断 `http/https` scheme |

如果你用过 Go 标准库的 `net/http`，那么上手会非常快——因为路由处理函数就是原生的 `http.HandlerFunc`。

---

## 2. 核心概念

在写第一行代码前，先理解三个核心概念：

### 2.1 Server（服务器）

`Server` 是整个抽象的入口，它负责：
- 创建并管理 `net.Listener`（监听端口）
- 处理 TLS 包装
- 维护 **中间件链**
- 将请求转发给底层 **Driver**

它实现了 `transport.Server` 接口，提供 `Start / Stop / Endpoint` 等方法。

### 2.2 Driver（驱动）

`Driver` 是一个接口，定义了"谁来真正处理路由和监听"：

```go
type Driver interface {
    Handle(method, path string, handler http.HandlerFunc)
    Start(ctx context.Context, ln net.Listener) error
    Stop(ctx context.Context) error
}
```

目前内置了 4 种驱动：

| 驱动 | 包路径 | 特点 |
|------|--------|------|
| **std** | `transport/http/driver/std` | 基于 `net/http`，零依赖，适合学习 |
| **gin** | `transport/http/driver/gin` | 基于 Gin 框架，性能优秀 |
| **chi** | `transport/http/driver/chi` | 基于 chi 路由，轻量且 100% 兼容 `net/http` |
| **fiber** | `transport/http/driver/fiber` | 基于 Fiber，极致性能（基于 fasthttp） |

> **关键点**：切换驱动只需要改一行 import 和 `NewDriver()` 调用，其余代码完全不变。

### 2.3 Middleware（中间件）

中间件是一个函数，它接收"下一个处理器"，返回"包装后的处理器"：

```go
type Middleware func(http.Handler) http.Handler
```

你可以在请求到达业务逻辑 **之前** 或 **之后** 插入自定义逻辑，例如：记录日志、鉴权、限流、错误恢复等。

```
请求 → [recovery] → [requestid] → [logging] → 业务Handler
                                                    ↓
响应 ← [recovery] ← [requestid] ← [logging] ← 业务Handler
```

---

## 3. 快速开始：30 秒跑起一个服务器

### 3.1 最小示例

这是你能写出的最简单的 Go-Wind HTTP 服务器：

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	httpServer "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/driver/std"
)

func main() {
	// 1. 创建服务器：监听 :8080，使用标准库驱动
	srv := httpServer.NewServer(":8080", httpServer.WithDriver(std.NewDriver()))

	// 2. 注册一个 GET 路由
	srv.GET("/hello", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, GoWind!")
	})

	// 3. 监听退出信号，实现优雅关闭
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("HTTP server listening on %s\n", srv.Endpoint())
	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("server stopped")
}
```

### 3.2 运行与测试

```bash
# 运行
go run .

# 另开一个终端测试
curl http://localhost:8080/hello
# 输出: Hello, GoWind!
```

### 3.3 代码解读

逐行理解关键部分：

| 代码 | 作用 |
|------|------|
| `NewServer(":8080", WithDriver(...))` | 创建服务器，`":8080"` 是监听地址 |
| `WithDriver(std.NewDriver())` | **必须**指定一个驱动，否则 `Handle/Start` 时会 panic |
| `srv.GET("/hello", handler)` | 注册路由，handler 是标准 `http.HandlerFunc` |
| `signal.NotifyContext(...)` | 捕获 Ctrl+C / kill 信号 |
| `srv.Start(ctx)` | 启动服务器并阻塞，当 `ctx` 被取消时执行优雅关闭 |
| `srv.Endpoint()` | 返回实际的访问地址（支持 `:0` 随机端口场景） |

> **初学者提示**：`Start` 会阻塞当前 goroutine，直到收到关闭信号。所以把它放在 `main` 函数末尾即可。

---

## 4. 路由注册

### 4.1 快捷方法

`Server` 为常用 HTTP 方法提供了快捷方法，内部都调用了 `Handle`：

```go
srv.GET("/users", listUsers)       // 查询用户列表
srv.POST("/users", createUser)     // 创建用户
srv.PUT("/users/:id", updateUser)  // 更新用户（完整替换）
srv.PATCH("/users/:id", patchUser) // 更新用户（部分修改）
srv.DELETE("/users/:id", delUser)  // 删除用户
srv.HEAD("/users/:id", headUser)   // 只获取头部信息
srv.OPTIONS("/users", optUsers)    // CORS 预检等
```

### 4.2 通用 Handle 方法

如果你需要用到非标准方法，可以直接用 `Handle`：

```go
srv.Handle("CONNECT", "/tunnel", connectHandler)
```

### 4.3 注意：路由能力取决于驱动

不同驱动对路由路径（如 `:id` 参数匹配、通配符、路由分组）的支持程度不同：

| 能力 | std | chi | gin |
|------|-----|-----|-----|
| 基础路径匹配 | ✅ | ✅ | ✅ |
| 路径参数 `/:id` | ❌ | ✅ | ✅ |
| 通配符 `/*` | ❌ | ✅ | ✅ |
| 路由分组 | ❌ | ✅ | ✅ |

如果你需要高级路由功能（路径参数、嵌套路由），建议使用 **chi** 或 **gin** 驱动。

---

## 5. 驱动系统：切换底层框架

### 5.1 为什么需要多驱动？

不同的项目有不同的需求：
- **学习/原型**：用 `std`，零额外依赖
- **生产环境**：用 `gin` 或 `chi`，功能丰富、社区成熟
- **极致性能**：用 `fiber`（注意：fiber 基于 fasthttp，与 `net/http` 接口不完全兼容）

### 5.2 切换示例：从 std 到 gin

只需改两行 import，业务代码完全不变：

```go
import (
	httpServer "github.com/tx7do/go-wind-plugins/transport/http"
	// 改动 1：把 std 换成 gin
	"github.com/tx7do/go-wind-plugins/transport/http/driver/gin"
)

func main() {
	// 改动 2：std.NewDriver() → gin.NewDriver()
	srv := httpServer.NewServer(":8080", httpServer.WithDriver(gin.NewDriver()))

	// 下面的代码和之前一模一样！
	srv.GET("/hello", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello from Gin!")
	})
	// ...
}
```

### 5.3 使用 chi 驱动（支持路径参数）

```go
import (
	httpServer "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/driver/chi"
)

func main() {
	srv := httpServer.NewServer(":8080", httpServer.WithDriver(chi.NewDriver()))

	// chi 支持路径参数（注意：在 handler 内需用 chi.URLParam 提取）
	srv.GET("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id") // 需 import "github.com/go-chi/chi/v5"
		fmt.Fprintf(w, "User ID: %s\n", id)
	})
	// ...
}
```

> **小结**：驱动系统的精髓是 **面向接口编程**。`Server` 只依赖 `Driver` 接口，与具体实现解耦。

---

## 6. 中间件入门

### 6.1 什么是中间件？

中间件是一种"洋葱模型"——请求从外向内穿过每一层，响应从内向外返回：

```
请求 ──► recovery ──► logging ──► 业务Handler
                                         │
响应 ◄── recovery ◄── logging ◄──────────┘
```

每一层中间件都可以：
- 在调用 `next` **之前** 执行逻辑（如鉴权、限流）
- 在调用 `next` **之后** 执行逻辑（如记录响应时间、修改响应头）
- 决定 **是否** 调用 `next`（如鉴权失败时直接返回 401）

### 6.2 注册中间件的两种方式

**方式一：`Use` 方法（推荐，更直观）**

```go
srv := httpServer.NewServer(":8080", httpServer.WithDriver(std.NewDriver()))

// 在注册路由之前调用 Use
srv.Use(
	recovery.Middleware(),
	logging.Middleware(),
)

srv.GET("/hello", handler)
```

**方式二：`WithMiddleware` 选项（创建时传入）**

```go
srv := httpServer.NewServer(":8080",
	httpServer.WithDriver(std.NewDriver()),
	httpServer.WithMiddleware(
		recovery.Middleware(),
		logging.Middleware(),
	),
)
```

两种方式效果相同，`Use` 更灵活（可在创建后按需添加）。

### 6.3 中间件顺序很重要！

中间件按 **注册顺序** 执行：**先注册的最外层**（最先被调用）。

```go
// 正确顺序示例
srv.Use(
	recovery.Middleware(),  // 最外层：捕获所有 panic
	requestid.Middleware(), // 生成请求 ID
	logging.Middleware(),   // 记录日志（能拿到请求 ID）
	codec.Middleware(),     // 内容协商（最内层，最贴近业务）
)
```

> **经验法则**：`recovery` 永远放最外层，`codec` 这类贴近业务的放最内层。

### 6.4 中间件的工作原理

当你调用 `srv.GET("/hello", handler)` 时，`Server` 内部会用中间件链包装 handler：

```go
// server.go 中的 wrapHandler 逻辑（简化版）
func (s *Server) wrapHandler(handler http.HandlerFunc) http.HandlerFunc {
	if len(s.middlewares) == 0 {
		return handler // 没有中间件，直接返回
	}
	h := http.Handler(handler)
	// 从后往前包裹：第一个注册的中间件变成最外层
	for i := len(s.middlewares) - 1; i >= 0; i-- {
		h = s.middlewares[i](h)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
	}
}
```

理解这段代码，你就理解了中间件机制的核心。

---

## 7. 内置中间件详解

Go-Wind 在 `transport/http/middleware/` 下提供了 17 个开箱即用的中间件：

| 中间件 | 作用 |
|--------|------|
| `recovery` | 捕获 panic，返回 500 |
| `requestid` | 生成/传播请求 ID |
| `logging` | 记录请求日志 |
| `cors` | 跨域资源共享 |
| `timeout` | 请求超时控制 |
| `codec` | 内容协商（JSON/XML/...） |
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
| `crypto` | 加解密 |

下面详细介绍最常用的 8 个。

---

### 7.1 recovery —— 异常恢复

**作用**：捕获 handler 中的 panic，记录日志并返回 500，防止服务器崩溃。

**应该始终放在中间件链的最外层。**

```go
import "github.com/tx7do/go-wind-plugins/transport/http/middleware/recovery"

// 基础用法
srv.Use(recovery.Middleware())

// 高级用法：自定义选项
srv.Use(recovery.Middleware(
	recovery.WithStackTrace(true),  // 记录堆栈信息（默认开启）
	recovery.WithLogger(myLogger),  // 使用自定义 logger
))
```

**工作流程**：

```go
// recovery 中间件的内部逻辑（简化）
return func(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rvr := recover(); rvr != nil {
				logger.Error(ctx, "panic recovered", "error", rvr)
				http.Error(w, "Internal Server Error", 500)
			}
		}()
		next.ServeHTTP(w, r) // 如果这里 panic，会被上面的 defer 捕获
	})
}
```

---

### 7.2 requestid —— 请求追踪

**作用**：为每个请求生成唯一 ID，放入 context 并写入响应头，便于日志追踪和链路排查。

```go
import "github.com/tx7do/go-wind-plugins/transport/http/middleware/requestid"

// 基础用法：使用默认的 "X-Request-ID" 头
srv.Use(requestid.Middleware())

// 自定义头名
srv.Use(requestid.Middleware(
	requestid.WithHeaderName("X-Correlation-ID"),
))
```

**在 handler 中获取请求 ID**：

```go
srv.GET("/hello", func(w http.ResponseWriter, r *http.Request) {
	id := requestid.FromContext(r.Context())
	fmt.Fprintf(w, "Hello! (request-id: %s)\n", id)
})
```

**行为说明**：
- 如果请求头带了 `X-Request-ID`，则使用该值（方便上游服务传递）
- 如果没有，则自动生成一个 32 字符的随机 hex ID
- 响应头中也会回写这个 ID，客户端可以看到

---

### 7.3 logging —— 访问日志

**作用**：记录每个请求的方法、路径、状态码、响应大小、耗时和客户端地址。

```go
import "github.com/tx7do/go-wind-plugins/transport/http/middleware/logging"

// 基础用法
srv.Use(logging.Middleware())

// 跳过健康检查路径的日志（避免刷屏）
srv.Use(logging.Middleware(
	logging.WithSkipPaths("/healthz", "/readyz"),
	logging.WithLogger(myLogger),
))
```

**日志输出示例**：

```
http request  method=GET path=/hello status=200 size=42 latency_ms=3 remote=127.0.0.1:54321
```

**智能日志级别**：
- 状态码 `>= 500` → `Error` 级别
- 状态码 `>= 400` → `Warn` 级别
- 其他 → `Info` 级别

> **建议**：`logging` 放在 `requestid` 之后，这样日志中可以关联请求 ID（需配合 logger 实现）。

---

### 7.4 cors —— 跨域资源共享

**作用**：处理浏览器的跨域请求，自动设置 `Access-Control-*` 响应头，并短路处理 OPTIONS 预检请求。

```go
import "github.com/tx7do/go-wind-plugins/transport/http/middleware/cors"

srv.Use(cors.Middleware(
	cors.WithAllowedOrigins("https://app.example.com"),  // 允许的前端域名
	cors.WithAllowedMethods("GET", "POST", "PUT", "DELETE"),
	cors.WithAllowedHeaders("Authorization", "Content-Type"),
	cors.WithAllowCredentials(true),   // 允许携带 Cookie
	cors.WithMaxAge(3600),             // 预检缓存 1 小时
))
```

**关键选项说明**：

| 选项 | 默认值 | 说明 |
|------|--------|------|
| `WithAllowedOrigins` | 空（允许所有 `*`） | 指定允许的源 |
| `WithAllowedMethods` | 常见 7 种方法 | 允许的 HTTP 方法 |
| `WithAllowCredentials` | `false` | 是否允许带 Cookie |
| `WithMaxAge` | 0（不缓存） | 预检结果缓存秒数 |

> **注意**：`cors` 应放在路由处理之前。对于没有 `Origin` 头的请求（非浏览器），会直接放行。

---

### 7.5 timeout —— 请求超时

**作用**：为每个请求设置超时时间，超时后取消 context 并返回错误响应，防止慢请求拖垮服务器。

```go
import (
	"time"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/timeout"
)

// 所有请求统一 30 秒超时
srv.Use(timeout.Middleware(30 * time.Second))

// 自定义超时响应
srv.Use(timeout.Middleware(30 * time.Second,
	timeout.WithStatus(http.StatusGatewayTimeout), // 返回 504
	timeout.WithMessage("Request timed out"),
))
```

**按路由设置不同超时**（高级用法）：

```go
srv.Use(timeout.Middleware(30*time.Second,
	// 长轮询接口跳过超时
	timeout.WithSkipFunc(func(r *http.Request) bool {
		return r.URL.Path == "/events/stream"
	}),
))
```

> **超时行为**：当超时触发时，handler 的 `context` 会被取消。你的业务代码应该监听 `ctx.Done()` 来及时终止耗时操作。

---

### 7.6 codec —— 内容协商

**作用**：根据请求的 `Content-Type` 自动选择编解码器（JSON/XML/YAML/...），让 handler 不用关心序列化细节。

```go
import (
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // 注册 JSON 编解码器
	_ "github.com/tx7do/go-wind-plugins/encoding/xml"  // 注册 XML 编解码器
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/codec"
)

srv.Use(codec.Middleware())
```

> **重要**：必须通过 `import _` 引入对应的 encoding 包来注册编解码器，否则会报 "unsupported content type"。

**在 handler 中使用**：

```go
type greetRequest struct {
	Name string `json:"name" xml:"name"`
}

type greetResponse struct {
	Message string `json:"message" xml:"message"`
}

srv.POST("/echo", func(w http.ResponseWriter, r *http.Request) {
	var req greetRequest
	// ReadBody 自动根据 Content-Type 反序列化
	if err := codec.ReadBody(r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	// Respond 自动序列化并设置 Content-Type
	codec.Respond(w, r, http.StatusOK, &greetResponse{
		Message: "Hello, " + req.Name + "!",
	})
})
```

**支持的格式**（需引入对应的 encoding 包）：

| MIME 类型 | 编解码器 |
|-----------|----------|
| `application/json` | json |
| `application/xml` | xml |
| `application/x-yaml` | yaml |
| `application/protobuf` | proto |
| `application/x-msgpack` | msgpack |
| `application/cbor` | cbor |
| `application/x-toml` | toml |
| ... | ... |

**测试内容协商**：

```bash
# 发送 JSON
curl -H "Content-Type: application/json" -d '{"name":"alice"}' http://localhost:8080/echo
# {"message":"Hello, alice!"}

# 发送 XML
curl -H "Content-Type: application/xml" -d '<greetRequest><name>bob</name></greetRequest>' http://localhost:8080/echo
# <greetResponse><message>Hello, bob!</message></greetResponse>
```

---

### 7.7 ratelimit —— 限流

**作用**：控制请求速率，防止服务被压垮。支持全局限流和按客户端限流。

**需要先创建一个限流器**（来自 `ratelimit` 模块）：

```go
import (
	"github.com/tx7do/go-wind-plugins/ratelimit"
	"github.com/tx7do/go-wind-plugins/ratelimit/tokenbucket"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/ratelimit"
)

// 创建令牌桶限流器：100 QPS，突发容量 200
limiter, _ := tokenbucket.New(100, 200)

// 模式一：拒绝模式（默认）—— 超限直接返回 429
srv.Use(ratelimit.Middleware(limiter))

// 模式二：等待模式 —— 超限后排队等待，平滑突发流量
srv.Use(ratelimit.Middleware(limiter, ratelimit.WithWait()))
```

**按客户端 IP 限流**（每个 IP 独立配额）：

```go
srv.Use(ratelimit.MiddlewareKeyed(
	// keyFn：从请求中提取限流键（这里用客户端 IP）
	func(r *http.Request) string {
		return r.RemoteAddr
	},
	// factory：为每个新 key 创建一个限流器
	func(_ string) ratelimit.Limiter {
		l, _ := tokenbucket.New(10, 20) // 每个 IP: 10 QPS, 突发 20
		return l
	},
))
```

> **拒绝模式 vs 等待模式**：拒绝模式响应快但用户体验差（直接 429）；等待模式更友好但会增加延迟。

---

### 7.8 authn —— 身份认证

**作用**：验证请求的身份（如 JWT Token），将认证结果注入 context 供下游使用。

需要配合 `security/authn` 模块的具体实现（如 JWT）：

```go
import (
	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/authn"
	jwtAuthn "github.com/tx7do/go-wind-plugins/security/authn/jwt"
)

// 1. 创建 JWT 认证器
authenticator, _ := jwtAuthn.NewAuthenticator(jwtAuthn.WithKey([]byte("my-secret")))

// 2. 应用认证中间件
srv.Use(authn.Middleware(authenticator))
```

**行为说明**：
- 从 `Authorization` 头提取 Token（可通过 `WithHeaderName` 自定义）
- 验证失败返回 401，**不会**调用后续 handler
- 验证成功将认证信息注入 context

**在 handler 中获取认证信息**：

```go
import engine "github.com/tx7do/go-wind-plugins/security/authn"

srv.GET("/api/profile", func(w http.ResponseWriter, r *http.Request) {
	claims, ok := engine.AuthClaimsFromContext(r.Context())
	if !ok {
		http.Error(w, "no claims", http.StatusUnauthorized)
		return
	}
	// 使用 claims 中的用户信息...
	fmt.Fprintf(w, "Welcome, user!")
})
```

---

## 8. 编写自定义中间件

### 8.1 最简单的中间件

中间件就是 `func(http.Handler) http.Handler`。下面写一个添加自定义响应头的中间件：

```go
// 定义中间件
func addHeader(key, value string) httpServer.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(key, value) // 调用 next 之前设置
			next.ServeHTTP(w, r)        // 继续执行链
		})
	}
}

// 使用
srv.Use(addHeader("X-Powered-By", "GoWind"))
```

### 8.2 带配置的中间件（函数式选项风格）

仿照内置中间件，用 functional options 模式编写可配置的中间件：

```go
// myLogger 中间件包

type Option func(*options)

type options struct {
	prefix string
}

func WithPrefix(p string) Option {
	return func(o *options) { o.prefix = p }
}

func Middleware(opts ...Option) httpServer.Middleware {
	cfg := &options{prefix: "[APP]"}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r) // 先执行下游
			// 执行完下游后记录耗时（after 逻辑）
			fmt.Printf("%s %s %s %v\n", cfg.prefix, r.Method, r.URL.Path, time.Since(start))
		})
	}
}
```

### 8.3 短路中间件（不调用 next）

某些场景下需要直接返回，不执行后续逻辑（如鉴权失败）：

```go
func requireAPIKey(validKey string) httpServer.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-API-Key")
			if key != validKey {
				http.Error(w, "invalid API key", http.StatusUnauthorized)
				return // 直接返回，不调用 next
			}
			next.ServeHTTP(w, r) // 验证通过，继续
		})
	}
}
```

---

## 9. 中间件组合：Chain

`Chain` 函数可以将多个中间件打包成一个，便于复用：

```go
// 定义一组通用中间件，封装成可复用的单元
commonMiddlewares := httpServer.Chain(
	recovery.Middleware(),
	requestid.Middleware(),
	logging.Middleware(),
)

// 应用到服务器
srv.Use(commonMiddlewares)
```

**使用场景**：当多个服务器（或路由组）需要共享相同的中间件集合时，用 `Chain` 避免重复代码。

`Chain` 的实现逻辑（从后往前包裹）：

```go
func Chain(middlewares ...Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			next = middlewares[i](next)
		}
		return next
	}
}
```

---

## 10. HTTPS / TLS 配置

### 10.1 使用证书文件

```go
srv := httpServer.NewServer(":8443",
	httpServer.WithDriver(std.NewDriver()),
	// 从证书文件和私钥文件加载
	httpServer.WithTLS("cert.pem", "key.pem"),
)
```

### 10.2 使用自定义 TLS 配置

```go
tlsConfig := &tls.Config{
	MinVersion: tls.VersionTLS12,
	// 其他自定义配置...
}

srv := httpServer.NewServer(":8443",
	httpServer.WithDriver(std.NewDriver()),
	httpServer.WithTLSConfig(tlsConfig),
)
```

### 10.3 Endpoint 自动识别协议

`Endpoint()` 方法会根据是否配置了 TLS 自动返回 `https://` 或 `http://`：

```go
srv := httpServer.NewServer(":8443",
	httpServer.WithDriver(std.NewDriver()),
	httpServer.WithTLS("cert.pem", "key.pem"),
)
fmt.Println(srv.Endpoint())
// 输出: https://localhost:8443
```

---

## 11. 生产级完整示例

下面这个示例整合了前面学到的所有内容，是一个接近生产环境的服务：

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	// 注册编解码器（副作用导入）
	_ "github.com/tx7do/go-wind-plugins/encoding/json"
	_ "github.com/tx7do/go-wind-plugins/encoding/xml"

	// 限流器
	"github.com/tx7do/go-wind-plugins/ratelimit/tokenbucket"

	httpServer "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/driver/gin" // 生产环境用 gin
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/codec"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/cors"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/logging"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/ratelimit"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/recovery"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/requestid"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/timeout"
)

// 请求/响应结构体
type greetRequest struct {
	Name string `json:"name" xml:"name"`
}

type greetResponse struct {
	Message   string `json:"message" xml:"message"`
	RequestID string `json:"request_id,omitempty"`
}

func main() {
	// 创建限流器：100 QPS，突发 200
	limiter, _ := tokenbucket.New(100, 200)

	srv := httpServer.NewServer(":8080",
		httpServer.WithDriver(gin.NewDriver()),
	)

	// 中间件链（顺序很重要！）
	srv.Use(
		recovery.Middleware(),                              // 1. 最外层：捕获 panic
		requestid.Middleware(),                             // 2. 生成请求 ID
		logging.Middleware(                                 // 3. 访问日志
			logging.WithSkipPaths("/healthz"),
		),
		cors.Middleware(                                    // 4. 跨域
			cors.WithAllowedOrigins("https://app.example.com"),
			cors.WithAllowCredentials(true),
		),
		timeout.Middleware(30*time.Second),                 // 5. 超时控制
		ratelimit.Middleware(limiter),                      // 6. 限流
		codec.Middleware(),                                 // 7. 内容协商（最内层）
	)

	// ---- 路由 ----

	// 健康检查
	srv.GET("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	// 业务接口：使用 codec 自动编解码
	srv.POST("/greet", func(w http.ResponseWriter, r *http.Request) {
		var req greetRequest
		if err := codec.ReadBody(r, &req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		codec.Respond(w, r, http.StatusOK, &greetResponse{
			Message:   "Hello, " + req.Name + "!",
			RequestID: requestid.FromContext(r.Context()),
		})
	})

	// 优雅关闭
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("HTTP server listening on %s\n", srv.Endpoint())
	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("server stopped gracefully")
}
```

---

## 12. 常见问题

### Q1: 启动时报 "no driver set" 怎么办？

**原因**：创建 Server 时忘记通过 `WithDriver()` 指定驱动。

**解决**：

```go
// ❌ 错误：缺少驱动
srv := httpServer.NewServer(":8080")

// ✅ 正确：指定驱动
srv := httpServer.NewServer(":8080", httpServer.WithDriver(std.NewDriver()))
```

### Q2: 中间件没生效？

**检查清单**：
1. `Use()` 必须在 `GET/POST/Handle` **之前**调用
2. 确认中间件包已正确 import
3. 检查中间件顺序是否合理（如 recovery 必须在最外层）

### Q3: 如何获取实际监听端口（用于 `:0` 随机端口）？

```go
srv := httpServer.NewServer(":0", httpServer.WithDriver(std.NewDriver()))
// 启动后
fmt.Println(srv.Endpoint()) // 输出实际端口，如 http://localhost:54321
```

`Endpoint()` 在服务器启动后会返回实际绑定的地址。

### Q4: 如何只对部分路由应用中间件？

目前 `Use()` 注册的是全局中间件。如果需要路由级中间件，有两种方案：

**方案一**：在 handler 内部手动调用中间件逻辑

```go
authMw := authn.Middleware(authenticator)
srv.GET("/api/private", func(w http.ResponseWriter, r *http.Request) {
	authMw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 实际业务逻辑
	})).ServeHTTP(w, r)
})
```

**方案二**：使用支持路由分组的驱动（如 chi/gin），在驱动层面配置分组中间件。

### Q5: std 驱动和 gin 驱动的主要区别？

| 方面 | std | gin |
|------|-----|-----|
| 路由匹配 | 仅按 path，方法需手动校验 | 原生支持 method + path |
| 路径参数 | 不支持 | 支持 `/:id` |
| 性能 | 一般 | 优秀 |
| 依赖 | 仅标准库 | 引入 gin 框架 |
| 适用场景 | 学习、简单服务 | 生产环境 |

---

## 附录：API 速查表

### Server 方法

| 方法 | 说明 |
|------|------|
| `NewServer(addr, opts...)` | 创建服务器 |
| `Use(mws...)` | 注册全局中间件 |
| `Handle(method, path, handler)` | 注册路由（通用） |
| `GET / POST / PUT / DELETE / PATCH / HEAD / OPTIONS` | 注册对应方法的路由 |
| `Start(ctx)` | 启动服务器（阻塞） |
| `Stop(ctx)` | 停止服务器 |
| `Endpoint()` | 获取访问地址 |
| `Addr()` | 获取监听地址 |

### Option 选项

| 选项 | 说明 |
|------|------|
| `WithDriver(d)` | 设置驱动（必需） |
| `WithMiddleware(mws...)` | 创建时设置中间件 |
| `WithTLSConfig(c)` | 设置 TLS 配置 |
| `WithTLS(cert, key)` | 从文件加载 TLS |

### 顶层函数

| 函数 | 说明 |
|------|------|
| `Chain(mws...)` | 将多个中间件组合为一个 |

---

> **下一步学习**：尝试运行 `_examples/http-basic` 和 `_examples/http-middleware` 目录下的示例代码，动手实践是掌握这些概念的最佳方式！

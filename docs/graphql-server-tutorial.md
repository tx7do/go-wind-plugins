# Go-Wind GraphQL 服务器从入门到精通

> 本教程面向初学者，基于 `transport/graphql/server.go`，循序渐进地讲解如何使用 Go-Wind 插件库搭建 GraphQL 服务器——从最简单的 Schema 定义和 Resolver 实现开始，逐步深入到中间件机制和 Playground 配置，最终构建一个生产级的 GraphQL API 服务。

---

## 目录

- [1. 简介](#1-简介)
- [2. 前置知识：GraphQL 基础](#2-前置知识graphql-基础)
- [3. 核心概念](#3-核心概念)
- [4. 快速开始：第一个 GraphQL 服务](#4-快速开始第一个-graphql-服务)
- [5. Schema 定义与代码生成](#5-schema-定义与代码生成)
- [6. 路由注册：Handle 与 HandleFunc](#6-路由注册handle-与-handlefunc)
- [7. 中间件入门](#7-中间件入门)
- [8. 内置中间件详解](#8-内置中间件详解)
  - [8.1 recovery —— 异常恢复](#81-recovery--异常恢复)
  - [8.2 requestid —— 请求追踪](#82-requestid--请求追踪)
  - [8.3 logging —— 访问日志](#83-logging--访问日志)
  - [8.4 cors —— 跨域资源共享](#84-cors--跨域资源共享)
  - [8.5 authn —— 身份认证](#85-authn--身份认证)
  - [8.6 ratelimit —— 限流](#86-ratelimit--限流)
- [9. 编写自定义中间件](#9-编写自定义中间件)
- [10. Playground：交互式 GraphQL IDE](#10-playground交互式-graphql-ide)
- [11. HTTPS / TLS 配置](#11-https--tls-配置)
- [12. 生产级完整示例](#12-生产级完整示例)
- [13. 常见问题](#13-常见问题)

---

## 1. 简介

`transport/graphql` 是 Go-Wind 插件库提供的 **GraphQL 服务器封装层**。它在 [gqlgen](https://gqlgen.com/) 的基础上，提供了：

| 特性 | 说明 |
|------|------|
| **基于 gqlgen** | 底层使用 gqlgen 代码生成，类型安全，Schema 优先开发 |
| **共享 HTTP 中间件** | 通过类型别名设计，所有 `transport/http/middleware` 中间件直接可用，无需适配器 |
| **优雅关闭** | 内置基于 `context.Context` 的优雅停机支持 |
| **TLS 友好** | 自动处理 HTTPS 监听器，自动推断 `http/https` scheme |
| **Playground 集成** | 一行代码注册 GraphQL Playground 交互式 IDE |

如果你用过 Go 标准库的 `net/http`，那么上手会非常快——因为 GraphQL 基于 HTTP 运行，服务器的中间件系统与你熟悉的 HTTP 中间件完全一致。

---

## 2. 前置知识：GraphQL 基础

在写代码之前，先了解几个 GraphQL 核心概念。

### 2.1 什么是 GraphQL？

GraphQL 是 Facebook 开源的查询语言，客户端可以精确指定需要哪些字段，避免过度获取或不足获取数据。

```
客户端 (GraphQL Client)  ──── HTTP POST + GraphQL Query ────►  服务端 (GraphQL Server)
                              ◄─── JSON Response ──────────────
```

与 REST 不同，GraphQL 只需要 **一个端点**（通常是 `/query`），所有操作都通过这个端点完成。

### 2.2 Query、Mutation 与 Subscription

GraphQL 支持三种操作类型：

| 操作 | 说明 | HTTP 类比 |
|------|------|-----------|
| **Query** | 读取数据 | `GET` 请求 |
| **Mutation** | 修改数据 | `POST` / `PUT` / `DELETE` |
| **Subscription** | 实时推送（基于 WebSocket） | WebSocket / SSE |

> 本教程主要聚焦于 **Query 和 Mutation**，它们覆盖了绝大多数使用场景。

### 2.3 GraphQL vs REST

| 对比维度 | REST | GraphQL |
|----------|------|---------|
| 端点 | 多个 URL（`/users`、`/posts`...） | 单一端点（`/query`） |
| 数据获取 | 服务器决定返回哪些字段 | 客户端精确指定所需字段 |
| 版本管理 | URL 版本（`/v1/`、`/v2/`） | Schema 演进（废弃字段而非版本） |
| 过度获取 | 常见（返回不需要的字段） | 消除（客户端按需选择字段） |
| 多次请求 | 获取关联数据需要多次请求 | 一次查询即可获取关联数据 |

### 2.4 你需要准备的工具

```bash
# 安装 gqlgen CLI（用于代码生成）
go install github.com/99designs/gqlgen@latest
```

---

## 3. 核心概念

在写第一行代码前，先理解三个核心概念。

### 3.1 Server（服务器）

`Server` 是整个封装的入口。它内部持有：

| 字段 | 类型 | 作用 |
|------|------|------|
| `addr` | `string` | 监听地址（如 `:8080`） |
| `tlsConfig` | `*tls.Config` | TLS 配置（可选） |
| `listener` | `net.Listener` | 网络监听器（Start 后创建） |
| `mux` | `*http.ServeMux` | HTTP 路由复用器 |
| `server` | `*http.Server` | 底层 HTTP 服务器（Start 后创建） |
| `middlewares` | `[]Middleware` | 中间件链 |

它实现了 `transport.Server` 接口，提供 `Start / Stop / Endpoint` 等方法。

### 3.2 Middleware —— 类型别名的魔力

这是 GraphQL 服务器设计中**最关键的一点**。

`transport/graphql` 定义中间件类型时，使用的是**类型别名**（注意 `=` 号），而非命名类型：

```go
// transport/graphql/server.go
type Middleware = func(http.Handler) http.Handler
```

而 `transport/http` 中定义的是命名类型：

```go
// transport/http/server.go
type Middleware func(http.Handler) http.Handler
```

虽然一个是别名、一个是命名类型，但它们的**底层类型完全相同**——都是 `func(http.Handler) http.Handler`。Go 语言允许底层类型相同的值直接赋值，所以：

```go
// recovery.Middleware() 返回 httpPlugin.Middleware（即 transport/http.Middleware）
// 但可以直接传给 graphql.Server.Use()，无需任何类型转换！
srv.Use(recovery.Middleware())   // ✅ 直接传入
srv.Use(logging.Middleware())    // ✅ 直接传入
srv.Use(requestid.Middleware())  // ✅ 直接传入
```

中间件流向示意：

```
┌─────────────────────────────────────────────────────────┐
│  transport/http/middleware/recovery                      │
│  transport/http/middleware/logging                       │
│  transport/http/middleware/requestid                     │
│  transport/http/middleware/cors                           │
│  ...（全部 17 个中间件）                                   │
│         │                                                │
│         │  返回类型: httpPlugin.Middleware               │
│         │  底层类型: func(http.Handler) http.Handler     │
│         ▼                                                │
│  transport/graphql Server.Use(Middleware)                │
│  Middleware = func(http.Handler) http.Handler (别名)     │
│  ✅ 无需适配器，直接传入                                   │
└─────────────────────────────────────────────────────────┘
```

> **关键点**：你为 HTTP 服务器写的所有中间件，GraphQL 服务器都能直接用。不需要导入额外的包，不需要写适配器，不需要类型转换。

### 3.3 gqlgen ExecutableSchema

gqlgen 的核心类型是 `graphql.ExecutableSchema`——一个可执行的 GraphQL Schema。gqlgen 的代码生成器会根据你的 `.graphql` 文件自动生成这个接口的实现。

```go
// gqlgen 生成的代码中会提供这个函数
func NewExecutableSchema(cfg Config) graphql.ExecutableSchema
```

你只需要提供 **Resolver**（解析器）实现，gqlgen 负责处理查询解析、字段解析调度、类型校验等全部底层逻辑。

---

## 4. 快速开始：第一个 GraphQL 服务

### 4.1 定义 Schema

创建 `schema.graphql` 文件：

```graphql
type Hygrothermograph {
    humidity: Float!
    temperature: Float!
}

type Query {
    hygrothermograph: Hygrothermograph!
}
```

这个 Schema 定义了一个查询 `hygrothermograph`，返回一个包含湿度和温度的对象。`!` 表示该字段不可为 null。

### 4.2 生成代码

创建 `gqlgen.yml` 配置文件：

```yaml
schema: schema.graphql
```

执行代码生成：

```bash
# 方式一：直接运行（推荐）
go run github.com/99designs/gqlgen generate

# 方式二：使用安装的 CLI
gqlgen generate
```

生成后会得到两个文件：

| 文件 | 内容 |
|------|------|
| `generated.go` | 执行引擎、`NewExecutableSchema`、`ResolverRoot` 和 `QueryResolver` 接口 |
| `models_gen.go` | GraphQL 类型对应的 Go 结构体 |

生成的关键接口：

```go
// ResolverRoot 是根解析器接口，你的 Resolver 必须实现它
type ResolverRoot interface {
    Query() QueryResolver
}

// QueryResolver 定义了 Query 类型下的所有字段解析方法
type QueryResolver interface {
    Hygrothermograph(ctx context.Context) (*Hygrothermograph, error)
}
```

生成的模型类型：

```go
type Hygrothermograph struct {
    Humidity    float64 `json:"humidity"`
    Temperature float64 `json:"temperature"`
}
```

### 4.3 实现 Resolver 并启动

```go
package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"syscall"

	graphqlAPI "your-module/graph/generated"               // gqlgen 生成的代码
	graphqlServer "github.com/tx7do/go-wind-plugins/transport/graphql" // GraphQL 服务器

	"github.com/99designs/gqlgen/graphql/playground"
)

// resolver 实现 gqlgen 生成的 ResolverRoot 和 QueryResolver 接口。
type resolver struct{}

func (r *resolver) Query() graphqlAPI.QueryResolver { return r }

func (r *resolver) Hygrothermograph(_ context.Context) (*graphqlAPI.Hygrothermograph, error) {
	return &graphqlAPI.Hygrothermograph{
		Humidity:    float64(rand.Intn(100)),
		Temperature: float64(rand.Intn(40)),
	}, nil
}

func main() {
	// 1. 创建 GraphQL 服务器，监听 :8080
	srv := graphqlServer.NewServer(":8080")

	// 2. 注册 GraphQL Schema
	schema := graphqlAPI.NewExecutableSchema(graphqlAPI.Config{
		Resolvers: &resolver{},
	})
	srv.Handle("/query", schema)

	// 3. 注册 Playground（交互式 IDE）
	srv.HandleFunc("/", playground.Handler("GraphQL Playground", "/query"))

	// 4. 优雅关闭
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("GraphQL server listening on %s\n", srv.Endpoint())
	fmt.Println("  Playground: http://localhost:8080/")
	fmt.Println("  Endpoint:   http://localhost:8080/query")

	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("server stopped")
}
```

### 4.4 运行与测试

```bash
# 运行
go run .

# 另开一个终端测试
curl -X POST http://localhost:8080/query \
  -H "Content-Type: application/json" \
  -d '{"query":"{ hygrothermograph { humidity temperature } }"}'

# 输出示例:
# {"data":{"hygrothermograph":{"humidity":42,"temperature":23}}}

# 或者直接在浏览器打开 http://localhost:8080/ 使用 Playground
```

### 4.5 代码解读

| 代码 | 作用 |
|------|------|
| `NewServer(":8080")` | 创建服务器，`:8080` 是监听地址 |
| `NewExecutableSchema(cfg)` | 从生成的代码创建可执行 Schema |
| `srv.Handle("/query", schema)` | 将 GraphQL 端点注册到 `/query` 路径 |
| `srv.HandleFunc("/", playground.Handler(...))` | 在根路径注册 Playground IDE |
| `signal.NotifyContext(...)` | 捕获 Ctrl+C / kill 信号 |
| `srv.Start(ctx)` | 启动服务器并阻塞，当 `ctx` 被取消时执行优雅关闭 |
| `srv.Endpoint()` | 返回实际访问地址（如 `http://localhost:8080`） |

> **初学者提示**：`Start` 会阻塞当前 goroutine，直到收到关闭信号。所以把它放在 `main` 函数末尾即可。

---

## 5. Schema 定义与代码生成

### 5.1 Schema 语法基础

GraphQL Schema 使用 SDL（Schema Definition Language）编写。核心语法元素：

| 语法 | 含义 | 示例 |
|------|------|------|
| `type` | 定义对象类型 | `type User { ... }` |
| `!` | 非空标记 | `name: String!` |
| `[Type]` | 列表 | `tags: [String]` |
| `[Type!]!` | 非空列表，元素也非空 | `users: [User!]!` |
| `input` | 定义输入类型（用于 Mutation 参数） | `input CreateUserInput { ... }` |

内置标量类型：

| 标量 | Go 对应类型 | 说明 |
|------|------------|------|
| `String` | `string` | UTF-8 字符串 |
| `Int` | `int32` | 32 位整数 |
| `Float` | `float64` | 双精度浮点数 |
| `Boolean` | `bool` | 布尔值 |
| `ID` | `string` | 唯一标识符 |

### 5.2 定义 Query 和 Mutation

下面是一个更完整的 Todo 应用 Schema：

```graphql
type Todo {
    id: ID!
    text: String!
    done: Boolean!
}

type Query {
    todos: [Todo!]!
    todo(id: ID!): Todo
}

type Mutation {
    createTodo(text: String!): Todo!
    toggleTodo(id: ID!): Todo!
}
```

### 5.3 gqlgen.yml 配置

最简配置只需指定 schema 文件：

```yaml
schema: schema.graphql
```

如果需要自定义模型映射或绑定 Go 包：

```yaml
schema: schema.graphql

# 自定义标量
models:
  DateTime:
    model: github.com/99designs/gqlgen/graphql.Time
  # 自定义类型映射
  Todo:
    model: myapp/graph/model.Todo
```

### 5.4 代码生成详解

运行 `gqlgen generate` 后，会生成以下接口（以 Todo Schema 为例）：

```go
// ResolverRoot —— 根解析器，必须实现
type ResolverRoot interface {
    Query() QueryResolver
    Mutation() MutationResolver
}

// QueryResolver —— Query 操作的字段解析
type QueryResolver interface {
    Todos(ctx context.Context) ([]Todo, error)
    Todo(ctx context.Context, id string) (*Todo, error)
}

// MutationResolver —— Mutation 操作的字段解析
type MutationResolver interface {
    CreateTodo(ctx context.Context, text string) (*Todo, error)
    ToggleTodo(ctx context.Context, id string) (*Todo, error)
}
```

> **注意**：不要手动编辑 `generated.go` 和 `models_gen.go`，每次修改 Schema 后重新运行 `gqlgen generate` 即可。

### 5.5 实现 Resolver

实现模式非常固定：实现 `ResolverRoot` 接口，返回子 Resolver（通常就是自身）：

```go
type resolver struct {
	mu    sync.Mutex
	todos map[string]*graphqlAPI.Todo
}

// Query 返回 QueryResolver
func (r *resolver) Query() graphqlAPI.QueryResolver { return r }

// Mutation 返回 MutationResolver
func (r *resolver) Mutation() graphqlAPI.MutationResolver { return r }

// --- QueryResolver 方法 ---

func (r *resolver) Todos(ctx context.Context) ([]*graphqlAPI.Todo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := make([]*graphqlAPI.Todo, 0, len(r.todos))
	for _, t := range r.todos {
		list = append(list, t)
	}
	return list, nil
}

func (r *resolver) Todo(ctx context.Context, id string) (*graphqlAPI.Todo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.todos[id]
	if !ok {
		return nil, fmt.Errorf("todo not found: %s", id)
	}
	return t, nil
}

// --- MutationResolver 方法 ---

func (r *resolver) CreateTodo(ctx context.Context, text string) (*graphqlAPI.Todo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	todo := &graphqlAPI.Todo{
		ID:   uuid.NewString(),
		Text: text,
		Done: false,
	}
	r.todos[todo.ID] = todo
	return todo, nil
}

func (r *resolver) ToggleTodo(ctx context.Context, id string) (*graphqlAPI.Todo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.todos[id]
	if !ok {
		return nil, fmt.Errorf("todo not found: %s", id)
	}
	t.Done = !t.Done
	return t, nil
}
```

---

## 6. 路由注册：Handle 与 HandleFunc

GraphQL 服务器提供两个路由注册方法。

### 6.1 Handle —— 注册 GraphQL Schema

```go
func (s *Server) Handle(path string, es graphql.ExecutableSchema)
```

`Handle` 接收一个 gqlgen `ExecutableSchema`，内部用 `handler.New(es)` 包装后注册到 HTTP 路由。

```go
schema := graphqlAPI.NewExecutableSchema(graphqlAPI.Config{
    Resolvers: &resolver{},
})
srv.Handle("/query", schema)  // GraphQL 端点在 /query
```

> **注意**：`Handle` 接收的是 gqlgen 的 `graphql.ExecutableSchema` 类型，不是 `http.Handler`。这是它与 `HandleFunc` 的关键区别。

### 6.2 HandleFunc —— 注册普通 HTTP 处理器

```go
func (s *Server) HandleFunc(path string, h http.HandlerFunc)
```

`HandleFunc` 用于注册普通 HTTP 处理器，典型场景包括：

```go
// Playground 交互式 IDE
srv.HandleFunc("/", playground.Handler("GraphQL Playground", "/query"))

// 健康检查
srv.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    fmt.Fprintln(w, "ok")
})
```

### 6.3 多端点注册

一个服务器可以注册多个路由，GraphQL 和普通 HTTP 端点共存：

```go
// GraphQL 端点
srv.Handle("/query", mainSchema)

// 另一个 GraphQL Schema（如管理后台 API）
srv.Handle("/admin/query", adminSchema)

// REST 风格的 Webhook
srv.HandleFunc("/webhook/payment", webhookHandler)

// 健康检查
srv.HandleFunc("/healthz", healthHandler)
```

---

## 7. 中间件入门

### 7.1 什么是中间件？

中间件是一种"洋葱模型"——请求从外向内穿过每一层，响应从内向外返回：

```
请求 ──► recovery ──► requestid ──► logging ──► GraphQL Handler
                                                          │
响应 ◄── recovery ◄── requestid ◄── logging ◄────────────┘
```

每一层中间件都可以：
- 在调用 `next` **之前** 执行逻辑（如鉴权、限流）
- 在调用 `next` **之后** 执行逻辑（如记录响应时间）
- 决定 **是否** 调用 `next`（如鉴权失败时直接返回 401）

### 7.2 注册中间件的两种方式

**方式一：`Use` 方法（推荐，更直观）**

```go
srv := graphqlServer.NewServer(":8080")

// 这些是 transport/http/middleware 下的中间件，直接可用！
srv.Use(
	recovery.Middleware(),   // 捕获 panic
	requestid.Middleware(),  // 生成请求 ID
	logging.Middleware(),    // 访问日志
)

// 然后注册路由
srv.Handle("/query", schema)
```

**方式二：`WithMiddleware` 选项（创建时传入）**

```go
srv := graphqlServer.NewServer(":8080",
	graphqlServer.WithMiddleware(
		recovery.Middleware(),
		requestid.Middleware(),
		logging.Middleware(),
	),
)
```

两种方式效果相同，`Use` 更灵活（可在创建后按需添加）。

> **重要提示**：这些中间件包的 import 路径是 `github.com/tx7do/go-wind-plugins/transport/http/middleware/*`——没错，它们来自 HTTP 中间件包。得益于类型别名设计，它们可以直接用于 GraphQL 服务器。

### 7.3 中间件顺序很重要！

中间件按 **注册顺序** 执行：**先注册的最外层**（最先被调用）。

```go
// 推荐顺序
srv.Use(
	recovery.Middleware(),   // 1. 最外层：捕获所有 panic
	requestid.Middleware(),  // 2. 生成请求 ID
	logging.Middleware(),    // 3. 记录日志（能拿到请求 ID）
	cors.Middleware(...),    // 4. 跨域处理（浏览器请求）
	authn.Middleware(auth),  // 5. 身份认证（最内层，贴近业务）
)
```

> **经验法则**：`recovery` 永远放最外层，`authn` 这类贴近业务的放最内层。

### 7.4 中间件的工作原理

在 `Start()` 方法中，中间件链会包裹整个 HTTP 路由：

```go
// server.go 中 Start() 的中间件链应用逻辑（简化版）
h := http.Handler(s.mux)                       // 路由器作为最内层
for i := len(s.middlewares) - 1; i >= 0; i-- { // 从后往前包裹
	h = s.middlewares[i](h)                     // 每个中间件包裹前一层
}
s.server = &http.Server{Handler: h}             // 最终的处理器
```

从后往前遍历意味着：**第一个注册的中间件变成最外层**（最先处理请求，最后处理响应）。

理解这段代码，你就理解了中间件机制的核心。

---

## 8. 内置中间件详解

Go-Wind 在 `transport/http/middleware/` 下提供了 17 个开箱即用的中间件，**全部兼容 GraphQL 服务器**：

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

下面详细介绍最常用的 6 个（对于 GraphQL 服务器而言）。其他中间件的用法请参考 [HTTP 服务器教程](./http-server-tutorial.md)。

---

### 8.1 recovery —— 异常恢复

**作用**：捕获 handler 中的 panic，记录日志并返回 500，防止服务器崩溃。

**应该始终放在中间件链的最外层。**

```go
import "github.com/tx7do/go-wind-plugins/transport/http/middleware/recovery"

// 基础用法
srv.Use(recovery.Middleware())

// 高级用法：自定义选项
srv.Use(recovery.Middleware(
	recovery.WithStackTrace(true),  // 记录堆栈信息（默认开启）
))
```

**工作流程**：

```go
// recovery 中间件的内部逻辑（简化）
return func(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rvr := recover(); rvr != nil {
				// 记录错误日志
				http.Error(w, "Internal Server Error", 500)
			}
		}()
		next.ServeHTTP(w, r) // 如果这里 panic，会被上面的 defer 捕获
	})
}
```

> **GraphQL 特有提示**：如果 Resolver 中发生 panic，gqlgen 自身有一层 panic 恢复，会返回 GraphQL 格式的错误。但 `recovery` 中间件提供了额外的安全网——即使在 gqlgen 处理之外（如 Playground 页面）发生 panic，也不会导致服务器崩溃。

---

### 8.2 requestid —— 请求追踪

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

**在 Resolver 中获取请求 ID**：

HTTP 中间件注入的 context 会通过 gqlgen 传递到 Resolver 方法，因此可以直接获取：

```go
func (r *resolver) Hygrothermograph(ctx context.Context) (*graphqlAPI.Hygrothermograph, error) {
	id := requestid.FromContext(ctx) // 从 context 中获取请求 ID
	fmt.Printf("handling request: %s\n", id)
	// ...
}
```

**行为说明**：
- 如果请求头带了 `X-Request-ID`，则使用该值（方便上游服务传递）
- 如果没有，则自动生成一个随机 ID
- 响应头中也会回写这个 ID，客户端可以看到

---

### 8.3 logging —— 访问日志

**作用**：记录每个请求的方法、路径、状态码、响应大小、耗时和客户端地址。

```go
import "github.com/tx7do/go-wind-plugins/transport/http/middleware/logging"

// 基础用法
srv.Use(logging.Middleware())

// 跳过特定路径的日志（避免 Playground 刷屏）
srv.Use(logging.Middleware(
	logging.WithSkipPaths("/", "/healthz"),
))
```

**日志输出示例**：

```
http request  method=POST path=/query status=200 size=128 latency_ms=3 remote=127.0.0.1:54321
```

**智能日志级别**：
- 状态码 `>= 500` → `Error` 级别
- 状态码 `>= 400` → `Warn` 级别
- 其他 → `Info` 级别

> **GraphQL 提示**：建议使用 `WithSkipPaths("/")` 跳过 Playground 路径，否则在浏览器中打开 Playground 会产生大量日志。

---

### 8.4 cors —— 跨域资源共享

**作用**：处理浏览器的跨域请求，自动设置 `Access-Control-*` 响应头。

GraphQL API 几乎总是被前端应用（React、Vue 等）调用，因此 CORS 配置尤为重要。

```go
import "github.com/tx7do/go-wind-plugins/transport/http/middleware/cors"

srv.Use(cors.Middleware(
	cors.WithAllowedOrigins("https://app.example.com"),  // 允许的前端域名
	cors.WithAllowedMethods("GET", "POST"),               // GraphQL 只需要 POST
	cors.WithAllowedHeaders("Authorization", "Content-Type"), // Content-Type 是必须的
	cors.WithAllowCredentials(true),                      // 允许携带 Cookie
	cors.WithMaxAge(3600),                                // 预检缓存 1 小时
))
```

**关键选项说明**：

| 选项 | 默认值 | 说明 |
|------|--------|------|
| `WithAllowedOrigins` | 空（允许所有 `*`） | 指定允许的源 |
| `WithAllowedMethods` | 常见 7 种方法 | 允许的 HTTP 方法 |
| `WithAllowedHeaders` | 基础头 | 允许的请求头 |
| `WithAllowCredentials` | `false` | 是否允许带 Cookie |
| `WithMaxAge` | 0（不缓存） | 预检结果缓存秒数 |

> **GraphQL 必须配置**：GraphQL 请求使用 `Content-Type: application/json`，确保在 `WithAllowedHeaders` 中包含它，否则浏览器的 CORS 预检会失败。

---

### 8.5 authn —— 身份认证

**作用**：通过认证引擎（如 JWT）验证请求的身份，将认证信息注入 context。

```go
import (
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/authn"
	jwtAuthn "github.com/tx7do/go-wind-plugins/security/authn/jwt"
)

// 创建 JWT 认证器
authenticator, _ := jwtAuthn.NewAuthenticator(
	jwtAuthn.WithKey([]byte("your-secret-key")),
)

// 注册中间件
srv.Use(authn.Middleware(authenticator))
```

**在 Resolver 中获取认证信息**：

```go
func (r *resolver) MyData(ctx context.Context) (*Data, error) {
	// 从 context 中获取认证声明（Claims）
	// 具体方法取决于你使用的 authn 引擎
	// ...
}
```

> **Playground 提示**：如果你为 GraphQL 端点启用了认证，Playground 也需要在 "HTTP Headers" 标签页中配置 `Authorization` 头才能正常工作。详见 [第 10 章](#10-playground交互式-graphql-ide)。

---

### 8.6 ratelimit —— 限流

**作用**：限制请求速率，保护服务器免受恶意请求或突发流量冲击。

```go
import (
	"github.com/tx7do/go-wind-plugins/ratelimit/tokenbucket"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/ratelimit"
)

// 创建令牌桶限流器：100 QPS，突发容量 200
limiter, _ := tokenbucket.New(100, 200)

srv.Use(ratelimit.Middleware(limiter))
```

> **GraphQL 提示**：HTTP 层面的限流是最基础的防线。GraphQL 还有一个独特的挑战——单个查询可能非常复杂（深层嵌套、大量字段），因此生产环境建议同时配合 gqlgen 的查询复杂度限制（`ComplexityRoot`）使用。

---

## 9. 编写自定义中间件

由于 GraphQL 中间件类型与 HTTP 完全一致，编写方式完全相同。

### 9.1 最简单的中间件

```go
// 添加自定义响应头
srv.Use(func(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Powered-By", "Go-Wind-GraphQL")
		next.ServeHTTP(w, r)
	})
})
```

### 9.2 带配置的中间件

使用函数选项模式，让中间件可配置：

```go
// 定义配置
type headerMiddlewareConfig struct {
	headers map[string]string
}

type HeaderOption func(*headerMiddlewareConfig)

func WithHeader(key, value string) HeaderOption {
	return func(c *headerMiddlewareConfig) {
		c.headers[key] = value
	}
}

// 创建中间件
func HeaderMiddleware(opts ...HeaderOption) func(http.Handler) http.Handler {
	cfg := &headerMiddlewareConfig{headers: map[string]string{}}
	for _, opt := range opts {
		opt(cfg)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for k, v := range cfg.headers {
				w.Header().Set(k, v)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// 使用
srv.Use(HeaderMiddleware(
	WithHeader("X-API-Version", "2.0"),
	WithHeader("X-Server", "my-graphql"),
))
```

### 9.3 短路中间件

短路中间件不调用 `next`，直接返回响应（如 API Key 验证）：

```go
func APIKeyMiddleware(validKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 放行 Playground 和健康检查
			if r.URL.Path == "/" || r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}
			key := r.Header.Get("X-API-Key")
			if key != validKey {
				http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
				return // 不调用 next，短路返回
			}
			next.ServeHTTP(w, r)
		})
	}
}

// 使用
srv.Use(APIKeyMiddleware("my-secret-api-key"))
```

---

## 10. Playground：交互式 GraphQL IDE

### 10.1 什么是 Playground？

[GraphQL Playground](https://github.com/prisma-labs/graphql-playground) 是一个浏览器端的交互式 GraphQL IDE，提供：

- 语法高亮和自动补全
- Schema 文档浏览
- 查询历史记录
- 多标签页编辑
- 请求头配置

### 10.2 注册 Playground

一行代码即可启用：

```go
import "github.com/99designs/gqlgen/graphql/playground"

srv.HandleFunc("/", playground.Handler("GraphQL Playground", "/query"))
```

两个参数的含义：
- `"GraphQL Playground"` —— 浏览器标签页的标题
- `"/query"` —— Playground 发送 GraphQL 请求的目标端点 URL

### 10.3 在 Playground 中测试 Query

启动服务器后，在浏览器打开 `http://localhost:8080/`，在左侧编辑器输入：

```graphql
query {
    hygrothermograph {
        humidity
        temperature
    }
}
```

点击播放按钮（或按 `Ctrl+Enter`），右侧会显示返回结果：

```json
{
  "data": {
    "hygrothermograph": {
      "humidity": 42,
      "temperature": 23
    }
  }
}
```

### 10.4 设置请求头（认证）

如果 API 需要认证，点击左下角的 **"HTTP HEADERS"** 标签页，输入 JSON 格式的请求头：

```json
{
  "Authorization": "Bearer your-jwt-token",
  "X-API-Key": "your-api-key"
}
```

Playground 会在每次请求中自动附带这些头。

### 10.5 生产环境中的 Playground

| 环境 | 建议 |
|------|------|
| **开发环境** | 启用，方便调试 |
| **测试环境** | 启用，供 QA 测试 |
| **生产环境** | 禁用或放在认证中间件之后 |

生产环境禁用 Playground 的方式：

```go
if os.Getenv("APP_ENV") != "production" {
	srv.HandleFunc("/", playground.Handler("GraphQL Playground", "/query"))
}
```

---

## 11. HTTPS / TLS 配置

### 11.1 使用证书文件

```go
srv := graphqlServer.NewServer(":8443",
	graphqlServer.WithTLS("cert.pem", "key.pem"),
)
```

`WithTLS` 内部调用 `tls.LoadX509KeyPair` 加载证书，如果文件不存在或格式错误会 **panic**。

### 11.2 使用自定义 TLS 配置

适合需要精细控制 TLS 参数的场景：

```go
tlsConfig := &tls.Config{
	MinVersion: tls.VersionTLS12,
	// 其他自定义配置...
}

srv := graphqlServer.NewServer(":8443",
	graphqlServer.WithTLSConfig(tlsConfig),
)
```

### 11.3 Endpoint 自动识别协议

`Endpoint()` 方法会根据是否配置了 TLS 自动返回 `https://` 或 `http://`：

```go
srv := graphqlServer.NewServer(":8443",
	graphqlServer.WithTLS("cert.pem", "key.pem"),
)
fmt.Println(srv.Endpoint())
// 输出: https://localhost:8443
```

---

## 12. 生产级完整示例

下面这个示例整合了前面学到的所有内容，是一个接近生产环境的 GraphQL 服务：

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/99designs/gqlgen/graphql/playground"

	// Go-Wind: GraphQL 服务器
	graphqlServer "github.com/tx7do/go-wind-plugins/transport/graphql"

	// Go-Wind: HTTP 中间件（共享使用）
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/cors"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/logging"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/ratelimit"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/recovery"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/requestid"

	// Go-Wind: 限流器
	"github.com/tx7do/go-wind-plugins/ratelimit/tokenbucket"

	// gqlgen 生成的代码
	graphqlAPI "yourapp/graph/generated"
)

// ---------- Resolver ----------

type resolver struct {
	mu    sync.Mutex
	todos map[string]*graphqlAPI.Todo
}

func (r *resolver) Query() graphqlAPI.QueryResolver       { return r }
func (r *resolver) Mutation() graphqlAPI.MutationResolver { return r }

func (r *resolver) Todos(ctx context.Context) ([]*graphqlAPI.Todo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := make([]*graphqlAPI.Todo, 0, len(r.todos))
	for _, t := range r.todos {
		list = append(list, t)
	}
	return list, nil
}

func (r *resolver) Todo(ctx context.Context, id string) (*graphqlAPI.Todo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.todos[id]
	if !ok {
		return nil, fmt.Errorf("todo not found: %s", id)
	}
	return t, nil
}

func (r *resolver) CreateTodo(ctx context.Context, text string) (*graphqlAPI.Todo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	todo := &graphqlAPI.Todo{
		ID:   fmt.Sprintf("todo-%d", len(r.todos)+1),
		Text: text,
		Done: false,
	}
	r.todos[todo.ID] = todo
	return todo, nil
}

func (r *resolver) ToggleTodo(ctx context.Context, id string) (*graphqlAPI.Todo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.todos[id]
	if !ok {
		return nil, fmt.Errorf("todo not found: %s", id)
	}
	t.Done = !t.Done
	return t, nil
}

// ---------- Main ----------

func main() {
	// 创建限流器：50 QPS，突发 100
	limiter, _ := tokenbucket.New(50, 100)

	// 创建服务器
	srv := graphqlServer.NewServer(":8080")

	// 中间件链（顺序很重要！）
	srv.Use(
		recovery.Middleware(), // 1. 最外层：捕获 panic
		requestid.Middleware(), // 2. 生成请求 ID
		logging.Middleware(     // 3. 访问日志（跳过 Playground 和健康检查）
			logging.WithSkipPaths("/", "/healthz"),
		),
		cors.Middleware( // 4. 跨域（前端应用必需）
			cors.WithAllowedOrigins("https://app.example.com"),
			cors.WithAllowedMethods("GET", "POST"),
			cors.WithAllowedHeaders("Authorization", "Content-Type"),
			cors.WithAllowCredentials(true),
			cors.WithMaxAge(3600),
		),
		ratelimit.Middleware(limiter), // 5. 限流
	)

	// 注册 GraphQL Schema
	schema := graphqlAPI.NewExecutableSchema(graphqlAPI.Config{
		Resolvers: &resolver{
			todos: make(map[string]*graphqlAPI.Todo),
		},
	})
	srv.Handle("/query", schema)

	// 健康检查端点
	srv.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	// Playground（开发环境）
	if os.Getenv("APP_ENV") != "production" {
		srv.HandleFunc("/", playground.Handler("GraphQL Playground", "/query"))
	}

	// 优雅关闭
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("GraphQL server listening on %s\n", srv.Endpoint())
	fmt.Println("  Playground: http://localhost:8080/")
	fmt.Println("  Endpoint:   http://localhost:8080/query")

	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("server stopped gracefully")
}
```

**测试命令**：

```bash
# 查询所有 Todo
curl -X POST http://localhost:8080/query \
  -H "Content-Type: application/json" \
  -d '{"query":"{ todos { id text done } }"}'

# 创建 Todo
curl -X POST http://localhost:8080/query \
  -H "Content-Type: application/json" \
  -d '{"query":"mutation { createTodo(text: \"Learn Go-Wind GraphQL\") { id text done } }"}'

# 切换 Todo 完成状态
curl -X POST http://localhost:8080/query \
  -H "Content-Type: application/json" \
  -d '{"query":"mutation { toggleTodo(id: \"todo-1\") { id text done } }"}'
```

---

## 13. 常见问题

### Q1: 中间件中的 context 如何传递到 GraphQL Resolver？

**回答**：HTTP 中间件包裹了整个 HTTP 路由器（包括 GraphQL 端点）。当请求到达 gqlgen 的 handler 时，`*http.Request` 中的 `context.Context` 会被 gqlgen 提取并传递给 Resolver 方法。

```
HTTP 请求 → recovery → requestid（注入 context）→ logging → gqlgen handler
                                                              ↓
                                                         Resolver.Method(ctx)
                                                         （ctx 包含 requestid 等信息）
```

因此，在 Resolver 中可以直接通过 `requestid.FromContext(ctx)` 获取中间件注入的数据。

---

### Q2: 为什么可以直接用 `transport/http/middleware` 的中间件？

**回答**：因为 `transport/graphql` 使用了**类型别名**（注意 `=` 号）：

```go
type Middleware = func(http.Handler) http.Handler
```

而 `transport/http` 的中间件函数返回的是 `httpPlugin.Middleware`（底层同样是 `func(http.Handler) http.Handler`）。Go 语言中，类型别名与其底层类型完全等价，所以可以直接赋值，无需任何适配器。

```go
// recovery.Middleware() 返回 httpPlugin.Middleware
mw := recovery.Middleware()
// srv.Use 接收 graphqlServer.Middleware（即 func(http.Handler) http.Handler）
srv.Use(mw) // ✅ 直接传入，无需转换
```

---

### Q3: 如何在 Playground 中设置认证头？

**回答**：点击 Playground 左下角的 **"HTTP HEADERS"** 标签页，输入 JSON：

```json
{
  "Authorization": "Bearer your-jwt-token"
}
```

Playground 会在每次请求中自动附带这些头。

---

### Q4: 如何在同一个端口同时提供 GraphQL 和 REST 接口？

**回答**：使用 `Handle` 注册 GraphQL，用 `HandleFunc` 注册 REST，两者互不冲突：

```go
srv.Handle("/query", graphqlSchema)        // GraphQL 端点
srv.HandleFunc("/healthz", healthHandler)  // 健康检查
srv.HandleFunc("/api/webhook", webhookHd)  // Webhook 回调
```

底层使用的是标准库的 `http.ServeMux`，按路径匹配路由。

---

### Q5: GraphQL 服务器需要像 HTTP 服务器那样指定 Driver 吗？

**回答**：**不需要**。与 `transport/http` 必须通过 `WithDriver()` 指定驱动不同，GraphQL 服务器直接使用标准库的 `http.ServeMux` 和 `http.Server`，没有驱动抽象层。

这意味着：
- **优点**：创建更简单，`NewServer(":8080")` 即可，无需额外的驱动参数
- **限制**：无法像 HTTP 服务器那样切换 gin/chi/fiber 等底层框架

如果你需要高级路由功能（路径参数、路由分组），可以在 gqlgen handler 外层自行包裹路由器。

---

### Q6: 如何处理 GraphQL Subscription（实时推送）？

**回答**：Subscription 需要通过 WebSocket 传输。gqlgen 提供了 `graphql/handler/transport` 包来支持 WebSocket Subscription。

基本思路是在注册 Schema 时添加 WebSocket transport：

```go
import (
    "github.com/99designs/gqlgen/graphql/handler"
    "github.com/99designs/gqlgen/graphql/handler/transport"
    "github.com/gorilla/websocket"
)

// 需要手动创建 gqlgen handler 并添加 transport
srv := graphqlServer.NewServer(":8080")
// 注意：这里需要绕过 Server.Handle，直接使用 handler.New 并添加 transport
// 具体实现请参考 gqlgen 官方文档：
// https://gqlgen.com/reference/subscription/
```

> Subscription 是高级主题，超出本教程范围。建议先掌握 Query 和 Mutation，再查阅 gqlgen 文档了解 Subscription。

---

## 附录：API 快速参考

### Server 方法

| 方法 | 说明 |
|------|------|
| `NewServer(addr, opts...)` | 创建 GraphQL 服务器 |
| `Handle(path, schema)` | 注册 GraphQL Schema 到指定路径 |
| `HandleFunc(path, handler)` | 注册普通 HTTP 处理器 |
| `Use(middlewares...)` | 注册全局中间件（必须在 Start 前调用） |
| `Start(ctx)` | 启动服务器（阻塞，直到 ctx 被取消） |
| `Stop(ctx)` | 优雅关闭服务器 |
| `Endpoint()` | 返回访问地址（`http://` 或 `https://`） |
| `Addr()` | 返回配置的监听地址 |

### Options

| 选项 | 说明 |
|------|------|
| `WithTLSConfig(c *tls.Config)` | 设置 TLS 配置，启用 HTTPS |
| `WithTLS(certFile, keyFile)` | 从证书文件加载 TLS 配置 |
| `WithMiddleware(mws...)` | 创建时传入中间件 |

### 三种服务器对比

| 概念 | HTTP | gRPC | GraphQL |
|------|------|------|---------|
| 创建 | `NewServer(addr, WithDriver(...))` | `NewServer(addr)` | `NewServer(addr)` |
| 中间件类型 | `func(http.Handler) http.Handler` | `grpc.UnaryServerInterceptor` | `func(http.Handler) http.Handler`（与 HTTP 共享） |
| 注册处理器 | `srv.GET(path, handler)` | `pb.RegisterXxxServer(...)` | `srv.Handle(path, schema)` |
| Schema 定义 | 无（代码优先） | `.proto` 文件（protoc） | `.graphql` 文件（gqlgen） |
| 交互式 IDE | 无 | 无 | Playground |
| 启动 | `srv.Start(ctx)` | `srv.Start(ctx)` | `srv.Start(ctx)` |

---

> **下一步学习**：尝试运行 `_examples/graphql-basic` 目录下的示例代码，动手实践是掌握这些概念的最佳方式！

# transport/http 选型与能力指南

本指南帮助你为项目选择合适的用法，并理解 transport/http 的能力边界。
性能数据见 [benchmark/README.md](benchmark/README.md)。

---

## 三档接入方式

transport/http 提供三档能力谱系，覆盖从"零依赖"到"完全框架能力"的全部需求。

### 第 1 档：标准接口（driver 无关，新代码首选）

```go
srv := httpPlugin.NewServer(":8080",
    httpPlugin.WithDriver(gin.NewDriver()),  // 任一 driver
    httpPlugin.WithMiddleware(cors.Middleware(...)),
)
srv.GET("/users/:id", func(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")  // 标准库 API
    json.NewEncoder(w).Encode(user)
})
```

| 维度 | 说明 |
|---|---|
| handler 签名 | `http.HandlerFunc`（标准库） |
| 中间件 | ✅ `Server.Use` 的全部标准中间件（17 个） |
| binding | ✅ 全部 binding 能力（BindQuery/BindBody/BindPath） |
| 框架能力 | ❌ 拿不到 `*gin.Context`/`*fiber.Ctx` |
| driver 可换 | ✅ 改一行 `WithDriver(...)` 即换底层框架 |
| 复用性 | ✅ 业务代码与 driver 完全解耦，可移植 |

**适用**：新项目、希望 driver 无关、需要统一中间件/binding 生态。

### 第 2 档：options 注入框架原生路由（复用存量框架代码）

```go
// 复用存量 gin 业务代码 —— 纯 options 风格
d := gin.New(
    gin.WithRoute("GET", "/users/:id", GetUser),       // func(*gin.Context)
    gin.WithRoute("POST", "/users", CreateUser),
    gin.WithMiddleware(gin.Logger(), gin.Recovery()),  // gin 原生中间件
)
srv := httpPlugin.NewServer(":8080", httpPlugin.WithDriver(d))
```

| 维度 | 说明 |
|---|---|
| handler 签名 | 框架原生（`func(*gin.Context)` / `func(*fiber.Ctx) error`） |
| 框架能力 | ✅ `c.Param`/`c.JSON`/`c.ShouldBind`/`c.QueryParser` 等全部 |
| 中间件 | ⚠️ **框架原生中间件**（不经过 `Server.Use` 的标准中间件链） |
| driver 可换 | ❌ 代码绑定到具体框架 |
| 复用性 | ✅ 存量 gin/fiber handler 可原样复用 |

**适用**：迁移存量 gin/fiber 项目、需要框架特有能力（路径参数、ShouldBind 等）。

### 第 3 档：裸用底层引擎（路由组/websocket 等重度场景）

```go
d := gin.New()
api := d.Engine().Group("/api/v1")  // gin 路由组
api.Use(authMW).GET("/users", listUsers)

// fiber 同理
fd := fiber.New()
fd.App().Use(middleware.Logger())   // fiber 原生中间件
```

| 维度 | 说明 |
|---|---|
| 能力 | ✅ 框架全部能力（路由组嵌套、原生中间件生态、websocket 等） |
| 代价 | ❌ 完全绕过 driver 抽象，代码与框架强耦合 |

**适用**：路由组嵌套、gin-contrib/fiber 中间件生态、websocket 等命令式重度场景。

---

## 三档对比一览

| 维度 | 标准接口（第1档） | options 注入（第2档） | 裸用引擎（第3档） |
|---|---|---|---|
| handler 拿到框架 Context | ❌ | ✅ | ✅ |
| 享受标准中间件链 | ✅ | ❌ | ❌ |
| driver 可替换 | ✅ | ❌ | ❌ |
| 复用存量框架代码 | ❌ | ✅ | ✅ |
| 路由组/原生中间件生态 | ❌ | 部分 | ✅ |
| 适合新项目 | ✅✅✅ | ⚠️ | ❌ |

---

## driver 选型

4 个 driver 的差异主要在**内存与功能**，延迟基本持平（端到端 ±10%，瓶颈在 net/http 标准库与 TCP，不在框架层，详见 [benchmark/README.md](benchmark/README.md)）。

| driver | 延迟 | 内存 | 功能 | 依赖 | 适用场景 |
|---|---|---|---|---|---|
| **std** | 最快（简单场景） | 中 | 弱（仅 path 匹配，无路由参数） | 零 | 极简、嵌入式、零依赖 |
| **chi** | 持平 | 中 | 完备（路由参数/嵌套/中间件组） | 轻量 | 需路由功能但不想引大依赖 |
| **gin** | 持平 | 中（POST body 场景偏高 ⚠️） | 完备 + 成熟生态 | 中 | 团队熟悉 gin、用 gin 中间件 |
| **fiber** | 持平 | **全场最低**（fasthttp 池化） | 完备 | 中 | 高并发、关注 GC 压力/内存 |

### 关于 gin 的 POST body 内存异常

gin v1.12 的 `responseWriter` 未实现 `io.ReaderFrom`，导致 handler 内 `io.Copy(w, r.Body)` 走 fallback 分配 **32KB 中间 buffer**（占 gin JSONEcho 44KB/op 的 74%）。这是**框架级限制**，driver 层无法安全修复（透传 ReadFrom 会破坏 gin 的状态记账）。若业务大量读请求体，考虑 chi/fiber。

### 关于 fiber 的延迟

fiber 基于 fasthttp，理论上更快，但本插件的统一接口（`http.HandlerFunc`）迫使 fiber 经 net/http↔fasthttp 适配层，吃掉了 fasthttp 的延迟优势。**所以 fiber 的价值在内存/GC，不在延迟**。要榨取 fasthttp 延迟，需用第 2/3 档（`HandleFiber`/`App()` 绕过适配层）。

---

## 中间件体系：两套并行，不互通

这是最重要的边界认知。

### 两套中间件

| 中间件类型 | 注册方式 | 作用范围 |
|---|---|---|
| **标准中间件**（17 个） | `Server.Use(...)` | 仅作用于**第 1 档**路由（`Handle`/`GET` 等） |
| **框架原生中间件** | `gin.WithMiddleware`/`fiber.WithMiddleware` | 仅作用于**第 2/3 档**路由（`WithRoute`/`Engine()`） |

### 为什么不互通

标准中间件收 `http.HandlerFunc`，框架中间件收 `*gin.Context`/`*fiber.Ctx`。两者的 Context 数据流根本不同：框架 handler 直接写 `c.Writer`，标准中间件包的 responseWriter 读不到。这是物理约束，**任何 adapter 都无法全通用**（见下）。

### 实践建议

- **一个 CORS 中间件要对全部路由生效** → 要么全用第 1 档（标准接口），要么全用第 2 档（框架原生）
- **混合用法** → 标准 CORS 对标准路由，框架 CORS 对原生路由，各配一份
- **避免**：在混合用法下期望"一份中间件管全部"——不成立

### 关于 adapter 的限制（为什么不做自动适配）

曾评估用 adapter 把标准中间件包成框架中间件实现"通用"。结论：**只覆盖前置类中间件（限流、鉴权、CORS 正常路径），读响应类（logging/metrics）会静默失效**——读不到真实 status，数据错误但不报错。这种"看起来工作实际不工作"的 API 比明确不支持更有害，故**不提供自动 adapt**，改为明确的文档约束。

---

## 已知限制汇总

| 限制 | 原因 | 规避 |
|---|---|---|
| gin POST body 内存 44KB/op | gin 未实现 `io.ReaderFrom`（框架级） | 大量读请求体用 chi/fiber |
| 两套中间件不互通 | Context 数据流差异（物理约束） | 按路由类型各配中间件 |
| fiber 经适配层无延迟优势 | 统一接口强制 net/http 适配 | 用 `HandleFiber`/`App()` 绕过（第 2/3 档） |
| 重度框架项目迁移成本高 | 路由组/gin-contrib 生态与标准接口不兼容 | 直接用第 3 档，或继续用原框架 |
| std 无路由参数 | 标准库 `ServeMux` 限制 | 用 chi/gin/fiber |

---

## 快速决策树

```
你的项目是？
├─ 全新项目
│   ├─ 想要 driver 无关、可移植      → 第 1 档（标准接口）+ chi 或 gin
│   ├─ 关注内存/GC                  → 第 1 档 + fiber
│   └─ 极简/零依赖                  → 第 1 档 + std
│
├─ 迁移存量 gin/fiber 项目
│   ├─ handler 轻度用框架 Context    → 第 2 档（options 注入 WithRoute）
│   ├─ 重度用框架特性（路由组等）    → 第 3 档（Engine()/App()）
│   └─ 标准库风格 handler           → 第 1 档（零改动接入）
│
└─ 已选定单一框架，不考虑替换
    └─ 直接用第 3 档（Engine/App），driver 抽象价值降低
```

---

## 相关文档

- [benchmark/README.md](benchmark/README.md) — 4 driver 性能横向对比、优化历程、pprof 归因
- 各 driver 包内文档：`driver/gin`、`driver/fiber`、`driver/chi`、`driver/std`
- 中间件包：`middleware/` 下 17 个独立中间件，各有测试与 benchmark

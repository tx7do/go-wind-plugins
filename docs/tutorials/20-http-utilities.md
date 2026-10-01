# Go-Wind 插件教程 · 第 20 章：HTTP 工具族

> 本篇是 [Go-Wind 插件系列教程](./README.md)的组成部分。HTTP 家族里除了协议服务器与中间件（[第 8 章](./08-transport-middleware.md)），还有四个工具模块：文档 UI 挂载（swagger / redoc）、请求绑定库（binding）、驱动基准测试（benchmark）。

## 1. `transport/http/swagger` —— 嵌入式 Swagger UI

把交互式 API 文档挂到任意 HTTP 服务器实例上，支持三种文档源：

```go
import (
    httpServer "github.com/tx7do/go-wind-plugins/transport/http"
    "github.com/tx7do/go-wind-plugins/transport/http/driver/std"
    "github.com/tx7do/go-wind-plugins/transport/http/swagger"
)

srv := httpServer.NewServer(":8080", httpServer.WithDriver(std.NewDriver()))

// 方式一：远程 URL——前端直接拉取 openapi.json
swagger.Register(srv,
    swagger.WithTitle("Petstore"),
    swagger.WithRemoteFileURL("https://petstore3.swagger.io/api/v3/openapi.json"),
    swagger.WithBasePath("/docs/"),
)

// 方式二：本地文件——服务端读取并托管
// swagger.Register(srv, swagger.WithTitle("API"),
//     swagger.WithLocalFile("./openapi.json"), swagger.WithBasePath("/docs/"))

// 方式三：内存数据——托管动态生成的文档
// swagger.Register(srv, swagger.WithTitle("API"),
//     swagger.WithMemoryData(data, "json"), swagger.WithBasePath("/docs/"))
```

访问 `http://localhost:8080/docs/` 即得文档页。三种源对应三种典型工作流：文档由独立服务托管（远程 URL）、随版本发布的静态文档（本地文件）、由运行时自描述动态生成（内存数据）。

## 2. `transport/http/redoc` —— ReDoc 渲染引擎

Swagger UI 的备选渲染器，面向阅读体验优化的单页文档。接口形态与 swagger 对称，支持远程 URL 与本地文件两种源：

```go
redoc.Register(srv,
    redoc.WithTitle("Petstore"),
    redoc.WithDescription("A sample API powered by ReDoc"),
    redoc.WithRemoteFileURL("https://petstore3.swagger.io/api/v3/openapi.json"),
    redoc.WithBasePath("/redoc/"),
)
```

两件事必须做对：文档端点（`/docs/`、`/redoc/`）在生产环境要纳入[第 10 章](./10-security.md)的认证授权面——OpenAPI 文档本身就是攻击者的地图；且文档源与实际路由的一致性由你的发布流程保证，挂载层不做校验。

## 3. `transport/http/binding` —— 请求绑定与响应写出库

服务器与业务 handler 之间的数据搬运层。请求侧把 URL query、路径参数、请求体绑定进结构体；响应侧统一写出。导出面（[模块源码](../../transport/http/binding/)，含 `binding.go` / `errors.go` / `field_cache.go` / `response.go`）：

| 函数 | 职责 |
|------|------|
| `BindQuery(req, url.Values)` | 把 query 参数绑定进结构体 |
| `BindPath` / `BindAllPaths` | 路径参数绑定（单字段 / 全量） |
| `BindBody` / `BindBodyField` | 请求体反序列化绑定（走 `SetCodec` 设定的编解码器，见[第 5 章](./05-encoding.md)） |
| `PathParam` / `SetPathParamFunc` | 路径参数提取与提取函数注入（驱动差异被盖住——gin/chi 的路径参数 API 不同） |
| `SetContentType` / `WriteResponse` / `WriteError` / `WriteStreamChunk` | 响应头与响应体写出（含流式分块） |
| `MapError(err, code)` / `RegisterError(err, code)` | 错误值 → HTTP 状态码映射注册表；`WriteError` 据此写出，与[第 19 章](./19-tools.md)errors 模块的领域错误模型衔接 |

`field_cache.go` 缓存反射字段元数据以降低重复绑定的开销。绑定层的存在让 handler 收到的是已填充的结构体——但**校验不在这一层**：结构体字段的合法性检查由 validate 中间件（[第 8 章](./08-transport-middleware.md)中间件套件）完成，两层各管一段。

## 4. `transport/http/benchmark` —— 驱动横向对比基准

一份持续维护的[驱动横向对比报告](../../transport/http/benchmark/README.md)，实测 std / chi / gin / fiber 四个驱动（[第 8 章](./08-transport-middleware.md)驱动系统）在多场景下的延迟、内存分配与分配次数。报告的关键结论：

- 四个驱动的延迟差距很小，高并发下进一步收窄——**选驱动看功能面而不是看跑分**（std 简单场景最快但功能最弱、chi 在未命中分支最快、fiber 内存效率最高、gin 在 `JSONEcho` 场景存在框架级 `io.ReaderFrom` 缺陷导致的内存异常）；
- 中间件层（requestid / metrics / logging）的优化实测与取舍记录在案——metrics 与 logging 优化成功，requestid 经验证后**放弃优化**并记录了原因；
- 报告附完整的复现方法（`go test -bench` 全场景中位数 + `benchstat` 严谨对比）。

引入新驱动或升级驱动版本时，先跑这套基准再合入——这是本仓维护驱动层的既定流程。

## 5. 深入阅读

- [`transport/http/benchmark/README.md`](../../transport/http/benchmark/README.md) —— 驱动对比报告全文与复现步骤
- [`transport/http/STATUS.md`](../../transport/http/STATUS.md) —— 三档接入方式、驱动选型（能力与内存维度）与中间件双轨边界的官方指南
- [`transport/http/driver/std/example_test.go`](../../transport/http/driver/std/example_test.go) —— 标准驱动使用示例
- [根 README · Swagger UI / ReDoc 文档](../../README.md)

# Go-Wind 插件教程 · 第 19 章：工具模块

> 本篇是 [Go-Wind 插件系列教程](./README.md)的组成部分，收尾四个不属于前面任何家族的小型工具模块。

## 1. `errors` —— 统一错误模型

[`errors`](../../errors/) 提供**领域错误**的统一定义与传递：错误码 + 错误类型 + 元数据的三元组在服务边界间无损流转，接收侧可程序化解析而不是对错误字符串做模式匹配。

文档（[`errors/README.md`](../../errors/README.md)）的四个使用切面：定义领域错误（错误码、HTTP 状态映射一次声明）、在业务逻辑中返回、附加元数据（错误链路上下文）、解析错误（消费侧还原结构）。传输层的 errors 中间件（[第 8 章](./08-transport-middleware.md)矩阵）负责把领域错误翻译成带正确状态码的协议响应——业务代码返回 `errors` 包的错误值，无需手写响应码。

## 2. `health` —— 探活端点

聚合式健康检查，按 Kubernetes 风格暴露两个端点（完整版见 [`health/example_test.go`](../../health/example_test.go)）：

```go
import "github.com/tx7do/go-wind-plugins/health"

h := health.New(health.WithTimeout(3 * time.Second))   // 聚合器与总超时

h.Register("database", health.PingFunc(func(ctx context.Context) error {
    return db.PingContext(ctx)    // 真实依赖的探活：数据库、下游、缓存
}))

mux := http.NewServeMux()
mux.Handle("/healthz", health.NewLivenessHandler())  // 存活探针：进程活着即 200
mux.Handle("/readyz", health.NewHandler(h))          // 就绪探针：聚合全部注册探针
```

`/healthz` 与 `/readyz` 的语义区分是 K8s 的标准约定：前者回答「要不要重启我」，后者回答「能不能给我流量」。**探针必须探真实依赖**——永远返回 nil 的就绪探针比没有探针更糟。就绪状态与[第 2 章](./02-registry.md)的注册发现联动：就绪为否的实例应从注册中心摘除或靠健康检查机制降权。

## 3. `pprof` —— 性能分析端点

[`pprof`](../../pprof/) 把 Go 标准库的 pprof 端点挂到服务器上（CPU / 内存 / goroutine / block profile），用于生产环境按需采样定位性能问题。该模块无独立 README，接入面见源码；生产暴露务必配[第 10 章](./10-security.md)的认证中间件或内网隔离——profile 端点泄露的运行时信息本身就是攻击面。

## 4. `retry` 与 `testing`

- **[`retry`](../../retry/)**：库形态的重试策略（指数退避、抖动、总时长上限），接口与示例见[第 9 章](./09-resilience.md)第 4 节——同时它也是传输层 retry 中间件的底层。
- **[`testing`](../../testing/)**：仓库的集成测试基建（测试门控与辅助工具），面向本仓插件的开发者而非接入方；参与本仓开发时先读该模块源码了解测试约定。

## 5. 深入阅读

- [`errors/README.md`](../../errors/README.md) · [`health/example_test.go`](../../health/example_test.go)
- [根 README · 其他工具模块矩阵](../../README.md#其他工具模块)

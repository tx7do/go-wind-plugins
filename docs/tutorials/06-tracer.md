# Go-Wind 插件教程 · 第 6 章：分布式追踪（Tracer）

> 本篇是 [Go-Wind 插件系列教程](./README.md)的组成部分。追踪层不复刻 OpenTelemetry 的接口，直接使用 OTel 原生类型。

## 1. 这一层解决什么问题

一个用户请求横跨网关、鉴权、订单、库存多个服务，任何一环变慢都要能定位。分布式追踪给每个请求分配 trace-id，跨服务传播并串联成调用链，在 Jaeger / Tempo 这类后端里还原完整时序。本仓的角色很克制：只提供把 span 数据经 OTLP 协议导出到任意后端的插件，不另造抽象。

## 2. 架构：OTel 原生，无自定义接口

| 类型 | 方法 | 说明 |
|------|------|------|
| `*sdktrace.TracerProvider` | `Tracer(name) trace.Tracer` | 创建标准 OTel Tracer |
| `*sdktrace.TracerProvider` | `Shutdown(ctx)` | 关闭 provider，刷新未导出的 span |
| `trace.Tracer` | `Start(ctx, name, opts...)` | 创建 Span 并注入 trace 上下文 |

业务代码直接面对 OTel SDK 的类型——`tracer := tp.Tracer("myapp")`，随后 `ctx, span := tracer.Start(ctx, "operation")`。学习成本就是 OTel 本身，本仓不增加第二套概念。

## 3. 插件

| 插件 | 模块路径 | 引擎 |
|------|---------|------|
| OTLP | `github.com/tx7do/go-wind-plugins/tracer/otlp` | OpenTelemetry Protocol |

OTLP 是 OTel 的标准导出协议，主流后端（Jaeger、Zipkin、SkyWalking、Tempo、Datadog、阿里云 ARMS、腾讯云 APM 等）均支持——切换后端只改 endpoint，不换插件。

## 4. 接入示例

`tracer/otlp` 的构造选项：`WithEndpoint`（采集端地址）、`WithServiceName`、`WithServiceVersion`、`WithInsecure`（本地开发跳过 TLS）、`WithHTTP`（HTTP 还是 gRPC 传输）、`WithSampleRatio`（采样率）：

```go
import (
    "go.opentelemetry.io/otel"
    oteltrace "github.com/tx7do/go-wind-plugins/tracer/otlp"
)

tp, err := oteltrace.New(
    oteltrace.WithEndpoint("localhost:4317"),
    oteltrace.WithServiceName("my-service"),
    oteltrace.WithInsecure(true),
)
if err != nil {
    return
}
otel.SetTracerProvider(tp)     // 注册为全局 provider
defer tp.Shutdown(ctx)         // 进程退出前刷新未导出 span
```

模块另提供 `NewTracer(kind, spanName, opts...)` 便捷封装，用于快速构造带 span kind 的 Tracer 包装；日常建议直接使用 OTel 原生 API 以保持可移植性。

## 5. 与日志、指标的联动

- **日志**：go-wind 框架的 context 工具（`WithTraceID(ctx, id)` 等）把 trace-id 放进 context；配合[第 3 章](./03-log.md)的 `Logger.With` 把它绑定为固定日志字段，日志平台即可按 trace 聚合。
- **指标**：若选 `metrics/otel`（见[第 4 章](./04-metrics.md)），指标与追踪共用同一条 OTLP 管道，endpoint 与凭证配置只有一份。
- **传播**：跨服务传播靠 OTel 标准的 propagator（W3C TraceContext），框架的 HTTP/gRPC 传输层中间件（`tracing`）已在链路两端自动注入/提取 trace 头。

## 6. 部署与采样建议

本地开发用 `WithInsecure(true)` + Jaeger all-in-one 容器即可起步；生产环境走 TLS 与网关代理。采样率从低起步（如 1%），确认后端容量与查询模式后再上调；对关键低流量路径可单独全采样。

## 7. 深入阅读

- [`tracer/otlp/`](../../tracer/otlp/) —— 插件源码与全部选项
- [OTel Go 文档](https://opentelemetry.io/docs/languages/go/)
- [根 README · 分布式追踪矩阵](../../README.md#分布式追踪tracer)

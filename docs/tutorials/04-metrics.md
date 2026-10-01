# Go-Wind 插件教程 · 第 4 章：指标监控（Metrics）

> 本篇是 [Go-Wind 插件教程系列](./README.md)的组成部分。指标插件实现根模块 [`github.com/tx7do/go-wind-plugins/metrics`](../../metrics/) 定义的 `Metrics` 接口。

## 1. 这一层解决什么问题

日志回答「发生了什么」，指标回答「系统现在什么状态、趋势如何」。QPS、延迟分布、队列深度、活跃连接数——这些量需要以极低开销持续上报，并汇入 Prometheus、Datadog 这类时序监控栈做告警与看板。指标层把「上报」抽象成三种原语，引擎插件负责对接各家后端。

## 2. 接口架构

| 接口 | 方法 | 说明 |
|------|------|------|
| `Metrics` | `Counter(ctx, name, value, labels)` | 单调递增计数器：请求总数、错误总数 |
| `Metrics` | `Histogram(ctx, name, value, labels)` | 值分布观测：延迟、请求体大小 |
| `Metrics` | `Gauge(ctx, name, value, labels)` | 瞬时值：队列深度、活跃连接数 |
| `Closer` | `Close() error` | 关闭并刷新未发送数据 |

三种原语对应监控领域的标准语义，任何后端都按同一套概念接收。标签（labels）用于维度切分（method/path/status 等），取值必须是**有界集合**——把用户 ID 之类无界值当标签会产生基数爆炸，这是指标系统最常见的事故来源。

## 3. 插件矩阵

| 插件 | 模块路径 | 引擎 |
|------|---------|------|
| Prometheus | `github.com/tx7do/go-wind-plugins/metrics/prometheus` | Prometheus client_golang |
| OpenTelemetry | `github.com/tx7do/go-wind-plugins/metrics/otel` | OTLP（gRPC / HTTP） |
| Datadog | `github.com/tx7do/go-wind-plugins/metrics/datadog` | DogStatsD |

## 4. 接入示例：Prometheus

（完整版本见 [`metrics/prometheus/example_test.go`](../../metrics/prometheus/example_test.go)）

```go
m, err := prometheus.New(
    prometheus.WithNamespace("myapp"),   // 指标命名空间：myapp_http_requests_total
)
if err != nil {
    return
}

// 三种原语的使用形态
m.Counter(ctx, "http_requests_total", 1,
    map[string]string{"method": "GET", "path": "/"})
m.Histogram(ctx, "http_request_duration_seconds", 0.42,
    map[string]string{"method": "GET", "path": "/"})
m.Gauge(ctx, "active_connections", 7, nil)

reg := m.Registry()   // 原生 prometheus.Registerer，供 /metrics 端点挂载
```

Prometheus 引擎可用选项：`WithNamespace`（命名空间前缀）、`WithSubsystem`（子系统前缀）、`WithRegistry`（自定义 Registerer）、`NewWithDefaultRegistry`（快捷构造）。`Registry()` 返回原生注册器，`/metrics` 抓取端点用标准库的 `promhttp.HandlerFor` 挂载即可。

## 5. 框架内建埋点

本仓的传输层与消息代理服务器模块（`transport/*`、`broker/*`）内置了指标埋点接线：通过各服务器构造选项 `WithMetrics` 注入 `Metrics` 实例后，请求计数、请求延迟等指标会自动打点——无需在业务 handler 里手写。业务自定义指标再按上节 API 补充。

## 6. 选型建议

| 场景 | 建议 |
|------|------|
| 自建 Prometheus 栈 | `prometheus`（pull 模型，生态最全） |
| OTel 统一采集管道（指标+追踪同管道） | `otel`（见[第 6 章](./06-tracer.md)） |
| Datadog 商业平台 | `datadog` |

命名规范上建议全程开启 `Namespace` / `Subsystem` 前缀，避免多服务指标名冲突；告警规则按服务粒度组织。

## 7. 深入阅读

- [`metrics/prometheus/example_test.go`](../../metrics/prometheus/example_test.go)
- [根 README · 指标监控矩阵](../../README.md#指标监控metrics)
- [第 6 章 · 分布式追踪](./06-tracer.md) —— OTel 管道的另一面

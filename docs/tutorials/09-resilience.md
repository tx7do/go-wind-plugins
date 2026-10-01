# Go-Wind 插件教程 · 第 9 章：熔断、限流与重试（弹性治理）

> 本篇是 [Go-Wind 插件系列教程](./README.md)的组成部分。三个家族的接口分别定义于根模块 [`circuitbreaker/`](../../circuitbreaker/)、[`ratelimit/`](../../ratelimit/)、[`retry/`](../../retry/)。

## 1. 三件套的分工

出站调用外部依赖（数据库、第三方 API、下游服务）时，三类故障需要三类工具：

- **重试（retry）**：瞬时抖动——网络闪断、503——重放几次就成功。
- **熔断（circuit breaker）**：持续故障——重试只会火上浇油，快速失败并给依赖恢复时间。
- **限流（rate limiter）**：过载保护——无论下游状态如何，把自身出站（或入站）速率压在容量之内。

三者回答的是不同问题，组合使用而非互相替代（见第 5 节）。

## 2. 接口架构

**熔断**（根模块 `circuitbreaker.CircuitBreaker`）：

| 方法 | 说明 |
|------|------|
| `Allow() error` | 请求通行票据；熔断打开时返回 `ErrCircuitOpen` |
| `Execute(ctx, fn) error` | 包裹执行，按 fn 的返回值自动标记成败 |
| `MarkSuccess() / MarkFailure()` | 手动标记（用于成功/失败不由返回值决定的场景） |
| `State() string` | 当前状态（closed / open / half-open） |
| `Close() error` | 释放 |

**限流**（根模块 `ratelimit.Limiter`）：

| 方法 | 说明 |
|------|------|
| `Allow() (bool, error)` | 非阻塞判定；被拒返回 `ErrLimited` |
| `Wait(ctx) error` | 阻塞等待令牌，或随 context 超时 |
| `Close() error` | 释放 |

**重试**（`retry` 包，非接口形态，直接 `retry.New(...)` 构造策略）。

## 3. 插件矩阵

| 家族 | 插件 | 模块路径 | 引擎 |
|------|------|---------|------|
| 熔断 | Hystrix | `github.com/tx7do/go-wind-plugins/circuitbreaker/hystrix` | afex/hystrix-go |
| 熔断 | Sentinel | `github.com/tx7do/go-wind-plugins/circuitbreaker/sentinel` | alibaba/sentinel-golang |
| 熔断 | SRE | `github.com/tx7do/go-wind-plugins/circuitbreaker/sres` | SRE 自适应熔断 |
| 熔断 | Vegas | `github.com/tx7do/go-wind-plugins/circuitbreaker/vegas` | Vegas 自适应限流 |
| 限流 | Token Bucket | `github.com/tx7do/go-wind-plugins/ratelimit/tokenbucket` | 令牌桶算法 |
| 限流 | BBR | `github.com/tx7do/go-wind-plugins/ratelimit/bbr` | BBR 自适应限流 |
| 限流 | Sentinel | `github.com/tx7do/go-wind-plugins/ratelimit/sentinel` | alibaba/sentinel-golang |

## 4. 接入示例

**SRE 熔断器**（完整版见 [`circuitbreaker/sres/example_test.go`](../../circuitbreaker/sres/example_test.go)）：

```go
cb := sres.New(
    sres.WithK(1.5),                 // 灵敏度：越低越激进
    sres.WithWindow(10*time.Second), // 统计窗口
    sres.WithBucketCount(10),        // 窗口内桶数
)
defer cb.Close()

err := cb.Execute(ctx, func() error {
    // 返回非 nil 即记为失败
    return callDependency()
})
if errors.Is(err, circuitbreaker.ErrCircuitOpen) {
    // 快速失败路径：走降级逻辑
}
```

**令牌桶限流器**（完整版见 [`ratelimit/tokenbucket/example_test.go`](../../ratelimit/tokenbucket/example_test.go)）：

```go
limiter, err := tokenbucket.New(10, 5) // 速率 10 令牌/秒，桶容量 5
if err != nil {
    return
}
defer limiter.Close()

if ok, err := limiter.Allow(); !ok {
    // err == ratelimit.ErrLimited，拒绝
}
```

**重试器**（完整版见 [`retry/example_test.go`](../../retry/example_test.go)）：

```go
r := retry.New(
    retry.WithMaxAttempts(5),
    retry.WithBackoff(retry.ExponentialBackoff{   // 或 retry.FixedBackoff(...)
        Initial: 100 * time.Millisecond,
        Factor:  2,
        Max:     2 * time.Second,
    }),
    retry.WithJitter(retry.FullJitter),           // 抖动防止重试风暴
    retry.WithMaxTotalWait(2*time.Second),        // 总时长上限
)
err := r.Do(ctx, func(ctx context.Context) error {
    return flakyOperation(ctx)
})
```

## 5. 组合与放置原则

经典分层是**限流在最外、熔断次之、重试最内**：限流保护自身容量（先于一切昂贵动作），熔断在依赖持续故障时切断重试循环，重试只处理瞬时抖动且受总时长约束。入站方向上，同一套引擎经传输层中间件接入（`transport/*/middleware/ratelimit`、`circuitbreaker`，见[第 8 章](./08-transport-middleware.md)）。

两条红线：重试的对象必须是幂等操作；熔断器的降级路径要真实可用——返回缓存值、默认值或明确报错，而不是抛 500。

## 6. 深入阅读

- [`circuitbreaker/sres/example_test.go`](../../circuitbreaker/sres/example_test.go) · [`ratelimit/tokenbucket/example_test.go`](../../ratelimit/tokenbucket/example_test.go) · [`retry/example_test.go`](../../retry/example_test.go)
- [根 README · 熔断器矩阵](../../README.md#熔断器circuit-breaker) · [限流器矩阵](../../README.md#限流器rate-limiter)

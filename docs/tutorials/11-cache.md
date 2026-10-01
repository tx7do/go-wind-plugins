# Go-Wind 插件教程 · 第 11 章：通用缓存（Cache）

> 本篇是 [Go-Wind 插件教程系列](./README.md)的组成部分。通用缓存插件实现根模块 [`github.com/tx7do/go-wind-plugins/cache`](../../cache/) 定义的 `Cache` 接口。注意与 [`crud/cache`](./15-crud-integrations.md)（数据访问层的 Cache-Aside 封装）区分——同名不同层。

## 1. 这一层解决什么问题

进程内或跨进程的临时 KV 存储：幂等去重表、限流计数器（[第 9 章](./09-resilience.md)部分引擎的底层）、会话数据、计算结果的短期复用。接口统一为字节数组的带 TTL 存取，引擎决定存储位置与淘汰策略。

## 2. 接口架构

根模块 `cache.Cache` 的方法面（以 [`cache/local/example_test.go`](../../cache/local/example_test.go) 为准）：

| 方法 | 说明 |
|------|------|
| `Set / Get / Has / Delete` | 单键读写与删除，`Set` 带独立 TTL 参数 |
| `SetNX` | 仅当键不存在时写入——分布式锁、防重入的基础原语 |
| `SetMulti / GetMulti` | 批量读写 |
| `EntryCount / HitCount / MissCount / EvacuateCount` | 命中率与驱逐统计——缓存调优的第一手数据 |
| `Close` | 释放 |

值类型是 `[]byte`——序列化由调用方负责（用[第 5 章](./05-encoding.md)的 Codec 或自带编解码），接口层不感知结构体。

## 3. 插件矩阵

| 插件 | 模块路径 | 引擎 |
|------|---------|------|
| Local | `github.com/tx7do/go-wind-plugins/cache/local` | 本地内存缓存（FreeCache 环形缓冲 + LRU 淘汰） |
| Redis | `github.com/tx7do/go-wind-plugins/cache/redis` | gomodule/redigo |

两者语义差异要清楚：**local** 是进程内的预分配环形缓冲，零网络往返、零外部依赖，但容量在构造时就固定（见下例 `WithSize`）、多实例间不共享、进程重启即清空；**redis** 跨实例共享且持久化由 Redis 配置决定，但每一次操作都是网络往返。选择标准就是这一条：数据需不需要被其他进程看到——需要就 redis，不需要就 local，没有中间态。

## 4. 接入示例：本地缓存

（完整版见 [`cache/local/example_test.go`](../../cache/local/example_test.go)）

```go
import (
    "github.com/tx7do/go-wind-plugins/cache"
    "github.com/tx7do/go-wind-plugins/cache/local"
)

c := local.New(
    local.WithSize(100*1024*1024),      // 预分配 100MB 环形缓冲
    local.WithDefaultTTL(5*time.Minute), // 未显式指定 TTL 时的默认值
)
defer c.Close()

_ = c.Set(ctx, "user:1", []byte("alice"), 10*time.Minute) // 单键带 TTL
v, _ := c.Get(ctx, "user:1")
_ = c.Delete(ctx, "user:1")

_ = c.SetNX(ctx, "lock:order:123", []byte("locked"), 30*time.Second) // 锁原语

_ = c.SetMulti(ctx, []cache.Item{ /* {Key, Value, TTL}... */ })      // 批量
_ = c.GetMulti(ctx, []string{"user:2", "user:3"})
```

Redis 引擎的构造形态：`redis.New(client, redis.WithKeyPrefix("myapp:"))`——第一个参数是调用方自建的 go-redis 客户端，`WithKeyPrefix` 给全部键加命名空间前缀避免多应用混用同一实例时的键冲突。

## 5. 使用纪律

- **TTL 必显式**：默认 TTL 只是兜底；每个键都应有业务语义明确的过期时间，永久键是泄漏源。
- **键要可枚举**：`<实体>:<id>` 的形态让批量清理（按前缀扫描）成为可能。
- **盯紧命中率**：`HitCount` / `MissCount` 持续偏低说明缓存的键集和访问模式不匹配，先修键设计再谈扩容。
- **写操作后的失效**：数据变更时主动 `Delete`，不要指望 TTL 兜底——陈旧窗口越长，业务越容易依赖错误数据。

## 6. 深入阅读

- [`cache/local/example_test.go`](../../cache/local/example_test.go)
- [根 README · 缓存矩阵](../../README.md#缓存cache)
- [第 15 章 · CRUD 集成](./15-crud-integrations.md) —— 面向数据访问层的另一套缓存抽象

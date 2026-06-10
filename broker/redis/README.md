# Redis Broker

基于 [gomodule/redigo](https://github.com/gomodule/redigo) 实现的 Redis 消息代理，支持两种驱动模式：

| 驱动 | 说明 | 适用场景 |
|------|------|----------|
| `DriverTypePubSub` | Pub/Sub 发布订阅模式 | 实时推送、轻量级消息广播 |
| `DriverTypeStream` | Stream 消息队列模式 | 需要持久化、消费确认、消费组的场景 |

## 两种模式对比

| 特性 | Pub/Sub | Stream |
|------|---------|--------|
| 消息持久化 | ❌ 离线消息丢失 | ✅ 持久化到磁盘 |
| 消费确认 (ACK) | ❌ | ✅ `XACK` |
| 消费组 | ❌ | ✅ `XGROUP` / `XREADGROUP` |
| 消息回溯 | ❌ | ✅ 按 ID 范围读取 |
| 断线自动重连 | ✅ 指数退避 (1s→30s) | ✅ 指数退避 (1s→30s) |
| 发布命令 | `PUBLISH` | `XADD` |
| 订阅命令 | `SUBSCRIBE` | `XREADGROUP` |
| 消息裁剪 | N/A | ✅ `MAXLEN` |

## 快速开始

### Pub/Sub 模式

```go
package main

import (
	"context"
	"github.com/tx7do/go-wind-plugins/broker"
	"github.com/tx7do/go-wind-plugins/broker/redis"
)

func main() {
	b := redis.NewBroker(redis.DriverTypePubSub,
		broker.WithAddress("127.0.0.1:6379"),
		broker.WithCodec("json"),
	)

	_ = b.Init()
	_ = b.Connect()
	defer b.Disconnect()

	// 发布
	_ = b.Publish(context.Background(), "test_topic", broker.NewMessage(msg))

	// 订阅
	sub, _ := b.Subscribe("test_topic", handler, binder)
}
```

### Stream 模式

```go
package main

import (
	"context"
	"time"
	"github.com/tx7do/go-wind-plugins/broker"
	"github.com/tx7do/go-wind-plugins/broker/redis"
)

func main() {
	b := redis.NewBroker(redis.DriverTypeStream,
		broker.WithAddress("127.0.0.1:6379"),
		broker.WithCodec("json"),
	)

	_ = b.Init()
	_ = b.Connect()
	defer b.Disconnect()

	// 发布（支持 MAXLEN 裁剪）
	_ = b.Publish(context.Background(), "mystream", broker.NewMessage(msg),
		redis.WithStreamMaxLen(10000),
	)

	// 订阅（消费组 + 自动确认）
	sub, _ := b.Subscribe("mystream", handler, binder,
		redis.WithStreamGroup("my-group"),
		redis.WithStreamConsumer("consumer-1"),
		redis.WithStreamBlockTime(5*time.Second),
		redis.WithStreamCount(100),
	)
}
```

## 配置选项

### 通用选项

| 选项 | 说明 | 默认值 |
|------|------|--------|
| `redis.WithAddress(addr)` | Redis 服务器地址 | `redis://127.0.0.1:6379` |
| `redis.WithCodec(name)` | 编解码器 | `` (原始数据) |
| `redis.WithConnectTimeout(d)` | 连接超时 | 30s |
| `redis.WithReadTimeout(d)` | 读超时 | 30s |
| `redis.WithWriteTimeout(d)` | 写超时 | 30s |
| `redis.WithIdleTimeout(d)` | 空闲连接超时 | 0 (不关闭) |
| `redis.WithMaxIdle(n)` | 最大空闲连接数 | 256 |
| `redis.WithMaxActive(n)` | 最大连接数 | 0 (不限制) |

### Stream 专属选项

| 选项 | 说明 | 默认值 | 类型 |
|------|------|--------|------|
| `redis.WithStreamGroup(name)` | 消费组名称 | `default-group` | SubscribeOption |
| `redis.WithStreamConsumer(name)` | 消费者名称 | `default-consumer` | SubscribeOption |
| `redis.WithStreamBlockTime(d)` | XREADGROUP 阻塞等待时间 | 5s | SubscribeOption |
| `redis.WithStreamCount(n)` | 每次读取的最大消息数 | 10 | SubscribeOption |
| `redis.WithStreamMaxLen(n)` | XADD 时 MAXLEN 限制 | 0 (不限制) | PublishOption |

## Docker 部署开发环境

```shell
docker pull bitnami/redis:latest

docker run -itd \
    --name redis-test \
    -p 6379:6379 \
    -e ALLOW_EMPTY_PASSWORD=yes \
    bitnami/redis:latest
```

## 管理工具

- [RedisInsight](https://redis.com/redis-enterprise/redis-insight/)
- [Another Redis Desktop Manager](https://github.com/qishibo/AnotherRedisDesktopManager/releases)

## 注意事项

- **Pub/Sub 模式**：消息不持久化，发送时无订阅者则消息丢失；不提供消息传输保障
- **Stream 模式**：消息持久化到磁盘，支持消费确认 (XACK) 和消费组，适合需要可靠消费的场景
- 两种模式共享连接池配置，切换模式只需更改 `DriverType` 参数

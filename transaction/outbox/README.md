# Transactional Outbox

事务性发件箱模式，基于 `broker` 包实现"本地事务 + 最终一致"的分布式事件投递。**不需要外部事务服务器**——数据库本身就是消息存储，提供与 DTM 二阶段消息（Msg）同级的保障，而运维面只有一张表。

## 核心概念

| 概念 | 说明 |
|------|------|
| `Enqueue` | 在业务本地事务内写入发件箱表，事件与业务数据原子提交 |
| `Store` | 发件箱表的存取抽象（`NewStore` 为 SQL 实现，测试可注入内存实现） |
| `Relay` | 后台轮询发件箱表，把已提交的事件发布到任意 `broker.Broker` 实现 |
| `Inbox` | 消费端去重中间件：同一 `(handler, eventID)` 只处理一次 |
| `Barrier` | 消费端事务屏障（对应 DTM 的 BranchBarrier）：业务写库与去重记录同一事务提交，数据库效果恰好一次 |

## 保证语义

- **投递**：at-least-once。Relay 崩溃后由租约（`WithLease`）回收重投，靠消费端去重吸收重复。
- **消费**：`Inbox` 记录先行——重复投递被抑制，但进程在记录后、处理前崩溃（或 handler 失败）时事件不重试；`Barrier` 把业务写库与去重记录放进同一事务，数据库效果恰好一次。**有数据库副作用的 handler 请一律使用 `Barrier`**。

## 快速开始

### 生产者：业务事务内追加事件

```go
// 事件类型即契约：默认主题名 = Go 类型名
type OrderCreated struct {
	OrderID string `json:"order_id"`
	Amount  int64  `json:"amount"`
}

tx, _ := db.BeginTx(ctx, nil)
// ... 业务写入（tx）...
err := outbox.Enqueue(ctx, tx, outbox.Event[OrderCreated]{
	Key:     orderID, // 分区/路由键（Kafka Key、RabbitMQ RoutingKey...）
	Payload: OrderCreated{OrderID: orderID, Amount: 100},
})
if err != nil {
	_ = tx.Rollback()
	return err
}
return tx.Commit()
```

### 启动 Relay

```go
store := outbox.NewStore(db)
relay := outbox.NewRelay(store, brokerImpl,
	outbox.WithPollInterval(time.Second),
	outbox.WithErrorHandler(func(stage string, row outbox.Row, err error) {
		log.Errorf("outbox %s %s: %v", stage, row.ID, err)
	}),
)
go relay.Run(ctx) // 阻塞运行，随 ctx 取消退出
```

### 消费者：类型化 + 去重

```go
// 写库效果：Barrier（恰好一次）——直接作为 broker.Handler 订阅
sub, _ := brokerImpl.Subscribe("OrderCreated",
	outbox.Barrier(db, "billing", func(ctx context.Context, tx *sql.Tx, e *OrderCreated) error {
		return charge(ctx, tx, e)
	}), nil)

// 非库副作用（发短信、调外部 API）：Subscribe 一步完成，内置 Inbox 去重
sub2, _ := outbox.Subscribe(brokerImpl, db, "notify", "OrderCreated",
	func(ctx context.Context, topic string, h broker.Headers, e *OrderCreated) error {
		return sendSMS(ctx, e)
	})
```

## 表结构与运维

`outbox.EnsureSchema(ctx, db)`（或 `relay.Run` 首次自动建表）创建两张表：

- `outbox_events`：发件箱，状态机 `pending → claimed → done`，超限进入 `dead`；
- `inbox_messages`：消费去重记录，主键 `(handler, event_id)`。

默认兼容 MySQL 8+ / MariaDB / PostgreSQL / SQLite；多实例并发 Relay 依赖 `SELECT ... FOR UPDATE SKIP LOCKED`（SQLite 或旧版 MySQL 用 `outbox.WithSkipLocked(false)` 关闭）。

运维要点：

- `WithMaxAttempts` + `WithRetryBackoff` 控制重试与死信（`status='dead'`，`last_error` 留痕）；
- `WithSweepAfter` 定期清理已完成行，或按 `completed_at` 自行清理；
- 主题名默认取事件类型名，可用 `Event.Topic` 覆盖；事件 ID 缺省为 UUID v4，消费端按它去重。

## 与其他事务模式的关系

| | transaction/dtm | transaction/outbox | transaction/saga | transaction/tcc | transaction/xa |
|---|---|---|---|---|---|
| 一致性模型 | Saga/TCC/XA/二阶段消息 | 二阶段消息（Outbox） | 进程内补偿 | 业务层预留 | 数据库两阶段提交 |
| 外部组件 | DTM Server | 无（复用业务库 + broker） | 无 | 无 | 无 |
| 载荷类型 | `interface{}` | 泛型，编译期检查 | 泛型状态 | 泛型状态 | 按资源命名 |
| 适用场景 | 强补偿回滚、跨服务编排 | 最终一致事件、跨服务通知 | 进程内回滚 | 进程内预留-确认 | 多库原子提交（短事务） |

选型顺序：能用本地事务就不用分布式；最终一致够用选 `outbox`；进程内回滚选 `saga`、预留-确认选 `tcc`；多库强一致短事务选 `xa`；跨进程持久编排选 `workflow/temporal`；以上都不满足再回到 `transaction/dtm`。

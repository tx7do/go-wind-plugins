# XA

多数据库 XA 两阶段提交协调器（MySQL / MariaDB）。与 `transaction/outbox`（最终一致）、`transaction/tcc`（业务层预留）不同，XA 在**数据库层**原子提交或回滚所有参与库——强一致，代价是阻塞锁与更低吞吐。只用于"短小、少分支、必须原子"的写操作。

## 使用

```go
err := xa.Run(ctx,
	func(ctx context.Context, s *xa.Session) error {
		if _, err := s.Exec(ctx, "orders",
			"UPDATE orders SET paid = 1 WHERE id = ?", "o-1"); err != nil {
			return err
		}
		_, err := s.Exec(ctx, "stocks",
			"UPDATE stocks SET locked = locked + 1 WHERE sku = ?", "sku-1")
		return err
	},
	xa.Resource{Name: "orders", DB: orderDB},
	xa.Resource{Name: "stocks", DB: stockDB},
)
```

## 协议流程

```
XA START 'windxa-…'     ×N（每个资源一条专用连接）
业务 SQL                ×N
XA END                  ×N
XA PREPARE              ×N   ← 任一拒绝：全部 XA ROLLBACK
XA COMMIT               ×N   ← 全部成功才逐个提交
```

任一资源在 PREPARE 前失败 → 全部回滚，函数返回错误；全部 PREPARE 成功后进入提交阶段，此时事务处于"两阶段提交已定"状态。

## 孤儿事务恢复

进程在 PREPARE 之后、COMMIT 之前崩溃（或提交确认丢失）时，数据库中会残留 prepared 状态的事务。本包铸造的 XID 一律带 `windxa-` 前缀，恢复工具只认自家的 XID：

```go
xids, _ := xa.Pending(ctx, db)        // XA RECOVER，过滤出 windxa- 前缀
xa.CommitPending(ctx, db, xids[0])    // 或 xa.RollbackPending
```

`CommitPending`/`RollbackPending` 会拒绝任何非 `windxa-` + `[0-9a-f-]` 字符集的 XID，防止注入外部语句。

## 适用边界

- 仅 MySQL / MariaDB 实现 XA 协议（PostgreSQL 的 `PREPARE TRANSACTION` 语义不同，未纳入）；XA 语句不支持占位符，本包自行铸造安全的 XID 后内插。
- XA 会长时间持有行锁，跨库锁依赖可能放大死锁——参与库越少越好，事务体越短越好。
- 能用 `transaction/outbox`（最终一致）或 `transaction/tcc`（业务预留）解决的场景，不建议使用 XA。

## 测试说明

单元测试基于 `go-sqlmock` 断言语句序列（START/END/PREPARE/COMMIT/ROLLBACK 与恢复流程），未连接真实 MySQL；接入生产前请用真实双库环境验证一轮。

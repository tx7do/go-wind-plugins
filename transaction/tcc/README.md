# TCC

极简泛型 TCC（Try-Confirm-Cancel）协调器：每个参与方先预留资源（Try），全部成功后统一确认（Confirm），任一失败则全部释放（Cancel）。纯进程内、零依赖，全程编译期类型安全。

## 使用

```go
type orderState struct {
	OrderID      string
	FrozenFundID string
	LockedStock  string
}

err := tcc.Run(ctx, &orderState{},
	tcc.Participant[orderState]{
		Name: "fund",
		Try: func(ctx context.Context, s *orderState) error {
			s.FrozenFundID = freezeFunds(ctx, s.OrderID) // 预留资金
			return nil
		},
		Confirm: func(ctx context.Context, s *orderState) error {
			return settleFunds(ctx, s.FrozenFundID) // 确认扣款
		},
		Cancel: func(ctx context.Context, s *orderState) error {
			return unfreezeFunds(ctx, s.FrozenFundID) // 释放资金；幂等
		},
	},
	tcc.Participant[orderState]{Name: "stock", Try: ..., Confirm: ..., Cancel: ...},
)
```

## 语义（与 DTM TCC 对齐）

- **空补偿**：任一 Try 失败时，Cancel 会作用于**所有已尝试**的参与方——包括失败的那个。因此 `Cancel` 必须容忍"Try 从未成功/未写入预留标识"的状态：幂等 + 无预留时 no-op。
- **Confirm 不回头**：全部 Try 成功后进入 Confirm 阶段，绝不再 Cancel。这是 TCC 的核心不变量——Confirm 不允许业务失败；若确实失败，`Run` 会继续完成其余参与方的 Confirm 并把所有 Confirm 错误经 `errors.Join` 返回，交由重试/告警兜底。
- 预留标识由 Try 写入共享状态 `S`，Confirm/Cancel 从状态读取——这就是类型安全的"资源句柄"。

## 适用边界

本包是**进程内**协调：程序中途整体崩溃会留下未决预留，需人工或定时任务清理。跨服务持久化 TCC 请使用 `workflow/temporal` 或 `transaction/dtm`；若是"预留-确认"语义能降级为"最终一致"，优先考虑 `transaction/outbox`。

| 场景 | 推荐方案 |
|---|---|
| 参与方在同一进程内 | 本包 `tcc.Run` |
| 最终一致的事件通知（跨服务） | `transaction/outbox` |
| 多库原子提交（强一致，短事务） | `transaction/xa` |
| 跨进程持久化编排 | `workflow/temporal` / `transaction/dtm` |

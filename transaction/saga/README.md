# Saga

极简泛型 Saga 编排器：按序执行带补偿的步骤，失败时逆序补偿已完成的步骤。纯进程内、零依赖，全程编译期类型安全。

## 使用

```go
type orderState struct {
	OrderID   string
	PaymentID string
}

err := saga.Run(ctx, &orderState{},
	saga.Step[orderState]{
		Name: "create-order",
		Do: func(ctx context.Context, s *orderState) error {
			s.OrderID = createOrder(ctx)
			return nil
		},
		Undo: func(ctx context.Context, s *orderState) error {
			return deleteOrder(ctx, s.OrderID)
		},
	},
	saga.Step[orderState]{
		Name: "charge",
		Do: func(ctx context.Context, s *orderState) error {
			return charge(ctx, s.OrderID)
		},
		// 无需补偿的步骤可省略 Undo
	},
)
```

失败语义：

- 只有 `Do` 已成功的步骤会被补偿，逆序执行；
- 某个 `Undo` 失败不会中断其余补偿——所有补偿错误与原始错误一起经 `errors.Join` 返回；
- `Undo` 必须幂等（崩溃恢复或手动重试可能重复调用）。

## 适用边界

本包是**进程内**编排：程序在步骤中途整体崩溃则状态丢失。按需求选择：

| 场景 | 推荐方案 |
|---|---|
| 步骤在同一进程内完成 | 本包 `saga.Run` |
| 最终一致的事件通知（跨服务） | `transaction/outbox` |
| 跨进程持久化编排、长事务、人工介入 | `workflow/temporal` |
| 强补偿回滚且接受外部 Server | `transaction/dtm` |

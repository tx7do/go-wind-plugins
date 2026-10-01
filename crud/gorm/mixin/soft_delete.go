package mixin

// SoftDelete 只负责软删除标记（deleted_at）；删除者字段（deleted_by）
// 由 OperatorID 提供，避免同时嵌入两者时字段重复。
type SoftDelete struct {
	DeletedAt
}

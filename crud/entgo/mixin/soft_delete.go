package mixin

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/mixin"

	"github.com/tx7do/go-wind-plugins/crud/entgo/interceptor"
)

// SoftDelete 只负责软删除标记（deleted_at）与查询过滤拦截器；
// 删除者字段（deleted_by）由 OperatorID / AuditorID 提供，避免同时
// 嵌入两者时字段重名导致 codegen 失败。
var _ ent.Mixin = (*SoftDelete)(nil)

type SoftDelete struct {
	mixin.Schema
}

func (SoftDelete) Fields() []ent.Field {
	return DeletedAt{}.Fields()
}

func (SoftDelete) Interceptors() []ent.Interceptor {
	return []ent.Interceptor{
		interceptor.SoftDeleteInterceptor(),
	}
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

var _ ent.Mixin = (*SoftDelete64)(nil)

type SoftDelete64 struct {
	mixin.Schema
}

func (SoftDelete64) Fields() []ent.Field {
	return DeletedAt{}.Fields()
}

func (SoftDelete64) Interceptors() []ent.Interceptor {
	return []ent.Interceptor{
		interceptor.SoftDeleteInterceptor(),
	}
}

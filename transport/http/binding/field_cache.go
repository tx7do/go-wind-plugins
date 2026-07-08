package binding

import (
	"reflect"
	"strings"
	"sync"
)

// 本文件提供结构体字段元数据缓存，消除 findField 中反复的字符串分配
// （strings.Split 解析 json tag 占了 BindQuery 65% 的分配）。
//
// 缓存按 reflect.Type 索引，预计算两份查找表：
//   - byJSON: json tag 名 → 字段 index（最高优先级）
//   - byLower: 字段名小写 → 字段 index（大小写不敏感回退）
// camelCase 回退仍走运行时 toCamelCase + v.FieldByName（标准库，非热点）。

// structMeta 是单个 struct 类型的字段查找元数据。
type structMeta struct {
	byJSON  map[string]int // json tag 名（已去除 ,omitempty 等选项）→ 字段 index
	byLower map[string]int // 字段名小写 → 字段 index（仅未冲突时注册，保证优先级）
}

var metaCache sync.Map // map[reflect.Type]*structMeta

// lookupFieldMeta 按 type 获取（或构建）字段元数据。
func lookupFieldMeta(t reflect.Type) *structMeta {
	if cached, ok := metaCache.Load(t); ok {
		return cached.(*structMeta)
	}
	m := buildFieldMeta(t)
	actual, _ := metaCache.LoadOrStore(t, m)
	return actual.(*structMeta)
}

// buildFieldMeta 遍历 struct 字段，预计算两份查找表。
//
// 优先级处理：byLower 只在该小写名"未被占用"时注册。
// 这样若两个字段名小写相同（极少见），先出现的优先（与原 findField 大小写不敏感
// 遍历的"先到先得"语义一致）。byJSON 不去重，因为 json tag 唯一性由用户保证。
func buildFieldMeta(t reflect.Type) *structMeta {
	m := &structMeta{
		byJSON:  make(map[string]int, t.NumField()),
		byLower: make(map[string]int, t.NumField()),
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		// json tag：取逗号前的名字部分（去除 ,omitempty 等选项）。
		if tag := field.Tag.Get("json"); tag != "" {
			name := tag
			if idx := strings.IndexByte(tag, ','); idx >= 0 {
				name = tag[:idx]
			}
			if name != "" && name != "-" { // "-" 表示显式忽略
				m.byJSON[name] = i
			}
		}
		// 大小写不敏感：字段名小写（仅未占用时注册，保证先到先得）。
		lower := strings.ToLower(field.Name)
		if _, exists := m.byLower[lower]; !exists {
			m.byLower[lower] = i
		}
	}
	return m
}

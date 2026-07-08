package binding

import (
	"net/url"
	"testing"
)

// 构造一个中等规模的 query（5 个 string 字段 + 1 个多值 slice），
// 使 benchmark 接近真实使用场景而非退化到空操作。

func makeQuery() url.Values {
	q := url.Values{}
	q.Set("name", "alice")
	q.Set("email", "alice@example.com")
	q.Set("role", "admin")
	q.Set("status", "active")
	q.Set("token", "abcdef0123456789")
	q.Add("tags", "go")
	q.Add("tags", "rust")
	q.Add("tags", "zig")
	return q
}

type benchStruct struct {
	Name   string   `json:"name"`
	Email  string   `json:"email"`
	Role   string   `json:"role"`
	Status string   `json:"status"`
	Token  string   `json:"token"`
	Tags   []string `json:"tags"`
}

// BenchmarkBindQuery 测完整路径：query → struct。
func BenchmarkBindQuery(b *testing.B) {
	q := makeQuery()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var got benchStruct
		if err := BindQuery(&got, q); err != nil {
			b.Fatal(err)
		}
	}
}

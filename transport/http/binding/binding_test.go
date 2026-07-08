package binding

import (
	"net/url"
	"testing"
)

// 这套测试定义 BindQuery 的正确行为契约（反射直填，与 BindPath 一致）。
//
// 行为约定（与主流框架 gin/echo 的 form binding 对齐）：
//   - 单值 → 按字段类型转换（string/int/uint/float/bool）
//   - 多值 → []string / []T
//   - 单值也能填进 slice 字段（自动包成单元素 slice）
//   - 字段匹配：json tag → camelCase 回退 → 大小写不敏感（与 BindPath 统一）

// --- 扁平结构体：string 字段 ---

type flatString struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func TestBindQuery_StringFields(t *testing.T) {
	q := url.Values{}
	q.Set("name", "alice")
	q.Set("email", "a@b.com")

	var got flatString
	if err := BindQuery(&got, q); err != nil {
		t.Fatalf("BindQuery: %v", err)
	}
	if got.Name != "alice" {
		t.Errorf("Name = %q, want %q", got.Name, "alice")
	}
	if got.Email != "a@b.com" {
		t.Errorf("Email = %q, want %q", got.Email, "a@b.com")
	}
}

// --- 标量类型转换：int / uint / float / bool ---
// （JSON 中转方案对这些会报错；反射直填能正确处理）

type scalars struct {
	Age    int     `json:"age"`
	Count  uint    `json:"count"`
	Score  float64 `json:"score"`
	Active bool    `json:"active"`
}

func TestBindQuery_ScalarTypes(t *testing.T) {
	q := url.Values{}
	q.Set("age", "21")
	q.Set("count", "7")
	q.Set("score", "98.5")
	q.Set("active", "true")

	var got scalars
	if err := BindQuery(&got, q); err != nil {
		t.Fatalf("BindQuery: %v", err)
	}
	if got.Age != 21 {
		t.Errorf("Age = %d, want 21", got.Age)
	}
	if got.Count != 7 {
		t.Errorf("Count = %d, want 7", got.Count)
	}
	if got.Score != 98.5 {
		t.Errorf("Score = %v, want 98.5", got.Score)
	}
	if !got.Active {
		t.Errorf("Active = false, want true")
	}
}

func TestBindQuery_InvalidInt(t *testing.T) {
	q := url.Values{}
	q.Set("age", "not-a-number")
	var got scalars
	if err := BindQuery(&got, q); err == nil {
		t.Error("expected error for non-numeric age, got nil")
	}
}

// --- 多值 → slice ---

type sliceStruct struct {
	Tags []string `json:"tags"`
}

func TestBindQuery_MultiValueSlice(t *testing.T) {
	q := url.Values{}
	q.Add("tags", "go")
	q.Add("tags", "rust")
	q.Add("tags", "zig")

	var got sliceStruct
	if err := BindQuery(&got, q); err != nil {
		t.Fatalf("BindQuery: %v", err)
	}
	if len(got.Tags) != 3 {
		t.Fatalf("Tags len = %d, want 3", len(got.Tags))
	}
	want := []string{"go", "rust", "zig"}
	for i, w := range want {
		if got.Tags[i] != w {
			t.Errorf("Tags[%d] = %q, want %q", i, got.Tags[i], w)
		}
	}
}

// --- 单值给 slice 字段：自动包成单元素 slice ---

func TestBindQuery_SingleValueToSlice(t *testing.T) {
	q := url.Values{}
	q.Set("tags", "go") // 单值

	var got sliceStruct
	if err := BindQuery(&got, q); err != nil {
		t.Fatalf("BindQuery: %v", err)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "go" {
		t.Errorf("Tags = %v, want [go]", got.Tags)
	}
}

// --- 空 query ---

func TestBindQuery_EmptyQuery(t *testing.T) {
	var got flatString
	if err := BindQuery(&got, url.Values{}); err != nil {
		t.Fatalf("BindQuery on empty: %v", err)
	}
	if got.Name != "" || got.Email != "" {
		t.Errorf("empty query should leave zero value, got %+v", got)
	}
}

// --- 空 values（key 存在但无值）---

func TestBindQuery_EmptyValues(t *testing.T) {
	q := url.Values{}
	q["name"] = []string{} // 显式空 slice

	var got flatString
	if err := BindQuery(&got, q); err != nil {
		t.Fatalf("BindQuery: %v", err)
	}
	if got.Name != "" {
		t.Errorf("empty values should be skipped, Name = %q", got.Name)
	}
}

// --- 字段名匹配：json tag → camelCase 回退（与 BindPath 统一）---

type tagAndCamel struct {
	UserName string `json:"user_name"` // 通过 tag 命中
	FullAddr string                  // 无 tag，通过 camelCase 命中 full_addr → FullAddr
}

func TestBindQuery_FieldNameMatching(t *testing.T) {
	q := url.Values{}
	q.Set("user_name", "bob") // 命中 json tag
	q.Set("full_addr", "x")   // 命中 toCamelCase("full_addr")=FullAddr

	var got tagAndCamel
	if err := BindQuery(&got, q); err != nil {
		t.Fatalf("BindQuery: %v", err)
	}
	if got.UserName != "bob" {
		t.Errorf("UserName = %q, want bob", got.UserName)
	}
	if got.FullAddr != "x" {
		t.Errorf("FullAddr = %q, want x", got.FullAddr)
	}
}

// --- 未匹配的 key 被忽略（不报错）---

func TestBindQuery_UnmatchedKeyIgnored(t *testing.T) {
	q := url.Values{}
	q.Set("unknown", "val")
	q.Set("name", "alice")

	var got flatString
	if err := BindQuery(&got, q); err != nil {
		t.Fatalf("BindQuery: %v", err)
	}
	if got.Name != "alice" {
		t.Errorf("Name = %q, want alice", got.Name)
	}
}

// --- 非 ptr req ---

func TestBindQuery_NonPointerReq(t *testing.T) {
	q := url.Values{}
	q.Set("name", "alice")
	var got flatString
	// 传值而非指针，应返回错误
	if err := BindQuery(got, q); err == nil {
		t.Error("expected error for non-pointer req, got nil")
	}
}

// --- 多字段混合 ---

type mixed struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
	Code string   `json:"code"`
}

func TestBindQuery_MixedFields(t *testing.T) {
	q := url.Values{}
	q.Set("name", "alice")
	q.Add("tags", "go")
	q.Add("tags", "rust")
	q.Set("code", "42")

	var got mixed
	if err := BindQuery(&got, q); err != nil {
		t.Fatalf("BindQuery: %v", err)
	}
	if got.Name != "alice" {
		t.Errorf("Name = %q", got.Name)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "go" || got.Tags[1] != "rust" {
		t.Errorf("Tags = %v", got.Tags)
	}
	if got.Code != "42" {
		t.Errorf("Code = %q", got.Code)
	}
}

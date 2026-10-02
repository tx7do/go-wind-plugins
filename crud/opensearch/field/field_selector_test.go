package field

import (
	"strings"
	"testing"

	"github.com/tx7do/go-wind-plugins/crud/opensearch/query"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// TestNewFieldSelector 确认构造器返回可用实例。
func TestNewFieldSelector(t *testing.T) {
	if NewFieldSelector() == nil {
		t.Fatal("NewFieldSelector must return a selector")
	}
}

// TestBuildSelector_NilBuilder nil builder 原样返回 nil（无错误）。
func TestBuildSelector_NilBuilder(t *testing.T) {
	out, err := (Selector{}).BuildSelector(nil, []string{"name"})
	if out != nil || err != nil {
		t.Fatalf("nil builder must return nil,nil, got %v,%v", out, err)
	}
}

// TestBuildSelector_EmptyFields 空 fields 不设置 _source。
func TestBuildSelector_EmptyFields(t *testing.T) {
	b := query.NewQueryBuilder()
	out, err := NewFieldSelector().BuildSelector(b, nil)
	if err != nil || out != b {
		t.Fatalf("empty fields must pass builder through, got %v,%v", out, err)
	}
	if b.Build()[_sourceKey] != nil {
		t.Fatal("_source must stay unset for empty fields")
	}
}

const _sourceKey = "_source"

// TestBuildSelector_NormalizesAndSnakes 验证字段规范化：空白清理、驼峰转
// snake_case、嵌套路径仅首段转换。
func TestBuildSelector_NormalizesAndSnakes(t *testing.T) {
	b := query.NewQueryBuilder()
	out, err := NewFieldSelector().BuildSelector(b, []string{"userName", "  userID  ", "meta.userName"})
	if err != nil || out != b {
		t.Fatalf("build selector failed: %v,%v", out, err)
	}
	dsl := b.Build()
	source, ok := dsl[_sourceKey].([]string)
	if !ok {
		t.Fatalf("_source must be set, got %v", dsl[_sourceKey])
	}
	joined := strings.Join(source, ",")
	for _, want := range []string{"user_name", "user_id", "meta.userName"} {
		if !strings.Contains(joined, want) {
			t.Errorf("_source %v must contain %q", source, want)
		}
	}

	// "*" passes NormalizePaths but is rejected by the selector whitelist
	b = query.NewQueryBuilder()
	if _, err = NewFieldSelector().BuildSelector(b, []string{"*"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.Build()[_sourceKey] != nil {
		t.Error("wildcard alone must not produce a _source projection")
	}
}

// TestBuildSelector_HostileFieldsDropped 非法字段被丢弃；全部非法时不设置 _source。
func TestBuildSelector_HostileFieldsDropped(t *testing.T) {
	// 部分非法：合法的保留
	b := query.NewQueryBuilder()
	_, err := NewFieldSelector().BuildSelector(b, []string{"name", `a"b`, "x;y"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	source := b.Build()[_sourceKey].([]string)
	if len(source) != 1 || source[0] != "name" {
		t.Fatalf("only the valid field must survive, got %v", source)
	}

	// 全部非法：_source 保持未设置
	b2 := query.NewQueryBuilder()
	out, err := NewFieldSelector().BuildSelector(b2, []string{`a"b`, "x;y"})
	if err != nil || out != b2 {
		t.Fatalf("all-invalid fields must pass through without error, got %v,%v", out, err)
	}
	if b2.Build()[_sourceKey] != nil {
		t.Fatal("_source must stay unset when every field is invalid")
	}
}

// TestNormalizeFieldMaskPaths 验证 FieldMask 路径规范化（snake 化由 fm.Normalize
// 与 NormalizePaths 共同完成；nil/空掩码为安全空操作）。
func TestNormalizeFieldMaskPaths(t *testing.T) {
	NormalizeFieldMaskPaths(nil) // must not panic

	empty := &fieldmaskpb.FieldMask{}
	NormalizeFieldMaskPaths(empty)

	fm := &fieldmaskpb.FieldMask{Paths: []string{"userName", "meta.userID", "bad path"}}
	NormalizeFieldMaskPaths(fm)
	if len(fm.Paths) != 3 {
		t.Fatalf("path count must be preserved, got %v", fm.Paths)
	}
	// fm.Normalize() 先排序并转为 JSON 名，NormalizePaths 再按段做白名单校验
	if fm.Paths[0] != "" || fm.Paths[1] != "meta.userID" || fm.Paths[2] != "userName" {
		t.Errorf("unexpected normalized paths, got %v", fm.Paths)
	}
	// 含空格的段未过白名单 → 整条置空
	if fm.Paths[0] != "" {
		t.Errorf("hostile path must be dropped, got %q", fm.Paths[0])
	}
}

// TestNormalizePaths_EdgeCases 补充空白与通配符分支。
func TestNormalizePaths_EdgeCases(t *testing.T) {
	out := NormalizePaths([]string{"  name  ", "", "a.*.b", "1abc"})
	if out[0] != "name" {
		t.Errorf("trimmed path expected, got %q", out[0])
	}
	if out[1] != "" {
		t.Errorf("blank path must stay blank, got %q", out[1])
	}
	if out[2] != "a.*.b" {
		t.Errorf("wildcard segments must be preserved, got %q", out[2])
	}
	if out[3] != "" {
		t.Errorf("segment starting with a digit must be dropped, got %q", out[3])
	}
}

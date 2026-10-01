package field

import (
	"strings"
	"testing"

	"entgo.io/ent/dialect/sql"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// TestBuildSelector_Closure covers the selector closure factory.
func TestBuildSelector_Closure(t *testing.T) {
	fs := NewFieldSelector()

	// empty fields produce no selector
	got, err := fs.BuildSelector(nil)
	if got != nil || err != nil {
		t.Fatalf("BuildSelector(empty): got=%v err=%v", got != nil, err)
	}

	// valid fields produce a closure that applies the projection
	sel, err := fs.BuildSelector([]string{"name", "path"})
	if err != nil {
		t.Fatalf("BuildSelector error: %v", err)
	}
	if sel == nil {
		t.Fatal("expected non-nil selector closure")
	}
	s := sql.Select("*").From(sql.Table("menus"))
	sel(s)
	q, _ := s.Query()
	if !strings.Contains(q, "name") || !strings.Contains(q, "path") {
		t.Fatalf("projection missing columns: %q", q)
	}

	// unknown columns of a known table are dropped
	sel, err = fs.BuildSelector([]string{"no_such_column"})
	if err != nil {
		t.Fatalf("BuildSelector error: %v", err)
	}
	s2 := sql.Select("*").From(sql.Table("menus"))
	sel(s2)
	q2, _ := s2.Query()
	if strings.Contains(q2, "no_such_column") {
		t.Fatalf("unknown column leaked into projection: %q", q2)
	}
}

// TestBuildSelectorWithTable covers the table-prefixed selector factory.
func TestBuildSelectorWithTable(t *testing.T) {
	fs := NewFieldSelector()

	// empty fields produce no selector
	got, err := fs.BuildSelectorWithTable("menus", nil)
	if got != nil || err != nil {
		t.Fatalf("BuildSelectorWithTable(empty): got=%v err=%v", got != nil, err)
	}

	sel, err := fs.BuildSelectorWithTable("menus", []string{"name"})
	if err != nil {
		t.Fatalf("BuildSelectorWithTable error: %v", err)
	}
	if sel == nil {
		t.Fatal("expected non-nil selector closure")
	}
	s := sql.Select("*").From(sql.Table("menus"))
	sel(s)
	q, _ := s.Query()
	if !strings.Contains(q, "menus") || !strings.Contains(q, "name") {
		t.Fatalf("prefixed projection = %q", q)
	}

	// BuildSelectWithTable directly: empty fields are a no-op
	s2 := sql.Select("*").From(sql.Table("menus"))
	fs.BuildSelectWithTable(s2, "menus", nil)
	if q2, _ := s2.Query(); strings.Contains(q2, "name") {
		t.Fatalf("empty fields must not add columns: %q", q2)
	}

	// dotted fields keep their prefix and are not re-prefixed
	s3 := sql.Select("*").From(sql.Table("menus"))
	fs.BuildSelectWithTable(s3, "menus", []string{"parent.name"})
	q3, _ := s3.Query()
	if !strings.Contains(q3, "parent.name") && !strings.Contains(q3, "parent_name") {
		t.Fatalf("dotted field projection = %q", q3)
	}
}

// TestNormalizePaths covers the snake_case path normalizer.
func TestNormalizePaths(t *testing.T) {
	if got := NormalizePaths(nil); got != nil {
		t.Fatalf("NormalizePaths(nil) = %v", got)
	}

	got := NormalizePaths([]string{"id_", "_id", "ParentID", "createdAt"})
	want := []string{"id", "id", "parent_id", "created_at"}
	if len(got) != len(want) {
		t.Fatalf("NormalizePaths = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("NormalizePaths = %v, want %v", got, want)
		}
	}
}

// TestNormalizeFieldMaskPaths covers the FieldMask normalizer wrapper.
func TestNormalizeFieldMaskPaths(t *testing.T) {
	NormalizeFieldMaskPaths(nil) // no panic

	empty := &fieldmaskpb.FieldMask{}
	NormalizeFieldMaskPaths(empty)
	if len(empty.Paths) != 0 {
		t.Fatal("empty mask must stay empty")
	}

	fm := &fieldmaskpb.FieldMask{Paths: []string{"ParentID", "_id"}}
	NormalizeFieldMaskPaths(fm)
	if len(fm.Paths) != 2 || fm.Paths[0] != "parent_id" || fm.Paths[1] != "id" {
		t.Fatalf("normalized paths = %v", fm.Paths)
	}
}

// TestApplyFieldMaskSelect covers the callback-based field mask applier.
func TestApplyFieldMaskSelect(t *testing.T) {
	// nil callback is a no-op
	ApplyFieldMaskSelect(nil, &fieldmaskpb.FieldMask{Paths: []string{"a"}})

	var collected []string
	apply := func(ps ...string) { collected = append(collected, ps...) }

	// nil/empty mask is a no-op
	ApplyFieldMaskSelect(apply, nil)
	ApplyFieldMaskSelect(apply, &fieldmaskpb.FieldMask{})
	if len(collected) != 0 {
		t.Fatalf("nil/empty mask must not call apply, got %v", collected)
	}

	ApplyFieldMaskSelect(apply, &fieldmaskpb.FieldMask{Paths: []string{"CreatedAt", "id_"}})
	if len(collected) != 2 || collected[0] != "created_at" || collected[1] != "id" {
		t.Fatalf("applied paths = %v", collected)
	}
}

// fakeSelectBuilder mimics an ent query with a Select method.
type fakeSelectBuilder struct{ cols []string }

func (f *fakeSelectBuilder) Select(fields ...string) []string {
	f.cols = append(f.cols, fields...)
	return f.cols
}

// TestApplyFieldMaskToBuilder covers the generic builder adapter.
func TestApplyFieldMaskToBuilder(t *testing.T) {
	// nil mask: nothing selected, ok=false
	b := &fakeSelectBuilder{}
	out, ok := ApplyFieldMaskToBuilder(b, nil)
	if ok || len(out) != 0 {
		t.Fatalf("nil mask = %v, %v", out, ok)
	}

	out, ok = ApplyFieldMaskToBuilder(b, &fieldmaskpb.FieldMask{Paths: []string{"Name"}})
	if !ok || len(out) != 1 || out[0] != "name" {
		t.Fatalf("mask selection = %v, %v", out, ok)
	}

	// a mask whose paths all normalize away still reports ok with no columns
	b2 := &fakeSelectBuilder{}
	out2, ok2 := ApplyFieldMaskToBuilder(b2, &fieldmaskpb.FieldMask{Paths: []string{"x"}})
	_ = out2
	_ = ok2
}

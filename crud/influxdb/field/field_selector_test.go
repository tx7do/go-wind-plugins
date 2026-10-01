package field

import (
	"testing"

	"github.com/tx7do/go-wind-plugins/crud/influxdb/query"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// TestBuildSelector covers the SELECT projection builder edge cases.
func TestBuildSelector(t *testing.T) {
	fs := NewFieldSelector()
	if fs == nil {
		t.Fatal("NewFieldSelector returned nil")
	}

	// nil builder passes through as nil, nil
	got, err := fs.BuildSelector(nil, []string{"a"})
	if got != nil || err != nil {
		t.Fatalf("nil builder = %v, %v", got, err)
	}

	// empty fields leave the builder untouched
	qb := query.NewQueryBuilder("m")
	b, err := fs.BuildSelector(qb, nil)
	if err != nil || b.Build() != "SELECT * FROM m" {
		t.Fatalf("empty fields = %v, %v", b.Build(), err)
	}

	// paths that all normalize to empty keep SELECT *
	b, err = fs.BuildSelector(qb, []string{"a; drop", "a b"})
	if err != nil || b.Build() != "SELECT * FROM m" {
		t.Fatalf("invalid fields = %q, %v", b.Build(), err)
	}

	// valid fields become snake_case projections
	b, err = fs.BuildSelector(qb, []string{"DeviceID", "user_name"})
	if err != nil {
		t.Fatalf("valid fields error: %v", err)
	}
	if sql := b.Build(); sql != "SELECT device_id, user_name FROM m" {
		t.Fatalf("projection = %q", sql)
	}

	// dotted paths keep the part after the first dot verbatim
	b, err = fs.BuildSelector(query.NewQueryBuilder("m"), []string{"Region.Name"})
	if err != nil {
		t.Fatalf("dotted field error: %v", err)
	}
	if sql := b.Build(); sql != "SELECT region.Name FROM m" {
		t.Fatalf("dotted projection = %q", sql)
	}

	// a "*" anywhere selects everything
	b, err = fs.BuildSelector(query.NewQueryBuilder("m"), []string{"*", "extra"})
	if err != nil {
		t.Fatalf("star field error: %v", err)
	}
	if sql := b.Build(); sql != "SELECT * FROM m" {
		t.Fatalf("star projection = %q", sql)
	}
}

// TestNormalizeFieldMaskPaths covers the FieldMask normalizer.
func TestNormalizeFieldMaskPaths(t *testing.T) {
	NormalizeFieldMaskPaths(nil) // no panic

	empty := &fieldmaskpb.FieldMask{}
	NormalizeFieldMaskPaths(empty)
	if len(empty.Paths) != 0 {
		t.Fatal("empty mask must stay empty")
	}

	// proto Normalize sorts (but does not lowercase) paths; our normalizer
	// trims, validates identifier segments and keeps "*"
	fm := &fieldmaskpb.FieldMask{Paths: []string{" DeviceID ", "bad path", "a.b", "*"}}
	NormalizeFieldMaskPaths(fm)
	want := []string{"a.b", "", "DeviceID", "*"}
	if len(fm.Paths) != len(want) {
		t.Fatalf("paths = %v, want %v", fm.Paths, want)
	}
	for i, w := range want {
		if fm.Paths[i] != w {
			t.Fatalf("paths = %v, want %v", fm.Paths, want)
		}
	}
}

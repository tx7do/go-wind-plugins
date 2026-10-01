package doris

import (
	"reflect"
	"testing"
	"time"
)

type innerStruct struct {
	A int `db:"a"`
}

type namedString string

// TestStructToColumnsAndValues walks the field-type matrix of the unexported
// struct extractor (used by map/proto based inserts).
func TestStructToColumnsAndValues(t *testing.T) {
	type Demo struct {
		Explicit   int            `db:"explicit"`
		ByJSON     string         `json:"by_json"`
		ByJSONAlt  string         `json:"by_json_alt,omitempty"`
		ByName     string         // falls back to lowercased field name
		Meta       map[string]any `db:"meta"`
		NilMap     map[string]any `db:"nil_map"`
		Tags       []string       `db:"tags"`
		NilTags    []string       `db:"nil_tags"`
		EmptyTags  []string       `db:"empty_tags"`
		Arr        [2]int         `db:"arr"`
		When       time.Time      `db:"when"`
		Inner      innerStruct    `db:"inner"`
		InnerPtr   *innerStruct   `db:"inner_ptr"`
		NilInner   *innerStruct   `db:"nil_inner"`
		WhenPtr    *time.Time     `db:"when_ptr"`
		NamedVal   namedString    `db:"named_val"`
		PlainPtr   *int           `db:"plain_ptr"`
		NilPlain   *int           `db:"nil_plain"`
		unexported string         // skipped
		Skipped    string         `db:"-"`
	}

	when := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	n := 5
	demo := Demo{
		Explicit:  1,
		ByJSON:    "j",
		ByJSONAlt: "ja",
		ByName:    "n",
		Meta:      map[string]any{"k": "v"},
		Tags:      []string{"x"},
		Arr:       [2]int{1, 2},
		When:      when,
		Inner:     innerStruct{A: 9},
		InnerPtr:  &innerStruct{A: 8},
		WhenPtr:   &when,
		NamedVal:  namedString("nv"),
		PlainPtr:  &n,
	}

	cols, vals, err := structToColumnsAndValues(reflect.ValueOf(demo))
	if err != nil {
		t.Fatalf("structToColumnsAndValues error: %v", err)
	}

	get := func(name string) (any, bool) {
		for i, c := range cols {
			if c == name {
				return vals[i], true
			}
		}
		return nil, false
	}

	if v, ok := get("explicit"); !ok || v != 1 {
		t.Errorf("explicit = %v (found=%v)", v, ok)
	}
	if v, ok := get("by_json"); !ok || v != "j" {
		t.Errorf("by_json = %v (found=%v)", v, ok)
	}
	if v, ok := get("by_json_alt"); !ok || v != "ja" {
		t.Errorf("by_json_alt = %v (found=%v)", v, ok)
	}
	if v, ok := get("byname"); !ok || v != "n" {
		t.Errorf("byname (fallback to lowercased name) = %v (found=%v)", v, ok)
	}
	if v, ok := get("meta"); !ok || v != `{"k":"v"}` {
		t.Errorf("meta must serialize to json, got %v (found=%v)", v, ok)
	}
	if v, ok := get("nil_map"); !ok || v != "null" {
		t.Errorf("nil map marshals to the literal \"null\", got %v (found=%v)", v, ok)
	}
	if v, ok := get("tags"); !ok || v != `["x"]` {
		t.Errorf("tags must serialize to json, got %v (found=%v)", v, ok)
	}
	if v, ok := get("nil_tags"); !ok || v != "[]" {
		t.Errorf("nil slice must become \"[]\", got %v (found=%v)", v, ok)
	}
	if v, ok := get("empty_tags"); !ok || v != "[]" {
		t.Errorf("empty slice must become \"[]\", got %v (found=%v)", v, ok)
	}
	if v, ok := get("arr"); !ok || v != `[1,2]` {
		t.Errorf("array must serialize to json, got %v (found=%v)", v, ok)
	}
	if v, ok := get("when"); !ok || v != when {
		t.Errorf("time.Time must pass through unchanged, got %v (found=%v)", v, ok)
	}
	if v, ok := get("inner"); !ok || v != `{"A":9}` {
		t.Errorf("inner struct must serialize to json, got %v (found=%v)", v, ok)
	}
	if v, ok := get("inner_ptr"); !ok || v == nil {
		t.Errorf("inner_ptr must be serialized (non-nil), got %v (found=%v)", v, ok)
	}
	if v, ok := get("nil_inner"); !ok || v != nil {
		t.Errorf("nil inner pointer must stay nil, got %v (found=%v)", v, ok)
	}
	if v, ok := get("when_ptr"); !ok || v == nil {
		t.Errorf("*time.Time must pass through (non-nil), got %v (found=%v)", v, ok)
	}
	if v, ok := get("named_val"); !ok || v != namedString("nv") {
		t.Errorf("named string type must pass through, got %v (found=%v)", v, ok)
	}
	if v, ok := get("plain_ptr"); !ok || v != &n {
		t.Errorf("non-struct pointer must pass through, got %v (found=%v)", v, ok)
	}
	if v, ok := get("nil_plain"); !ok || v != nil {
		t.Errorf("nil pointer must stay nil, got %v (found=%v)", v, ok)
	}
	// unexported and db:"-" fields are skipped
	for _, banned := range []string{"unexported", "skipped", ""} {
		for _, c := range cols {
			if c == banned {
				t.Errorf("column %q must be skipped", banned)
			}
		}
	}

	// pointer input is dereferenced
	cols2, _, err := structToColumnsAndValues(reflect.ValueOf(&demo))
	if err != nil {
		t.Fatalf("pointer input error: %v", err)
	}
	if !reflect.DeepEqual(cols, cols2) {
		t.Fatalf("pointer input changed columns: %v vs %v", cols2, cols)
	}

	// non-struct input is rejected
	if _, _, err := structToColumnsAndValues(reflect.ValueOf(42)); err == nil {
		t.Fatal("non-struct input must be rejected")
	}
	if _, _, err := structToColumnsAndValues(reflect.Value{}); err == nil {
		t.Fatal("invalid value input must be rejected")
	}
}

// TestMapToColumnsAndValues verifies flat maps pass through and nested maps
// serialize to json.
func TestMapToColumnsAndValues(t *testing.T) {
	cols, vals, err := mapToColumnsAndValues(map[string]any{
		"a": 1,
		"b": "x",
		"c": map[string]any{"k": "v"},
	})
	if err != nil {
		t.Fatalf("mapToColumnsAndValues error: %v", err)
	}
	if len(cols) != 3 || len(vals) != 3 {
		t.Fatalf("expected 3 columns/values, got %d/%d", len(cols), len(vals))
	}
	found := map[string]any{}
	for i, c := range cols {
		found[c] = vals[i]
	}
	if found["a"] != 1 || found["b"] != "x" {
		t.Fatalf("flat values mismatch: %v", found)
	}
	if found["c"] != `{"k":"v"}` {
		t.Fatalf("nested map must serialize to json, got %v", found["c"])
	}

	// marshal failures propagate (channels cannot be json encoded)
	if _, _, err := mapToColumnsAndValues(map[string]any{"bad": map[string]any{"ch": make(chan int)}}); err == nil {
		t.Fatal("json.Marshal failure must propagate")
	}
}

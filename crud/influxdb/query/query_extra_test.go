package query

import "testing"

type stringerValue struct{}

func (stringerValue) String() string { return "stringer-expr" }

// TestBuilder_WhereDispatch covers the generic Where dispatcher.
func TestBuilder_WhereDispatch(t *testing.T) {
	qb := NewQueryBuilder("m").Select(nil)

	// string routes to raw
	qb.Where("a = 1")
	// []string routes to raw
	qb.Where([]string{"b = 2", "WHERE c = 3"})
	// map routes to WhereFromMaps
	qb.Where(map[string]any{"d": 4})
	// unsupported types are ignored
	qb.Where(42)
	qb.Where(nil)

	got := qb.Build()
	want := "SELECT * FROM m WHERE a = 1 AND b = 2 AND c = 3 AND d = 4"
	if got != want {
		t.Fatalf("Build = %q, want %q", got, want)
	}
}

// TestBuilder_WhereFromAny covers the any-dispatcher including struct
// extraction via json tags.
func TestBuilder_WhereFromAny(t *testing.T) {
	qb := NewQueryBuilder("m")
	qb.WhereFromAny(nil, nil)
	if got := qb.Build(); got != "SELECT * FROM m" {
		t.Fatalf("nil filters changed the query: %q", got)
	}

	type cond struct {
		DeviceID string `json:"device_id"`
	}
	// note: field extraction keeps raw Go names for untagged fields and
	// includes zero values, so use fully tagged structs for stable output
	qb2 := NewQueryBuilder("m").WhereFromAny(&cond{DeviceID: "d1"},
		map[string]string{"device_id": "="})
	qb2.WhereFromAny(cond{DeviceID: "d2"}, nil) // value struct works too

	var nilPtr *cond
	qb3 := NewQueryBuilder("m")
	qb3.WhereFromAny(nilPtr, nil)
	if got := qb3.Build(); got != "SELECT * FROM m" {
		t.Fatalf("nil struct pointer changed the query: %q", got)
	}

	// scalar fallback becomes a raw fragment
	qb4 := NewQueryBuilder("m").WhereFromAny(42, nil)
	if got := qb4.Build(); got != "SELECT * FROM m WHERE 42" {
		t.Fatalf("scalar fallback = %q", got)
	}

	qb5 := NewQueryBuilder("m").WhereFromAny("", nil)
	if got := qb5.Build(); got != "SELECT * FROM m" {
		t.Fatalf("empty scalar fallback = %q", got)
	}

	sql := qb2.Build()
	// two conditions joined by AND; the single-condition set is deterministic
	if sql != "SELECT * FROM m WHERE device_id = 'd1' AND device_id = 'd2'" {
		t.Fatalf("struct conditions = %q", sql)
	}
}

// TestBuilder_WhereFromAnys covers the mixed-argument dispatcher.
func TestBuilder_WhereFromAnys(t *testing.T) {
	qb := NewQueryBuilder("m")
	qb.WhereFromAnys() // empty is a no-op

	qb.WhereFromAnys(
		nil, // skipped
		"a = 1",
		[]string{"b = 2"},
		[]any{"c = 3", map[string]any{"e": 5}},
		map[string]any{"f": 6},
		stringerValue{},
		7,
	)

	got := qb.Build()
	want := "SELECT * FROM m WHERE a = 1 AND b = 2 AND c = 3 AND e = 5 AND f = 6 AND stringer-expr AND 7"
	if got != want {
		t.Fatalf("WhereFromAnys = %q, want %q", got, want)
	}
}

// TestBuilder_WhereFromRaw covers prefix stripping and blank filtering.
func TestBuilder_WhereFromRaw(t *testing.T) {
	qb := NewQueryBuilder("m")
	qb.WhereFromRaw()
	qb.WhereFromRaw("", "   ", "WHERE a = 1", "b = 2")
	if got := qb.Build(); got != "SELECT * FROM m WHERE a = 1 AND b = 2" {
		t.Fatalf("WhereFromRaw = %q", got)
	}
}

// TestBuilder_OrderByAndPaging covers order-by variants and paging controls.
func TestBuilder_OrderByAndPaging(t *testing.T) {
	qb := NewQueryBuilder("m")
	qb.OrderBy("", true) // empty field is ignored
	qb.OrderBy("time", true)
	qb.OrderBy("tag", false)
	qb.Limit(10).Offset(20)

	got := qb.Build()
	want := "SELECT * FROM m ORDER BY time DESC, tag ASC LIMIT 10 OFFSET 20"
	if got != want {
		t.Fatalf("Build = %q, want %q", got, want)
	}

	// paging is omitted for aggregation queries
	noPaging := qb.BuildWithoutPaging()
	wantNoPaging := "SELECT * FROM m ORDER BY time DESC, tag ASC"
	if noPaging != wantNoPaging {
		t.Fatalf("BuildWithoutPaging = %q, want %q", noPaging, wantNoPaging)
	}

	// negative limit/offset are omitted
	minimal := NewQueryBuilder("m").Limit(-1).Offset(-1).Build()
	if minimal != "SELECT * FROM m" {
		t.Fatalf("negative paging = %q", minimal)
	}
}

// TestFormatValue walks the value formatter used inside where expressions.
func TestFormatValue(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, "NULL"},
		{"plain", "'plain'"},
		{"it's", `'it\'s'`},
		{`back\slash`, `'back\\slash'`},
		{true, "true"},
		{false, "false"},
		{42, "42"},
		{int8(1), "1"},
		{int16(2), "2"},
		{int32(3), "3"},
		{int64(4), "4"},
		{uint(5), "5"},
		{uint8(6), "6"},
		{uint16(7), "7"},
		{uint32(8), "8"},
		{uint64(9), "9"},
		{1.5, "1.5"},
		{float32(2.5), "2.5"},
		{[]string{"a", "b"}, "('a','b')"},
		{[]int{1, 2}, "(1,2)"},
		{[2]int{1, 2}, "(1,2)"},
		{stringerValue{}, "'stringer-expr'"},
		{complex(1, 2), "'(1+2i)'"}, // non-numeric, non-stringer falls back to %v
	}
	for _, tc := range cases {
		if got := formatValue(tc.in); got != tc.want {
			t.Errorf("formatValue(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestFormatRegex covers the /.../ regex wrapper and slash escaping.
func TestFormatRegex(t *testing.T) {
	if got := formatRegex("a/b"); got != `/a\/b/` {
		t.Errorf("formatRegex(string) = %q", got)
	}
	if got := formatRegex(42); got != "/42/" {
		t.Errorf("formatRegex(int) = %q", got)
	}
}

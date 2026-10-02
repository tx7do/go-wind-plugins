package filter

import (
	"reflect"
	"testing"

	bsonV2 "go.mongodb.org/mongo-driver/v2/bson"

	paginationV1 "github.com/tx7do/go-wind-plugins/crud/api/gen/go/pagination/v1"
	"github.com/tx7do/go-wind-plugins/crud/mongodb/query"
)

// filterOf returns a snapshot of the builder's current filter.
func filterOf(t *testing.T, qb *query.Builder) bsonV2.M {
	t.Helper()
	f, _ := qb.Build()
	return f
}

// strVal implements interface{ String() string } for Stringer code paths.
type strVal struct{ s string }

func (v strVal) String() string { return v.s }

// falseExpr is the永假 condition used by the processor for empty IN sets.
var falseExpr = bsonV2.M{"$expr": bsonV2.A{bsonV2.M{"$eq": bsonV2.A{1, 0}}}}

// ---------------------------------------------------------------------------
// makeKey — field-name mapping and json-key whitelisting
// ---------------------------------------------------------------------------

func TestMakeKey(t *testing.T) {
	tests := []struct {
		name  string
		field string
		want  string
	}{
		{"empty", "", ""},
		{"whitespace only", "   ", ""},
		{"simple", "Name", "name"},
		{"camel case", "userName", "user_name"},
		{"dot path", "preferences.daily_email", "preferences.daily_email"},
		{"dot path camel json key", "userProfile.avatarUrl", "user_profile.avatarUrl"},
		{"invalid json key space", "meta.avat ar", ""},
		{"invalid json key dollar", "meta.$meta", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proc := NewProcessor()
			if got := proc.makeKey(tt.field); got != tt.want {
				t.Fatalf("makeKey(%q) = %q, want %q", tt.field, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Process — nil builder / unknown operator
// ---------------------------------------------------------------------------

func TestProcess_NilBuilderReturnsNil(t *testing.T) {
	proc := NewProcessor()
	if got := proc.Process(nil, paginationV1.Operator_EQ, "name", "v", nil); got != nil {
		t.Fatalf("expected nil builder, got %v", got)
	}
}

func TestProcess_UnknownOperatorLeavesFilterUntouched(t *testing.T) {
	proc := NewProcessor()
	qb := &query.Builder{}
	got := proc.Process(qb, paginationV1.Operator(999), "name", "v", nil)
	if got != qb {
		t.Fatal("expected same builder returned")
	}
	if f := filterOf(t, qb); len(f) != 0 {
		t.Fatalf("expected empty filter, got %v", f)
	}
}

// ---------------------------------------------------------------------------
// Comparison operators
// ---------------------------------------------------------------------------

func TestComparisonOperators(t *testing.T) {
	proc := NewProcessor()
	tests := []struct {
		name  string
		op    paginationV1.Operator
		value any
		want  bsonV2.M
	}{
		{"EQ", paginationV1.Operator_EQ, "tom", bsonV2.M{"name": "tom"}},
		{"NEQ", paginationV1.Operator_NEQ, "tom", bsonV2.M{"name": bsonV2.M{"$ne": "tom"}}},
		{"GT", paginationV1.Operator_GT, 18, bsonV2.M{"name": bsonV2.M{"$gt": 18}}},
		{"GTE", paginationV1.Operator_GTE, 18, bsonV2.M{"name": bsonV2.M{"$gte": 18}}},
		{"LT", paginationV1.Operator_LT, 30, bsonV2.M{"name": bsonV2.M{"$lt": 30}}},
		{"LTE", paginationV1.Operator_LTE, 30, bsonV2.M{"name": bsonV2.M{"$lte": 30}}},
		{"IS_NULL", paginationV1.Operator_IS_NULL, nil, bsonV2.M{"name": nil}},
		{"IS_NOT_NULL", paginationV1.Operator_IS_NOT_NULL, nil, bsonV2.M{"name": bsonV2.M{"$ne": nil}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := &query.Builder{}
			got := proc.Process(qb, tt.op, "name", tt.value, nil)
			if got != qb {
				t.Fatal("expected same builder returned")
			}
			f := filterOf(t, qb)
			if !reflect.DeepEqual(f, tt.want) {
				t.Fatalf("got %#v, want %#v", f, tt.want)
			}
		})
	}
}

func TestOperators_InvalidKeyLeaveFilterUntouched(t *testing.T) {
	proc := NewProcessor()
	invalidFields := []string{"", "   ", "meta.avat ar", "meta.$meta"}

	ops := []paginationV1.Operator{
		paginationV1.Operator_EQ, paginationV1.Operator_NEQ,
		paginationV1.Operator_GT, paginationV1.Operator_GTE,
		paginationV1.Operator_LT, paginationV1.Operator_LTE,
		paginationV1.Operator_IN, paginationV1.Operator_NIN,
		paginationV1.Operator_BETWEEN,
		paginationV1.Operator_IS_NULL, paginationV1.Operator_IS_NOT_NULL,
		paginationV1.Operator_CONTAINS, paginationV1.Operator_ICONTAINS,
		paginationV1.Operator_STARTS_WITH, paginationV1.Operator_ISTARTS_WITH,
		paginationV1.Operator_ENDS_WITH, paginationV1.Operator_IENDS_WITH,
		paginationV1.Operator_EXACT, paginationV1.Operator_IEXACT,
		paginationV1.Operator_REGEXP, paginationV1.Operator_IREGEXP,
	}

	for _, field := range invalidFields {
		for _, op := range ops {
			qb := &query.Builder{}
			proc.Process(qb, op, field, "v", nil)
			if f := filterOf(t, qb); len(f) != 0 {
				t.Fatalf("field %q op %v: expected untouched filter, got %v", field, op, f)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// In
// ---------------------------------------------------------------------------

func TestIn(t *testing.T) {
	proc := NewProcessor()
	tests := []struct {
		name   string
		value  any
		values []any
		want   bsonV2.M // full expected filter; nil => filter must stay untouched
	}{
		{"json array", `["a","b"]`, nil, bsonV2.M{"f": bsonV2.M{"$in": []any{"a", "b"}}}},
		{"json empty array", `[]`, nil, falseExpr},
		{"comma separated", "a, b ,c", nil, bsonV2.M{"f": bsonV2.M{"$in": []any{"a", "b", "c"}}}},
		{"single string", "abc", nil, bsonV2.M{"f": bsonV2.M{"$in": []any{"abc"}}}},
		{"empty string falls back to values", "   ", []any{"x", "y"}, bsonV2.M{"f": bsonV2.M{"$in": []any{"x", "y"}}}},
		{"bytes json array", []byte(`["a"]`), nil, bsonV2.M{"f": bsonV2.M{"$in": []any{"a"}}}},
		{"bytes comma", []byte("a,b"), nil, bsonV2.M{"f": bsonV2.M{"$in": []any{"a", "b"}}}},
		{"bytes empty falls back to values", []byte(""), []any{"z"}, bsonV2.M{"f": bsonV2.M{"$in": []any{"z"}}}},
		{"slice of any", []any{1, 2}, nil, bsonV2.M{"f": bsonV2.M{"$in": []any{1, 2}}}},
		{"empty slice of any", []any{}, nil, falseExpr},
		{"nil value uses values", nil, []any{"q"}, bsonV2.M{"f": bsonV2.M{"$in": []any{"q"}}}},
		{"scalar fallback", 42, nil, bsonV2.M{"f": bsonV2.M{"$in": []any{42}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := &query.Builder{}
			proc.In(qb, "f", tt.value, tt.values)
			f := filterOf(t, qb)
			if tt.want == nil {
				if len(f) != 0 {
					t.Fatalf("expected untouched filter, got %v", f)
				}
				return
			}
			if !reflect.DeepEqual(f, tt.want) {
				t.Fatalf("got %#v, want %#v", f, tt.want)
			}
		})
	}
}

func TestIn_AllEmptyLeavesFilterUntouched(t *testing.T) {
	proc := NewProcessor()
	qb := &query.Builder{}
	proc.In(qb, "f", nil, nil)
	if f := filterOf(t, qb); len(f) != 0 {
		t.Fatalf("expected untouched filter, got %v", f)
	}
}

// ---------------------------------------------------------------------------
// NotIn
// ---------------------------------------------------------------------------

func TestNotIn(t *testing.T) {
	proc := NewProcessor()
	tests := []struct {
		name   string
		value  any
		values []any
		want   bsonV2.M // nil want => filter must stay untouched
	}{
		{"json array", `["a","b"]`, nil, bsonV2.M{"f": bsonV2.M{"$nin": []any{"a", "b"}}}},
		{"comma separated", "a,b", nil, bsonV2.M{"f": bsonV2.M{"$nin": []any{"a", "b"}}}},
		{"single string", "abc", nil, bsonV2.M{"f": bsonV2.M{"$nin": []any{"abc"}}}},
		{"empty string falls back to values", "", []any{"x"}, bsonV2.M{"f": bsonV2.M{"$nin": []any{"x"}}}},
		{"bytes json array", []byte(`["a"]`), nil, bsonV2.M{"f": bsonV2.M{"$nin": []any{"a"}}}},
		{"bytes comma", []byte("a,b"), nil, bsonV2.M{"f": bsonV2.M{"$nin": []any{"a", "b"}}}},
		{"bytes empty falls back to values", []byte(""), []any{"w"}, bsonV2.M{"f": bsonV2.M{"$nin": []any{"w"}}}},
		{"slice of any", []any{1}, nil, bsonV2.M{"f": bsonV2.M{"$nin": []any{1}}}},
		{"json empty array adds nothing", `[]`, nil, nil},
		{"empty slice of any adds nothing", []any{}, nil, nil},
		{"nil value and values adds nothing", nil, nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := &query.Builder{}
			proc.NotIn(qb, "f", tt.value, tt.values)
			f := filterOf(t, qb)
			if tt.want == nil {
				if len(f) != 0 {
					t.Fatalf("expected untouched filter, got %v", f)
				}
				return
			}
			if !reflect.DeepEqual(f, tt.want) {
				t.Fatalf("got %#v, want %#v", f, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Range (BETWEEN)
// ---------------------------------------------------------------------------

func TestRange(t *testing.T) {
	proc := NewProcessor()
	tests := []struct {
		name   string
		value  any
		values []any
		want   bsonV2.M // full expected filter; nil => filter must stay untouched
	}{
		{"json array", `["1","5"]`, nil, bsonV2.M{"f": bsonV2.M{"$gte": "1", "$lte": "5"}}},
		{"comma separated", "2020-01-01, 2021-01-01", nil, bsonV2.M{"f": bsonV2.M{"$gte": "2020-01-01", "$lte": "2021-01-01"}}},
		{"single string equality", "solo", nil, bsonV2.M{"f": "solo"}},
		{"bytes json array", []byte(`["1","9"]`), nil, bsonV2.M{"f": bsonV2.M{"$gte": "1", "$lte": "9"}}},
		{"bytes comma", []byte("a,b"), nil, bsonV2.M{"f": bsonV2.M{"$gte": "a", "$lte": "b"}}},
		{"bytes single equality", []byte("solo"), nil, bsonV2.M{"f": "solo"}},
		{"slice of two", []any{"x", "y"}, nil, bsonV2.M{"f": bsonV2.M{"$gte": "x", "$lte": "y"}}},
		{"slice of one equality", []any{"x"}, nil, bsonV2.M{"f": "x"}},
		{"empty slice with one value", []any{}, []any{"z"}, bsonV2.M{"f": "z"}},
		{"empty slice with two values", []any{}, []any{"a", "b"}, bsonV2.M{"f": bsonV2.M{"$gte": "a", "$lte": "b"}}},
		{"nil with two values", nil, []any{"a", "b"}, bsonV2.M{"f": bsonV2.M{"$gte": "a", "$lte": "b"}}},
		{"scalar equality", 42, nil, bsonV2.M{"f": 42}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := &query.Builder{}
			proc.Range(qb, "f", tt.value, tt.values)
			f := filterOf(t, qb)
			if tt.want == nil {
				if len(f) != 0 {
					t.Fatalf("expected untouched filter, got %v", f)
				}
				return
			}
			if !reflect.DeepEqual(f, tt.want) {
				t.Fatalf("got %#v, want %#v", f, tt.want)
			}
		})
	}
}

func TestRange_AllEmptyLeavesFilterUntouched(t *testing.T) {
	proc := NewProcessor()
	qb := &query.Builder{}
	proc.Range(qb, "f", nil, nil)
	if f := filterOf(t, qb); len(f) != 0 {
		t.Fatalf("expected untouched filter, got %v", f)
	}
}

// ---------------------------------------------------------------------------
// String matchers (Contains / StartsWith / EndsWith / Exact / Regex / Search)
// ---------------------------------------------------------------------------

func TestStringMatchers(t *testing.T) {
	proc := NewProcessor()
	tests := []struct {
		name  string
		op    paginationV1.Operator
		value any
		want  bsonV2.M
	}{
		{"contains", paginationV1.Operator_CONTAINS, "val", bsonV2.M{"f": bsonV2.M{"$regex": ".*val.*"}}},
		{"contains escapes regex chars", paginationV1.Operator_CONTAINS, "a.b+", bsonV2.M{"f": bsonV2.M{"$regex": `.*a\.b\+.*`}}},
		{"contains bytes", paginationV1.Operator_CONTAINS, []byte("val"), bsonV2.M{"f": bsonV2.M{"$regex": ".*val.*"}}},
		{"contains stringer", paginationV1.Operator_CONTAINS, strVal{"val"}, bsonV2.M{"f": bsonV2.M{"$regex": ".*val.*"}}},
		{"contains empty no-op", paginationV1.Operator_CONTAINS, "   ", nil},
		{"contains unsupported no-op", paginationV1.Operator_CONTAINS, 42, nil},
		{"icontains", paginationV1.Operator_ICONTAINS, "Val", bsonV2.M{"f": bsonV2.M{"$regex": ".*Val.*", "$options": "i"}}},
		{"starts with", paginationV1.Operator_STARTS_WITH, "val", bsonV2.M{"f": bsonV2.M{"$regex": "^val"}}},
		{"istarts with", paginationV1.Operator_ISTARTS_WITH, "val", bsonV2.M{"f": bsonV2.M{"$regex": "^val", "$options": "i"}}},
		{"ends with", paginationV1.Operator_ENDS_WITH, "val", bsonV2.M{"f": bsonV2.M{"$regex": "val$"}}},
		{"iends with", paginationV1.Operator_IENDS_WITH, "val", bsonV2.M{"f": bsonV2.M{"$regex": "val$", "$options": "i"}}},
		{"iexact", paginationV1.Operator_IEXACT, "Val", bsonV2.M{"f": bsonV2.M{"$regex": "^Val$", "$options": "i"}}},
		{"regexp raw", paginationV1.Operator_REGEXP, `^a.*z$`, bsonV2.M{"f": bsonV2.M{"$regex": `^a.*z$`}}},
		{"iregexp raw", paginationV1.Operator_IREGEXP, `^a`, bsonV2.M{"f": bsonV2.M{"$regex": `^a`, "$options": "i"}}},
		{"regexp empty no-op", paginationV1.Operator_REGEXP, "", nil},
		{"regexp unsupported no-op", paginationV1.Operator_REGEXP, 42, nil},
		{"search delegates to contains", paginationV1.Operator_SEARCH, "query", bsonV2.M{"f": bsonV2.M{"$regex": ".*query.*"}}},
		{"search empty no-op", paginationV1.Operator_SEARCH, "  ", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := &query.Builder{}
			proc.Process(qb, tt.op, "f", tt.value, nil)
			f := filterOf(t, qb)
			if tt.want == nil {
				if len(f) != 0 {
					t.Fatalf("expected untouched filter, got %v", f)
				}
				return
			}
			if len(f) != 1 {
				t.Fatalf("expected 1 entry, got %v", f)
			}
			got, ok := f["f"]
			if !ok {
				t.Fatalf("missing key f in %v", f)
			}
			wantM := tt.want["f"].(bsonV2.M)
			gotM, ok := got.(bsonV2.M)
			if !ok {
				t.Fatalf("expected bsonV2.M, got %T (%#v)", got, got)
			}
			if len(gotM) != len(wantM) {
				t.Fatalf("got %v, want %v", gotM, wantM)
			}
			for k, wv := range wantM {
				if gotM[k] != wv {
					t.Fatalf("key %q: got %#v, want %#v", k, gotM[k], wv)
				}
			}
		})
	}
}

func TestExact(t *testing.T) {
	proc := NewProcessor()

	// string: trimmed, empty skipped
	qb := &query.Builder{}
	proc.Exact(qb, "f", "  tom  ")
	if f := filterOf(t, qb); f["f"] != "tom" {
		t.Fatalf("expected trimmed value, got %#v", f)
	}

	// empty string skipped
	qb = &query.Builder{}
	proc.Exact(qb, "f", "   ")
	if f := filterOf(t, qb); len(f) != 0 {
		t.Fatalf("expected untouched filter, got %v", f)
	}

	// []byte
	qb = &query.Builder{}
	proc.Exact(qb, "f", []byte("tom"))
	if f := filterOf(t, qb); f["f"] != "tom" {
		t.Fatalf("expected bytes value, got %#v", f)
	}

	// Stringer
	qb = &query.Builder{}
	proc.Exact(qb, "f", strVal{"tom"})
	if f := filterOf(t, qb); f["f"] != "tom" {
		t.Fatalf("expected stringer value, got %#v", f)
	}

	// nil skipped
	qb = &query.Builder{}
	proc.Exact(qb, "f", nil)
	if f := filterOf(t, qb); len(f) != 0 {
		t.Fatalf("expected untouched filter, got %v", f)
	}

	// non-string scalar
	qb = &query.Builder{}
	proc.Exact(qb, "f", 7)
	if f := filterOf(t, qb); f["f"] != 7 {
		t.Fatalf("expected scalar value, got %#v", f)
	}
}

func TestInsensitiveExact(t *testing.T) {
	proc := NewProcessor()

	qb := &query.Builder{}
	proc.InsensitiveExact(qb, "f", "Val")
	f := filterOf(t, qb)
	want := bsonV2.M{"$regex": "^Val$", "$options": "i"}
	got, ok := f["f"].(bsonV2.M)
	if !ok || len(got) != 2 || got["$regex"] != want["$regex"] || got["$options"] != want["$options"] {
		t.Fatalf("got %#v, want %v", f, want)
	}

	// empty value no-op
	qb = &query.Builder{}
	proc.InsensitiveExact(qb, "f", "")
	if f := filterOf(t, qb); len(f) != 0 {
		t.Fatalf("expected untouched filter, got %v", f)
	}
}

// ---------------------------------------------------------------------------
// Misc surface
// ---------------------------------------------------------------------------

func TestDatePartAndJsonbField_AreStubs(t *testing.T) {
	proc := NewProcessor()
	if got := proc.DatePartField("year", "created_at"); got != "" {
		t.Fatalf("DatePartField should be a stub, got %q", got)
	}
	if got := proc.JsonbField("meta", "data"); got != "" {
		t.Fatalf("JsonbField should be a stub, got %q", got)
	}
}

func TestAppendFilter_EmptyConditionNoOp(t *testing.T) {
	proc := NewProcessor()
	qb := &query.Builder{}
	got := proc.appendFilter(qb, nil)
	if got != qb {
		t.Fatal("expected same builder returned")
	}
	if f := filterOf(t, qb); len(f) != 0 {
		t.Fatalf("expected untouched filter, got %v", f)
	}
}

// ---------------------------------------------------------------------------
// Matcher value-type coverage: []byte and fmt.Stringer on every text op
// ---------------------------------------------------------------------------

func TestStringMatchers_ValueTypes(t *testing.T) {
	proc := NewProcessor()
	tests := []struct {
		name  string
		op    paginationV1.Operator
		value any
		want  bsonV2.M
	}{
		{"icontains bytes", paginationV1.Operator_ICONTAINS, []byte("val"), bsonV2.M{"f": bsonV2.M{"$regex": ".*val.*", "$options": "i"}}},
		{"icontains stringer", paginationV1.Operator_ICONTAINS, strVal{"val"}, bsonV2.M{"f": bsonV2.M{"$regex": ".*val.*", "$options": "i"}}},
		{"istarts bytes", paginationV1.Operator_ISTARTS_WITH, []byte("val"), bsonV2.M{"f": bsonV2.M{"$regex": "^val", "$options": "i"}}},
		{"istarts stringer", paginationV1.Operator_ISTARTS_WITH, strVal{"val"}, bsonV2.M{"f": bsonV2.M{"$regex": "^val", "$options": "i"}}},
		{"ends bytes", paginationV1.Operator_ENDS_WITH, []byte("val"), bsonV2.M{"f": bsonV2.M{"$regex": "val$"}}},
		{"ends stringer", paginationV1.Operator_ENDS_WITH, strVal{"val"}, bsonV2.M{"f": bsonV2.M{"$regex": "val$"}}},
		{"iends bytes", paginationV1.Operator_IENDS_WITH, []byte("val"), bsonV2.M{"f": bsonV2.M{"$regex": "val$", "$options": "i"}}},
		{"iends stringer", paginationV1.Operator_IENDS_WITH, strVal{"val"}, bsonV2.M{"f": bsonV2.M{"$regex": "val$", "$options": "i"}}},
		{"iexact bytes", paginationV1.Operator_IEXACT, []byte("val"), bsonV2.M{"f": bsonV2.M{"$regex": "^val$", "$options": "i"}}},
		{"iexact stringer", paginationV1.Operator_IEXACT, strVal{"val"}, bsonV2.M{"f": bsonV2.M{"$regex": "^val$", "$options": "i"}}},
		{"iexact empty no-op", paginationV1.Operator_IEXACT, "  ", nil},
		{"regexp bytes", paginationV1.Operator_REGEXP, []byte(`^a`), bsonV2.M{"f": bsonV2.M{"$regex": `^a`}}},
		{"regexp stringer", paginationV1.Operator_REGEXP, strVal{`^a`}, bsonV2.M{"f": bsonV2.M{"$regex": `^a`}}},
		{"iregexp bytes", paginationV1.Operator_IREGEXP, []byte(`^a`), bsonV2.M{"f": bsonV2.M{"$regex": `^a`, "$options": "i"}}},
		{"iregexp stringer", paginationV1.Operator_IREGEXP, strVal{`^a`}, bsonV2.M{"f": bsonV2.M{"$regex": `^a`, "$options": "i"}}},
		{"iregexp empty no-op", paginationV1.Operator_IREGEXP, "", nil},
		{"search bytes", paginationV1.Operator_SEARCH, []byte("q"), bsonV2.M{"f": bsonV2.M{"$regex": ".*q.*"}}},
		{"search stringer", paginationV1.Operator_SEARCH, strVal{"q"}, bsonV2.M{"f": bsonV2.M{"$regex": ".*q.*"}}},
		{"search unsupported no-op", paginationV1.Operator_SEARCH, 42, nil},
		{"istarts unsupported no-op", paginationV1.Operator_ISTARTS_WITH, 42, nil},
		{"ends unsupported no-op", paginationV1.Operator_ENDS_WITH, 42, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := &query.Builder{}
			proc.Process(qb, tt.op, "f", tt.value, nil)
			f := filterOf(t, qb)
			if tt.want == nil {
				if len(f) != 0 {
					t.Fatalf("expected untouched filter, got %v", f)
				}
				return
			}
			if !reflect.DeepEqual(f, tt.want) {
				t.Fatalf("got %#v, want %#v", f, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// In / NotIn / Range — remaining edge inputs
// ---------------------------------------------------------------------------

func TestIn_CommaOnlyAndPlainBytes(t *testing.T) {
	proc := NewProcessor()

	// "," splits into only empty parts → 永假 falseExpr.
	qb := &query.Builder{}
	proc.In(qb, "f", " ,  ,", nil)
	f := filterOf(t, qb)
	if !reflect.DeepEqual(f, falseExpr) {
		t.Fatalf("got %#v, want falseExpr", f)
	}

	// plain (non-JSON, non-comma) bytes → single-element $in.
	qb = &query.Builder{}
	proc.In(qb, "f", []byte("abc"), nil)
	f = filterOf(t, qb)
	want := bsonV2.M{"f": bsonV2.M{"$in": []any{"abc"}}}
	if !reflect.DeepEqual(f, want) {
		t.Fatalf("got %#v, want %#v", f, want)
	}
}

func TestNotIn_CommaOnlyAndPlainBytes(t *testing.T) {
	proc := NewProcessor()

	// "," splits into only empty parts → adds nothing.
	qb := &query.Builder{}
	proc.NotIn(qb, "f", " , ,", nil)
	if f := filterOf(t, qb); len(f) != 0 {
		t.Fatalf("expected untouched filter, got %v", f)
	}

	// plain bytes → single-element $nin.
	qb = &query.Builder{}
	proc.NotIn(qb, "f", []byte("abc"), nil)
	f := filterOf(t, qb)
	want := bsonV2.M{"f": bsonV2.M{"$nin": []any{"abc"}}}
	if !reflect.DeepEqual(f, want) {
		t.Fatalf("got %#v, want %#v", f, want)
	}
}

func TestRange_StringJsonArrayWrongLength(t *testing.T) {
	proc := NewProcessor()

	// A JSON array that is not length 2 falls through to equality of the raw string.
	qb := &query.Builder{}
	proc.Range(qb, "f", `["1"]`, nil)
	f := filterOf(t, qb)
	want := bsonV2.M{"f": `["1"]`}
	if !reflect.DeepEqual(f, want) {
		t.Fatalf("got %#v, want %#v", f, want)
	}
}

// ---------------------------------------------------------------------------
// Final branch coverage
// ---------------------------------------------------------------------------

func TestStringMatchers_FinalBranches(t *testing.T) {
	proc := NewProcessor()
	tests := []struct {
		name  string
		op    paginationV1.Operator
		value any
		want  bsonV2.M // nil => untouched
	}{
		{"starts bytes", paginationV1.Operator_STARTS_WITH, []byte("val"), bsonV2.M{"f": bsonV2.M{"$regex": "^val"}}},
		{"starts stringer", paginationV1.Operator_STARTS_WITH, strVal{"val"}, bsonV2.M{"f": bsonV2.M{"$regex": "^val"}}},
		{"starts unsupported no-op", paginationV1.Operator_STARTS_WITH, 42, nil},
		{"icontains unsupported no-op", paginationV1.Operator_ICONTAINS, 42, nil},
		{"iends empty no-op", paginationV1.Operator_IENDS_WITH, "  ", nil},
		{"iregexp unsupported no-op", paginationV1.Operator_IREGEXP, 42, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := &query.Builder{}
			proc.Process(qb, tt.op, "f", tt.value, nil)
			f := filterOf(t, qb)
			if tt.want == nil {
				if len(f) != 0 {
					t.Fatalf("expected untouched filter, got %v", f)
				}
				return
			}
			if !reflect.DeepEqual(f, tt.want) {
				t.Fatalf("got %#v, want %#v", f, tt.want)
			}
		})
	}
}

func TestExact_StringerEmptySkipped(t *testing.T) {
	proc := NewProcessor()
	qb := &query.Builder{}
	proc.Exact(qb, "f", strVal{"  "})
	if f := filterOf(t, qb); len(f) != 0 {
		t.Fatalf("expected untouched filter, got %v", f)
	}
}

func TestRange_TooManyValuesIgnored(t *testing.T) {
	proc := NewProcessor()

	// values with more than 2 entries are not a valid range; a nil value
	// then leaves the filter untouched.
	qb := &query.Builder{}
	proc.Range(qb, "f", nil, []any{"a", "b", "c"})
	if f := filterOf(t, qb); len(f) != 0 {
		t.Fatalf("expected untouched filter, got %v", f)
	}
}

func TestIn_BytesCommaOnlyYieldsFalseExpr(t *testing.T) {
	proc := NewProcessor()
	qb := &query.Builder{}
	proc.In(qb, "f", []byte(" , "), nil)
	f := filterOf(t, qb)
	if !reflect.DeepEqual(f, falseExpr) {
		t.Fatalf("got %#v, want falseExpr", f)
	}
}

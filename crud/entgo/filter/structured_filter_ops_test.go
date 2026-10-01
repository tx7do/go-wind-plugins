package filter

import (
	"testing"

	"entgo.io/ent/dialect/sql"

	paginationV1 "github.com/tx7do/go-wind-plugins/crud/api/gen/go/pagination/v1"
)

// structuredCond builds a filter condition for the structured filter.
func structuredCond(field string, op paginationV1.Operator, value string, values ...string) *paginationV1.FilterCondition {
	return &paginationV1.FilterCondition{
		Field:      field,
		Op:         op,
		ValueOneof: &paginationV1.FilterCondition_Value{Value: value},
		Values:     values,
	}
}

// TestStructuredFilter_AllOperators walks every operator through the
// StructuredFilter dispatcher (unknown table => column whitelist fails open).
func TestStructuredFilter_AllOperators(t *testing.T) {
	sf := NewStructuredFilter()
	s := sql.Select().From(sql.Table("test_items"))

	cases := []struct {
		name    string
		cond    *paginationV1.FilterCondition
		wantNil bool
	}{
		{"eq", structuredCond("name", paginationV1.Operator_EQ, "tom"), false},
		{"neq", structuredCond("name", paginationV1.Operator_NEQ, "tom"), false},
		{"in json", structuredCond("name", paginationV1.Operator_IN, `["a","b"]`), false},
		{"in values", structuredCond("name", paginationV1.Operator_IN, "", "a", "b"), false},
		{"in nothing", structuredCond("name", paginationV1.Operator_IN, ""), true},
		{"nin json", structuredCond("name", paginationV1.Operator_NIN, `["a"]`), false},
		{"nin values", structuredCond("name", paginationV1.Operator_NIN, "", "a"), false},
		{"nin nothing", structuredCond("name", paginationV1.Operator_NIN, ""), true},
		{"gte", structuredCond("created_at", paginationV1.Operator_GTE, "2024-01-01"), false},
		{"gt", structuredCond("created_at", paginationV1.Operator_GT, "2024-01-01"), false},
		{"lte", structuredCond("created_at", paginationV1.Operator_LTE, "2024-12-31"), false},
		{"lt", structuredCond("created_at", paginationV1.Operator_LT, "2024-12-31"), false},
		{"between json", structuredCond("created_at", paginationV1.Operator_BETWEEN, `["1","2"]`), false},
		{"between values", structuredCond("created_at", paginationV1.Operator_BETWEEN, "", "1", "2"), false},
		{"between bad", structuredCond("created_at", paginationV1.Operator_BETWEEN, `["1"]`), true},
		{"is null", structuredCond("deleted_at", paginationV1.Operator_IS_NULL, ""), false},
		{"is not null", structuredCond("deleted_at", paginationV1.Operator_IS_NOT_NULL, ""), false},
		{"contains", structuredCond("name", paginationV1.Operator_CONTAINS, "go"), false},
		{"icontains", structuredCond("name", paginationV1.Operator_ICONTAINS, "go"), false},
		{"starts", structuredCond("name", paginationV1.Operator_STARTS_WITH, "go"), false},
		{"istarts", structuredCond("name", paginationV1.Operator_ISTARTS_WITH, "go"), false},
		{"ends", structuredCond("name", paginationV1.Operator_ENDS_WITH, "go"), false},
		{"iends", structuredCond("name", paginationV1.Operator_IENDS_WITH, "go"), false},
		{"exact", structuredCond("name", paginationV1.Operator_EXACT, "go"), false},
		{"iexact", structuredCond("name", paginationV1.Operator_IEXACT, "go"), false},
		{"regex", structuredCond("name", paginationV1.Operator_REGEXP, "^g"), false},
		{"iregex", structuredCond("name", paginationV1.Operator_IREGEXP, "^g"), false},
		{"search", structuredCond("name", paginationV1.Operator_SEARCH, "term"), false},
		{"search empty", structuredCond("name", paginationV1.Operator_SEARCH, "  "), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sf.Process(s, sql.P(), tc.cond)
			if tc.wantNil && got != nil {
				t.Fatalf("expected nil predicate, got %v", got != nil)
			}
			if !tc.wantNil && got == nil {
				t.Fatal("expected a predicate, got nil")
			}
		})
	}

	// unknown operator and hostile/empty fields yield nil
	if got := sf.Process(s, sql.P(), structuredCond("name", paginationV1.Operator(9999), "x")); got != nil {
		t.Fatal("unknown operator must yield nil")
	}
	if got := sf.Process(s, sql.P(), structuredCond("a b", paginationV1.Operator_EQ, "x")); got != nil {
		t.Fatal("hostile field must yield nil")
	}
	if got := sf.Process(s, sql.P(), nil); got != nil {
		t.Fatal("nil condition must yield nil")
	}
}

// TestStructuredFilter_ProcessCondition verifies the aggregation helper that
// turns a condition list into a predicate slice.
func TestStructuredFilter_ProcessCondition(t *testing.T) {
	sf := NewStructuredFilter()
	s := sql.Select().From(sql.Table("test_items"))

	ps, err := sf.processCondition(s, []*paginationV1.FilterCondition{
		structuredCond("name", paginationV1.Operator_EQ, "tom"),
		structuredCond("bad field", paginationV1.Operator_EQ, "x"), // skipped
	})
	if err != nil {
		t.Fatalf("processCondition error: %v", err)
	}
	if len(ps) != 1 {
		t.Fatalf("expected 1 usable predicate, got %d", len(ps))
	}

	ps, err = sf.processCondition(s, nil)
	if err != nil || ps != nil {
		t.Fatalf("processCondition(empty) = %v, %v", ps, err)
	}
}

// TestStructuredFilter_JsonPathField covers the JSONB condition path.
func TestStructuredFilter_JsonPathField(t *testing.T) {
	sf := NewStructuredFilter()
	s := sql.Select().From(sql.Table("test_items"))

	jp := "daily_email"
	cond := structuredCond("preferences", paginationV1.Operator_EQ, "true")
	cond.JsonPath = &jp

	got := sf.Process(s, sql.P(), cond)
	if got == nil {
		t.Fatal("json path condition must produce a predicate")
	}

	// a hostile json path on an unknown table still passes the container
	// column check but the key pattern is validated inside JsonbField
	jpBad := "a b"
	bad := structuredCond("preferences", paginationV1.Operator_EQ, "true")
	bad.JsonPath = &jpBad
	if got := sf.Process(s, sql.P(), bad); got == nil {
		t.Log("hostile json path skipped (key pattern rejected)")
	}
}

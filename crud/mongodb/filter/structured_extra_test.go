package filter

import (
	"reflect"
	"testing"

	bsonV2 "go.mongodb.org/mongo-driver/v2/bson"

	paginationV1 "github.com/tx7do/go-wind-plugins/crud/api/gen/go/pagination/v1"
	"github.com/tx7do/go-wind-plugins/crud/mongodb/query"
)

// cond builds a FilterCondition with a string value.
func cond(field string, op paginationV1.Operator, value string, values ...string) *paginationV1.FilterCondition {
	c := &paginationV1.FilterCondition{
		Field:      field,
		Op:         op,
		ValueOneof: &paginationV1.FilterCondition_Value{Value: value},
	}
	if len(values) > 0 {
		c.Values = values
	}
	return c
}

func TestBuildCond_NilAndInvalidInputs(t *testing.T) {
	sf := NewStructuredFilter()

	if got := sf.buildCond(nil); got != nil {
		t.Fatalf("nil cond should yield nil, got %v", got)
	}

	// Empty / whitespace field.
	for _, field := range []string{"", "   "} {
		if got := sf.buildCond(cond(field, paginationV1.Operator_EQ, "v")); got != nil {
			t.Fatalf("field %q should yield nil, got %v", field, got)
		}
	}

	// Field that maps to an invalid key (json part fails the whitelist).
	if got := sf.buildCond(cond("meta.$dollar", paginationV1.Operator_EQ, "v")); got != nil {
		t.Fatalf("invalid key should yield nil, got %v", got)
	}
}

func TestBuildCond_OperatorMatrix(t *testing.T) {
	sf := NewStructuredFilter()
	tests := []struct {
		name   string
		op     paginationV1.Operator
		value  string
		values []string
		want   bsonV2.M
	}{
		{"EQ", paginationV1.Operator_EQ, "v", nil, bsonV2.M{"f": "v"}},
		{"NEQ", paginationV1.Operator_NEQ, "v", nil, bsonV2.M{"f": bsonV2.M{"$ne": "v"}}},
		{"IN json", paginationV1.Operator_IN, `["a","b"]`, nil, bsonV2.M{"f": bsonV2.M{"$in": []any{"a", "b"}}}},
		{"IN json empty", paginationV1.Operator_IN, `[]`, nil, falseExpr},
		{"IN values", paginationV1.Operator_IN, "", []string{"a", "b"}, bsonV2.M{"f": bsonV2.M{"$in": []any{"a", "b"}}}},
		{"NIN json", paginationV1.Operator_NIN, `["a"]`, nil, bsonV2.M{"f": bsonV2.M{"$nin": []any{"a"}}}},
		{"NIN json empty", paginationV1.Operator_NIN, `[]`, nil, nil},
		{"NIN values", paginationV1.Operator_NIN, "", []string{"a"}, bsonV2.M{"f": bsonV2.M{"$nin": []any{"a"}}}},
		{"NIN none", paginationV1.Operator_NIN, "", nil, nil},
		{"IN none", paginationV1.Operator_IN, "", nil, nil},
		{"GTE", paginationV1.Operator_GTE, "10", nil, bsonV2.M{"f": bsonV2.M{"$gte": "10"}}},
		{"GT", paginationV1.Operator_GT, "10", nil, bsonV2.M{"f": bsonV2.M{"$gt": "10"}}},
		{"LTE", paginationV1.Operator_LTE, "10", nil, bsonV2.M{"f": bsonV2.M{"$lte": "10"}}},
		{"LT", paginationV1.Operator_LT, "10", nil, bsonV2.M{"f": bsonV2.M{"$lt": "10"}}},
		{"BETWEEN json", paginationV1.Operator_BETWEEN, `["1","5"]`, nil, bsonV2.M{"f": bsonV2.M{"$gte": "1", "$lte": "5"}}},
		{"BETWEEN values", paginationV1.Operator_BETWEEN, "", []string{"1", "5"}, bsonV2.M{"f": bsonV2.M{"$gte": "1", "$lte": "5"}}},
		{"BETWEEN comma", paginationV1.Operator_BETWEEN, "1,5", nil, bsonV2.M{"f": bsonV2.M{"$gte": "1", "$lte": "5"}}},
		{"BETWEEN single", paginationV1.Operator_BETWEEN, "solo", nil, bsonV2.M{"f": "solo"}},
		{"BETWEEN none", paginationV1.Operator_BETWEEN, "", nil, nil},
		{"IS_NULL", paginationV1.Operator_IS_NULL, "", nil, bsonV2.M{"f": nil}},
		{"IS_NOT_NULL", paginationV1.Operator_IS_NOT_NULL, "", nil, bsonV2.M{"f": bsonV2.M{"$ne": nil}}},
		{"CONTAINS", paginationV1.Operator_CONTAINS, "a.b", nil, bsonV2.M{"f": bsonV2.M{"$regex": "a\\.b"}}},
		{"CONTAINS empty", paginationV1.Operator_CONTAINS, "  ", nil, nil},
		{"ICONTAINS", paginationV1.Operator_ICONTAINS, "v", nil, bsonV2.M{"f": bsonV2.M{"$regex": "v", "$options": "i"}}},
		{"ICONTAINS empty", paginationV1.Operator_ICONTAINS, "", nil, nil},
		{"STARTS_WITH", paginationV1.Operator_STARTS_WITH, "v", nil, bsonV2.M{"f": bsonV2.M{"$regex": "^v"}}},
		{"STARTS_WITH empty", paginationV1.Operator_STARTS_WITH, "", nil, nil},
		{"ISTARTS_WITH", paginationV1.Operator_ISTARTS_WITH, "v", nil, bsonV2.M{"f": bsonV2.M{"$regex": "^v", "$options": "i"}}},
		{"ENDS_WITH", paginationV1.Operator_ENDS_WITH, "v", nil, bsonV2.M{"f": bsonV2.M{"$regex": "v$"}}},
		{"ENDS_WITH empty", paginationV1.Operator_ENDS_WITH, "", nil, nil},
		{"IENDS_WITH", paginationV1.Operator_IENDS_WITH, "v", nil, bsonV2.M{"f": bsonV2.M{"$regex": "v$", "$options": "i"}}},
		{"EXACT", paginationV1.Operator_EXACT, "v", nil, bsonV2.M{"f": "v"}},
		{"EXACT empty value kept", paginationV1.Operator_EXACT, "", nil, bsonV2.M{"f": ""}},
		{"IEXACT", paginationV1.Operator_IEXACT, "v", nil, bsonV2.M{"f": bsonV2.M{"$regex": "^v$", "$options": "i"}}},
		{"REGEXP", paginationV1.Operator_REGEXP, "^a", nil, bsonV2.M{"f": bsonV2.M{"$regex": "^a"}}},
		{"REGEXP empty", paginationV1.Operator_REGEXP, " ", nil, nil},
		{"IREGEXP", paginationV1.Operator_IREGEXP, "^a", nil, bsonV2.M{"f": bsonV2.M{"$regex": "^a", "$options": "i"}}},
		{"IREGEXP empty", paginationV1.Operator_IREGEXP, "", nil, nil},
		{"SEARCH", paginationV1.Operator_SEARCH, "q", nil, bsonV2.M{"f": bsonV2.M{"$regex": "q"}}},
		{"SEARCH empty", paginationV1.Operator_SEARCH, "", nil, nil},
		{"unknown op with value", paginationV1.Operator(999), "v", nil, bsonV2.M{"f": "v"}},
		{"unknown op without value", paginationV1.Operator(999), "", nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sf.buildCond(cond("f", tt.op, tt.value, tt.values...))
			if tt.want == nil {
				if got != nil {
					t.Fatalf("expected nil, got %#v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected %#v, got nil", tt.want)
			}
			if !reflect.DeepEqual(map[string]any(got), map[string]any(tt.want)) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestBuildCond_IN_ValuesFallbackIgnoresValueWithoutJson(t *testing.T) {
	sf := NewStructuredFilter()

	// Value "abc" is not JSON and not comma separated → parseArray fails,
	// so the values list is used.
	got := sf.buildCond(cond("f", paginationV1.Operator_IN, "abc", "x", "y"))
	want := bsonV2.M{"f": bsonV2.M{"$in": []any{"x", "y"}}}
	if !reflect.DeepEqual(map[string]any(got), map[string]any(want)) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

// ---------------------------------------------------------------------------
// BuildSelectors — structure of AND / OR / groups
// ---------------------------------------------------------------------------

func TestBuildSelectors_NilBuilderCreatesOne(t *testing.T) {
	sf := NewStructuredFilter()
	expr := &paginationV1.FilterExpr{
		Type: paginationV1.ExprType_AND,
		Conditions: []*paginationV1.FilterCondition{
			cond("name", paginationV1.Operator_EQ, "alice"),
		},
	}
	b, err := sf.BuildSelectors(nil, expr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil builder")
	}
}

func TestBuildSelectors_UnknownExprTypeNoFilter(t *testing.T) {
	sf := NewStructuredFilter()
	qb := &query.Builder{}
	expr := &paginationV1.FilterExpr{Type: paginationV1.ExprType(999)}
	b, err := sf.BuildSelectors(qb, expr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b != qb {
		t.Fatal("expected same builder")
	}
	if f, _ := qb.Build(); len(f) != 0 {
		t.Fatalf("expected untouched filter, got %v", f)
	}
}

func TestBuildSelectors_AndMultipleConditions(t *testing.T) {
	sf := NewStructuredFilter()
	qb := &query.Builder{}
	expr := &paginationV1.FilterExpr{
		Type: paginationV1.ExprType_AND,
		Conditions: []*paginationV1.FilterCondition{
			cond("name", paginationV1.Operator_EQ, "alice"),
			cond("age", paginationV1.Operator_GT, "18"),
		},
	}
	if _, err := sf.BuildSelectors(qb, expr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	f, _ := qb.Build()
	and, ok := f["$and"].(bsonV2.A)
	if !ok {
		t.Fatalf("expected top-level $and, got %#v", f)
	}
	if len(and) != 2 {
		t.Fatalf("expected 2 parts, got %v", and)
	}
	want0 := bsonV2.M{"name": "alice"}
	if !reflect.DeepEqual(map[string]any(and[0].(bsonV2.M)), map[string]any(want0)) {
		t.Fatalf("part 0: got %#v, want %#v", and[0], want0)
	}
}

func TestBuildSelectors_AndSingleConditionUnwrapped(t *testing.T) {
	sf := NewStructuredFilter()
	qb := &query.Builder{}
	expr := &paginationV1.FilterExpr{
		Type: paginationV1.ExprType_AND,
		Conditions: []*paginationV1.FilterCondition{
			cond("name", paginationV1.Operator_EQ, "alice"),
		},
	}
	if _, err := sf.BuildSelectors(qb, expr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// A single AND part is returned directly, without a $and wrapper.
	f, _ := qb.Build()
	want := bsonV2.M{"name": "alice"}
	if !reflect.DeepEqual(map[string]any(f), map[string]any(want)) {
		t.Fatalf("got %#v, want %#v", f, want)
	}
}

func TestBuildSelectors_OrStructure(t *testing.T) {
	sf := NewStructuredFilter()
	qb := &query.Builder{}
	expr := &paginationV1.FilterExpr{
		Type: paginationV1.ExprType_OR,
		Conditions: []*paginationV1.FilterCondition{
			cond("status", paginationV1.Operator_EQ, "active"),
			cond("status", paginationV1.Operator_EQ, "pending"),
		},
	}
	if _, err := sf.BuildSelectors(qb, expr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	f, _ := qb.Build()
	or, ok := f["$or"].(bsonV2.A)
	if !ok || len(or) != 2 {
		t.Fatalf("expected top-level $or with 2 parts, got %#v", f)
	}
}

func TestBuildSelectors_AndWithNestedOrGroup(t *testing.T) {
	sf := NewStructuredFilter()
	qb := &query.Builder{}
	expr := &paginationV1.FilterExpr{
		Type: paginationV1.ExprType_AND,
		Conditions: []*paginationV1.FilterCondition{
			cond("tenant", paginationV1.Operator_EQ, "acme"),
		},
		Groups: []*paginationV1.FilterExpr{
			{
				Type: paginationV1.ExprType_OR,
				Conditions: []*paginationV1.FilterCondition{
					cond("role", paginationV1.Operator_EQ, "admin"),
					cond("role", paginationV1.Operator_EQ, "owner"),
				},
			},
		},
	}
	if _, err := sf.BuildSelectors(qb, expr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	f, _ := qb.Build()
	and, ok := f["$and"].(bsonV2.A)
	if !ok || len(and) != 2 {
		t.Fatalf("expected $and with [cond, group], got %#v", f)
	}
	or, ok := and[1].(bsonV2.M)["$or"].(bsonV2.A)
	if !ok || len(or) != 2 {
		t.Fatalf("expected nested $or group with 2 parts, got %#v", and[1])
	}
}

func TestBuildSelectors_AndOnlyEmptyConditionsNoFilter(t *testing.T) {
	sf := NewStructuredFilter()
	qb := &query.Builder{}

	// CONTAINS with an empty value yields nil conds → AND has no parts →
	// no filter is set at all.
	expr := &paginationV1.FilterExpr{
		Type: paginationV1.ExprType_AND,
		Conditions: []*paginationV1.FilterCondition{
			cond("title", paginationV1.Operator_CONTAINS, ""),
			cond("body", paginationV1.Operator_CONTAINS, " "),
		},
	}
	if _, err := sf.BuildSelectors(qb, expr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if f, _ := qb.Build(); len(f) != 0 {
		t.Fatalf("expected untouched filter, got %v", f)
	}
}

func TestBuildSelectors_SingleGroupUnwrapped(t *testing.T) {
	sf := NewStructuredFilter()
	qb := &query.Builder{}

	// AND whose only part is a single-condition group → the group's doc is
	// returned directly (no wrapper).
	expr := &paginationV1.FilterExpr{
		Type: paginationV1.ExprType_AND,
		Groups: []*paginationV1.FilterExpr{
			{
				Type: paginationV1.ExprType_OR,
				Conditions: []*paginationV1.FilterCondition{
					cond("role", paginationV1.Operator_EQ, "admin"),
				},
			},
		},
	}
	if _, err := sf.BuildSelectors(qb, expr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	f, _ := qb.Build()
	want := bsonV2.M{"role": "admin"}
	if !reflect.DeepEqual(map[string]any(f), map[string]any(want)) {
		t.Fatalf("got %#v, want %#v", f, want)
	}
}

func TestBuildSelectors_OrOnlyEmptyNoFilter(t *testing.T) {
	sf := NewStructuredFilter()
	qb := &query.Builder{}
	expr := &paginationV1.FilterExpr{
		Type: paginationV1.ExprType_OR,
		Conditions: []*paginationV1.FilterCondition{
			cond("title", paginationV1.Operator_REGEXP, ""),
		},
	}
	if _, err := sf.BuildSelectors(qb, expr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f, _ := qb.Build(); len(f) != 0 {
		t.Fatalf("expected untouched filter, got %v", f)
	}
}

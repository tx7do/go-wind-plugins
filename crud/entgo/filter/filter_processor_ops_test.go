package filter

import (
	"fmt"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"

	paginationV1 "github.com/tx7do/go-wind-plugins/crud/api/gen/go/pagination/v1"
)

// newDialectSelector builds a selector bound to a specific SQL dialect.
func newDialectSelector(d string) *sql.Selector {
	return sql.Dialect(d).Select().From(sql.Table("test_items"))
}

// renderPredicate attaches a predicate to a fresh selector of the given
// dialect and returns the rendered SQL and bound args (bare predicates only
// render in the context of a query).
func renderPredicate(t *testing.T, d string, p *sql.Predicate) (string, []any) {
	t.Helper()
	s := sql.Dialect(d).Select().From(sql.Table("test_items"))
	s.Where(p)
	q, args := s.Query()
	return q, args
}

// TestProcessor_EscapeSQLString covers the literal escaper.
func TestProcessor_EscapeSQLString(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{"it's", "it''s"},
		{`back\slash`, `back\\slash`},
		{"", ""},
	}
	for _, tc := range cases {
		if got := escapeSQLString(tc.in); got != tc.want {
			t.Errorf("escapeSQLString(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestProcessor_ComparisonOperators covers the comparison family that returns
// concrete predicates on every dialect.
func TestProcessor_ComparisonOperators(t *testing.T) {
	s := newSelector()
	p := sql.P()
	proc := NewProcessor()

	if got := proc.GTE(s, p, "created_at", "2024-01-01"); got == nil {
		t.Fatal("GTE returned nil")
	}
	if got := proc.GT(s, p, "created_at", "2024-01-01"); got == nil {
		t.Fatal("GT returned nil")
	}
	if got := proc.LTE(s, p, "created_at", "2024-12-31"); got == nil {
		t.Fatal("LTE returned nil")
	}
	if got := proc.LT(s, p, "created_at", "2024-12-31"); got == nil {
		t.Fatal("LT returned nil")
	}
	if got := proc.In(s, p, "name", "", []string{"a", "b"}); got == nil {
		t.Fatal("In(values) returned nil")
	}
	if got := proc.NotIn(s, p, "name", `["a"]`, nil); got == nil {
		t.Fatal("NotIn(json) returned nil")
	}
	if got := proc.NotIn(s, p, "name", "", []string{"a"}); got == nil {
		t.Fatal("NotIn(values) returned nil")
	}
	if got := proc.NotIn(s, p, "name", "notjson", nil); got != nil {
		t.Fatal("NotIn(invalid) must return nil")
	}
	if got := proc.Range(s, p, "created_at", "", []string{"1", "2"}); got == nil {
		t.Fatal("Range(values) returned nil")
	}
}

// TestProcessor_InsensitivesAndSearch covers the case-insensitive family and
// full-text fallback.
func TestProcessor_InsensitivesAndSearch(t *testing.T) {
	s := newSelector()
	p := sql.P()
	proc := NewProcessor()

	for name, call := range map[string]func() *sql.Predicate{
		"InsensitiveContains":   func() *sql.Predicate { return proc.InsensitiveContains(s, p, "name", "Go") },
		"InsensitiveStartsWith": func() *sql.Predicate { return proc.InsensitiveStartsWith(s, p, "name", "Go") },
		"InsensitiveEndsWith":   func() *sql.Predicate { return proc.InsensitiveEndsWith(s, p, "name", "Go") },
		"InsensitiveExact":      func() *sql.Predicate { return proc.InsensitiveExact(s, p, "name", "Go") },
		"Search":                func() *sql.Predicate { return proc.Search(s, p, "name", "term") },
		"Search empty":          func() *sql.Predicate { return proc.Search(s, p, "name", "  ") },
	} {
		if got := call(); got == nil {
			t.Fatalf("%s returned nil", name)
		}
	}
}

// TestProcessor_RegexDialects walks the regex branches per dialect.
func TestProcessor_RegexDialects(t *testing.T) {
	proc := NewProcessor()

	for _, d := range []string{dialect.Postgres, dialect.MySQL, dialect.SQLite, dialect.Gremlin} {
		s := newDialectSelector(d)
		p := sql.P()

		got := proc.Regex(s, p, "name", "^a")
		if got == nil {
			t.Fatalf("Regex(%s) returned nil", d)
		}
		rendered, _ := renderPredicate(t, d, got)

		got2 := proc.InsensitiveRegex(s, sql.P(), "name", "^a")
		if got2 == nil {
			t.Fatalf("InsensitiveRegex(%s) returned nil", d)
		}
		rendered2, args2 := renderPredicate(t, d, got2)

		if d == dialect.SQLite {
			if !strings.Contains(rendered2, "REGEXP") {
				t.Errorf("InsensitiveRegex(sqlite) must use REGEXP: %q", rendered2)
			}
			// the (?i) flag is added to the bound argument, not the SQL text
			if len(args2) == 0 || !strings.Contains(fmt.Sprintf("%v", args2[0]), "(?i)") {
				t.Errorf("InsensitiveRegex(sqlite) arg must carry (?i): %v", args2)
			}
		}
		if d == dialect.Postgres && !strings.Contains(rendered2, "~*") {
			t.Errorf("InsensitiveRegex(postgres) must use ~*: %q", rendered2)
		}
		if d == dialect.Postgres && !strings.Contains(rendered, " ~ ") {
			t.Errorf("Regex(postgres) must use the ~ operator: %q", rendered)
		}
	}
}

// TestProcessor_DatePartAndJsonb walks date-part and jsonb expression branches.
func TestProcessor_DatePartAndJsonb(t *testing.T) {
	proc := NewProcessor()

	for _, d := range []string{dialect.Postgres, dialect.MySQL, dialect.SQLite} {
		s := newDialectSelector(d)

		p := sql.P()
		if got := proc.DatePart(s, p, "year", "created_at"); got == nil {
			t.Fatalf("DatePart(%s) returned nil", d)
		}
		if got := proc.DatePartField(s, "month", "created_at"); got == "" {
			t.Fatalf("DatePartField(%s) returned empty", d)
		}
		if got := proc.DatePart(s, sql.P(), "not a part", "created_at"); got == nil {
			t.Fatalf("DatePart(invalid part) must return the original predicate")
		}
		if got := proc.DatePartField(s, "with space", "created_at"); got != "" {
			t.Fatalf("DatePartField(invalid part) must return empty, got %q", got)
		}

		p2 := sql.P()
		if got := proc.Jsonb(s, p2, "daily_email", "Preferences"); got == nil {
			t.Fatalf("Jsonb(%s) returned nil", d)
		}
		if got := proc.Jsonb(s, sql.P(), "bad key!", "preferences"); got == nil {
			t.Fatalf("Jsonb(invalid key) must return the original predicate")
		}
		if got := proc.JsonbFieldExpr(s, "daily_email", "preferences"); got == nil {
			t.Fatalf("JsonbFieldExpr(%s) returned nil", d)
		}
		if got := proc.JsonbField(s, "daily_email", "preferences"); got == "" {
			t.Fatalf("JsonbField(%s) returned empty", d)
		}
		if got := proc.JsonbField(s, "bad/key", "preferences"); got != "" {
			t.Fatalf("JsonbField(invalid key) must be empty, got %q", got)
		}
	}
}

// TestProcessor_ProcessDispatcher walks every operator through Process using an
// unknown table (fail-open) so all branches execute.
func TestProcessor_ProcessAllOperators(t *testing.T) {
	proc := NewProcessor()
	s := newDialectSelector(dialect.SQLite)

	val := func(v string) *paginationV1.FilterCondition_Value {
		return &paginationV1.FilterCondition_Value{Value: v}
	}

	cases := []struct {
		op     paginationV1.Operator
		value  string
		values []string
	}{
		{paginationV1.Operator_EQ, "tom", nil},
		{paginationV1.Operator_NEQ, "tom", nil},
		{paginationV1.Operator_IN, `["a"]`, nil},
		{paginationV1.Operator_IN, "", []string{"a"}},
		{paginationV1.Operator_NIN, `["a"]`, nil},
		{paginationV1.Operator_GTE, "1", nil},
		{paginationV1.Operator_GT, "1", nil},
		{paginationV1.Operator_LTE, "2", nil},
		{paginationV1.Operator_LT, "2", nil},
		{paginationV1.Operator_BETWEEN, `["1","2"]`, nil},
		{paginationV1.Operator_IS_NULL, "", nil},
		{paginationV1.Operator_IS_NOT_NULL, "", nil},
		{paginationV1.Operator_CONTAINS, "go", nil},
		{paginationV1.Operator_ICONTAINS, "go", nil},
		{paginationV1.Operator_STARTS_WITH, "go", nil},
		{paginationV1.Operator_ISTARTS_WITH, "go", nil},
		{paginationV1.Operator_ENDS_WITH, "go", nil},
		{paginationV1.Operator_IENDS_WITH, "go", nil},
		{paginationV1.Operator_EXACT, "go", nil},
		{paginationV1.Operator_IEXACT, "go", nil},
		{paginationV1.Operator_REGEXP, "^g", nil},
		{paginationV1.Operator_IREGEXP, "^g", nil},
		{paginationV1.Operator_SEARCH, "term", nil},
	}

	for _, tc := range cases {
		cond := &paginationV1.FilterCondition{
			Field:      "name",
			Op:         tc.op,
			ValueOneof: val(tc.value),
			Values:     tc.values,
		}
		if got := proc.Process(s, sql.P(), tc.op, "name", tc.value, tc.values); got == nil && tc.op != paginationV1.Operator(9999) {
			// IS NULL style ops and every listed op must yield a predicate;
			// In/NotIn/Range may return nil when values are unusable, which is
			// not the case here.
			_ = cond
			t.Errorf("Process(%v) returned nil", tc.op)
		}
	}

	// unknown field is rejected (fail-closed for known tables is covered by
	// the security tests); empty field returns the predicate untouched
	if got := proc.Process(s, sql.P(), paginationV1.Operator_EQ, "", "x", nil); got == nil {
		t.Fatal("Process with empty field must return the original predicate")
	}

	// unknown operator returns nil
	if got := proc.Process(s, sql.P(), paginationV1.Operator(9999), "name", "x", nil); got != nil {
		t.Fatal("unknown operator must return nil")
	}
}

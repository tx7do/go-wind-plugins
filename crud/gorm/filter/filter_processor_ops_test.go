package filter

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	paginationV1 "github.com/tx7do/go-wind-plugins/crud/api/gen/go/pagination/v1"
)

// hermeticFilterDB opens an isolated in-memory sqlite (single connection) so
// execution tests never share state through the cache=shared helper above.
func hermeticFilterDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

// drySQL builds the SQL for a selector without executing it.
func drySQL(db *gorm.DB, apply func(*gorm.DB) *gorm.DB) (string, error) {
	tx := apply(db.Session(&gorm.Session{DryRun: true}).Model(&User{}))
	var out []User
	err := tx.Find(&out).Error
	if tx.Statement == nil {
		return "", err
	}
	return tx.Statement.SQL.String(), err
}

// TestBuildExpression_AllOperators walks the full operator matrix of
// BuildExpression against the sqlite dialect (the default branch of every
// dialect switch).
func TestBuildExpression_AllOperators(t *testing.T) {
	db := hermeticFilterDB(t)
	proc := Processor{codec: NewProcessor().codec}

	cases := []struct {
		name     string
		op       paginationV1.Operator
		field    string
		value    string
		values   []string
		wantSQL  []string
		wantArgs int
		wantOK   bool
	}{
		{"eq", paginationV1.Operator_EQ, "name", "tom", nil, []string{"name = ?"}, 1, true},
		{"neq", paginationV1.Operator_NEQ, "name", "tom", nil, []string{"NOT (name = ?)"}, 1, true},
		{"gte", paginationV1.Operator_GTE, "age", "18", nil, []string{"age >= ?"}, 1, true},
		{"gt", paginationV1.Operator_GT, "age", "18", nil, []string{"age > ?"}, 1, true},
		{"lte", paginationV1.Operator_LTE, "age", "65", nil, []string{"age <= ?"}, 1, true},
		{"lt", paginationV1.Operator_LT, "age", "65", nil, []string{"age < ?"}, 1, true},
		{"in json value", paginationV1.Operator_IN, "name", `["a","b"]`, nil, []string{"name IN ?"}, 1, true},
		{"in values", paginationV1.Operator_IN, "name", "", []string{"a", "b"}, []string{"name IN ?"}, 1, true},
		{"in nothing", paginationV1.Operator_IN, "name", "", nil, nil, 0, false},
		{"nin json value", paginationV1.Operator_NIN, "name", `["a"]`, nil, []string{"name NOT IN ?"}, 1, true},
		{"nin values", paginationV1.Operator_NIN, "name", "", []string{"a"}, []string{"name NOT IN ?"}, 1, true},
		{"nin nothing", paginationV1.Operator_NIN, "name", "", nil, nil, 0, false},
		{"between json", paginationV1.Operator_BETWEEN, "created_at", `["2020-01-01","2021-01-01"]`, nil, []string{">= ?", "<= ?"}, 2, true},
		{"between values", paginationV1.Operator_BETWEEN, "created_at", "", []string{"2020-01-01", "2021-01-01"}, []string{">= ?", "<= ?"}, 2, true},
		{"between bad arity", paginationV1.Operator_BETWEEN, "created_at", "", []string{"only-one"}, nil, 0, false},
		{"is null", paginationV1.Operator_IS_NULL, "deleted_at", "", nil, []string{"deleted_at IS NULL"}, 0, true},
		{"is not null", paginationV1.Operator_IS_NOT_NULL, "deleted_at", "", nil, []string{"deleted_at IS NOT NULL"}, 0, true},
		{"contains", paginationV1.Operator_CONTAINS, "title", "go", nil, []string{"title LIKE ?"}, 1, true},
		{"icontains", paginationV1.Operator_ICONTAINS, "title", "Go", nil, []string{"LOWER(title) LIKE ?"}, 1, true},
		{"starts with", paginationV1.Operator_STARTS_WITH, "title", "go", nil, []string{"title LIKE ?"}, 1, true},
		{"istarts with", paginationV1.Operator_ISTARTS_WITH, "title", "Go", nil, []string{"LOWER(title) LIKE ?"}, 1, true},
		{"ends with", paginationV1.Operator_ENDS_WITH, "title", "go", nil, []string{"title LIKE ?"}, 1, true},
		{"iends with", paginationV1.Operator_IENDS_WITH, "title", "Go", nil, []string{"LOWER(title) LIKE ?"}, 1, true},
		{"exact", paginationV1.Operator_EXACT, "status", "active", nil, []string{"status = ?"}, 1, true},
		{"iexact", paginationV1.Operator_IEXACT, "status", "Active", nil, []string{"LOWER(status) = ?"}, 1, true},
		{"regexp", paginationV1.Operator_REGEXP, "title", "^a", nil, []string{"title REGEXP ?"}, 1, true},
		{"iregexp adds flag", paginationV1.Operator_IREGEXP, "title", "^a", nil, []string{"title REGEXP ?"}, 1, true},
		{"search sqlite falls back to like", paginationV1.Operator_SEARCH, "title", "q", nil, []string{"title LIKE ?"}, 1, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, ok := proc.BuildExpression(db, tc.op, tc.field, tc.value, tc.values)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (sql=%q args=%v)", ok, tc.wantOK, sql, args)
			}
			if !tc.wantOK {
				return
			}
			if sql == "" {
				t.Fatal("expected non-empty sql fragment")
			}
			lsql := strings.ToLower(sql)
			for _, want := range tc.wantSQL {
				if !strings.Contains(lsql, strings.ToLower(want)) {
					t.Fatalf("sql %q missing %q", sql, want)
				}
			}
			if len(args) != tc.wantArgs {
				t.Fatalf("args = %v, want %d entries", args, tc.wantArgs)
			}
		})
	}

	// iregexp must normalize the case-insensitive flag exactly once
	_, args, _ := proc.BuildExpression(db, paginationV1.Operator_IREGEXP, "title", "(?i)kept", nil)
	if len(args) != 1 || args[0] != "(?i)kept" {
		t.Fatalf("iregexp must not double the (?i) flag, got %v", args)
	}

	// guard paths: nil db and hostile field are rejected
	if _, _, ok := proc.BuildExpression(nil, paginationV1.Operator_EQ, "name", "x", nil); ok {
		t.Fatal("nil db must yield ok=false")
	}
	if _, _, ok := proc.BuildExpression(db, paginationV1.Operator_EQ, "name; --", "x", nil); ok {
		t.Fatal("hostile field must yield ok=false")
	}
	// value-required operators skip empty values instead of comparing to ''
	if _, _, ok := proc.BuildExpression(db, paginationV1.Operator_EQ, "name", "  ", nil); ok {
		t.Fatal("empty value for EQ must yield ok=false")
	}
}

// TestProcessor_MissingOperatorMethods covers the operator helpers and error
// paths not exercised elsewhere: comparisons, NotIn and the insensitive family.
func TestProcessor_MissingOperatorMethods(t *testing.T) {
	db := hermeticFilterDB(t)
	proc := NewProcessor()

	likeCases := []struct {
		name  string
		apply func(*gorm.DB) *gorm.DB
		want  []string
	}{
		{"GTE", func(tx *gorm.DB) *gorm.DB { return proc.GTE(tx, "status", "b") }, []string{"status >= ?"}},
		{"GT", func(tx *gorm.DB) *gorm.DB { return proc.GT(tx, "status", "a") }, []string{"status > ?"}},
		{"LTE", func(tx *gorm.DB) *gorm.DB { return proc.LTE(tx, "status", "z") }, []string{"status <= ?"}},
		{"LT", func(tx *gorm.DB) *gorm.DB { return proc.LT(tx, "status", "z") }, []string{"status < ?"}},
		{"NotIn values", func(tx *gorm.DB) *gorm.DB { return proc.NotIn(tx, "status", "", []string{"a", "b"}) }, []string{"NOT", "IN ("}},
		{"NotIn json", func(tx *gorm.DB) *gorm.DB { return proc.NotIn(tx, "status", `["a"]`, nil) }, []string{"NOT", "IN ("}},
		{"InsensitiveContains", func(tx *gorm.DB) *gorm.DB { return proc.InsensitiveContains(tx, "title", "go") }, []string{"LOWER(title) LIKE ?"}},
		{"InsensitiveStartsWith", func(tx *gorm.DB) *gorm.DB { return proc.InsensitiveStartsWith(tx, "title", "go") }, []string{"LOWER(title) LIKE ?"}},
		{"InsensitiveEndsWith", func(tx *gorm.DB) *gorm.DB { return proc.InsensitiveEndsWith(tx, "title", "go") }, []string{"LOWER(title) LIKE ?"}},
		{"InsensitiveExact", func(tx *gorm.DB) *gorm.DB { return proc.InsensitiveExact(tx, "status", "Active") }, []string{"LOWER(status) = ?"}},
		{"InsensitiveRegex", func(tx *gorm.DB) *gorm.DB { return proc.InsensitiveRegex(tx, "title", "go") }, []string{"REGEXP"}},
		{"Search", func(tx *gorm.DB) *gorm.DB { return proc.Search(tx, "title", "go") }, []string{"LIKE"}},
	}

	for _, tc := range likeCases {
		t.Run(tc.name, func(t *testing.T) {
			sql, err := drySQL(db, tc.apply)
			if err != nil {
				t.Fatalf("build sql error: %v", err)
			}
			if sql == "" {
				t.Fatal("expected non-empty sql")
			}
			lsql := strings.ToLower(sql)
			for _, w := range tc.want {
				if !strings.Contains(lsql, strings.ToLower(w)) {
					t.Fatalf("sql %q missing %q", sql, w)
				}
			}
		})
	}

	// insensitive regex on sqlite prefixes (?i) when missing
	tx := db.Session(&gorm.Session{DryRun: true})
	tx = proc.InsensitiveRegex(tx.Model(&User{}), "title", "^go")
	var out []User
	if err := tx.Find(&out).Error; err != nil {
		t.Fatalf("InsensitiveRegex build error: %v", err)
	}
	if len(tx.Statement.Vars) != 1 || tx.Statement.Vars[0] != "(?i)^go" {
		t.Fatalf("InsensitiveRegex arg = %v, want [(?i)^go]", tx.Statement.Vars)
	}

	// empty values never add conditions for value-required operators
	for name, apply := range map[string]func(*gorm.DB) *gorm.DB{
		"GTE":                 func(tx *gorm.DB) *gorm.DB { return proc.GTE(tx, "status", "  ") },
		"GT":                  func(tx *gorm.DB) *gorm.DB { return proc.GT(tx, "status", "") },
		"LTE":                 func(tx *gorm.DB) *gorm.DB { return proc.LTE(tx, "status", "") },
		"LT":                  func(tx *gorm.DB) *gorm.DB { return proc.LT(tx, "status", "") },
		"NotIn":               func(tx *gorm.DB) *gorm.DB { return proc.NotIn(tx, "status", "", nil) },
		"InsensitiveContains": func(tx *gorm.DB) *gorm.DB { return proc.InsensitiveContains(tx, "title", "") },
		"InsensitiveStartsWith": func(tx *gorm.DB) *gorm.DB {
			return proc.InsensitiveStartsWith(tx, "title", "")
		},
		"InsensitiveEndsWith": func(tx *gorm.DB) *gorm.DB { return proc.InsensitiveEndsWith(tx, "title", "") },
		"InsensitiveExact":    func(tx *gorm.DB) *gorm.DB { return proc.InsensitiveExact(tx, "status", "") },
		"InsensitiveRegex":    func(tx *gorm.DB) *gorm.DB { return proc.InsensitiveRegex(tx, "title", "") },
		"Search":              func(tx *gorm.DB) *gorm.DB { return proc.Search(tx, "title", "") },
	} {
		t.Run("empty value "+name, func(t *testing.T) {
			sql, err := drySQL(db, apply)
			if err != nil {
				t.Fatalf("build sql error: %v", err)
			}
			// the base soft-delete clause is always present; assert the
			// filtered column was not added to the where clause
			for _, col := range []string{"title", "status"} {
				if strings.Contains(strings.ToLower(sql), col) {
					t.Fatalf("empty value must not filter on %s, got %q", col, sql)
				}
			}
		})
	}

	// hostile fields fail closed on every method that validates input
	hostile := "id; drop table"
	for name, apply := range map[string]func(*gorm.DB) *gorm.DB{
		"GTE":                   func(tx *gorm.DB) *gorm.DB { return proc.GTE(tx, hostile, "1") },
		"GT":                    func(tx *gorm.DB) *gorm.DB { return proc.GT(tx, hostile, "1") },
		"LTE":                   func(tx *gorm.DB) *gorm.DB { return proc.LTE(tx, hostile, "1") },
		"LT":                    func(tx *gorm.DB) *gorm.DB { return proc.LT(tx, hostile, "1") },
		"NotIn":                 func(tx *gorm.DB) *gorm.DB { return proc.NotIn(tx, hostile, "", []string{"a"}) },
		"IsNull":                func(tx *gorm.DB) *gorm.DB { return proc.IsNull(tx, hostile) },
		"IsNotNull":             func(tx *gorm.DB) *gorm.DB { return proc.IsNotNull(tx, hostile) },
		"Contains":              func(tx *gorm.DB) *gorm.DB { return proc.Contains(tx, hostile, "1") },
		"InsensitiveContains":   func(tx *gorm.DB) *gorm.DB { return proc.InsensitiveContains(tx, hostile, "1") },
		"StartsWith":            func(tx *gorm.DB) *gorm.DB { return proc.StartsWith(tx, hostile, "1") },
		"InsensitiveStartsWith": func(tx *gorm.DB) *gorm.DB { return proc.InsensitiveStartsWith(tx, hostile, "1") },
		"EndsWith":              func(tx *gorm.DB) *gorm.DB { return proc.EndsWith(tx, hostile, "1") },
		"InsensitiveEndsWith":   func(tx *gorm.DB) *gorm.DB { return proc.InsensitiveEndsWith(tx, hostile, "1") },
		"Exact":                 func(tx *gorm.DB) *gorm.DB { return proc.Exact(tx, hostile, "1") },
		"InsensitiveExact":      func(tx *gorm.DB) *gorm.DB { return proc.InsensitiveExact(tx, hostile, "1") },
		"Regex":                 func(tx *gorm.DB) *gorm.DB { return proc.Regex(tx, hostile, "1") },
		"InsensitiveRegex":      func(tx *gorm.DB) *gorm.DB { return proc.InsensitiveRegex(tx, hostile, "1") },
		"Search":                func(tx *gorm.DB) *gorm.DB { return proc.Search(tx, hostile, "1") },
	} {
		t.Run("hostile field "+name, func(t *testing.T) {
			tx := apply(db.Session(&gorm.Session{NewDB: true}))
			if tx.Error == nil {
				t.Fatal("hostile field must raise an error")
			}
		})
	}
}

// TestProcessor_ProcessDispatchAll routes every operator through the Process
// dispatcher and checks the resulting SQL shape (plus no-op for unknown ops).
func TestProcessor_ProcessDispatchAll(t *testing.T) {
	db := hermeticFilterDB(t)
	proc := NewProcessor()

	val := func(v string) *paginationV1.FilterCondition_Value {
		return &paginationV1.FilterCondition_Value{Value: v}
	}
	_ = val

	cases := []struct {
		op      paginationV1.Operator
		value   string
		values  []string
		wantSQL []string
	}{
		{paginationV1.Operator_NEQ, "tom", nil, []string{"NOT"}},
		{paginationV1.Operator_NIN, "", []string{"tom"}, []string{"NOT", "IN ("}},
		{paginationV1.Operator_GTE, "18", nil, []string{">="}},
		{paginationV1.Operator_GT, "18", nil, []string{">"}},
		{paginationV1.Operator_LTE, "65", nil, []string{"<="}},
		{paginationV1.Operator_LT, "65", nil, []string{"<"}},
		{paginationV1.Operator_ICONTAINS, "Go", nil, []string{"LOWER"}},
		{paginationV1.Operator_ISTARTS_WITH, "Go", nil, []string{"LOWER"}},
		{paginationV1.Operator_IENDS_WITH, "Go", nil, []string{"LOWER"}},
		{paginationV1.Operator_IEXACT, "Active", nil, []string{"LOWER"}},
		{paginationV1.Operator_IREGEXP, "go", nil, []string{"REGEXP"}},
	}

	for _, tc := range cases {
		t.Run(tc.op.String(), func(t *testing.T) {
			sql, err := drySQL(db, func(tx *gorm.DB) *gorm.DB {
				return proc.Process(tx, tc.op, "title", tc.value, tc.values)
			})
			if err != nil {
				t.Fatalf("Process(%v) build error: %v", tc.op, err)
			}
			if sql == "" {
				t.Fatalf("Process(%v) produced empty sql", tc.op)
			}
			lsql := strings.ToLower(sql)
			for _, w := range tc.wantSQL {
				if !strings.Contains(lsql, strings.ToLower(w)) {
					t.Fatalf("Process(%v) sql %q missing %q", tc.op, sql, w)
				}
			}
		})
	}

	// unknown operator leaves the query untouched (no where, no error)
	sql, err := drySQL(db, func(tx *gorm.DB) *gorm.DB {
		return proc.Process(tx, paginationV1.Operator(9999), "title", "x", nil)
	})
	if err != nil {
		t.Fatalf("unknown op must not error: %v", err)
	}
	if strings.Contains(strings.ToLower(sql), "title") {
		t.Fatalf("unknown operator must not filter, got %q", sql)
	}

	// a field that is still invalid after snake_case conversion fails closed
	// through Process too (plain hostile text gets neutralized by ToSnakeCase)
	tx := proc.Process(db.Session(&gorm.Session{NewDB: true}), paginationV1.Operator_EQ, "", "x", nil)
	if tx.Error == nil {
		t.Fatal("invalid field through Process must raise an error")
	}
}

func hostileFieldName() string { return "name) OR (1=1" }

// TestProcessor_ParseJSONValues covers the shared JSON array parser.
func TestProcessor_ParseJSONValues(t *testing.T) {
	proc := NewProcessor()

	got, err := proc.parseJSONValues(`["a","b"]`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 values, got %d", len(got))
	}

	if _, err := proc.parseJSONValues(`not-json`); err == nil {
		t.Fatal("expected error for malformed json")
	}
}

// TestProcessor_DatePartValidation covers date-part validation and error paths.
func TestProcessor_DatePartValidation(t *testing.T) {
	db := hermeticFilterDB(t)
	proc := NewProcessor()

	if IsValidDatePartString("") || !IsValidDatePartString("year") || IsValidDatePartString("ye@r") {
		t.Fatal("IsValidDatePartString misclassifies input")
	}

	// invalid date part silently ignores the condition
	sql, err := drySQL(db, func(tx *gorm.DB) *gorm.DB { return proc.DatePart(tx, "not a part", "created_at") })
	if err != nil {
		t.Fatalf("invalid date part must be ignored: %v", err)
	}
	if strings.Contains(strings.ToLower(sql), "created_at") {
		t.Fatalf("invalid date part must not filter, got %q", sql)
	}

	// hostile column fails closed
	tx := proc.DatePart(db.Session(&gorm.Session{NewDB: true}), "year", hostileFieldName())
	if tx.Error == nil {
		t.Fatal("hostile date-part field must raise an error")
	}
}

// TestProcessor_JsonbGuards covers the jsonb helper guard clauses.
func TestProcessor_JsonbGuards(t *testing.T) {
	db := hermeticFilterDB(t)
	proc := NewProcessor()

	// empty or hostile json key: condition ignored, no error
	for _, key := range []string{"", "bad key!", "key-with-dash"} {
		sql, err := drySQL(db, func(tx *gorm.DB) *gorm.DB { return proc.Jsonb(tx, key, "preferences") })
		if err != nil {
			t.Fatalf("Jsonb(%q) must be ignored, got error %v", key, err)
		}
		if strings.Contains(strings.ToLower(sql), "preferences") {
			t.Fatalf("Jsonb(%q) must not filter, got %q", key, sql)
		}
	}

	// hostile column errors
	tx := proc.Jsonb(db.Session(&gorm.Session{NewDB: true}), "daily_email", hostileFieldName())
	if tx.Error == nil {
		t.Fatal("hostile column must raise an error")
	}

	// JsonbFieldExpr: valid key yields the sqlite expression; empty key and
	// hostile column yield an empty expression
	expr, _ := proc.JsonbFieldExpr(db, "daily_email", "preferences")
	if expr != "preferences ->> 'daily_email'" {
		t.Fatalf("unexpected expr %q", expr)
	}
	if got, _ := proc.JsonbFieldExpr(db, "", "preferences"); got != "" {
		t.Fatalf("empty key must give empty expr, got %q", got)
	}
	if got, _ := proc.JsonbFieldExpr(db, "daily_email", hostileFieldName()); got != "" {
		t.Fatalf("hostile column must give empty expr, got %q", got)
	}
	if got := proc.JsonbField(db, "daily_email", "preferences"); got != expr {
		t.Fatalf("JsonbField must mirror JsonbFieldExpr, got %q", got)
	}
}

// TestStructuredFilter_NestedGroups verifies recursive group expansion:
// nested groups become parenthesised fragments joined by the group operator.
func TestStructuredFilter_NestedGroups(t *testing.T) {
	db := hermeticFilterDB(t)
	sf := NewStructuredFilter()

	expr := &paginationV1.FilterExpr{
		Type: paginationV1.ExprType_AND,
		Conditions: []*paginationV1.FilterCondition{
			{Field: "name", Op: paginationV1.Operator_EQ, ValueOneof: &paginationV1.FilterCondition_Value{Value: "alice"}},
		},
		Groups: []*paginationV1.FilterExpr{
			{
				Type: paginationV1.ExprType_OR,
				Conditions: []*paginationV1.FilterCondition{
					{Field: "status", Op: paginationV1.Operator_EQ, ValueOneof: &paginationV1.FilterCondition_Value{Value: "active"}},
					{Field: "title", Op: paginationV1.Operator_CONTAINS, ValueOneof: &paginationV1.FilterCondition_Value{Value: "go"}},
				},
				Groups: []*paginationV1.FilterExpr{
					{
						Type: paginationV1.ExprType_AND,
						Conditions: []*paginationV1.FilterCondition{
							{Field: "status", Op: paginationV1.Operator_EQ, ValueOneof: &paginationV1.FilterCondition_Value{Value: "pending"}},
						},
					},
					{Type: paginationV1.ExprType_AND}, // empty group is skipped
				},
			},
		},
	}

	sels, err := sf.BuildSelectors(expr)
	if err != nil {
		t.Fatalf("BuildSelectors error: %v", err)
	}
	if len(sels) != 1 {
		t.Fatalf("expected a single selector, got %d", len(sels))
	}

	sql, err := drySQL(db, sels[0])
	if err != nil {
		t.Fatalf("apply selector error: %v", err)
	}
	for _, want := range []string{"name = ?", "(status = ? OR title LIKE ?", "status = ?)"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("sql %q missing %q", sql, want)
		}
	}
}

// TestStructuredFilter_EmptyValuesAreSkipped verifies that conditions with
// empty values inside a group do not abort the whole filter.
func TestStructuredFilter_EmptyValuesAreSkipped(t *testing.T) {
	db := hermeticFilterDB(t)
	sf := NewStructuredFilter()

	expr := &paginationV1.FilterExpr{
		Type: paginationV1.ExprType_AND,
		Conditions: []*paginationV1.FilterCondition{
			{Field: "name", Op: paginationV1.Operator_EQ, ValueOneof: &paginationV1.FilterCondition_Value{Value: ""}},
			{Field: "status", Op: paginationV1.Operator_IN, Values: []string{}},
			{Field: "title", Op: paginationV1.Operator_EQ, ValueOneof: &paginationV1.FilterCondition_Value{Value: "kept"}},
		},
	}
	sels, err := sf.BuildSelectors(expr)
	if err != nil {
		t.Fatalf("BuildSelectors error: %v", err)
	}
	sql, err := drySQL(db, sels[0])
	if err != nil {
		t.Fatalf("apply selector error: %v", err)
	}
	if !strings.Contains(sql, "title = ?") || strings.Contains(sql, "name = ?") || strings.Contains(sql, "status IN") {
		t.Fatalf("only the non-empty condition should survive, got %q", sql)
	}

	// a filter whose conditions are all empty produces no where clause at all
	empty := &paginationV1.FilterExpr{
		Type: paginationV1.ExprType_AND,
		Conditions: []*paginationV1.FilterCondition{
			{Field: "name", Op: paginationV1.Operator_EQ, ValueOneof: &paginationV1.FilterCondition_Value{Value: ""}},
		},
	}
	sels2, err := sf.BuildSelectors(empty)
	if err != nil {
		t.Fatalf("BuildSelectors error: %v", err)
	}
	sql2, err := drySQL(db, sels2[0])
	if err != nil {
		t.Fatalf("apply selector error: %v", err)
	}
	if strings.Contains(sql2, "name = ?") {
		t.Fatalf("expected no condition for all-empty filter, got %q", sql2)
	}
}

// TestStructuredFilter_JsonFieldAndHostileGroup verifies dotted JSON field
// conditions and fail-closed behaviour inside groups.
func TestStructuredFilter_JsonFieldAndHostileGroup(t *testing.T) {
	db := hermeticFilterDB(t)
	sf := NewStructuredFilter()

	// dotted field routes through JsonbFieldExpr (sqlite: ->>)
	expr := &paginationV1.FilterExpr{
		Type: paginationV1.ExprType_AND,
		Conditions: []*paginationV1.FilterCondition{
			{Field: "preferences.daily_email", Op: paginationV1.Operator_EQ, ValueOneof: &paginationV1.FilterCondition_Value{Value: "true"}},
		},
	}
	sels, err := sf.BuildSelectors(expr)
	if err != nil {
		t.Fatalf("BuildSelectors error: %v", err)
	}
	sql, err := drySQL(db, sels[0])
	if err != nil {
		t.Fatalf("apply selector error: %v", err)
	}
	if !strings.Contains(sql, "preferences ->> 'daily_email'") {
		t.Fatalf("expected json expression in sql, got %q", sql)
	}

	// hostile dotted field fails closed
	bad := &paginationV1.FilterExpr{
		Type: paginationV1.ExprType_AND,
		Conditions: []*paginationV1.FilterCondition{
			{Field: "preferences.a b", Op: paginationV1.Operator_EQ, ValueOneof: &paginationV1.FilterCondition_Value{Value: "x"}},
		},
	}
	sels2, err := sf.BuildSelectors(bad)
	if err != nil {
		t.Fatalf("BuildSelectors error: %v", err)
	}
	tx := sels2[0](db)
	var out []User
	if err := tx.Find(&out).Error; err == nil {
		t.Fatal("hostile json field must fail the query")
	}
}

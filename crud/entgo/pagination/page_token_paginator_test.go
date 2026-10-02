package pagination

import (
	"strings"
	"testing"

	"entgo.io/ent/dialect/sql"

	curdPagination "github.com/tx7do/go-wind-plugins/crud/pagination"
)

func applySelector(t *testing.T, sel func(*sql.Selector)) string {
	t.Helper()
	s := sql.Select("*").From(sql.Table("users"))
	sel(s)
	q, _ := s.Query()
	return q
}

func TestPagePaginator_BuildSelector(t *testing.T) {
	p := NewPagePaginator()
	if p == nil {
		t.Fatal("NewPagePaginator must return a paginator")
	}

	// page 3 with size 20 => OFFSET 40 LIMIT 20
	q := applySelector(t, p.BuildSelector(3, 20))
	if !strings.Contains(q, "LIMIT 20") {
		t.Errorf("expected LIMIT 20, got %q", q)
	}
	if !strings.Contains(q, "OFFSET 40") {
		t.Errorf("expected OFFSET 40, got %q", q)
	}

	// page below 1 is clamped to the first page
	q = applySelector(t, p.BuildSelector(0, 5))
	if !strings.Contains(q, "LIMIT 5") || !strings.Contains(q, "OFFSET 0") {
		t.Errorf("expected first page window, got %q", q)
	}

	// the selector stays reusable and reflects the latest arguments
	q = applySelector(t, p.BuildSelector(2, 7))
	if !strings.Contains(q, "LIMIT 7") || !strings.Contains(q, "OFFSET 7") {
		t.Errorf("expected OFFSET 7 LIMIT 7, got %q", q)
	}
}

func TestTokenPaginator_BuildSelector_EmptyToken(t *testing.T) {
	p := NewTokenPaginator()
	q := applySelector(t, p.BuildSelector("", 15))
	if !strings.Contains(q, "LIMIT 15") {
		t.Errorf("expected LIMIT 15, got %q", q)
	}
	if strings.Contains(q, "WHERE") {
		t.Errorf("empty token must not add a cursor predicate, got %q", q)
	}
}

func TestTokenPaginator_BuildSelector_InvalidToken(t *testing.T) {
	p := NewTokenPaginator()
	// malformed cursors degrade to a plain page-size limit instead of
	// injecting attacker-controlled SQL
	for _, tok := range []string{"garbage", "v2.abc.def", "!!!!"} {
		q := applySelector(t, p.BuildSelector(tok, 9))
		if !strings.Contains(q, "LIMIT 9") {
			t.Errorf("token %q: expected LIMIT 9, got %q", tok, q)
		}
		if strings.Contains(q, "WHERE") {
			t.Errorf("token %q: invalid token must not add predicates, got %q", tok, q)
		}
	}
}

func TestTokenPaginator_BuildSelector_ValidToken(t *testing.T) {
	p := NewTokenPaginator()
	token := curdPagination.EncodeAndSign(42, curdPagination.TokenSecret())
	q := applySelector(t, p.BuildSelector(token, 10))
	if !strings.Contains(q, "LIMIT 10") {
		t.Errorf("expected LIMIT 10, got %q", q)
	}
	// the decoded cursor becomes the keyset predicate: id > lastID
	// (the value itself is bound as a query argument)
	s := sql.Select("*").From(sql.Table("users"))
	p.BuildSelector(token, 10)(s)
	_, args := s.Query()
	if len(args) != 1 {
		t.Fatalf("expected one bound cursor argument, got %v", args)
	}
	if id, ok := args[0].(int64); !ok || id != 42 {
		t.Fatalf("expected bound cursor 42, got %v", args[0])
	}
}

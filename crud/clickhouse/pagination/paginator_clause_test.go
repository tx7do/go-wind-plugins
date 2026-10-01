package pagination

import (
	"strings"
	"testing"

	"github.com/tx7do/go-wind-plugins/crud/clickhouse/query"
)

// TestOffsetPaginator_BuildClause covers skip/limit clause generation.
func TestOffsetPaginator_BuildClause(t *testing.T) {
	// offset beyond zero yields LIMIT ... OFFSET ...
	b := query.NewQueryBuilder("t1", nil)
	NewOffsetPaginator().BuildClause(b, 20, 5)
	sqlStr, _ := b.Build()
	if !strings.Contains(sqlStr, "LIMIT 5") || !strings.Contains(sqlStr, "OFFSET 20") {
		t.Fatalf("expected LIMIT 5 OFFSET 20 in %q", sqlStr)
	}

	// zero offset yields only the limit
	b2 := query.NewQueryBuilder("t1", nil)
	NewOffsetPaginator().BuildClause(b2, 0, 7)
	sql2, _ := b2.Build()
	if !strings.Contains(sql2, "LIMIT 7") || strings.Contains(sql2, "OFFSET") {
		t.Fatalf("expected LIMIT 7 without OFFSET in %q", sql2)
	}

	// negative offset clamps to zero; zero limit clamps to 1
	b3 := query.NewQueryBuilder("t1", nil)
	NewOffsetPaginator().BuildClause(b3, -5, 0)
	sql3, _ := b3.Build()
	if !strings.Contains(sql3, "LIMIT 1") || strings.Contains(sql3, "OFFSET") {
		t.Fatalf("expected LIMIT 1 without OFFSET in %q", sql3)
	}
}

// TestPagePaginator_BuildClause covers page/size to LIMIT/OFFSET conversion.
func TestPagePaginator_BuildClause(t *testing.T) {
	// page 3, size 5 => offset (3-1)*5 = 10
	b := query.NewQueryBuilder("t1", nil)
	NewPagePaginator().BuildClause(b, 3, 5)
	sqlStr, _ := b.Build()
	if !strings.Contains(sqlStr, "LIMIT 5") || !strings.Contains(sqlStr, "OFFSET 10") {
		t.Fatalf("expected LIMIT 5 OFFSET 10 in %q", sqlStr)
	}

	// invalid page/size clamp to the first page / limit 1
	b2 := query.NewQueryBuilder("t1", nil)
	NewPagePaginator().BuildClause(b2, 0, 0)
	sql2, _ := b2.Build()
	if !strings.Contains(sql2, "LIMIT 1") || strings.Contains(sql2, "OFFSET") {
		t.Fatalf("expected LIMIT 1 without OFFSET in %q", sql2)
	}
}

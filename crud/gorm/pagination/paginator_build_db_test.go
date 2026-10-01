package pagination

import (
	"testing"

	"github.com/tx7do/go-wind-plugins/crud/pagination/paginator"
)

// TestOffsetPaginator_BuildDB verifies that the offset/limit pair is really
// applied to the query: rows after the offset are returned, at most limit of
// them, and a nil db passes straight through.
func TestOffsetPaginator_BuildDB(t *testing.T) {
	db := openTokenPaginatorDB(t)

	// skip the first row, take two
	var rows []tokenRow
	if err := NewOffsetPaginator().BuildDB(1, 2)(db.Model(&tokenRow{})).Find(&rows).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].ID != 2 || rows[1].ID != 3 {
		t.Fatalf("expected ids 2,3 after offset 1, got %d,%d", rows[0].ID, rows[1].ID)
	}

	// offset beyond the result set yields no rows
	var empty []tokenRow
	if err := NewOffsetPaginator().BuildDB(1000, 2)(db.Model(&tokenRow{})).Find(&empty).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected 0 rows past the end, got %d", len(empty))
	}

	// negative values are clamped by the underlying paginator (offset 0,
	// default-ish limit) instead of producing invalid SQL
	var first []tokenRow
	if err := NewOffsetPaginator().BuildDB(-5, -1)(db.Model(&tokenRow{})).Find(&first).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(first) == 0 || first[0].ID != 1 {
		t.Fatalf("expected first page from negative input, got %d rows", len(first))
	}

	// nil db must be returned untouched
	if got := NewOffsetPaginator().BuildDB(0, 10)(nil); got != nil {
		t.Fatal("BuildDB closure must pass nil db through")
	}
}

// TestPagePaginator_BuildDB verifies page/size semantics: page is 1-based and
// converts to (page-1)*size offset; invalid input falls back to page 1.
func TestPagePaginator_BuildDB(t *testing.T) {
	db := openTokenPaginatorDB(t)

	// page 3, size 2 => offset 4, ids 5 and 6
	var rows []tokenRow
	if err := NewPagePaginator().BuildDB(3, 2)(db.Model(&tokenRow{})).Find(&rows).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].ID != 5 || rows[1].ID != 6 {
		t.Fatalf("expected ids 5,6 on page 3, got %d,%d", rows[0].ID, rows[1].ID)
	}

	// page/size below 1 fall back to the first page
	var first []tokenRow
	if err := NewPagePaginator().BuildDB(0, 0)(db.Model(&tokenRow{})).Find(&first).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(first) == 0 || first[0].ID != 1 {
		t.Fatalf("expected first page for invalid input, got %d rows", len(first))
	}

	// oversized page size is capped by paginator.MaxLimit
	orig := paginator.MaxLimit
	defer func() { paginator.MaxLimit = orig }()
	paginator.MaxLimit = 40

	var capped []tokenRow
	if err := NewPagePaginator().BuildDB(1, 4000000000)(db.Model(&tokenRow{})).Find(&capped).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(capped) != 40 {
		t.Fatalf("expected capped 40 rows, got %d", len(capped))
	}

	// nil db must be returned untouched
	if got := NewPagePaginator().BuildDB(1, 10)(nil); got != nil {
		t.Fatal("BuildDB closure must pass nil db through")
	}
}

package paginator

import (
	"testing"

	"github.com/tx7do/go-wind-plugins/crud/pagination"
)

// TestOffsetPaginatorBehavior exercises the offset paginator accessors,
// chainable setters and clamping rules end to end.
func TestOffsetPaginatorBehavior(t *testing.T) {
	// invalid constructor input falls back to defaults and clamping
	p := NewOffsetPaginator(-5, 0)
	if p.Offset() != DefaultOffset {
		t.Fatalf("negative offset must fall back to %d, got %d", DefaultOffset, p.Offset())
	}
	if p.Limit() != DefaultLimit {
		t.Fatalf("invalid limit must fall back to %d, got %d", DefaultLimit, p.Limit())
	}
	if p.Mode() != pagination.ModeOffset {
		t.Fatalf("mode = %v, want ModeOffset", p.Mode())
	}

	// defaults constructor
	d := NewOffsetPaginatorWithDefault()
	if d.Offset() != DefaultOffset || d.Limit() != DefaultLimit {
		t.Fatalf("default offset paginator = (%d,%d)", d.Offset(), d.Limit())
	}

	// Page is derived from offset/limit
	p.WithOffset(20).WithLimit(10)
	if got := p.Page(); got != 3 {
		t.Fatalf("Page() = %d, want 3", got)
	}
	if got := p.Size(); got != 10 {
		t.Fatalf("Size() = %d, want 10", got)
	}

	// negative stored offset still reads as 0
	neg := NewOffsetPaginatorWithDefault()
	neg.WithOffset(-3)
	if neg.Offset() != 0 {
		t.Fatalf("WithOffset(-3) must clamp to 0, got %d", neg.Offset())
	}

	// WithLimit below 1 clamps to 1
	p.WithLimit(0)
	if p.Limit() != 1 {
		t.Fatalf("WithLimit(0) must clamp to 1, got %d", p.Limit())
	}
	// WithSize is an alias of WithLimit
	p.WithSize(5)
	if p.Limit() != 5 {
		t.Fatalf("WithSize(5) must set limit 5, got %d", p.Limit())
	}

	// WithPage maps page back to offset ((page-1)*limit)
	p.WithLimit(10).WithPage(3)
	if p.Offset() != 20 {
		t.Fatalf("WithPage(3) offset = %d, want 20", p.Offset())
	}
	p.WithPage(0)
	if p.Offset() != 0 {
		t.Fatalf("WithPage(0) must clamp to page 1, got offset %d", p.Offset())
	}

	// token surface is a no-op for offset mode
	if p.Token() != "" || p.NextToken() != "" || p.PrevToken() != "" {
		t.Fatal("offset paginator must not carry tokens")
	}
	p.SetToken("t")
	p.SetNextToken("n")
	p.SetPrevToken("p")
	if p.Token() != "" || p.NextToken() != "" || p.PrevToken() != "" {
		t.Fatal("offset paginator token setters must be no-ops")
	}
	if p.WithToken("x") == nil {
		t.Fatal("WithToken must return the paginator")
	}

	// total / paging stats
	p.SetTotal(-1)
	if p.Total() != 0 {
		t.Fatalf("negative total must clamp to 0, got %d", p.Total())
	}
	if p.TotalPages() != 1 {
		t.Fatalf("TotalPages(0 total) = %d, want 1", p.TotalPages())
	}
	p.SetTotal(25)
	p.WithLimit(10)
	if p.TotalPages() != 3 {
		t.Fatalf("TotalPages(25/10) = %d, want 3", p.TotalPages())
	}
	if !p.HasNext() {
		t.Fatal("offset 0 of 25 at limit 10 must have next")
	}
	if p.HasPrev() {
		t.Fatal("offset 0 must not have prev")
	}
	p.WithOffset(20)
	if p.HasNext() {
		t.Fatal("offset 20 of 25 at limit 10 must not have next")
	}
	if !p.HasPrev() {
		t.Fatal("offset 20 must have prev")
	}
}

// TestPagePaginatorBehavior exercises the page paginator accessors, setters
// and the offset-to-page conversion.
func TestPagePaginatorBehavior(t *testing.T) {
	// invalid constructor input falls back to defaults
	p := NewPagePaginator(0, -1)
	if p.Page() != DefaultPage || p.Size() != DefaultPageSize {
		t.Fatalf("invalid input must fall back to (%d,%d), got (%d,%d)",
			DefaultPage, DefaultPageSize, p.Page(), p.Size())
	}
	if p.Mode() != pagination.ModePage {
		t.Fatalf("mode = %v, want ModePage", p.Mode())
	}

	d := NewPagePaginatorWithDefault()
	if d.Page() != DefaultPage || d.Size() != DefaultPageSize {
		t.Fatalf("default page paginator = (%d,%d)", d.Page(), d.Size())
	}

	// Offset derives from (page-1)*size
	p.WithPage(3).WithSize(10)
	if got := p.Offset(); got != 20 {
		t.Fatalf("Offset() = %d, want 20", got)
	}
	if got := p.Limit(); got != 10 {
		t.Fatalf("Limit() = %d, want 10", got)
	}

	// page below 1 clamps to 1, size below 1 clamps to 1
	p.WithPage(-2)
	if p.Page() != 1 {
		t.Fatalf("WithPage(-2) must clamp to 1, got %d", p.Page())
	}
	p.WithSize(0)
	if p.Size() != 1 {
		t.Fatalf("WithSize(0) must clamp to 1, got %d", p.Size())
	}
	// WithLimit is an alias of WithSize
	p.WithLimit(7)
	if p.Size() != 7 {
		t.Fatalf("WithLimit(7) must set size 7, got %d", p.Size())
	}

	// WithOffset converts back to a page (ceil division)
	p.WithOffset(15)
	if p.Page() != 3 {
		t.Fatalf("WithOffset(15) at size 7 must yield page 3, got %d", p.Page())
	}
	p.WithOffset(-1)
	if p.Page() != 1 {
		t.Fatalf("WithOffset(-1) must clamp to page 1, got %d", p.Page())
	}

	// token surface is a no-op for page mode
	p.SetToken("t")
	if p.Token() != "" || p.NextToken() != "" || p.PrevToken() != "" {
		t.Fatal("page paginator token setters must be no-ops")
	}
	if p.WithToken("x") == nil {
		t.Fatal("WithToken must return the paginator")
	}

	// totals and neighbors
	p.SetTotal(-3)
	if p.Total() != 0 {
		t.Fatalf("negative total must clamp to 0, got %d", p.Total())
	}
	if p.TotalPages() != 1 {
		t.Fatalf("TotalPages(0 total) = %d, want 1", p.TotalPages())
	}
	p.SetTotal(15)
	p.WithSize(10)
	if p.TotalPages() != 2 {
		t.Fatalf("TotalPages(15/10) = %d, want 2", p.TotalPages())
	}
	if !p.HasNext() || p.HasPrev() {
		t.Fatal("page 1 of 2 must have next and no prev")
	}
	p.WithPage(2)
	if p.HasNext() || !p.HasPrev() {
		t.Fatal("page 2 of 2 must have prev and no next")
	}
}

// TestTokenPaginatorBehavior exercises the token paginator accessors and the
// unknown-total TotalPages convention.
func TestTokenPaginatorBehavior(t *testing.T) {
	p := NewTokenPaginator("start", -1)
	if p.Limit() != DefaultLimit {
		t.Fatalf("invalid limit must fall back to %d, got %d", DefaultLimit, p.Limit())
	}
	if p.Token() != "start" {
		t.Fatalf("constructor token = %q", p.Token())
	}
	if p.Mode() != pagination.ModeToken {
		t.Fatalf("mode = %v, want ModeToken", p.Mode())
	}

	d := NewTokenPaginatorWithDefault()
	if d.Limit() != DefaultLimit || d.Token() != "" {
		t.Fatalf("default token paginator = (%q,%d)", d.Token(), d.Limit())
	}

	// token mode reports page 1 / offset 0 regardless of state
	p.WithPage(5).WithOffset(99)
	if p.Page() != 1 || p.Offset() != 0 {
		t.Fatalf("token mode must ignore page/offset, got (%d,%d)", p.Page(), p.Offset())
	}

	// size clamping and alias
	p.WithSize(0)
	if p.Size() != 1 {
		t.Fatalf("WithSize(0) must clamp to 1, got %d", p.Size())
	}
	p.WithSize(30)
	if p.Limit() != 30 {
		t.Fatalf("Size/Limit mismatch: %d vs %d", p.Size(), p.Limit())
	}
	p.WithLimit(0)
	if p.Limit() != 1 {
		t.Fatalf("WithLimit(0) must clamp to 1, got %d", p.Limit())
	}

	// token getters and setters round-trip
	p.SetToken("cur")
	p.SetNextToken("next")
	p.SetPrevToken("prev")
	if p.Token() != "cur" || p.NextToken() != "next" || p.PrevToken() != "prev" {
		t.Fatalf("token round-trip failed: %q %q %q", p.Token(), p.NextToken(), p.PrevToken())
	}
	if p.WithToken("cur2").Token() != "cur2" {
		t.Fatal("WithToken must set the token")
	}

	// totals: unknown total reports 0 pages
	p.SetTotal(-1)
	if p.Total() != 0 {
		t.Fatalf("negative total must clamp to 0, got %d", p.Total())
	}
	if p.TotalPages() != 0 {
		t.Fatalf("TotalPages(unknown) = %d, want 0", p.TotalPages())
	}
	p.SetTotal(25)
	p.WithSize(10)
	if p.TotalPages() != 3 {
		t.Fatalf("TotalPages(25/10) = %d, want 3", p.TotalPages())
	}

	// HasNext/HasPrev are driven by the next/prev tokens (still set above)
	if !p.HasNext() || !p.HasPrev() {
		t.Fatal("next/prev tokens set but HasNext/HasPrev false")
	}
	fresh := NewTokenPaginatorWithDefault()
	if fresh.HasNext() || fresh.HasPrev() {
		t.Fatal("fresh paginator must not have next/prev")
	}
}

package pagination

import (
	"testing"

	"github.com/tx7do/go-wind-plugins/crud/mongodb/query"
)

// TestOffsetPaginator_BuildClause covers skip/limit application and clamping.
func TestOffsetPaginator_BuildClause(t *testing.T) {
	// offset beyond zero sets both skip and limit
	b := query.NewQueryBuilder()
	NewOffsetPaginator().BuildClause(b, 20, 5)
	_, opts := b.Build()
	if opts == nil || opts.Skip == nil || *opts.Skip != int64(20) {
		t.Fatalf("expected skip 20, got %+v", opts)
	}
	if opts.Limit == nil || *opts.Limit != int64(5) {
		t.Fatalf("expected limit 5, got %+v", opts)
	}

	// zero offset sets only the limit
	b2 := query.NewQueryBuilder()
	NewOffsetPaginator().BuildClause(b2, 0, 7)
	_, opts2 := b2.Build()
	if opts2 == nil || opts2.Skip != nil {
		t.Fatalf("zero offset must not set skip, got %+v", opts2)
	}
	if opts2.Limit == nil || *opts2.Limit != int64(7) {
		t.Fatalf("expected limit 7, got %+v", opts2)
	}

	// negative offset clamps to zero, zero limit clamps to 1
	b3 := query.NewQueryBuilder()
	NewOffsetPaginator().BuildClause(b3, -5, 0)
	_, opts3 := b3.Build()
	if opts3 == nil || opts3.Skip != nil || opts3.Limit == nil || *opts3.Limit != int64(1) {
		t.Fatalf("negative input clamping wrong: %+v", opts3)
	}
}

// TestPagePaginator_BuildClause covers page/size to skip/limit conversion.
func TestPagePaginator_BuildClause(t *testing.T) {
	// page 3, size 5 => skip 10, limit 5
	b := query.NewQueryBuilder()
	NewPagePaginator().BuildClause(b, 3, 5)
	_, opts := b.Build()
	if opts == nil || opts.Skip == nil || *opts.Skip != int64(10) {
		t.Fatalf("expected skip 10, got %+v", opts)
	}
	if opts.Limit == nil || *opts.Limit != int64(5) {
		t.Fatalf("expected limit 5, got %+v", opts)
	}

	// invalid page/size clamp to page 1 / size 1
	b2 := query.NewQueryBuilder()
	NewPagePaginator().BuildClause(b2, 0, 0)
	_, opts2 := b2.Build()
	if opts2 == nil || opts2.Skip != nil {
		t.Fatalf("first page must not set skip, got %+v", opts2)
	}
	if opts2.Limit == nil || *opts2.Limit != int64(1) {
		t.Fatalf("invalid size must clamp to 1, got %+v", opts2)
	}

}

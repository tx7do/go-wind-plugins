package pagination

import (
	"strings"
	"testing"

	"github.com/tx7do/go-wind-plugins/crud/doris/query"
)

// TestOffsetPaginator_BuildClause 验证 offset 分页子句的生成：
// offset>0 时 LIMIT/OFFSET 同现，offset=0 时仅 LIMIT，limit<=0 时不改写 builder。
func TestOffsetPaginator_BuildClause(t *testing.T) {
	// offset > 0：LIMIT 与 OFFSET 同现
	p := NewOffsetPaginator()
	b := query.NewQueryBuilder("t1", nil)
	p.BuildClause(b, 30, 20)
	sql, args := b.Build()
	if !strings.Contains(sql, "LIMIT 20") || !strings.Contains(sql, "OFFSET 30") {
		t.Errorf("expected LIMIT 20 OFFSET 30, got %q", sql)
	}
	if len(args) != 0 {
		t.Errorf("paging clause must not bind args, got %v", args)
	}

	// offset == 0：仅 LIMIT
	b = query.NewQueryBuilder("t1", nil)
	NewOffsetPaginator().BuildClause(b, 0, 5)
	sql, _ = b.Build()
	if !strings.Contains(sql, "LIMIT 5") || strings.Contains(sql, "OFFSET") {
		t.Errorf("expected bare LIMIT 5, got %q", sql)
	}

	// NOTE: 文档注释称 limit<=0 时返回空子句，但实现里 Limit() 会把非正值
	// 兜底为 1，实际产出 "LIMIT 1"。测试对齐当前行为（注释与行为不一致，
	// 见报告）。offset 仍被保留。
	b = query.NewQueryBuilder("t1", nil)
	ret := NewOffsetPaginator().BuildClause(b, 10, 0)
	if ret == nil {
		t.Fatal("BuildClause must return the builder")
	}
	sql, _ = b.Build()
	if !strings.Contains(sql, "LIMIT 1 OFFSET 10") {
		t.Errorf("invalid limit must fall back to LIMIT 1, got %q", sql)
	}
}

// TestPagePaginator_BuildClause 验证页码分页子句的生成：
// page>1 时 LIMIT/OFFSET 同现，第一页仅 LIMIT，size<=0 时不改写 builder。
func TestPagePaginator_BuildClause(t *testing.T) {
	// page 3, size 10 => OFFSET 20 LIMIT 10
	p := NewPagePaginator()
	b := query.NewQueryBuilder("t1", nil)
	p.BuildClause(b, 3, 10)
	sql, args := b.Build()
	if !strings.Contains(sql, "LIMIT 10") || !strings.Contains(sql, "OFFSET 20") {
		t.Errorf("expected LIMIT 10 OFFSET 20, got %q", sql)
	}
	if len(args) != 0 {
		t.Errorf("paging clause must not bind args, got %v", args)
	}

	// 第一页：仅 LIMIT
	b = query.NewQueryBuilder("t1", nil)
	NewPagePaginator().BuildClause(b, 1, 7)
	sql, _ = b.Build()
	if !strings.Contains(sql, "LIMIT 7") || strings.Contains(sql, "OFFSET") {
		t.Errorf("expected bare LIMIT 7 on the first page, got %q", sql)
	}

	// NOTE: 同上，size<=0 被 Size() 兜底为 1，产出 "LIMIT 1 OFFSET 1"。
	b = query.NewQueryBuilder("t1", nil)
	NewPagePaginator().BuildClause(b, 2, 0)
	sql, _ = b.Build()
	if !strings.Contains(sql, "LIMIT 1 OFFSET 1") {
		t.Errorf("invalid size must fall back to LIMIT 1, got %q", sql)
	}
}

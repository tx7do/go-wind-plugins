package doris

import (
	"context"
	"strings"
	"testing"

	"github.com/tx7do/go-wind-plugins/crud/viewer"
)

// TestInjectTenantFilterIntoBaseWhere_NonScoped passes the where clause through
// untouched for entities without a tenant mixin.
func TestInjectTenantFilterIntoBaseWhere_NonScoped(t *testing.T) {
	ctx := viewer.WithContext(context.Background(), testEnforceViewer{tid: 7})
	where, args, err := InjectTenantFilterIntoBaseWhere[nonScopedEntity](ctx, "id = ?", []any{1})
	if err != nil {
		t.Fatalf("non-scoped entity must pass through: %v", err)
	}
	if where != "id = ?" || len(args) != 1 {
		t.Fatalf("where/args changed: %q %v", where, args)
	}
}

// TestInjectTenantFilterIntoBaseWhere_ScopedTenantContext covers the injection
// matrix for tenant-scoped entities in a tenant business view.
func TestInjectTenantFilterIntoBaseWhere_ScopedTenantContext(t *testing.T) {
	ctx := viewer.WithContext(context.Background(), testEnforceViewer{tid: 7})

	// empty baseWhere becomes a bare tenant predicate
	where, args, err := InjectTenantFilterIntoBaseWhere[scopedEntity](ctx, "", nil)
	if err != nil {
		t.Fatalf("empty baseWhere error: %v", err)
	}
	if where != "WHERE tenant_id = ?" || len(args) != 1 {
		t.Fatalf("empty baseWhere => %q %v", where, args)
	}

	// condition without WHERE prefix gets one
	where, args, err = InjectTenantFilterIntoBaseWhere[scopedEntity](ctx, "id = ?", []any{1})
	if err != nil {
		t.Fatalf("bare condition error: %v", err)
	}
	if !strings.HasPrefix(where, "WHERE id = ? AND tenant_id = ?") {
		t.Fatalf("bare condition => %q", where)
	}
	if len(args) != 2 {
		t.Fatalf("tenant arg must be appended, got %v", args)
	}

	// condition already starting with WHERE is appended in place
	where, _, err = InjectTenantFilterIntoBaseWhere[scopedEntity](ctx, "WHERE x = 1", nil)
	if err != nil {
		t.Fatalf("where-prefixed condition error: %v", err)
	}
	if where != "WHERE x = 1 AND tenant_id = ?" {
		t.Fatalf("where-prefixed condition => %q", where)
	}

	// surrounding whitespace is trimmed before the WHERE detection
	where, _, err = InjectTenantFilterIntoBaseWhere[scopedEntity](ctx, "   WHERE x = 1  ", nil)
	if err != nil {
		t.Fatalf("whitespace condition error: %v", err)
	}
	if where != "WHERE x = 1 AND tenant_id = ?" {
		t.Fatalf("whitespace condition => %q", where)
	}
}

// TestInjectTenantFilterIntoBaseWhere_PlatformContext keeps the predicate out
// for platform viewers.
func TestInjectTenantFilterIntoBaseWhere_PlatformContext(t *testing.T) {
	ctx := viewer.WithContext(context.Background(), testEnforceViewer{tid: 0, platform: true})

	where, args, err := InjectTenantFilterIntoBaseWhere[scopedEntity](ctx, "id = ?", []any{1})
	if err != nil {
		t.Fatalf("platform context error: %v", err)
	}
	if where != "id = ?" || len(args) != 1 {
		t.Fatalf("platform context must not inject: %q %v", where, args)
	}
}

// TestInjectTenantFilterIntoBaseWhere_MissingViewer fails closed without a
// viewer context.
func TestInjectTenantFilterIntoBaseWhere_MissingViewer(t *testing.T) {
	if _, _, err := InjectTenantFilterIntoBaseWhere[scopedEntity](context.Background(), "id = ?", nil); err == nil {
		t.Fatal("missing viewer must fail closed")
	}
}

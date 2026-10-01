package mixin_test

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	gormmixin "github.com/tx7do/go-wind-plugins/crud/gorm/mixin"
	"github.com/tx7do/go-wind-plugins/crud/viewer"
)

// ---------------------------------------------------------------------------
// DB-backed fixtures (pure-Go in-memory sqlite, mirroring the parent module's
// tenant_enforce_test.go conventions).
// ---------------------------------------------------------------------------

type tenantHookEntity struct {
	gorm.Model
	gormmixin.TenantID
	Name string
}

type versionedEntity struct {
	gorm.Model
	gormmixin.Version
	Name string
}

// treeNode exercises the generic Tree mixin (Parent/Children self relation).
type treeNode struct {
	gormmixin.AutoIncrementID
	gormmixin.Tree[treeNode]
	Name string
}

func openHookTestDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	all := append([]any{&tenantHookEntity{}, &versionedEntity{}}, models...)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(all...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

// viewerContext builds a context carrying a stub viewer with the given tenant.
func viewerContext(tid uint64) context.Context {
	return viewer.WithContext(context.Background(), &hookStubViewer{tid: tid})
}

type hookStubViewer struct{ tid uint64 }

func (s *hookStubViewer) UserID() uint64                 { return 0 }
func (s *hookStubViewer) TenantID() uint64               { return s.tid }
func (s *hookStubViewer) OrgUnitID() uint64              { return 0 }
func (s *hookStubViewer) Permissions() []string          { return nil }
func (s *hookStubViewer) Roles() []string                { return nil }
func (s *hookStubViewer) DataScope() []viewer.DataScope  { return nil }
func (s *hookStubViewer) TraceID() string                { return "" }
func (s *hookStubViewer) HasPermission(_, _ string) bool { return false }
func (s *hookStubViewer) IsPlatformContext() bool        { return s.tid == 0 }
func (s *hookStubViewer) IsTenantContext() bool          { return s.tid > 0 }
func (s *hookStubViewer) IsSystemContext() bool          { return false }
func (s *hookStubViewer) ShouldAudit() bool              { return false }

// readTenant loads a row bypassing viewer context (no callbacks are
// registered on this connection, so reads are unfiltered).
func readTenant(t *testing.T, db *gorm.DB, id uint) *uint32 {
	t.Helper()
	var out tenantHookEntity
	if err := db.First(&out, id).Error; err != nil {
		t.Fatalf("re-read row %d: %v", id, err)
	}
	return out.TenantID.TenantID
}

// ---------------------------------------------------------------------------
// TenantID.BeforeCreate (mixin-level tenant enforcement)
// ---------------------------------------------------------------------------

// TestTenantIDHook_ForcesTenantOnCreate: in a tenant business view, Create
// must overwrite an explicit cross-tenant value with the viewer's tenant.
func TestTenantIDHook_ForcesTenantOnCreate(t *testing.T) {
	db := openHookTestDB(t)
	ctx := viewerContext(7)

	other := uint32(99)
	e := &tenantHookEntity{Name: "x"}
	e.TenantID.TenantID = &other

	if err := db.WithContext(ctx).Create(e).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := readTenant(t, db, e.ID); got == nil || *got != 7 {
		t.Errorf("tenant_id must be force-set to 7, got %v", got)
	}
}

// TestTenantIDHook_PlatformContextKeepsExplicitValue: platform view respects
// the explicitly set tenant (admin choosing the target tenant).
func TestTenantIDHook_PlatformContextKeepsExplicitValue(t *testing.T) {
	db := openHookTestDB(t)
	ctx := viewerContext(0) // platform

	other := uint32(99)
	e := &tenantHookEntity{Name: "x"}
	e.TenantID.TenantID = &other

	if err := db.WithContext(ctx).Create(e).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := readTenant(t, db, e.ID); got == nil || *got != 99 {
		t.Errorf("platform context must keep explicit tenant_id=99, got %v", got)
	}
}

// TestTenantIDHook_SystemContextKeepsExplicitValue: same semantics as the
// platform view for system background jobs.
func TestTenantIDHook_SystemContextKeepsExplicitValue(t *testing.T) {
	db := openHookTestDB(t)
	ctx := viewer.WithContext(context.Background(), &systemViewer{tid: 0})

	other := uint32(99)
	e := &tenantHookEntity{Name: "x"}
	e.TenantID.TenantID = &other

	if err := db.WithContext(ctx).Create(e).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := readTenant(t, db, e.ID); got == nil || *got != 99 {
		t.Errorf("system context must keep explicit tenant_id=99, got %v", got)
	}
}

type systemViewer struct{ tid uint64 }

func (s *systemViewer) UserID() uint64                 { return 0 }
func (s *systemViewer) TenantID() uint64               { return s.tid }
func (s *systemViewer) OrgUnitID() uint64              { return 0 }
func (s *systemViewer) Permissions() []string          { return nil }
func (s *systemViewer) Roles() []string                { return nil }
func (s *systemViewer) DataScope() []viewer.DataScope  { return nil }
func (s *systemViewer) TraceID() string                { return "" }
func (s *systemViewer) HasPermission(_, _ string) bool { return false }
func (s *systemViewer) IsPlatformContext() bool        { return false }
func (s *systemViewer) IsTenantContext() bool          { return false }
func (s *systemViewer) IsSystemContext() bool          { return true }
func (s *systemViewer) ShouldAudit() bool              { return false }

// TestTenantIDHook_MissingViewerFailsClosed: creating a tenant-scoped entity
// without a ViewerContext must abort.
func TestTenantIDHook_MissingViewerFailsClosed(t *testing.T) {
	db := openHookTestDB(t)

	e := &tenantHookEntity{Name: "x"}
	if err := db.WithContext(context.Background()).Create(e).Error; err == nil {
		t.Error("missing ViewerContext must fail-closed with an error")
	}

	var count int64
	if err := db.Model(&tenantHookEntity{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("failed create must not persist a row, got %d", count)
	}
}

// TestTenantIDHook_TenantContextFillsZeroValue: without an explicit value the
// hook still stamps the viewer's tenant.
func TestTenantIDHook_TenantContextFillsZeroValue(t *testing.T) {
	db := openHookTestDB(t)
	ctx := viewerContext(3)

	e := &tenantHookEntity{Name: "x"}
	if err := db.WithContext(ctx).Create(e).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := readTenant(t, db, e.ID); got == nil || *got != 3 {
		t.Errorf("tenant_id must default to the viewer tenant 3, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// OptimisticUpdate
// ---------------------------------------------------------------------------

func TestOptimisticUpdate_SucceedsOnVersionMatch(t *testing.T) {
	db := openHookTestDB(t)

	e := &versionedEntity{Name: "before"}
	if err := db.Create(e).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	// The Version mixin hook stamps the initial version=1.
	row := &versionedEntity{}
	if err := db.First(row, e.ID).Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	if row.Version.Version != 1 {
		t.Fatalf("created row must carry version=1, got %d", row.Version.Version)
	}

	if err := gormmixin.OptimisticUpdate(db, &versionedEntity{Model: gorm.Model{ID: e.ID}}, 1, map[string]any{"name": "after"}); err != nil {
		t.Fatalf("OptimisticUpdate: %v", err)
	}

	got := &versionedEntity{}
	if err := db.First(got, e.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Name != "after" {
		t.Errorf("update must apply, got name %q", got.Name)
	}
}

func TestOptimisticUpdate_FailsOnVersionMismatch(t *testing.T) {
	db := openHookTestDB(t)

	e := &versionedEntity{Name: "before"}
	if err := db.Create(e).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	err := gormmixin.OptimisticUpdate(db, &versionedEntity{Model: gorm.Model{ID: e.ID}}, 999, map[string]any{"name": "after"})
	if err == nil {
		t.Fatal("stale version must surface a version mismatch error")
	}
	if err.Error() != "optimistic lock: version mismatch" {
		t.Errorf("error message = %q, want %q", err.Error(), "optimistic lock: version mismatch")
	}

	got := &versionedEntity{}
	if err := db.First(got, e.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Name != "before" {
		t.Errorf("failed update must not apply, got name %q", got.Name)
	}
}

// ---------------------------------------------------------------------------
// Generic Tree mixin
// ---------------------------------------------------------------------------

// TestTreeMixinMigration: the generic Tree mixin (embedded ParentID plus
// Parent/Children self relation) parses and migrates.
func TestTreeMixinMigration(t *testing.T) {
	db := openHookTestDB(t, &treeNode{})

	if !db.Migrator().HasTable("tree_nodes") && !db.Migrator().HasTable("tree_node") {
		// NamingStrategy pluralization: assert via column presence instead.
	}
	if !db.Migrator().HasColumn(&treeNode{}, "parent_id") {
		t.Error("tree node table must have the parent_id column from the Tree mixin")
	}

	// Self-referencing create: parent + child, then verify the stored FK.
	parent := &treeNode{Name: "root"}
	if err := db.Create(parent).Error; err != nil {
		t.Fatalf("create parent: %v", err)
	}
	childPID := uint32(parent.ID)
	child := &treeNode{Name: "leaf"}
	child.ParentID.ParentID = &childPID
	if err := db.Create(child).Error; err != nil {
		t.Fatalf("create child: %v", err)
	}

	got := &treeNode{}
	if err := db.First(got, child.ID).Error; err != nil {
		t.Fatalf("reload child: %v", err)
	}
	if got.ParentID.ParentID == nil || *got.ParentID.ParentID != childPID {
		t.Errorf("child parent_id = %v, want %d", got.ParentID.ParentID, childPID)
	}
}

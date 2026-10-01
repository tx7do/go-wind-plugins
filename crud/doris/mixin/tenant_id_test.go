package mixin_test

import (
	"context"
	"testing"

	"github.com/tx7do/go-wind-plugins/crud/doris/mixin"
	"github.com/tx7do/go-wind-plugins/crud/viewer"
)

// scopedDoc embeds the TenantID mixin the way repository entities do.
type scopedDoc struct {
	mixin.TenantID
	Name string
}

type plainDoc struct {
	Name string
}

func TestTenantIDDefaultsToNil(t *testing.T) {
	m := &mixin.TenantID{}
	if got := m.GetTenantID(); got != nil {
		t.Errorf("GetTenantID on a fresh mixin = %v, want nil", got)
	}
}

func TestTenantIDGetSetRoundTrip(t *testing.T) {
	m := &mixin.TenantID{}
	m.SetTenantID(7)
	got := m.GetTenantID()
	if got == nil {
		t.Fatal("GetTenantID must return the value set by SetTenantID")
	}
	if *got != 7 {
		t.Errorf("GetTenantID = %d, want 7", *got)
	}
}

func TestTenantIDOverwrite(t *testing.T) {
	m := &mixin.TenantID{}
	m.SetTenantID(7)
	m.SetTenantID(9)
	if got := m.GetTenantID(); got == nil || *got != 9 {
		t.Errorf("SetTenantID must overwrite the previous value, got %v", got)
	}
}

func TestTenantIDInstancesAreIndependent(t *testing.T) {
	a := &mixin.TenantID{}
	b := &mixin.TenantID{}
	a.SetTenantID(1)
	b.SetTenantID(2)
	if *a.GetTenantID() != 1 || *b.GetTenantID() != 2 {
		t.Errorf("instances must not share state, got a=%d b=%d", *a.GetTenantID(), *b.GetTenantID())
	}
}

func TestTenantIDImplementsScopedModel(t *testing.T) {
	var _ viewer.ScopedModel = (*mixin.TenantID)(nil)

	var m any = &mixin.TenantID{}
	if _, ok := m.(viewer.ScopedModel); !ok {
		t.Error("*TenantID must satisfy viewer.ScopedModel at runtime")
	}
}

func TestTenantIDScopedTypeDetection(t *testing.T) {
	if !viewer.IsTenantScopedType[scopedDoc]() {
		t.Error("a struct embedding TenantID must be detected as tenant scoped")
	}
	if viewer.IsTenantScopedType[plainDoc]() {
		t.Error("a struct without TenantID must not be detected as tenant scoped")
	}
}

func TestTenantIDEnforceOnInstance(t *testing.T) {
	tenantCtx := viewer.WithContext(context.Background(), &scopedStubViewer{tid: 42})

	doc := &scopedDoc{}
	if err := viewer.EnforceOnScopedInstance(tenantCtx, doc); err != nil {
		t.Fatalf("EnforceOnScopedInstance: %v", err)
	}
	if doc.TenantID.TenantID == nil || *doc.TenantID.TenantID != 42 {
		t.Errorf("tenant context must force tenant_id=42, got %v", doc.TenantID.TenantID)
	}
}

type scopedStubViewer struct{ tid uint64 }

func (s *scopedStubViewer) UserID() uint64                 { return 0 }
func (s *scopedStubViewer) TenantID() uint64               { return s.tid }
func (s *scopedStubViewer) OrgUnitID() uint64              { return 0 }
func (s *scopedStubViewer) Permissions() []string          { return nil }
func (s *scopedStubViewer) Roles() []string                { return nil }
func (s *scopedStubViewer) DataScope() []viewer.DataScope  { return nil }
func (s *scopedStubViewer) TraceID() string                { return "" }
func (s *scopedStubViewer) HasPermission(_, _ string) bool { return false }
func (s *scopedStubViewer) IsPlatformContext() bool        { return s.tid == 0 }
func (s *scopedStubViewer) IsTenantContext() bool          { return s.tid > 0 }
func (s *scopedStubViewer) IsSystemContext() bool          { return false }
func (s *scopedStubViewer) ShouldAudit() bool              { return false }

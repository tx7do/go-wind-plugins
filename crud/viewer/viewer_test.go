package viewer_test

import (
	"context"
	"errors"
	"testing"

	"github.com/tx7do/go-wind-plugins/crud/viewer"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// stubViewer is a configurable viewer.Context for tests.
type stubViewer struct {
	uid         uint64
	tid         uint64
	orgUnit     uint64
	perms       []string
	roles       []string
	scopes      []viewer.DataScope
	traceID     string
	system      bool
	shouldAudit bool
}

func (s *stubViewer) UserID() uint64                { return s.uid }
func (s *stubViewer) TenantID() uint64              { return s.tid }
func (s *stubViewer) OrgUnitID() uint64             { return s.orgUnit }
func (s *stubViewer) Permissions() []string         { return s.perms }
func (s *stubViewer) Roles() []string               { return s.roles }
func (s *stubViewer) DataScope() []viewer.DataScope { return s.scopes }
func (s *stubViewer) TraceID() string               { return s.traceID }

func (s *stubViewer) HasPermission(action, resource string) bool {
	want := action + ":" + resource
	for _, p := range s.perms {
		if p == want {
			return true
		}
	}
	return false
}

func (s *stubViewer) IsPlatformContext() bool { return s.tid == 0 }
func (s *stubViewer) IsTenantContext() bool   { return s.tid > 0 }
func (s *stubViewer) IsSystemContext() bool   { return s.system }
func (s *stubViewer) ShouldAudit() bool       { return s.shouldAudit }

// scopedEntity mimics an entity embedding a tenant-scoped mixin.
type scopedEntity struct {
	tenantID *uint32
}

func (e *scopedEntity) GetTenantID() *uint32 { return e.tenantID }
func (e *scopedEntity) SetTenantID(v uint32) {
	x := v
	e.tenantID = &x
}

// plainEntity does not implement viewer.ScopedModel.
type plainEntity struct {
	Name string
}

// ---------------------------------------------------------------------------
// ScopeType constants
// ---------------------------------------------------------------------------

func TestScopeTypeConstants(t *testing.T) {
	cases := []struct {
		name string
		st   viewer.ScopeType
		want string
	}{
		{"ScopeTypeSelf", viewer.ScopeTypeSelf, "SELF"},
		{"ScopeTypeUnit", viewer.ScopeTypeUnit, "UNIT"},
		{"ScopeTypeUser", viewer.ScopeTypeUser, "USER"},
		{"ScopeTypeAll", viewer.ScopeTypeAll, "ALL"},
		{"ScopeTypeNone", viewer.ScopeTypeNone, "NONE"},
	}
	for _, tc := range cases {
		if string(tc.st) != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, string(tc.st), tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Noop context
// ---------------------------------------------------------------------------

func TestNoopContextZeroValues(t *testing.T) {
	nc := viewer.NewNoopContext()
	if nc == nil {
		t.Fatal("NewNoopContext must not return nil")
	}
	if nc.UserID() != 0 || nc.TenantID() != 0 || nc.OrgUnitID() != 0 {
		t.Error("noop context must return zero IDs")
	}
	if nc.Permissions() != nil || nc.Roles() != nil || nc.DataScope() != nil {
		t.Error("noop context must return nil permission/role/scope slices")
	}
	if nc.TraceID() != "" {
		t.Error("noop context must return empty trace id")
	}
	if nc.HasPermission("update", "user") {
		t.Error("noop context must deny all permissions")
	}
	if nc.IsPlatformContext() || nc.IsTenantContext() || nc.IsSystemContext() {
		t.Error("noop context must not claim platform/tenant/system views")
	}
	if nc.ShouldAudit() {
		t.Error("noop context must not request auditing")
	}
}

// ---------------------------------------------------------------------------
// Context propagation
// ---------------------------------------------------------------------------

func TestWithContextRoundTrip(t *testing.T) {
	vc := &stubViewer{uid: 1, tid: 7, traceID: "trace-x"}
	ctx := viewer.WithContext(context.Background(), vc)

	got, ok := viewer.FromContext(ctx)
	if !ok {
		t.Fatal("FromContext must find the viewer injected by WithContext")
	}
	if got != vc {
		t.Errorf("FromContext returned %T, want the injected *stubViewer", got)
	}
	if got.TenantID() != 7 || got.TraceID() != "trace-x" {
		t.Errorf("round-trip viewer mismatch: tid=%d trace=%q", got.TenantID(), got.TraceID())
	}
}

func TestFromContextMissing(t *testing.T) {
	got, ok := viewer.FromContext(context.Background())
	if ok {
		t.Error("FromContext on empty context must return ok=false")
	}
	if got != nil {
		t.Errorf("FromContext on empty context must return nil, got %T", got)
	}
}

func TestMustFromContextDefaults(t *testing.T) {
	nc := viewer.MustFromContext(context.Background())
	if nc == nil {
		t.Fatal("MustFromContext must return a default context, got nil")
	}
	// The fallback is the anonymous context: everything is denied/zero.
	if nc.TenantID() != 0 || nc.HasPermission("read", "user") {
		t.Error("fallback context must behave like the noop context")
	}
}

func TestMustFromContextNilContext(t *testing.T) {
	if nc := viewer.MustFromContext(nil); nc == nil {
		t.Fatal("MustFromContext(nil) must return a default context, got nil")
	}
}

func TestMustFromContextReturnsInjected(t *testing.T) {
	vc := &stubViewer{tid: 9}
	ctx := viewer.WithContext(context.Background(), vc)
	if got := viewer.MustFromContext(ctx); got != vc {
		t.Errorf("MustFromContext must return the injected viewer, got %T", got)
	}
}

// ---------------------------------------------------------------------------
// HasPermission
// ---------------------------------------------------------------------------

func TestHasPermission(t *testing.T) {
	vc := &stubViewer{perms: []string{"update:user", "read:order"}}
	if !vc.HasPermission("update", "user") {
		t.Error("HasPermission must match an injected action:resource permission")
	}
	if !vc.HasPermission("read", "order") {
		t.Error("HasPermission must match an injected action:resource permission")
	}
	if vc.HasPermission("delete", "user") {
		t.Error("HasPermission must not match an absent permission")
	}
}

// ---------------------------------------------------------------------------
// EnforceTenant decision
// ---------------------------------------------------------------------------

func TestEnforceTenant_MissingViewerFailClosed(t *testing.T) {
	dec, err := viewer.EnforceTenant(context.Background())
	if !errors.Is(err, viewer.ErrMissingViewer) {
		t.Errorf("EnforceTenant without viewer must return ErrMissingViewer, got %v", err)
	}
	if dec.Enforce {
		t.Error("EnforceTenant without viewer must not report Enforce")
	}
}

func TestEnforceTenant_PlatformContextPassThrough(t *testing.T) {
	ctx := viewer.WithContext(context.Background(), &stubViewer{tid: 0})
	dec, err := viewer.EnforceTenant(ctx)
	if err != nil {
		t.Fatalf("platform context must pass through, got %v", err)
	}
	if dec.Enforce {
		t.Errorf("platform context must not enforce, got %+v", dec)
	}
}

func TestEnforceTenant_SystemContextPassThrough(t *testing.T) {
	ctx := viewer.WithContext(context.Background(), &stubViewer{tid: 0, system: true})
	dec, err := viewer.EnforceTenant(ctx)
	if err != nil {
		t.Fatalf("system context must pass through, got %v", err)
	}
	if dec.Enforce {
		t.Errorf("system context must not enforce, got %+v", dec)
	}
}

func TestEnforceTenant_TenantContextEnforces(t *testing.T) {
	ctx := viewer.WithContext(context.Background(), &stubViewer{tid: 42})
	dec, err := viewer.EnforceTenant(ctx)
	if err != nil {
		t.Fatalf("tenant context must not error, got %v", err)
	}
	if !dec.Enforce {
		t.Error("tenant context must enforce tenant isolation")
	}
	if dec.TenantID != 42 {
		t.Errorf("EnforceTenant.TenantID = %d, want 42", dec.TenantID)
	}
}

func TestErrMissingViewerMessage(t *testing.T) {
	if viewer.ErrMissingViewer == nil {
		t.Fatal("ErrMissingViewer must be defined")
	}
	if viewer.ErrMissingViewer.Error() == "" {
		t.Error("ErrMissingViewer must have a non-empty message")
	}
}

// ---------------------------------------------------------------------------
// ScopedModel type detection
// ---------------------------------------------------------------------------

func TestIsTenantScopedType(t *testing.T) {
	if !viewer.IsTenantScopedType[scopedEntity]() {
		t.Error("scopedEntity implements ScopedModel and must be detected as tenant scoped")
	}
	if viewer.IsTenantScopedType[plainEntity]() {
		t.Error("plainEntity does not implement ScopedModel and must not be detected as tenant scoped")
	}
	if viewer.IsTenantScopedType[*scopedEntity]() {
		t.Error("*scopedEntity does not implement ScopedModel (pointer receiver) and must not be detected as scoped")
	}
}

// ---------------------------------------------------------------------------
// EnforceOnScopedInstance
// ---------------------------------------------------------------------------

func TestEnforceOnScopedInstance_NilInstance(t *testing.T) {
	ctx := viewer.WithContext(context.Background(), &stubViewer{tid: 7})
	if err := viewer.EnforceOnScopedInstance(ctx, (*scopedEntity)(nil)); err != nil {
		t.Errorf("EnforceOnScopedInstance(nil) must be a no-op, got %v", err)
	}
}

func TestEnforceOnScopedInstance_NonScoped(t *testing.T) {
	ctx := viewer.WithContext(context.Background(), &stubViewer{tid: 7})
	if err := viewer.EnforceOnScopedInstance(ctx, &plainEntity{Name: "x"}); err != nil {
		t.Errorf("non-scoped instance must be a no-op, got %v", err)
	}
}

func TestEnforceOnScopedInstance_TenantContextForcesSet(t *testing.T) {
	ctx := viewer.WithContext(context.Background(), &stubViewer{tid: 7})
	e := &scopedEntity{}
	if err := viewer.EnforceOnScopedInstance(ctx, e); err != nil {
		t.Fatalf("EnforceOnScopedInstance: %v", err)
	}
	if e.tenantID == nil || *e.tenantID != 7 {
		t.Errorf("tenant context must force tenant_id=7, got %v", e.tenantID)
	}
}

func TestEnforceOnScopedInstance_PlatformContextKeepsValue(t *testing.T) {
	ctx := viewer.WithContext(context.Background(), &stubViewer{tid: 0})
	existing := uint32(99)
	e := &scopedEntity{tenantID: &existing}
	if err := viewer.EnforceOnScopedInstance(ctx, e); err != nil {
		t.Fatalf("EnforceOnScopedInstance: %v", err)
	}
	if *e.tenantID != 99 {
		t.Errorf("platform context must not overwrite tenant_id, got %d", *e.tenantID)
	}
}

func TestEnforceOnScopedInstance_MissingViewerFailClosed(t *testing.T) {
	e := &scopedEntity{}
	if err := viewer.EnforceOnScopedInstance(context.Background(), e); !errors.Is(err, viewer.ErrMissingViewer) {
		t.Errorf("missing viewer must fail-closed with ErrMissingViewer, got %v", err)
	}
	if e.tenantID != nil {
		t.Errorf("failed enforcement must not set tenant_id, got %v", *e.tenantID)
	}
}

// ---------------------------------------------------------------------------
// EnforceOnScopedInstanceAny
// ---------------------------------------------------------------------------

func TestEnforceOnScopedInstanceAny(t *testing.T) {
	t.Run("nil instance", func(t *testing.T) {
		ctx := viewer.WithContext(context.Background(), &stubViewer{tid: 7})
		if err := viewer.EnforceOnScopedInstanceAny(ctx, nil); err != nil {
			t.Errorf("nil instance must be a no-op, got %v", err)
		}
	})

	t.Run("non scoped instance", func(t *testing.T) {
		ctx := viewer.WithContext(context.Background(), &stubViewer{tid: 7})
		if err := viewer.EnforceOnScopedInstanceAny(ctx, plainEntity{Name: "x"}); err != nil {
			t.Errorf("non-scoped instance must be a no-op, got %v", err)
		}
		if err := viewer.EnforceOnScopedInstanceAny(ctx, &plainEntity{Name: "x"}); err != nil {
			t.Errorf("non-scoped pointer must be a no-op, got %v", err)
		}
	})

	t.Run("tenant context forces set", func(t *testing.T) {
		ctx := viewer.WithContext(context.Background(), &stubViewer{tid: 5})
		e := &scopedEntity{}
		if err := viewer.EnforceOnScopedInstanceAny(ctx, e); err != nil {
			t.Fatalf("EnforceOnScopedInstanceAny: %v", err)
		}
		if e.tenantID == nil || *e.tenantID != 5 {
			t.Errorf("tenant context must force tenant_id=5, got %v", e.tenantID)
		}
	})

	t.Run("value type not scoped", func(t *testing.T) {
		// scopedEntity methods use pointer receivers, so the value type does
		// not satisfy ScopedModel and the helper must skip it silently.
		ctx := viewer.WithContext(context.Background(), &stubViewer{tid: 5})
		e := scopedEntity{}
		if err := viewer.EnforceOnScopedInstanceAny(ctx, e); err != nil {
			t.Errorf("value instance must be a no-op, got %v", err)
		}
		if e.tenantID != nil {
			t.Error("value instance must not be mutated")
		}
	})

	t.Run("missing viewer fail closed", func(t *testing.T) {
		e := &scopedEntity{}
		if err := viewer.EnforceOnScopedInstanceAny(context.Background(), e); !errors.Is(err, viewer.ErrMissingViewer) {
			t.Errorf("missing viewer must fail-closed, got %v", err)
		}
	})
}

package engine

import (
	"context"
	"sync"
	"testing"

	errs "github.com/tx7do/go-wind-plugins/errors"
)

// ---------------------------------------------------------------------------
// Make* helper constructors
// ---------------------------------------------------------------------------

func TestMakeSubjects(t *testing.T) {
	got := MakeSubjects("alice", "bob")
	if len(got) != 2 {
		t.Fatalf("MakeSubjects length = %d, want 2", len(got))
	}
	if got[0] != Subject("alice") || got[1] != Subject("bob") {
		t.Errorf("MakeSubjects = %v, want [alice bob]", got)
	}

	// Variadic call with no arguments yields an empty (non-nil) slice.
	if empty := MakeSubjects(); len(empty) != 0 {
		t.Errorf("MakeSubjects() with no args should be empty, got %v", empty)
	}
}

func TestMakeProjects(t *testing.T) {
	got := MakeProjects("p1", "p2")
	if len(got) != 2 {
		t.Fatalf("MakeProjects length = %d, want 2", len(got))
	}
	if got[0] != Project("p1") || got[1] != Project("p2") {
		t.Errorf("MakeProjects = %v, want [p1 p2]", got)
	}
}

func TestMakeActions(t *testing.T) {
	got := MakeActions("read", "write")
	if len(got) != 2 {
		t.Fatalf("MakeActions length = %d, want 2", len(got))
	}
	if got[0] != Action("read") || got[1] != Action("write") {
		t.Errorf("MakeActions = %v, want [read write]", got)
	}
}

func TestMakeResources(t *testing.T) {
	got := MakeResources("res1", "res2")
	if len(got) != 2 {
		t.Fatalf("MakeResources length = %d, want 2", len(got))
	}
	if got[0] != Resource("res1") || got[1] != Resource("res2") {
		t.Errorf("MakeResources = %v, want [res1 res2]", got)
	}
}

func TestMakePair(t *testing.T) {
	got := MakePair("res1", "read")
	if got.Resource != Resource("res1") {
		t.Errorf("MakePair().Resource = %v, want %v", got.Resource, Resource("res1"))
	}
	if got.Action != Action("read") {
		t.Errorf("MakePair().Action = %v, want %v", got.Action, Action("read"))
	}
}

func TestMakePairs(t *testing.T) {
	p1 := MakePair("res1", "read")
	p2 := MakePair("res2", "write")
	got := MakePairs(p1, p2)
	if len(got) != 2 {
		t.Fatalf("MakePairs length = %d, want 2", len(got))
	}
	if got[0] != p1 || got[1] != p2 {
		t.Errorf("MakePairs = %v, want [%v %v]", got, p1, p2)
	}
}

// ---------------------------------------------------------------------------
// Engine Type constants
// ---------------------------------------------------------------------------

func TestTypeConstants(t *testing.T) {
	tests := []struct {
		name string
		typ  Type
		want string
	}{
		{"Noop", Noop, "noop"},
		{"Acl", Acl, "acl"},
		{"Rbac", Rbac, "rbac"},
		{"Casbin", Casbin, "casbin"},
		{"Opa", Opa, "opa"},
		{"Zanzibar", Zanzibar, "zanzibar"},
		{"Cedar", Cedar, "cedar"},
		{"Cerbos", Cerbos, "cerbos"},
		{"AwsIam", AwsIam, "awsiam"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.typ) != tt.want {
				t.Errorf("Type constant %s = %q, want %q", tt.name, tt.typ, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Context claims injection / extraction
// ---------------------------------------------------------------------------

func TestContextWithAuthClaims_Roundtrip(t *testing.T) {
	claims := &AuthClaims{
		Subjects: &[]string{"alice"},
	}
	ctx := ContextWithAuthClaims(context.Background(), claims)

	got, ok := AuthClaimsFromContext(ctx)
	if !ok {
		t.Fatal("AuthClaimsFromContext should find claims injected by ContextWithAuthClaims")
	}
	if got != claims {
		t.Error("AuthClaimsFromContext should return the exact claims instance")
	}
	if got.Subjects == nil || len(*got.Subjects) != 1 || (*got.Subjects)[0] != "alice" {
		t.Errorf("claims.Subjects = %v, want [alice]", got.Subjects)
	}
}

func TestAuthClaimsFromContext_Missing(t *testing.T) {
	// A plain background context carries no claims.
	if claims, ok := AuthClaimsFromContext(context.Background()); ok || claims != nil {
		t.Error("AuthClaimsFromContext should return (nil, false) for a context without claims")
	}

	// A context carrying a value of a different type must not be mistaken
	// for auth claims.
	if claims, ok := AuthClaimsFromContext(context.WithValue(context.Background(), authClaimsContextKey, "not-claims")); ok || claims != nil {
		t.Error("AuthClaimsFromContext should return (nil, false) for a wrong value type")
	}
}

func TestContextWithAuthClaims_NilClaims(t *testing.T) {
	// Injecting nil stores a typed nil pointer; extraction reports ok=true
	// with a nil claims pointer. This documents the current behaviour.
	ctx := ContextWithAuthClaims(context.Background(), nil)
	got, ok := AuthClaimsFromContext(ctx)
	if !ok {
		t.Error("AuthClaimsFromContext should report ok=true for a stored nil *AuthClaims")
	}
	if got != nil {
		t.Errorf("AuthClaimsFromContext should return nil claims, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// Sentinel errors
// ---------------------------------------------------------------------------

func TestSentinelErrors(t *testing.T) {
	if ErrMissingAuthClaims == nil {
		t.Fatal("ErrMissingAuthClaims should not be nil")
	}
	if ErrInvalidClaims == nil {
		t.Fatal("ErrInvalidClaims should not be nil")
	}

	if code := errs.Code(ErrMissingAuthClaims); code != errs.StatusForbidden {
		t.Errorf("ErrMissingAuthClaims code = %d, want %d", code, errs.StatusForbidden)
	}
	if code := errs.Code(ErrInvalidClaims); code != errs.StatusForbidden {
		t.Errorf("ErrInvalidClaims code = %d, want %d", code, errs.StatusForbidden)
	}

	if ErrMissingAuthClaims.Reason != "AUTHZ_MISSING_CLAIMS" {
		t.Errorf("ErrMissingAuthClaims.Reason = %q, want %q", ErrMissingAuthClaims.Reason, "AUTHZ_MISSING_CLAIMS")
	}
	if ErrInvalidClaims.Reason != "AUTHZ_INVALID_CLAIMS" {
		t.Errorf("ErrInvalidClaims.Reason = %q, want %q", ErrInvalidClaims.Reason, "AUTHZ_INVALID_CLAIMS")
	}
}

// ---------------------------------------------------------------------------
// Engine factory registry
// ---------------------------------------------------------------------------

// mockEngine is a minimal Engine implementation used to exercise the factory.
type mockEngine struct {
	name string
}

func (m *mockEngine) Name() string { return m.name }

func (m *mockEngine) ProjectsAuthorized(_ context.Context, _ Subjects, _ Action, _ Resource, projects Projects) (Projects, error) {
	return projects, nil
}

func (m *mockEngine) FilterAuthorizedPairs(_ context.Context, _ Subjects, pairs Pairs) (Pairs, error) {
	return pairs, nil
}

func (m *mockEngine) FilterAuthorizedProjects(_ context.Context, _ Subjects) (Projects, error) {
	return nil, nil
}

func (m *mockEngine) IsAuthorized(_ context.Context, _ Subject, _ Action, _ Resource, _ Project) (bool, error) {
	return true, nil
}

func (m *mockEngine) SetPolicies(_ context.Context, _ PolicyMap, _ RoleMap) error {
	return nil
}

// uniqueType returns a factory Type that no other test in this package uses,
// so the global registry state cannot leak between tests.
func uniqueType(name string) Type {
	return Type("test-factory-" + name)
}

func TestFactory_RegisterGetUnregister(t *testing.T) {
	typ := uniqueType("register")
	factory := func(_ context.Context, _ ...any) (Engine, error) {
		return &mockEngine{name: "registered"}, nil
	}

	if got, ok := GetFactory(typ); ok || got != nil {
		t.Fatal("GetFactory should not find an unregistered factory")
	}

	if err := Register(typ, factory); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	got, ok := GetFactory(typ)
	if !ok || got == nil {
		t.Fatal("GetFactory should find the registered factory")
	}

	engine, err := got(context.Background())
	if err != nil {
		t.Fatalf("factory invocation failed: %v", err)
	}
	if engine.Name() != "registered" {
		t.Errorf("engine.Name() = %q, want %q", engine.Name(), "registered")
	}

	if !Unregister(typ) {
		t.Error("Unregister should return true for a registered factory")
	}
	if Unregister(typ) {
		t.Error("Unregister should return false for an already removed factory")
	}
	if _, ok := GetFactory(typ); ok {
		t.Error("GetFactory should not find the factory after Unregister")
	}
}

func TestFactory_RegisterDuplicate(t *testing.T) {
	typ := uniqueType("duplicate")
	factory := func(_ context.Context, _ ...any) (Engine, error) {
		return &mockEngine{name: "dup"}, nil
	}

	if err := Register(typ, factory); err != nil {
		t.Fatalf("first Register failed: %v", err)
	}
	defer Unregister(typ)

	err := Register(typ, factory)
	if err == nil {
		t.Fatal("Registering the same type twice should fail")
	}

	// The original registration must remain untouched.
	engine, err := NewEngine(context.Background(), typ)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	if engine.Name() != "dup" {
		t.Errorf("engine.Name() = %q, want %q", engine.Name(), "dup")
	}
}

func TestNewEngine_UnknownType(t *testing.T) {
	_, err := NewEngine(context.Background(), uniqueType("unknown"))
	if err == nil {
		t.Fatal("NewEngine should fail for an unregistered engine type")
	}
}

func TestNewEngine_ForwardsOptions(t *testing.T) {
	typ := uniqueType("options")
	factory := func(_ context.Context, options ...any) (Engine, error) {
		if len(options) != 2 {
			return nil, nil
		}
		name, _ := options[0].(string)
		return &mockEngine{name: name}, nil
	}
	if err := Register(typ, factory); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	defer Unregister(typ)

	engine, err := NewEngine(context.Background(), typ, "forwarded", 42)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	if engine == nil || engine.Name() != "forwarded" {
		t.Errorf("NewEngine did not forward options, engine = %v", engine)
	}
}

func TestListFactories(t *testing.T) {
	types := []Type{uniqueType("list-a"), uniqueType("list-b")}
	for _, typ := range types {
		typ := typ
		if err := Register(typ, func(_ context.Context, _ ...any) (Engine, error) {
			return &mockEngine{name: string(typ)}, nil
		}); err != nil {
			t.Fatalf("Register(%s) failed: %v", typ, err)
		}
		defer Unregister(typ)
	}

	listed := make(map[Type]bool, len(ListFactories()))
	for _, typ := range ListFactories() {
		listed[typ] = true
	}
	for _, typ := range types {
		if !listed[typ] {
			t.Errorf("ListFactories() is missing registered type %q", typ)
		}
	}
}

// TestFactory_ConcurrentAccess exercises the registry under concurrent use;
// the run is primarily meant to be executed with -race.
func TestFactory_ConcurrentAccess(t *testing.T) {
	typ := uniqueType("concurrent")
	if err := Register(typ, func(_ context.Context, _ ...any) (Engine, error) {
		return &mockEngine{name: "concurrent"}, nil
	}); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	defer Unregister(typ)

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = GetFactory(typ)
			_ = ListFactories()
			_, _ = NewEngine(context.Background(), typ)
		}()
	}
	wg.Wait()
}

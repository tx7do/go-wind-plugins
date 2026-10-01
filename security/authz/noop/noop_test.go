package noop

import (
	"context"
	"testing"

	engine "github.com/tx7do/go-wind-plugins/security/authz"
)

// ---------------------------------------------------------------------------
// Construction
// ---------------------------------------------------------------------------

func TestNewEngine(t *testing.T) {
	s, err := NewEngine(context.Background())
	if err != nil {
		t.Fatalf("NewEngine returned error: %v", err)
	}
	if s == nil {
		t.Fatal("NewEngine returned nil state")
	}
}

func TestName(t *testing.T) {
	s, err := NewEngine(context.Background())
	if err != nil {
		t.Fatalf("NewEngine returned error: %v", err)
	}
	if got := s.Name(); got != "noop" {
		t.Errorf("Name() = %q, want %q", got, "noop")
	}
}

// TestInterfaceCompliance ensures the noop engine satisfies the authz Engine
// interface both by pointer and by value.
func TestInterfaceCompliance(t *testing.T) {
	var _ engine.Engine = (*State)(nil)
	var _ engine.Engine = State{}
}

// ---------------------------------------------------------------------------
// Authorization behaviour
// ---------------------------------------------------------------------------

func TestIsAuthorized(t *testing.T) {
	tests := []struct {
		name     string
		subject  engine.Subject
		action   engine.Action
		resource engine.Resource
		project  engine.Project
	}{
		{"regular values", "alice", "read", "res1", "p1"},
		{"empty values", "", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := State{}
			allowed, err := s.IsAuthorized(
				context.Background(),
				tt.subject, tt.action, tt.resource, tt.project,
			)
			if err != nil {
				t.Fatalf("IsAuthorized returned error: %v", err)
			}
			if !allowed {
				t.Error("noop engine should always authorize")
			}
		})
	}
}

func TestProjectsAuthorized(t *testing.T) {
	s := State{}
	got, err := s.ProjectsAuthorized(
		context.Background(),
		engine.MakeSubjects("alice"),
		"read",
		"res1",
		engine.MakeProjects("p1", "p2"),
	)
	if err != nil {
		t.Fatalf("ProjectsAuthorized returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ProjectsAuthorized = %v, want empty", got)
	}
}

func TestFilterAuthorizedPairs(t *testing.T) {
	s := State{}
	got, err := s.FilterAuthorizedPairs(
		context.Background(),
		engine.MakeSubjects("alice"),
		engine.MakePairs(engine.MakePair("res1", "read"), engine.MakePair("res2", "write")),
	)
	if err != nil {
		t.Fatalf("FilterAuthorizedPairs returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("FilterAuthorizedPairs = %v, want empty", got)
	}
}

func TestFilterAuthorizedProjects(t *testing.T) {
	s := State{}
	got, err := s.FilterAuthorizedProjects(
		context.Background(),
		engine.MakeSubjects("alice"),
	)
	if err != nil {
		t.Fatalf("FilterAuthorizedProjects returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("FilterAuthorizedProjects = %v, want empty", got)
	}
}

func TestSetPolicies(t *testing.T) {
	s := State{}
	policies := engine.PolicyMap{"policies": []string{"p1"}}
	roles := engine.RoleMap{"roles": []string{"r1"}}
	if err := s.SetPolicies(context.Background(), policies, roles); err != nil {
		t.Errorf("SetPolicies returned error: %v", err)
	}
	// Nil arguments must be tolerated as well.
	if err := s.SetPolicies(context.Background(), nil, nil); err != nil {
		t.Errorf("SetPolicies(nil, nil) returned error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Factory registration
// ---------------------------------------------------------------------------

func TestFactoryRegistration(t *testing.T) {
	factory, ok := engine.GetFactory(engine.Noop)
	if !ok {
		t.Fatal("engine.Noop factory should be registered via init()")
	}

	created, err := factory(context.Background())
	if err != nil {
		t.Fatalf("factory invocation failed: %v", err)
	}
	if created == nil {
		t.Fatal("factory returned nil engine")
	}
	if got := created.Name(); got != "noop" {
		t.Errorf("factory engine Name() = %q, want %q", got, "noop")
	}
}

func TestNewEngineViaRegistry(t *testing.T) {
	created, err := engine.NewEngine(context.Background(), engine.Noop)
	if err != nil {
		t.Fatalf("engine.NewEngine(noop) failed: %v", err)
	}
	if created == nil {
		t.Fatal("engine.NewEngine(noop) returned nil")
	}
	if got := created.Name(); got != "noop" {
		t.Errorf("engine.Name() = %q, want %q", got, "noop")
	}
}

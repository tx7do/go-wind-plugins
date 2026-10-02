package casbin

import (
	"testing"

	"github.com/casbin/casbin/v2/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	engine "github.com/tx7do/go-wind-plugins/security/authz"
	"github.com/tx7do/go-wind-plugins/security/authz/casbin/assets"
)

var extraTestPolicies = map[string]interface{}{
	"policies": []PolicyRule{
		{PType: "p", V0: "alice", V1: "/api/*", V2: "(GET)|(POST)", V3: "project1"},
		{PType: "p", V0: "alice", V1: "/api/*", V2: "(GET)|(POST)", V3: "*"},
		{PType: "g", V0: "admin", V1: "admin_role", V2: "*"},
		{PType: "p", V0: "admin_role", V1: "/api/*", V2: "(GET)|(POST)", V3: "*"},
	},
	"projects": engine.Projects{"project1", "project2"},
}

// ---------------------------------------------------------------------------
// IsAuthorized (not covered by the earlier tests)
// ---------------------------------------------------------------------------

func TestIsAuthorized(t *testing.T) {
	s, err := NewEngine(t.Context(), WithPolices(extraTestPolicies))
	require.NoError(t, err)

	tests := []struct {
		name     string
		subject  engine.Subject
		action   engine.Action
		resource engine.Resource
		project  engine.Project
		allowed  bool
	}{
		{
			name: "allowed in a named project", subject: "alice", action: "GET",
			resource: "/api/users", project: "project1", allowed: true,
		},
		{
			name: "denied action", subject: "alice", action: "DELETE",
			resource: "/api/users", project: "project1", allowed: false,
		},
		{
			name: "unknown subject", subject: "mallory", action: "GET",
			resource: "/api/users", project: "project1", allowed: false,
		},
		{
			name: "empty project falls back to the wildcard", subject: "alice", action: "GET",
			resource: "/api/users", project: "", allowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, err := s.IsAuthorized(t.Context(), tt.subject, tt.action, tt.resource, tt.project)
			require.NoError(t, err)
			assert.Equal(t, tt.allowed, allowed)
		})
	}
}

func TestName(t *testing.T) {
	s, err := NewEngine(t.Context())
	require.NoError(t, err)
	assert.Equal(t, string(engine.Casbin), s.Name())
}

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

func TestWithDefaultModel(t *testing.T) {
	for _, name := range []string{"rbac", "rbac_with_domains", "abac", "acl", "restfull", "restfull_with_role"} {
		t.Run(name, func(t *testing.T) {
			s, err := NewEngine(t.Context(), WithDefaultModel(name))
			require.NoError(t, err)
			assert.NotNil(t, s.model)
		})
	}

	t.Run("unknown name leaves a nil model, init falls back to default", func(t *testing.T) {
		s, err := NewEngine(t.Context(), WithDefaultModel("unknown-name"))
		require.NoError(t, err)
		assert.NotNil(t, s.model)
	})
}

func TestWithStringModel(t *testing.T) {
	t.Run("valid model", func(t *testing.T) {
		s, err := NewEngine(t.Context(), WithStringModel(assets.DefaultRestfullWithRoleModel))
		require.NoError(t, err)
		assert.NotNil(t, s.model)
	})

	t.Run("invalid model falls back to the default", func(t *testing.T) {
		// WithStringModel swallows the parse error, leaving the model nil;
		// init then rebuilds the compiled-in default model.
		s, err := NewEngine(t.Context(), WithStringModel("not a casbin model"))
		require.NoError(t, err)
		assert.NotNil(t, s.model)
	})
}

func TestWithWildcardItem(t *testing.T) {
	s, err := NewEngine(t.Context(),
		WithPolices(extraTestPolicies),
		WithWildcardItem("*"),
	)
	require.NoError(t, err)
	assert.Equal(t, "*", s.wildcardItem)
}

func TestWithProjects(t *testing.T) {
	projects := engine.Projects{"proj-x"}
	s, err := NewEngine(t.Context(),
		WithPolices(extraTestPolicies),
		WithProjects(projects),
	)
	require.NoError(t, err)
	assert.Equal(t, projects, s.projects)

	// FilterAuthorizedProjects iterates s.projects, so the option feeds it.
	// Both alice (direct policy, dom '*') and admin (via the g role link)
	// match any requested project because their grants use the wildcard
	// domain; an unknown subject matches nothing.
	r, err := s.FilterAuthorizedProjects(t.Context(), engine.Subjects{"alice"})
	require.NoError(t, err)
	assert.Equal(t, engine.Projects{"proj-x"}, r)

	r, err = s.FilterAuthorizedProjects(t.Context(), engine.Subjects{"admin"})
	require.NoError(t, err)
	assert.Equal(t, engine.Projects{"proj-x"}, r)

	r, err = s.FilterAuthorizedProjects(t.Context(), engine.Subjects{"mallory"})
	require.NoError(t, err)
	assert.Empty(t, r)
}

func TestWithAuthorizedProjectsMatcher(t *testing.T) {
	s, err := NewEngine(t.Context(),
		WithPolices(extraTestPolicies),
		WithProjects(engine.Projects{"project1", "project2"}),
		WithAuthorizedProjectsMatcher(DefaultAuthorizedProjectsMatcher),
	)
	require.NoError(t, err)
	assert.Equal(t, DefaultAuthorizedProjectsMatcher, s.authorizedProjectsMatcher)

	r, err := s.FilterAuthorizedProjects(t.Context(), engine.Subjects{"alice"})
	require.NoError(t, err)
	// alice's wildcard-domain policy grants both candidate projects.
	assert.Equal(t, engine.Projects{"project1", "project2"}, r)
}

func TestWithPolicyAdapter(t *testing.T) {
	adapter := newAdapter()
	adapter.SetPolicies(extraTestPolicies)

	s, err := NewEngine(t.Context(), WithPolicyAdapter(adapter))
	require.NoError(t, err)
	assert.Same(t, adapter, s.policy)

	// Policies already present on the adapter are loaded by the enforcer.
	allowed, err := s.IsAuthorized(t.Context(), "alice", "GET", "/api/users", "project1")
	require.NoError(t, err)
	assert.True(t, allowed)
}

// ---------------------------------------------------------------------------
// Adapter behavior
// ---------------------------------------------------------------------------

func TestAdapter_UnimplementedOperations(t *testing.T) {
	a := newAdapter()
	assert.Error(t, a.SavePolicy(nil))
	assert.Error(t, a.AddPolicy("", "", nil))
	assert.Error(t, a.RemovePolicy("", "", nil))
	assert.Error(t, a.RemoveFilteredPolicy("", "", 0))
}

func TestAdapter_LoadPolicy_EmptyAndMissing(t *testing.T) {
	m, err := model.NewModelFromString(assets.DefaultRestfullWithRoleModel)
	require.NoError(t, err)

	// No "policies" key: LoadPolicy is a no-op.
	a := newAdapter()
	assert.NoError(t, a.LoadPolicy(m))

	// Nil map: same.
	a.SetPolicies(nil)
	assert.NoError(t, a.LoadPolicy(m))
}

func TestPolicyRule_LoadPolicyLine(t *testing.T) {
	m, err := model.NewModelFromString(assets.DefaultRestfullWithRoleModel)
	require.NoError(t, err)

	a := newAdapter()
	a.SetPolicies(map[string]interface{}{
		"policies": []PolicyRule{
			{PType: "p", V0: "alice", V1: "/api/*", V2: "GET", V3: "*"},
		},
	})
	require.NoError(t, a.LoadPolicy(m))

	allowed, err := m.GetPolicy("p", "p") // nolint — verify the line landed
	require.NoError(t, err)
	assert.NotEmpty(t, allowed)
}

// ---------------------------------------------------------------------------
// Factory registration
// ---------------------------------------------------------------------------

func TestFactoryRegistration(t *testing.T) {
	factory, ok := engine.GetFactory(engine.Casbin)
	require.True(t, ok, "package init should register the casbin factory")

	eng, err := factory(t.Context())
	require.NoError(t, err)
	require.NotNil(t, eng)
	assert.Equal(t, string(engine.Casbin), eng.Name())

	// Non-OptFunc options are filtered out instead of panicking.
	eng, err = factory(t.Context(), "not-an-option")
	require.NoError(t, err)
	assert.NotNil(t, eng)
}

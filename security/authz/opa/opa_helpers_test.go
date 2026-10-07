package opa

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	windlog "github.com/tx7do/go-wind/log"

	engine "github.com/tx7do/go-wind-plugins/security/authz"
)

// ---------------------------------------------------------------------------
// Engine construction and defaults
// ---------------------------------------------------------------------------

// loggedState builds a bare State with a logger attached; several State
// methods (e.g. projectsFromPartialResults, ParseProjectsQuery) call s.log on
// error paths and panic when the engine was not built via NewEngine.
func loggedState() *State {
	return &State{log: windlog.GetLogger()}
}

func TestNewEngineDefaults(t *testing.T) {
	s, err := NewEngine(t.Context())
	require.NoError(t, err)

	assert.Equal(t, string(engine.Opa), s.Name())
	assert.Equal(t, ast.DefaultRegoVersion, s.regoVersion)
	assert.False(t, s.enableQueryTracer)
	assert.NotNil(t, s.compiler)
	assert.Len(t, s.modules, 3, "the three compiled-in policies should be loaded")

	require.Len(t, s.queries, 3)
	assert.Contains(t, s.queries, AuthzProjectsQueryKey)
	assert.Contains(t, s.queries, FilteredPairsQueryKey)
	assert.Contains(t, s.queries, FilteredProjectsQueryKey)
}

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

func TestWithRegoVersion(t *testing.T) {
	tests := []struct {
		version string
		want    ast.RegoVersion
	}{
		{"v0", ast.RegoV0},
		{"v0v1", ast.RegoV0CompatV1},
		{"v1", ast.RegoV1},
		{"unknown-falls-back-to-v1", ast.RegoV1},
		{"", ast.RegoV1},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			s := &State{regoVersion: ast.DefaultRegoVersion}
			WithRegoVersion(tt.version)(s)
			assert.Equal(t, tt.want, s.regoVersion)
		})
	}
}

func TestWithEnableQueryTracer(t *testing.T) {
	s := loggedState()
	WithEnableQueryTracer(true)(s)
	assert.True(t, s.enableQueryTracer)
	WithEnableQueryTracer(false)(s)
	assert.False(t, s.enableQueryTracer)
}

func TestWithLogger(t *testing.T) {
	s := loggedState()
	WithLogger(&nopLogger{})(s)
	assert.NotNil(t, s.log)
}

// nopLogger is a minimal windlog.Logger fake for option wiring.
type nopLogger struct{}

func (l *nopLogger) Debug(_ context.Context, _ string, _ ...any) {}
func (l *nopLogger) Info(_ context.Context, _ string, _ ...any)  {}
func (l *nopLogger) Warn(_ context.Context, _ string, _ ...any)  {}
func (l *nopLogger) Error(_ context.Context, _ string, _ ...any) {}
func (l *nopLogger) Enabled(_ windlog.Level) bool                { return false }
func (l *nopLogger) With(_ ...any) windlog.Logger                { return l }

func TestWithModules(t *testing.T) {
	mod, err := ast.ParseModule("custom.rego", testModuleSrc)
	require.NoError(t, err)

	s, err := NewEngine(t.Context(), WithModules(map[string]*ast.Module{"custom.rego": mod}))
	require.NoError(t, err)
	require.Len(t, s.modules, 1, "supplied modules replace the compiled-in assets")
	assert.Contains(t, s.modules, "custom.rego")
}

const testModuleSrc = `package custom

allow { true }
`

func TestWithModulesFromString(t *testing.T) {
	t.Run("valid module", func(t *testing.T) {
		s := loggedState()
		WithModulesFromString(map[string]string{"m.rego": testModuleSrc})(s)
		require.Len(t, s.modules, 1)
		assert.Contains(t, s.modules, "m.rego")
	})

	t.Run("invalid module is logged and ignored", func(t *testing.T) {
		s := loggedState()
		WithModulesFromString(map[string]string{"m.rego": "not valid rego }{"})
		assert.Nil(t, s.modules, "failed parses must not populate modules")
	})
}

func TestWithModulesFromFiles(t *testing.T) {
	t.Run("valid module file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "m.rego")
		require.NoError(t, os.WriteFile(path, []byte(testModuleSrc), 0o644))

		s := loggedState()
		WithModulesFromFiles(map[string]string{"m.rego": path})(s)
		require.Len(t, s.modules, 1)
		assert.Contains(t, s.modules, "m.rego")
	})

	t.Run("missing file is logged and ignored", func(t *testing.T) {
		s := loggedState()
		WithModulesFromFiles(map[string]string{"m.rego": filepath.Join(t.TempDir(), "nope.rego")})
		assert.Nil(t, s.modules)
	})
}

func TestWithQueryOverrides(t *testing.T) {
	s, err := NewEngine(t.Context(),
		WithProjectsAuthorizedQuery("data.authz.authorized_project[project]"),
		WithFilterAuthorizedPairsQuery("data.authz.introspection.authorized_pair[_]"),
		WithFilterAuthorizedProjectsQuery("data.authz.introspection.authorized_project"),
	)
	require.NoError(t, err)
	assert.Equal(t, "data.authz.authorized_project[project]", s.authzProjectsQuery)
	assert.Equal(t, "data.authz.introspection.authorized_pair[_]", s.filteredPairsQuery)
	assert.Equal(t, "data.authz.introspection.authorized_project", s.filteredProjectsQuery)
}

// ---------------------------------------------------------------------------
// ParseProjectsQuery / ParseFilterPairsQuery / ParseFilterProjectsQuery
// ---------------------------------------------------------------------------

func TestParseQueries(t *testing.T) {
	t.Run("valid query replaces default", func(t *testing.T) {
		s, err := NewEngine(t.Context())
		require.NoError(t, err)

		require.NoError(t, s.ParseProjectsQuery("data.authz.allow"))
		assert.Equal(t, "data.authz.allow", s.authzProjectsQuery)
		assert.Contains(t, s.queries[AuthzProjectsQueryKey].String(), "data.authz.allow")
	})

	t.Run("empty query falls back to default", func(t *testing.T) {
		s, err := NewEngine(t.Context())
		require.NoError(t, err)

		require.NoError(t, s.ParseProjectsQuery(""))
		assert.Equal(t, defaultAuthzProjectsQuery, s.authzProjectsQuery)

		require.NoError(t, s.ParseFilterPairsQuery(""))
		assert.Equal(t, defaultFilteredPairsQuery, s.filteredPairsQuery)

		require.NoError(t, s.ParseFilterProjectsQuery(""))
		assert.Equal(t, defaultFilteredProjectsQuery, s.filteredProjectsQuery)
	})

	t.Run("invalid query returns parse error", func(t *testing.T) {
		// Note: a bare &State{} has a nil logger and would panic on this error
		// path, so engines must be built through NewEngine here.
		s, err := NewEngine(t.Context())
		require.NoError(t, err)
		assert.Error(t, s.ParseProjectsQuery("this is not rego }{"))

		s2, err := NewEngine(t.Context())
		require.NoError(t, err)
		assert.Error(t, s2.ParseFilterPairsQuery("this is not rego }{"))

		s3, err := NewEngine(t.Context())
		require.NoError(t, err)
		assert.Error(t, s3.ParseFilterProjectsQuery("this is not rego }{"))
	})

	t.Run("nil query map is initialized", func(t *testing.T) {
		s := loggedState() // queries map is nil
		require.NoError(t, s.ParseProjectsQuery("true"))
		require.Len(t, s.queries, 1)

		s2 := loggedState()
		require.NoError(t, s2.ParseFilterPairsQuery("true"))
		require.Len(t, s2.queries, 1)

		s3 := loggedState()
		require.NoError(t, s3.ParseFilterProjectsQuery("true"))
		require.Len(t, s3.queries, 1)
	})
}

// ---------------------------------------------------------------------------
// Result conversion helpers
// ---------------------------------------------------------------------------

func expr(value interface{}) *rego.ExpressionValue {
	return &rego.ExpressionValue{Value: value, Text: "x"}
}

func resultSet(values ...interface{}) rego.ResultSet {
	rs := make(rego.ResultSet, 0, len(values))
	for _, v := range values {
		rs = append(rs, rego.Result{Expressions: []*rego.ExpressionValue{expr(v)}})
	}
	return rs
}

func TestPairsFromResults(t *testing.T) {
	s := loggedState()

	t.Run("valid pairs", func(t *testing.T) {
		rs := resultSet(
			map[string]interface{}{"resource": "doc:1", "action": "doc:read"},
			map[string]interface{}{"resource": "doc:2", "action": "doc:write"},
		)
		pairs, err := s.pairsFromResults(rs)
		require.NoError(t, err)
		assert.Equal(t, engine.Pairs{
			{Resource: "doc:1", Action: "doc:read"},
			{Resource: "doc:2", Action: "doc:write"},
		}, pairs)
	})

	t.Run("wrong expression count", func(t *testing.T) {
		rs := rego.ResultSet{{Expressions: []*rego.ExpressionValue{expr("a"), expr("b")}}}
		_, err := s.pairsFromResults(rs)
		assert.Error(t, err)
		var e *UnexpectedResultExpressionError
		assert.ErrorAs(t, err, &e)
	})

	t.Run("value is not a map", func(t *testing.T) {
		_, err := s.pairsFromResults(resultSet("scalar"))
		assert.Error(t, err)
	})

	t.Run("resource is not a string", func(t *testing.T) {
		_, err := s.pairsFromResults(resultSet(map[string]interface{}{"resource": 1, "action": "a"}))
		assert.Error(t, err)
	})

	t.Run("action is not a string", func(t *testing.T) {
		_, err := s.pairsFromResults(resultSet(map[string]interface{}{"resource": "r", "action": 2}))
		assert.Error(t, err)
	})

	t.Run("empty result set yields empty pairs", func(t *testing.T) {
		pairs, err := s.pairsFromResults(rego.ResultSet{})
		require.NoError(t, err)
		assert.Empty(t, pairs)
	})
}

func TestPairsFromAllowed(t *testing.T) {
	s := loggedState()

	t.Run("matching pair is allowed", func(t *testing.T) {
		rs := resultSet(map[string]interface{}{"resource": "doc:1", "action": "doc:read"})
		allowed, err := s.pairsFromAllowed(rs)
		require.NoError(t, err)
		assert.True(t, allowed)
	})

	t.Run("empty result set is denied", func(t *testing.T) {
		allowed, err := s.pairsFromAllowed(rego.ResultSet{})
		require.NoError(t, err)
		assert.False(t, allowed)
	})

	t.Run("malformed results are errors", func(t *testing.T) {
		two := rego.ResultSet{{Expressions: []*rego.ExpressionValue{expr("a"), expr("b")}}}
		_, err := s.pairsFromAllowed(two)
		assert.Error(t, err)

		_, err = s.pairsFromAllowed(resultSet("scalar"))
		assert.Error(t, err)

		_, err = s.pairsFromAllowed(resultSet(map[string]interface{}{"action": "a"}))
		assert.Error(t, err)

		_, err = s.pairsFromAllowed(resultSet(map[string]interface{}{"resource": "r"}))
		assert.Error(t, err)
	})
}

func TestProjectsFromPartialResults(t *testing.T) {
	s := loggedState()

	t.Run("valid project array", func(t *testing.T) {
		rs := resultSet([]interface{}{"proj-1", "proj-2"})
		projects, err := s.projectsFromPartialResults(rs)
		require.NoError(t, err)
		assert.Equal(t, engine.Projects{"proj-1", "proj-2"}, projects)
	})

	t.Run("more than one result", func(t *testing.T) {
		rs := resultSet([]interface{}{"proj-1"}, []interface{}{"proj-2"})
		_, err := s.projectsFromPartialResults(rs)
		var e *UnexpectedResultSetError
		assert.ErrorAs(t, err, &e)
	})

	t.Run("wrong expression count", func(t *testing.T) {
		rs := rego.ResultSet{{Expressions: []*rego.ExpressionValue{expr("a"), expr("b")}}}
		_, err := s.projectsFromPartialResults(rs)
		assert.Error(t, err)
	})

	t.Run("value is not an array", func(t *testing.T) {
		_, err := s.projectsFromPartialResults(resultSet("scalar"))
		assert.Error(t, err)
	})

	t.Run("array with non-string element", func(t *testing.T) {
		_, err := s.projectsFromPartialResults(resultSet([]interface{}{"proj-1", 42}))
		assert.Error(t, err)
	})
}

func TestStringArrayFromResults(t *testing.T) {
	s := loggedState()

	vals, err := s.stringArrayFromResults([]*rego.ExpressionValue{expr([]interface{}{"a", "b"})})
	require.NoError(t, err)
	assert.Equal(t, engine.Projects{"a", "b"}, vals)

	_, err = s.stringArrayFromResults([]*rego.ExpressionValue{expr("not-an-array")})
	assert.Error(t, err)
}

func TestProjectsFromPreparedEvalQuery(t *testing.T) {
	s := loggedState()

	rs := rego.ResultSet{
		{Bindings: map[string]interface{}{"project": "proj-1"}},
		{Bindings: map[string]interface{}{"project": "proj-2"}},
		{Bindings: map[string]interface{}{"project": "proj-1"}}, // duplicate
	}
	projects, err := s.projectsFromPreparedEvalQuery(rs)
	require.NoError(t, err)
	assert.Equal(t, engine.Projects{"proj-1", "proj-2"}, projects, "duplicates must be removed")

	_, err = s.projectsFromPreparedEvalQuery(rego.ResultSet{
		{Bindings: map[string]interface{}{"other": 1}},
	})
	assert.Error(t, err)
}

func TestAllowedFromPreparedEvalQuery(t *testing.T) {
	s := loggedState()

	allowed, err := s.allowedFromPreparedEvalQuery(rego.ResultSet{
		{Bindings: map[string]interface{}{"project": "proj-1"}},
	})
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = s.allowedFromPreparedEvalQuery(rego.ResultSet{})
	require.NoError(t, err)
	assert.False(t, allowed)

	_, err = s.allowedFromPreparedEvalQuery(rego.ResultSet{
		{Bindings: map[string]interface{}{"nope": 1}},
	})
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// Error types
// ---------------------------------------------------------------------------

func TestErrorTypes(t *testing.T) {
	exprErr := &UnexpectedResultExpressionError{exps: []*rego.ExpressionValue{expr("x")}}
	assert.Contains(t, exprErr.Error(), "unexpected result expressions")

	setErr := &UnexpectedResultSetError{set: rego.ResultSet{}}
	assert.Contains(t, setErr.Error(), "unexpected result set")

	evalErr := &EvaluationError{e: errors.New("boom")}
	assert.Contains(t, evalErr.Error(), "error in query evaluation")
	assert.Contains(t, evalErr.Error(), "boom")
}

// ---------------------------------------------------------------------------
// evalQuery tracer branch
// ---------------------------------------------------------------------------

func TestEvalQuery_WithTracer(t *testing.T) {
	s, err := NewEngine(t.Context(), WithEnableQueryTracer(true))
	require.NoError(t, err)

	query, err := ast.ParseBody("1 == 1")
	require.NoError(t, err)

	rs, err := s.evalQuery(t.Context(), query, nil, s.snap.Load().store)
	require.NoError(t, err)
	require.NotEmpty(t, rs, "the trivial query should produce one result")
	assert.True(t, s.enableQueryTracer)
}

// ---------------------------------------------------------------------------
// Engine-level behavior over a hand-built policy store (fully in-memory)
// ---------------------------------------------------------------------------

func testPoliciesAndRoles() (engine.PolicyMap, engine.RoleMap) {
	policies := engine.PolicyMap{
		"pol-1": map[string]interface{}{
			"members": engine.MakeSubjects("user:alice", "team:admins"),
			"statements": map[string]interface{}{
				"s-allow-doc": map[string]interface{}{
					"effect":    "allow",
					"resources": engine.MakeResources("doc:*"),
					"actions":   engine.MakeActions("doc:document:read"),
					"projects":  engine.MakeProjects("proj-1"),
				},
				"s-deny-secret": map[string]interface{}{
					"effect":    "deny",
					"resources": engine.MakeResources("secret:*"),
					"actions":   engine.MakeActions("*"),
					"projects":  engine.MakeProjects("proj-1"),
				},
			},
		},
	}
	roles := engine.RoleMap{}
	return policies, roles
}

func TestEngine_AuthorizationFlow(t *testing.T) {
	s, err := NewEngine(t.Context())
	require.NoError(t, err)

	policies, roles := testPoliciesAndRoles()
	require.NoError(t, s.SetPolicies(t.Context(), policies, roles))
	snap := s.snap.Load()
	require.NotNil(t, snap)
	assert.NotNil(t, snap.preparedEvalProjects, "SetPolicies must build the prepared projects query")

	alice := engine.Subjects{"user:alice"}

	t.Run("IsAuthorized with project uses the prepared query", func(t *testing.T) {
		allowed, err := s.IsAuthorized(t.Context(), "user:alice", "doc:document:read", "doc:report-1", "proj-1")
		require.NoError(t, err)
		assert.True(t, allowed)

		allowed, err = s.IsAuthorized(t.Context(), "user:bob", "doc:document:read", "doc:report-1", "proj-1")
		require.NoError(t, err)
		assert.False(t, allowed, "bob is not a member of any policy")
	})

	t.Run("IsAuthorized without project falls back to filtered pairs", func(t *testing.T) {
		allowed, err := s.IsAuthorized(t.Context(), "user:alice", "doc:document:read", "doc:report-1", "")
		require.NoError(t, err)
		assert.True(t, allowed)

		allowed, err = s.IsAuthorized(t.Context(), "user:alice", "secret:vault:open", "secret:vault", "")
		require.NoError(t, err)
		assert.False(t, allowed, "the deny statement on secret:* must win")
	})

	t.Run("ProjectsAuthorized filters the candidate list", func(t *testing.T) {
		projects, err := s.ProjectsAuthorized(t.Context(),
			alice, "doc:document:read", "doc:report-1",
			engine.Projects{"proj-1", "proj-2"})
		require.NoError(t, err)
		assert.Equal(t, engine.Projects{"proj-1"}, projects)
	})

	t.Run("FilterAuthorizedPairs keeps allowed pairs", func(t *testing.T) {
		pairs, err := s.FilterAuthorizedPairs(t.Context(), alice, engine.Pairs{
			{Resource: "doc:report-1", Action: "doc:document:read"},
			{Resource: "secret:vault", Action: "secret:vault:open"},
		})
		require.NoError(t, err)
		assert.Equal(t, engine.Pairs{
			{Resource: "doc:report-1", Action: "doc:document:read"},
		}, pairs)
	})

	t.Run("FilterAuthorizedProjects lists projects with any membership", func(t *testing.T) {
		projects, err := s.FilterAuthorizedProjects(t.Context(), alice)
		require.NoError(t, err)
		assert.Equal(t, engine.Projects{"proj-1"}, projects)

		projects, err = s.FilterAuthorizedProjects(t.Context(), engine.Subjects{"user:bob"})
		require.NoError(t, err)
		assert.Empty(t, projects)
	})
}

package zanzibar

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	engine "github.com/tx7do/go-wind-plugins/security/authz"
)

// The keto client uses lazy gRPC connections (grpc.NewClient does not dial
// until the first RPC) and the REST client only builds configuration, so
// constructing engines with WithKeto is hermetic.
//
// WithOpenFga is intentionally NOT exercised here without the KRATOS_IT gate:
// openfga.NewClient eagerly lists stores over HTTP inside its init.

func TestNewEngine_RequiresAtLeastOneClient(t *testing.T) {
	s, err := NewEngine(t.Context())
	assert.Nil(t, s)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "zanzibar client is nil")
}

func TestNewEngine_WithKetoGRPC(t *testing.T) {
	s, err := NewEngine(t.Context(), WithKeto("127.0.0.1:4466", "127.0.0.1:4467", true))
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.NotNil(t, s.ketoClient, "keto client should be wired by WithKeto")
	assert.Nil(t, s.openfgaClient)
	assert.Equal(t, string(engine.Zanzibar), s.Name())
}

func TestNewEngine_WithKetoREST(t *testing.T) {
	s, err := NewEngine(t.Context(), WithKeto("http://127.0.0.1:4466", "http://127.0.0.1:4467", false))
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.NotNil(t, s.ketoClient)
}

func TestState_NoClientHelpers(t *testing.T) {
	// A bare State has no client wired; the passthrough helpers must return
	// empty, successful results without touching the network.
	s := &State{}

	assert.Equal(t, string(engine.Zanzibar), s.Name())

	projects, err := s.ProjectsAuthorized(t.Context(),
		engine.Subjects{"anne"}, engine.Action("read"), engine.Resource("doc"), engine.Projects{"p1"})
	require.NoError(t, err)
	assert.Empty(t, projects)

	pairs, err := s.FilterAuthorizedPairs(t.Context(), engine.Subjects{"anne"}, engine.Pairs{})
	require.NoError(t, err)
	assert.Empty(t, pairs)

	projects, err = s.FilterAuthorizedProjects(t.Context(), engine.Subjects{"anne"})
	require.NoError(t, err)
	assert.Empty(t, projects)

	allowed, err := s.IsAuthorized(t.Context(), "anne", "read", "doc", "p1")
	require.NoError(t, err)
	assert.False(t, allowed)

	assert.NoError(t, s.SetPolicies(t.Context(), nil, nil))
}

func TestFactoryRegistration(t *testing.T) {
	factory, ok := engine.GetFactory(engine.Zanzibar)
	require.True(t, ok, "package init should register the zanzibar factory")

	// Building through the factory without options surfaces the same
	// "client is nil" guard as NewEngine.
	eng, err := factory(t.Context())
	assert.Nil(t, eng)
	require.Error(t, err)

	// And with options it produces a working engine.
	eng, err = factory(t.Context(), WithKeto("127.0.0.1:4466", "127.0.0.1:4467", true))
	require.NoError(t, err)
	require.NotNil(t, eng)
	assert.Equal(t, string(engine.Zanzibar), eng.Name())

	// Non-OptFunc options are filtered out instead of panicking; with none
	// left, NewEngine still surfaces the nil-client guard.
	eng, err = factory(t.Context(), "not-an-option", 42)
	assert.Nil(t, eng)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "zanzibar client is nil")
}

func TestWithOpenFga_OptionIsLazy(t *testing.T) {
	token := "tok"
	clientID := "cid"
	clientSecret := "secret"
	audience := "aud"
	issuer := "https://issuer.example.com"

	// WithOpenFga only closes over its arguments here; the openfga client is
	// constructed (and performs network I/O in its init) only when the
	// returned option is applied, so it is deliberately left unapplied in
	// hermetic runs.
	opt := WithOpenFga("http://127.0.0.1:8080", "store-1", &token,
		&clientID, &clientSecret, &audience, &issuer)
	assert.NotNil(t, opt)

	assert.NotNil(t, WithOpenFga("http://127.0.0.1:8080", "store-1", nil, nil, nil, nil, nil))
}

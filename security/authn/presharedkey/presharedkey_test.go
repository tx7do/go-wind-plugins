package presharedkey

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	engine "github.com/tx7do/go-wind-plugins/security/authn"
)

// ---------------------------------------------------------------------------
// test helpers
// ---------------------------------------------------------------------------

var testKeys = []string{"key-alpha", "key-beta", "key-gamma"}

// createAuthCtx creates a gRPC context with a Bearer token stored in
// incoming metadata.
func createAuthCtx(token string) context.Context {
	md := metadata.Pairs(engine.HeaderAuthorize, engine.BearerWord+" "+token)
	return metadata.NewIncomingContext(context.Background(), md)
}

// ---------------------------------------------------------------------------
// NewAuthenticator / Options
// ---------------------------------------------------------------------------

func TestNewAuthenticator_NoKeys(t *testing.T) {
	auth, err := NewAuthenticator()
	assert.Nil(t, err)
	assert.NotNil(t, auth)
}

func TestNewAuthenticator_WithKeys(t *testing.T) {
	auth, err := NewAuthenticator(WithKeys(testKeys))
	assert.Nil(t, err)
	assert.NotNil(t, auth)
}

func TestWithKeys_BuildsKeySet(t *testing.T) {
	opts := &Options{}
	WithKeys(testKeys)(opts)
	assert.Len(t, opts.ValidKeys, len(testKeys))
	for _, k := range testKeys {
		assert.True(t, opts.ValidKeys[k], "key %q should be in the set", k)
	}
}

// ---------------------------------------------------------------------------
// AuthenticateToken
// ---------------------------------------------------------------------------

func TestAuthenticateToken_Valid(t *testing.T) {
	auth, err := NewAuthenticator(WithKeys(testKeys))
	require.Nil(t, err)

	decoded, err := auth.AuthenticateToken("key-beta")
	assert.Nil(t, err)
	assert.NotNil(t, decoded)
}

func TestAuthenticateToken_Invalid(t *testing.T) {
	auth, err := NewAuthenticator(WithKeys(testKeys))
	require.Nil(t, err)

	_, err = auth.AuthenticateToken("wrong-key")
	assert.NotNil(t, err)
	assert.Equal(t, engine.ErrUnauthenticated, err)
}

func TestAuthenticateToken_EmptyString(t *testing.T) {
	auth, err := NewAuthenticator(WithKeys(testKeys))
	require.Nil(t, err)

	_, err = auth.AuthenticateToken("")
	assert.NotNil(t, err)
	assert.Equal(t, engine.ErrUnauthenticated, err)
}

func TestAuthenticateToken_NoKeysConfigured(t *testing.T) {
	auth, err := NewAuthenticator() // no keys
	require.Nil(t, err)

	_, err = auth.AuthenticateToken("any-token")
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "at least one key")
}

// ---------------------------------------------------------------------------
// Authenticate (via gRPC context)
// ---------------------------------------------------------------------------

func TestAuthenticate_ValidToken(t *testing.T) {
	auth, err := NewAuthenticator(WithKeys(testKeys))
	require.Nil(t, err)

	ctx := createAuthCtx("key-gamma")
	decoded, err := auth.Authenticate(ctx)
	assert.Nil(t, err)
	assert.NotNil(t, decoded)
}

func TestAuthenticate_InvalidToken(t *testing.T) {
	auth, err := NewAuthenticator(WithKeys(testKeys))
	require.Nil(t, err)

	ctx := createAuthCtx("bad-token")
	_, err = auth.Authenticate(ctx)
	assert.Equal(t, engine.ErrUnauthenticated, err)
}

func TestAuthenticate_MissingBearerToken(t *testing.T) {
	auth, err := NewAuthenticator(WithKeys(testKeys))
	require.Nil(t, err)

	_, err = auth.Authenticate(context.Background())
	assert.Equal(t, engine.ErrMissingBearerToken, err)
}

// ---------------------------------------------------------------------------
// CreateIdentity
// ---------------------------------------------------------------------------

func TestCreateIdentity_ReturnsValidKey(t *testing.T) {
	auth, err := NewAuthenticator(WithKeys(testKeys))
	require.Nil(t, err)

	token, err := auth.CreateIdentity(engine.AuthClaims{})
	assert.Nil(t, err)
	assert.Contains(t, testKeys, token, "should return one of the configured keys")

	// the generated token must authenticate successfully
	_, err = auth.AuthenticateToken(token)
	assert.Nil(t, err)
}

func TestCreateIdentity_NoKeys(t *testing.T) {
	auth, err := NewAuthenticator()
	require.Nil(t, err)

	token, err := auth.CreateIdentity(engine.AuthClaims{})
	assert.Nil(t, err)
	assert.Empty(t, token)
}

// ---------------------------------------------------------------------------
// CreateIdentityWithContext
// ---------------------------------------------------------------------------

func TestCreateIdentityWithContext_Success(t *testing.T) {
	auth, err := NewAuthenticator(WithKeys(testKeys))
	require.Nil(t, err)

	// CreateIdentity picks a random key from the set, so we just verify
	// the call succeeds and returns a usable context.
	ctx, err := auth.CreateIdentityWithContext(
		context.Background(),
		engine.AuthClaims{},
	)
	assert.Nil(t, err)
	assert.NotNil(t, ctx)
}

func TestCreateIdentityWithContext_RoundTrip(t *testing.T) {
	auth, err := NewAuthenticator(WithKeys(testKeys))
	require.Nil(t, err)

	token, err := auth.CreateIdentity(engine.AuthClaims{})
	require.Nil(t, err)
	require.NotEmpty(t, token)

	// manually inject the token into incoming metadata, then authenticate
	ctx := createAuthCtx(token)
	_, err = auth.Authenticate(ctx)
	assert.Nil(t, err)
}

func TestCreateIdentityWithContext_NoKeys(t *testing.T) {
	auth, err := NewAuthenticator()
	require.Nil(t, err)

	ctx, err := auth.CreateIdentityWithContext(
		context.Background(),
		engine.AuthClaims{},
	)
	assert.Nil(t, err) // no error even with no keys (empty token)
	assert.NotNil(t, ctx)
}

// ---------------------------------------------------------------------------
// Close
// ---------------------------------------------------------------------------

func TestClose_NoPanic(t *testing.T) {
	auth, err := NewAuthenticator(WithKeys(testKeys))
	require.Nil(t, err)

	assert.NotPanics(t, func() {
		auth.Close()
	})
}

// ---------------------------------------------------------------------------
// Interface compliance
// ---------------------------------------------------------------------------

func TestAuthenticator_ImplementsEngineAuthenticator(t *testing.T) {
	var _ engine.Authenticator = (*Authenticator)(nil)
}

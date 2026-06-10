package jwt

import (
	"context"
	"strings"
	"testing"
	"time"

	jwtV5 "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	engine "github.com/tx7do/go-wind-plugins/security/authn"
)

// ---------------------------------------------------------------------------
// test helpers
// ---------------------------------------------------------------------------

var testKey = []byte("my-secret-key")

// createAuthCtx creates a gRPC context with a Bearer token stored in
// incoming metadata.  This is what the server-side sees.
func createAuthCtx(token string) context.Context {
	md := metadata.Pairs(engine.HeaderAuthorize, engine.BearerWord+" "+token)
	return metadata.NewIncomingContext(context.Background(), md)
}

// ---------------------------------------------------------------------------
// NewAuthenticator
// ---------------------------------------------------------------------------

func TestNewAuthenticator_DefaultSigningMethod(t *testing.T) {
	auth, err := NewAuthenticator()
	assert.Nil(t, err)
	assert.NotNil(t, auth)
}

func TestNewAuthenticator_WithSigningMethod(t *testing.T) {
	auth, err := NewAuthenticator(
		WithSigningMethod("HS256"),
		WithKey(testKey),
	)
	assert.Nil(t, err)
	assert.NotNil(t, auth)
}

// ---------------------------------------------------------------------------
// CreateIdentity
// ---------------------------------------------------------------------------

func TestCreateIdentity_Success(t *testing.T) {
	auth, err := NewAuthenticator(WithKey(testKey))
	require.Nil(t, err)

	token, err := auth.CreateIdentity(engine.AuthClaims{
		engine.ClaimFieldSubject: "alice",
	})
	assert.Nil(t, err)
	assert.NotEmpty(t, token)

	parts := strings.Split(token, ".")
	assert.Equal(t, 3, len(parts), "JWT should have 3 parts")
}

func TestCreateIdentity_MissingKeyFunc(t *testing.T) {
	auth, err := NewAuthenticator()
	require.Nil(t, err)

	token, err := auth.CreateIdentity(engine.AuthClaims{})
	assert.Empty(t, token)
	assert.Equal(t, engine.ErrMissingKeyFunc, err)
}

func TestCreateIdentity_ClaimsRoundTrip(t *testing.T) {
	auth, err := NewAuthenticator(WithKey(testKey))
	require.Nil(t, err)

	claims := engine.AuthClaims{
		engine.ClaimFieldSubject:  "bob",
		engine.ClaimFieldIssuer:   "test-issuer",
		engine.ClaimFieldAudience: []string{"aud1", "aud2"},
		engine.ClaimFieldScope:    []string{"read", "write"},
	}

	token, err := auth.CreateIdentity(claims)
	require.Nil(t, err)

	decoded, err := auth.AuthenticateToken(token)
	require.Nil(t, err)

	sub, _ := decoded.GetSubject()
	assert.Equal(t, "bob", sub)

	iss, _ := decoded.GetIssuer()
	assert.Equal(t, "test-issuer", iss)

	scopes, _ := decoded.GetScopes()
	assert.Equal(t, []string{"read", "write"}, []string(scopes))
}

// ---------------------------------------------------------------------------
// AuthenticateToken
// ---------------------------------------------------------------------------

func TestAuthenticateToken_Valid(t *testing.T) {
	auth, err := NewAuthenticator(WithKey(testKey))
	require.Nil(t, err)

	token, err := auth.CreateIdentity(engine.AuthClaims{
		engine.ClaimFieldSubject: "charlie",
	})
	require.Nil(t, err)

	decoded, err := auth.AuthenticateToken(token)
	assert.Nil(t, err)
	assert.NotNil(t, decoded)

	sub, _ := decoded.GetSubject()
	assert.Equal(t, "charlie", sub)
}

func TestAuthenticateToken_Malformed(t *testing.T) {
	auth, err := NewAuthenticator(WithKey(testKey))
	require.Nil(t, err)

	_, err = auth.AuthenticateToken("not.a.valid.jwt.token")
	assert.NotNil(t, err)
	assert.Equal(t, engine.ErrInvalidToken, err)
}

func TestAuthenticateToken_EmptyString(t *testing.T) {
	auth, err := NewAuthenticator(WithKey(testKey))
	require.Nil(t, err)

	_, err = auth.AuthenticateToken("")
	assert.NotNil(t, err)
	assert.Equal(t, engine.ErrInvalidToken, err)
}

func TestAuthenticateToken_InvalidSignature(t *testing.T) {
	auth, err := NewAuthenticator(WithKey(testKey))
	require.Nil(t, err)

	token, err := auth.CreateIdentity(engine.AuthClaims{
		engine.ClaimFieldSubject: "dave",
	})
	require.Nil(t, err)

	// verify with a different key
	auth2, err := NewAuthenticator(WithKey([]byte("different-key")))
	require.Nil(t, err)

	_, err = auth2.AuthenticateToken(token)
	assert.NotNil(t, err)
	assert.Equal(t, engine.ErrSignTokenFailed, err)
}

func TestAuthenticateToken_Expired(t *testing.T) {
	auth, err := NewAuthenticator(WithKey(testKey))
	require.Nil(t, err)

	// create a token that already expired
	expiredToken := jwtV5.NewWithClaims(jwtV5.SigningMethodHS256, jwtV5.MapClaims{
		engine.ClaimFieldSubject:        "eve",
		engine.ClaimFieldExpirationTime: float64(time.Now().Add(-1 * time.Hour).Unix()),
	})
	tokenStr, err := expiredToken.SignedString(testKey)
	require.Nil(t, err)

	_, err = auth.AuthenticateToken(tokenStr)
	assert.NotNil(t, err)
	assert.Equal(t, engine.ErrTokenExpired, err)
}

func TestAuthenticateToken_NotValidYet(t *testing.T) {
	auth, err := NewAuthenticator(WithKey(testKey))
	require.Nil(t, err)

	// create a token that is not valid yet (nbf far in the future)
	futureToken := jwtV5.NewWithClaims(jwtV5.SigningMethodHS256, jwtV5.MapClaims{
		engine.ClaimFieldSubject:   "frank",
		engine.ClaimFieldNotBefore: float64(time.Now().Add(1 * time.Hour).Unix()),
	})
	tokenStr, err := futureToken.SignedString(testKey)
	require.Nil(t, err)

	_, err = auth.AuthenticateToken(tokenStr)
	assert.NotNil(t, err)
	assert.Equal(t, engine.ErrTokenExpired, err)
}

func TestAuthenticateToken_UnsupportedSigningMethod(t *testing.T) {
	// create token with HS512
	authHS512, err := NewAuthenticator(
		WithSigningMethod("HS512"),
		WithKey(testKey),
	)
	require.Nil(t, err)

	token, err := authHS512.CreateIdentity(engine.AuthClaims{
		engine.ClaimFieldSubject: "grace",
	})
	require.Nil(t, err)

	// try to verify with HS256 authenticator (default)
	authHS256, err := NewAuthenticator(WithKey(testKey))
	require.Nil(t, err)

	_, err = authHS256.AuthenticateToken(token)
	assert.NotNil(t, err)
	assert.Equal(t, engine.ErrUnsupportedSigningMethod, err)
}

func TestAuthenticateToken_MissingKeyFunc(t *testing.T) {
	auth, err := NewAuthenticator() // no key
	require.Nil(t, err)

	// parseToken returns (nil, ErrMissingKeyFunc), but AuthenticateToken
	// checks jwtToken == nil first and returns ErrInvalidToken.
	_, err = auth.AuthenticateToken("anything")
	assert.NotNil(t, err)
	assert.Equal(t, engine.ErrInvalidToken, err)
}

// ---------------------------------------------------------------------------
// Authenticate (via gRPC context)
// ---------------------------------------------------------------------------

func TestAuthenticate_GrpcRoundTrip(t *testing.T) {
	auth, err := NewAuthenticator(WithKey(testKey))
	require.Nil(t, err)

	token, err := auth.CreateIdentity(engine.AuthClaims{
		engine.ClaimFieldSubject: "heidi",
	})
	require.Nil(t, err)

	ctx := createAuthCtx(token)
	decoded, err := auth.Authenticate(ctx)
	assert.Nil(t, err)
	assert.NotNil(t, decoded)

	sub, _ := decoded.GetSubject()
	assert.Equal(t, "heidi", sub)
}

func TestAuthenticate_MissingBearerToken(t *testing.T) {
	auth, err := NewAuthenticator(WithKey(testKey))
	require.Nil(t, err)

	// empty context → no token in incoming metadata
	_, err = auth.Authenticate(context.Background())
	assert.NotNil(t, err)
	assert.Equal(t, engine.ErrMissingBearerToken, err)
}

func TestAuthenticate_InvalidTokenInContext(t *testing.T) {
	auth, err := NewAuthenticator(WithKey(testKey))
	require.Nil(t, err)

	ctx := createAuthCtx("garbage.token.value")
	_, err = auth.Authenticate(ctx)
	assert.NotNil(t, err)
	assert.Equal(t, engine.ErrInvalidToken, err)
}

// ---------------------------------------------------------------------------
// CreateIdentityWithContext
// ---------------------------------------------------------------------------

func TestCreateIdentityWithContext_MissingKeyFunc(t *testing.T) {
	auth, err := NewAuthenticator()
	require.Nil(t, err)

	ctx := context.Background()
	outCtx, err := auth.CreateIdentityWithContext(ctx, engine.AuthClaims{})
	assert.NotNil(t, err)
	assert.Equal(t, engine.ErrMissingKeyFunc, err)
	assert.Equal(t, ctx, outCtx)
}

// ---------------------------------------------------------------------------
// Close
// ---------------------------------------------------------------------------

func TestClose_NoPanic(t *testing.T) {
	auth, err := NewAuthenticator(WithKey(testKey))
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

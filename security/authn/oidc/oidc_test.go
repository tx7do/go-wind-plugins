package oidc

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"testing"
	"time"

	jwtV5 "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	engine "github.com/tx7do/go-wind-plugins/security/authn"
)

// ---------------------------------------------------------------------------
// shared mock OIDC server (started once for all tests)
// ---------------------------------------------------------------------------

const (
	mockServerURL = "http://localhost:8083"
	mockAudience  = "wind.dev"
)

var mockServer *MockOidcServer

func TestMain(m *testing.M) {
	var err error
	mockServer, err = NewMockOidcServer(mockServerURL)
	if err != nil {
		panic("failed to start mock OIDC server: " + err.Error())
	}
	// give the server a brief moment to start listening
	time.Sleep(100 * time.Millisecond)
	os.Exit(m.Run())
}

// ---------------------------------------------------------------------------
// test helpers
// ---------------------------------------------------------------------------

// createAuthCtx creates a gRPC context with a Bearer token in incoming metadata.
func createAuthCtx(token string) context.Context {
	md := metadata.Pairs(engine.HeaderAuthorize, engine.BearerWord+" "+token)
	return metadata.NewIncomingContext(context.Background(), md)
}

// newTestAuthenticator creates an Authenticator pointing to the shared mock server.
func newTestAuthenticator(t *testing.T) *Authenticator {
	auth, err := NewAuthenticator(
		WithIssuerURL(mockServerURL),
		WithAudience(mockAudience),
		WithSigningMethod("RS256"),
	)
	require.Nil(t, err)
	return auth.(*Authenticator)
}

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

func TestOptions(t *testing.T) {
	opts := &Options{}
	WithIssuerURL("https://example.com")(opts)
	WithAudience("my-audience")(opts)
	WithSigningMethod("RS256")(opts)

	assert.Equal(t, "https://example.com", opts.IssuerURL)
	assert.Equal(t, "my-audience", opts.Audience)
	assert.NotNil(t, opts.signingMethod)
}

// ---------------------------------------------------------------------------
// CreateIdentity / CreateIdentityWithContext (no-op for OIDC)
// ---------------------------------------------------------------------------

func TestCreateIdentity_NoOp(t *testing.T) {
	auth := &Authenticator{options: &Options{}}
	token, err := auth.CreateIdentity(engine.AuthClaims{})
	assert.Empty(t, token)
	assert.Nil(t, err)
}

func TestCreateIdentityWithContext_NoOp(t *testing.T) {
	auth := &Authenticator{options: &Options{}}
	ctx := context.Background()
	outCtx, err := auth.CreateIdentityWithContext(ctx, engine.AuthClaims{})
	assert.Equal(t, ctx, outCtx)
	assert.Nil(t, err)
}

// ---------------------------------------------------------------------------
// IDToken.VerifyAccessToken
// ---------------------------------------------------------------------------

func TestIDToken_VerifyAccessToken_EmptyHash(t *testing.T) {
	idToken := &IDToken{}
	err := idToken.VerifyAccessToken("some-token")
	assert.Equal(t, engine.ErrNoAtHash, err)
}

func TestIDToken_VerifyAccessToken_RS256_Valid(t *testing.T) {
	accessToken := "test-access-token"
	h := sha256.New()
	h.Write([]byte(accessToken))
	sum := h.Sum(nil)[:h.Size()/2]
	hash := base64.RawURLEncoding.EncodeToString(sum)

	idToken := &IDToken{
		AccessTokenHash: hash,
		sigAlgorithm:    RS256,
	}
	err := idToken.VerifyAccessToken(accessToken)
	assert.Nil(t, err)
}

func TestIDToken_VerifyAccessToken_RS256_Invalid(t *testing.T) {
	idToken := &IDToken{
		AccessTokenHash: "wrong-hash",
		sigAlgorithm:    RS256,
	}
	err := idToken.VerifyAccessToken("test-access-token")
	assert.Equal(t, engine.ErrInvalidAtHash, err)
}

func TestIDToken_VerifyAccessToken_UnsupportedAlgorithm(t *testing.T) {
	idToken := &IDToken{
		AccessTokenHash: "some-hash",
		sigAlgorithm:    "HS256",
	}
	err := idToken.VerifyAccessToken("test-access-token")
	assert.NotNil(t, err)
}

// ---------------------------------------------------------------------------
// UserInfo.Claims
// ---------------------------------------------------------------------------

func TestUserInfo_Claims_NotSet(t *testing.T) {
	u := &UserInfo{}
	err := u.Claims(nil)
	assert.NotNil(t, err)
}

// ---------------------------------------------------------------------------
// Integration tests (require mock OIDC server)
// ---------------------------------------------------------------------------

func TestNewAuthenticator_WithMockServer(t *testing.T) {
	auth := newTestAuthenticator(t)
	assert.NotNil(t, auth)
	assert.NotEmpty(t, auth.JwksURI)
	assert.NotNil(t, auth.JWKs)
}

func TestAuthenticate_RoundTrip(t *testing.T) {
	auth := newTestAuthenticator(t)

	token, err := mockServer.GetToken(mockAudience, "user_name")
	require.Nil(t, err)

	ctx := createAuthCtx(token)
	authToken, err := auth.Authenticate(ctx)
	assert.Nil(t, err)
	assert.NotNil(t, authToken)

	sub, _ := authToken.GetSubject()
	assert.Equal(t, "user_name", sub)
}

func TestAuthenticateToken_Valid(t *testing.T) {
	auth := newTestAuthenticator(t)

	token, err := mockServer.GetToken(mockAudience, "user_name")
	require.Nil(t, err)

	decoded, err := auth.AuthenticateToken(token)
	assert.Nil(t, err)
	assert.NotNil(t, decoded)

	sub, _ := decoded.GetSubject()
	assert.Equal(t, "user_name", sub)
}

func TestAuthenticateToken_Malformed(t *testing.T) {
	auth := newTestAuthenticator(t)

	_, err := auth.AuthenticateToken("not.a.valid.jwt")
	assert.NotNil(t, err)
	assert.Equal(t, engine.ErrInvalidToken, err)
}

func TestAuthenticateToken_EmptyString(t *testing.T) {
	auth := newTestAuthenticator(t)

	_, err := auth.AuthenticateToken("")
	assert.NotNil(t, err)
}

func TestAuthenticateToken_WrongAudience(t *testing.T) {
	auth := newTestAuthenticator(t)

	// mock server signs with RS256 + matching kid, but audience differs
	token := jwtV5.NewWithClaims(jwtV5.SigningMethodRS256, jwtV5.MapClaims{
		engine.ClaimFieldIssuer:   mockServerURL,
		engine.ClaimFieldAudience: []string{"wrong-audience"},
		engine.ClaimFieldSubject:  "user_name",
	})
	token.Header["kid"] = kidHeader
	tokenStr, err := token.SignedString(mockServer.privateKey)
	require.Nil(t, err)

	_, err = auth.AuthenticateToken(tokenStr)
	assert.Equal(t, engine.ErrInvalidAudience, err)
}

func TestAuthenticateToken_WrongIssuer(t *testing.T) {
	auth := newTestAuthenticator(t)

	token := jwtV5.NewWithClaims(jwtV5.SigningMethodRS256, jwtV5.MapClaims{
		engine.ClaimFieldIssuer:   "http://wrong-issuer",
		engine.ClaimFieldAudience: []string{mockAudience},
		engine.ClaimFieldSubject:  "user_name",
	})
	token.Header["kid"] = kidHeader
	tokenStr, err := token.SignedString(mockServer.privateKey)
	require.Nil(t, err)

	_, err = auth.AuthenticateToken(tokenStr)
	assert.Equal(t, engine.ErrInvalidIssuer, err)
}

func TestAuthenticate_MissingBearerToken(t *testing.T) {
	auth := newTestAuthenticator(t)

	_, err := auth.Authenticate(context.Background())
	assert.Equal(t, engine.ErrMissingBearerToken, err)
}

func TestAuthenticate_InvalidTokenInContext(t *testing.T) {
	auth := newTestAuthenticator(t)

	ctx := createAuthCtx("garbage.token.value")
	_, err := auth.Authenticate(ctx)
	assert.NotNil(t, err)
}

func TestGetConfiguration(t *testing.T) {
	auth := newTestAuthenticator(t)

	cfg, err := auth.GetConfiguration()
	assert.Nil(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, mockServerURL, cfg.Issuer)
	assert.NotEmpty(t, cfg.JWKSURL)
	assert.NotEmpty(t, cfg.TokenURL)
	assert.NotEmpty(t, cfg.AuthURL)
}

func TestGetKeyfunc(t *testing.T) {
	auth := newTestAuthenticator(t)

	keys, err := auth.GetKeyfunc()
	assert.Nil(t, err)
	assert.NotNil(t, keys)
}

func TestClose_NoPanic(t *testing.T) {
	auth := newTestAuthenticator(t)
	assert.NotPanics(t, func() {
		auth.Close()
	})
}

// ---------------------------------------------------------------------------
// Interface compliance
// ---------------------------------------------------------------------------

func TestAuthenticator_ImplementsInterfaces(t *testing.T) {
	var _ engine.Authenticator = (*Authenticator)(nil)
	var _ Configurator = (*Authenticator)(nil)
}

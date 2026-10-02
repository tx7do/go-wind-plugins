package authn

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/metadata"

	errs "github.com/tx7do/go-wind-plugins/errors"
)

// ---------------------------------------------------------------------------
// Claims getters (standard JWT claims)
// ---------------------------------------------------------------------------

func TestAuthClaims_StandardGetters(t *testing.T) {
	exp := 1700000000.0
	claims := AuthClaims{
		ClaimFieldJwtID:          "jwt-id-1",
		ClaimFieldIssuer:         "https://issuer.example.com",
		ClaimFieldSubject:        "user-42",
		ClaimFieldAudience:       []string{"aud-1", "aud-2"},
		ClaimFieldExpirationTime: exp,
		ClaimFieldIssuedAt:       json.Number("1700000001"),
		ClaimFieldScope:          "read:users write:posts",
	}

	got, err := claims.GetJwtID()
	require.NoError(t, err)
	assert.Equal(t, "jwt-id-1", got)

	got, err = claims.GetIssuer()
	require.NoError(t, err)
	assert.Equal(t, "https://issuer.example.com", got)

	got, err = claims.GetSubject()
	require.NoError(t, err)
	assert.Equal(t, "user-42", got)

	aud, err := claims.GetAudience()
	require.NoError(t, err)
	assert.Equal(t, []string{"aud-1", "aud-2"}, []string(aud))

	scopes, err := claims.GetScopes()
	require.NoError(t, err)
	// A single space-delimited scope string is NOT split by the parser; it is
	// returned as one element (matches jwt/v5 ClaimStrings semantics).
	assert.Equal(t, []string{"read:users write:posts"}, []string(scopes))

	expDate, err := claims.GetExpirationTime()
	require.NoError(t, err)
	require.NotNil(t, expDate)
	assert.Equal(t, int64(1700000000), expDate.Unix())

	iatDate, err := claims.GetIssuedAt()
	require.NoError(t, err)
	require.NotNil(t, iatDate)
	assert.Equal(t, int64(1700000001), iatDate.Unix())

	// nbf is absent from the map.
	nbfDate, err := claims.GetNotBefore()
	require.NoError(t, err)
	assert.Nil(t, nbfDate)
}

func TestAuthClaims_GetExpirationTime_ZeroFloatYieldsNil(t *testing.T) {
	claims := AuthClaims{ClaimFieldExpirationTime: float64(0)}
	nd, err := claims.GetExpirationTime()
	require.NoError(t, err)
	assert.Nil(t, nd, "a zero float64 expiration should produce a nil NumericDate")
}

func TestAuthClaims_NumericDate_InvalidType(t *testing.T) {
	claims := AuthClaims{ClaimFieldExpirationTime: "not-a-number"}
	nd, err := claims.GetExpirationTime()
	assert.Nil(t, nd)
	assert.ErrorIs(t, err, ErrorInvalidType)
}

// ---------------------------------------------------------------------------
// GetString / GetClaimStrings / GetStrings
// ---------------------------------------------------------------------------

func TestAuthClaims_GetString(t *testing.T) {
	tests := []struct {
		name    string
		claims  AuthClaims
		key     string
		want    string
		wantErr bool
	}{
		{name: "present string", claims: AuthClaims{"k": "v"}, key: "k", want: "v"},
		{name: "missing key", claims: AuthClaims{}, key: "k", want: ""},
		{name: "wrong type", claims: AuthClaims{"k": 42}, key: "k", want: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.claims.GetString(tt.key)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrorInvalidType)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAuthClaims_GetStrings(t *testing.T) {
	tests := []struct {
		name    string
		claims  AuthClaims
		key     string
		want    []string
		wantErr bool
	}{
		{name: "single string", claims: AuthClaims{"k": "v"}, key: "k", want: []string{"v"}},
		{name: "string slice", claims: AuthClaims{"k": []string{"a", "b"}}, key: "k", want: []string{"a", "b"}},
		{
			name:   "interface slice",
			claims: AuthClaims{"k": []interface{}{"a", "b"}},
			key:    "k",
			want:   []string{"a", "b"},
		},
		{name: "missing key", claims: AuthClaims{}, key: "k", want: nil},
		{
			name:    "interface slice with non-string",
			claims:  AuthClaims{"k": []interface{}{"a", 1}},
			key:     "k",
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.claims.GetStrings(tt.key)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrorInvalidType)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAuthClaims_GetClaimStrings(t *testing.T) {
	tests := []struct {
		name    string
		claims  AuthClaims
		key     string
		want    []string
		wantErr bool
	}{
		{name: "single string", claims: AuthClaims{"k": "v"}, key: "k", want: []string{"v"}},
		{name: "string slice", claims: AuthClaims{"k": []string{"a", "b"}}, key: "k", want: []string{"a", "b"}},
		{
			name:   "interface slice",
			claims: AuthClaims{"k": []interface{}{"a", "b"}},
			key:    "k",
			want:   []string{"a", "b"},
		},
		{name: "missing key", claims: AuthClaims{}, key: "k", want: nil},
		{
			name:    "interface slice with non-string",
			claims:  AuthClaims{"k": []interface{}{true}},
			key:     "k",
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.claims.GetClaimStrings(tt.key)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrorInvalidType)
			} else {
				assert.NoError(t, err)
			}
			if tt.want == nil {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, tt.want, []string(got))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Typed number getters: present / missing / wrong-type, for every type.
// ---------------------------------------------------------------------------

func TestAuthClaims_GetNumberTypes(t *testing.T) {
	claims := AuthClaims{
		"i":   int(42),
		"i8":  int8(8),
		"i16": int16(16),
		"i32": int32(32),
		"i64": int64(64),
		"u":   uint(1),
		"u8":  uint8(2),
		"u16": uint16(3),
		"u32": uint32(4),
		"u64": uint64(5),
		"f32": float32(1.5),
		"f64": float64(2.5),
		"jn":  json.Number("7"),
	}

	t.Run("int getters dispatch", func(t *testing.T) {
		v, err := claims.GetInt("i")
		require.NoError(t, err)
		assert.Equal(t, 42, v)

		v8, err := claims.GetInt8("i8")
		require.NoError(t, err)
		assert.Equal(t, int8(8), v8)

		v16, err := claims.GetInt16("i16")
		require.NoError(t, err)
		assert.Equal(t, int16(16), v16)

		v32, err := claims.GetInt32("i32")
		require.NoError(t, err)
		assert.Equal(t, int32(32), v32)

		v64, err := claims.GetInt64("i64")
		require.NoError(t, err)
		assert.Equal(t, int64(64), v64)
	})
	t.Run("uint getters dispatch", func(t *testing.T) {
		v, err := claims.GetUint("u")
		require.NoError(t, err)
		assert.Equal(t, uint(1), v)

		v8, err := claims.GetUint8("u8")
		require.NoError(t, err)
		assert.Equal(t, uint8(2), v8)

		v16, err := claims.GetUint16("u16")
		require.NoError(t, err)
		assert.Equal(t, uint16(3), v16)

		v32, err := claims.GetUint32("u32")
		require.NoError(t, err)
		assert.Equal(t, uint32(4), v32)

		v64, err := claims.GetUint64("u64")
		require.NoError(t, err)
		assert.Equal(t, uint64(5), v64)
	})
	t.Run("floats", func(t *testing.T) {
		got32, err := claims.GetFloat32("f32")
		require.NoError(t, err)
		assert.Equal(t, float32(1.5), got32)

		got64, err := claims.GetFloat64("f64")
		require.NoError(t, err)
		assert.Equal(t, float64(2.5), got64)
	})
	t.Run("cross-type conversions via parseNumber", func(t *testing.T) {
		// A single int value can be read through every numeric type.
		got, err := parseNumber[int8](claims["i"])
		require.NoError(t, err)
		assert.Equal(t, int8(42), got)

		gotU, err := parseNumber[uint64](claims["i"])
		require.NoError(t, err)
		assert.Equal(t, uint64(42), gotU)

		gotF, err := parseNumber[float32](claims["i"])
		require.NoError(t, err)
		assert.Equal(t, float32(42), gotF)
	})
	t.Run("json.Number converts", func(t *testing.T) {
		v, err := claims.GetInt("jn")
		require.NoError(t, err)
		assert.Equal(t, 7, v)

		f, err := claims.GetFloat64("jn")
		require.NoError(t, err)
		assert.Equal(t, 7.0, f)

		bad := json.Number("not-a-number")
		_, err = parseNumber[float64](bad)
		assert.Error(t, err, "malformed json.Number should surface the strconv error")
	})
	t.Run("missing keys return zero value and no error", func(t *testing.T) {
		missing := AuthClaims{}

		n, err := missing.GetInt("nope")
		require.NoError(t, err)
		assert.Zero(t, n)

		f, err := missing.GetFloat64("nope")
		require.NoError(t, err)
		assert.Zero(t, f)

		u, err := missing.GetUint64("nope")
		require.NoError(t, err)
		assert.Zero(t, u)
	})
	t.Run("wrong type returns error", func(t *testing.T) {
		bad := AuthClaims{"k": "string-value"}

		_, err := bad.GetInt("k")
		assert.ErrorIs(t, err, ErrorInvalidType)

		_, err = bad.GetFloat64("k")
		assert.ErrorIs(t, err, ErrorInvalidType)
	})
}

// ---------------------------------------------------------------------------
// Context propagation
// ---------------------------------------------------------------------------

func TestContextWithAuthClaims(t *testing.T) {
	claims := AuthClaims{ClaimFieldSubject: "user-42"}
	ctx := ContextWithAuthClaims(context.Background(), &claims)

	got, ok := AuthClaimsFromContext(ctx)
	require.True(t, ok, "claims should be retrievable from the context")
	require.NotNil(t, got)
	assert.Equal(t, "user-42", (*got)[ClaimFieldSubject])
}

func TestAuthClaimsFromContext_Absent(t *testing.T) {
	got, ok := AuthClaimsFromContext(context.Background())
	assert.False(t, ok)
	assert.Nil(t, got)
}

func TestAuthClaimsFromContext_WrongValueType(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxKey("authn-claims"), "not claims")
	got, ok := AuthClaimsFromContext(ctx)
	assert.False(t, ok)
	assert.Nil(t, got)
}

// ---------------------------------------------------------------------------
// gRPC metadata helpers
// ---------------------------------------------------------------------------

func TestFormatToken(t *testing.T) {
	assert.Equal(t, "Bearer abc", formatToken(BearerWord, "abc"))
	assert.Equal(t, "Basic abc", formatToken(BasicWord, "abc"))
	assert.Equal(t, "Digest abc", formatToken(DigestWord, "abc"))
}

func TestAuthFromMD(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		scheme     string
		wantToken  string
		wantErr    bool
	}{
		{
			name:       "bearer token round trip",
			authHeader: "Bearer tok-1",
			scheme:     BearerWord,
			wantToken:  "tok-1",
		},
		{
			name:       "scheme matching is case insensitive",
			authHeader: "bearer tok-2",
			scheme:     BearerWord,
			wantToken:  "tok-2",
		},
		{
			name:       "basic scheme",
			authHeader: "Basic dXNlcjpwYXNz",
			scheme:     BasicWord,
			wantToken:  "dXNlcjpwYXNz",
		},
		{
			name:       "token may contain spaces",
			authHeader: "Bearer a b c",
			scheme:     BearerWord,
			wantToken:  "a b c",
		},
		{
			name:       "missing header",
			authHeader: "",
			scheme:     BearerWord,
			wantErr:    true,
		},
		{
			name:       "header without separator",
			authHeader: "BearerNoSpace",
			scheme:     BearerWord,
			wantErr:    true,
		},
		{
			name:       "scheme mismatch",
			authHeader: "Basic dXNlcjpwYXNz",
			scheme:     BearerWord,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.authHeader != "" {
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(HeaderAuthorize, tt.authHeader))
			}

			tok, err := AuthFromMD(ctx, tt.scheme)
			if tt.wantErr {
				require.Error(t, err)
				var e *errs.Error
				assert.ErrorAs(t, err, &e)
				assert.Equal(t, errs.StatusUnauthorized, e.Code)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantToken, tok)
		})
	}
}

// TestMDWithAuth_AttachesOutgoingMetadata asserts that MDWithAuth stores the
// Authorization header back into the context with ToOutgoing(ctx); without it
// grpc's FromOutgoingContext hands out a copy and the value is discarded.
func TestMDWithAuth_AttachesOutgoingMetadata(t *testing.T) {
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.MD{})
	ctx = MDWithAuth(ctx, BearerWord, "tok-9")

	md, ok := metadata.FromOutgoingContext(ctx)
	require.True(t, ok)
	vals, present := md[strings.ToLower(HeaderAuthorize)]
	require.True(t, present, "MDWithAuth must attach the Authorization header to the outgoing metadata")
	require.Len(t, vals, 1)
	assert.Equal(t, "Bearer tok-9", vals[0])
}

// TestMDWithAuth_IdempotentOnRepeatedCalls asserts that repeated MDWithAuth
// calls do not stack duplicate header entries nor clobber other metadata.
func TestMDWithAuth_IdempotentOnRepeatedCalls(t *testing.T) {
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-other", "keep"))
	ctx = MDWithAuth(ctx, BearerWord, "tok-9")
	ctx = MDWithAuth(ctx, BearerWord, "tok-9")

	md, ok := metadata.FromOutgoingContext(ctx)
	require.True(t, ok)

	vals, present := md[strings.ToLower(HeaderAuthorize)]
	require.True(t, present)
	assert.Equal(t, []string{"Bearer tok-9"}, vals, "repeated calls should not stack duplicate header values")
	assert.Equal(t, []string{"keep"}, md.Get("x-other"), "unrelated metadata must survive")
}

// ---------------------------------------------------------------------------
// Authenticator interface wiring with an in-test fake
// ---------------------------------------------------------------------------

type fakeAuthenticator struct {
	authenticateClaims *AuthClaims
	authenticateErr    error
	tokenClaims        *AuthClaims
	tokenErr           error
	createdToken       string
	createErr          error
	identCtx           context.Context
	createCtxErr       error
	closed             bool
}

func (f *fakeAuthenticator) Authenticate(_ context.Context) (*AuthClaims, error) {
	return f.authenticateClaims, f.authenticateErr
}

func (f *fakeAuthenticator) AuthenticateToken(_ string) (*AuthClaims, error) {
	return f.tokenClaims, f.tokenErr
}

func (f *fakeAuthenticator) CreateIdentityWithContext(ctx context.Context, claims AuthClaims) (context.Context, error) {
	if f.createCtxErr != nil {
		return nil, f.createCtxErr
	}
	f.identCtx = ContextWithAuthClaims(ctx, &claims)
	return f.identCtx, nil
}

func (f *fakeAuthenticator) CreateIdentity(claims AuthClaims) (string, error) {
	return f.createdToken, f.createErr
}

func (f *fakeAuthenticator) Close() {
	f.closed = true
}

// Compile-time assertion: the fake satisfies the Authenticator contract.
var _ Authenticator = (*fakeAuthenticator)(nil)

func TestAuthenticatorInterface_Fake(t *testing.T) {
	claims := AuthClaims{ClaimFieldSubject: "u1"}
	fake := &fakeAuthenticator{
		authenticateClaims: &claims,
		tokenClaims:        &claims,
		createdToken:       "signed-token",
	}

	var auth Authenticator = fake

	got, err := auth.Authenticate(context.Background())
	require.NoError(t, err)
	assert.Equal(t, &claims, got)

	got, err = auth.AuthenticateToken("raw")
	require.NoError(t, err)
	assert.Equal(t, &claims, got)

	tok, err := auth.CreateIdentity(claims)
	require.NoError(t, err)
	assert.Equal(t, "signed-token", tok)

	ctx, err := auth.CreateIdentityWithContext(context.Background(), claims)
	require.NoError(t, err)
	outClaims, ok := AuthClaimsFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, "u1", (*outClaims)[ClaimFieldSubject])

	auth.Close()
	assert.True(t, fake.closed)
}

// ---------------------------------------------------------------------------
// Error sentinels
// ---------------------------------------------------------------------------

func TestErrorSentinels(t *testing.T) {
	// The unauthorized family must be 401s; the infrastructure ones 500s.
	unauthorized := []*errs.Error{
		ErrInvalidJwtID, ErrMissingJwtId, ErrInvalidSubject, ErrInvalidAudience,
		ErrInvalidIssuer, ErrInvalidExpiration, ErrInvalidNotBefore, ErrInvalidIssuedAt,
		ErrInvalidClaims, ErrInvalidToken, ErrMissingBearerToken, ErrUnauthenticated,
		ErrTokenExpired, ErrNoAtHash, ErrInvalidAtHash,
	}
	for _, e := range unauthorized {
		assert.Equal(t, errs.StatusUnauthorized, e.Code, "%s should be unauthorized", e.Reason)
	}

	internal := []*errs.Error{
		ErrorInvalidType, ErrUnsupportedSigningMethod, ErrMissingKeyFunc,
		ErrSignTokenFailed, ErrGetKeyFailed,
	}
	for _, e := range internal {
		assert.Equal(t, errs.StatusInternalServerError, e.Code, "%s should be internal", e.Reason)
	}

	assert.Equal(t, "Bearer", BearerWord)
	assert.Equal(t, "Basic", BasicWord)
	assert.Equal(t, "Digest", DigestWord)
	assert.Equal(t, "Authorization", HeaderAuthorize)
}

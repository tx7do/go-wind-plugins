package openfga

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/openfga/go-sdk/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The ClientOption functions are tested below. NewClient is exercised
// hermetically: it must construct without any network I/O, and store
// discovery must run lazily on the first API call. The live-server surface
// remains covered by the KRATOS_IT-gated tests in openfga_test.go.

func TestWithApiUrl(t *testing.T) {
	c := &Client{}
	WithApiUrl("http://127.0.0.1:8080")(c)
	assert.Equal(t, "http://127.0.0.1:8080", c.apiUrl)
}

func TestWithStoreId(t *testing.T) {
	c := &Client{}
	WithStoreId("store-42")(c)
	assert.Equal(t, "store-42", c.storeId)
}

// TestWithToken asserts the guard: an empty token is a no-op, a real token
// installs API-token credentials.
func TestWithToken(t *testing.T) {
	t.Run("empty token is a no-op", func(t *testing.T) {
		c := &Client{credentials: credentials.Credentials{}}
		WithToken("")(c)
		assert.NotEqual(t, credentials.CredentialsMethodApiToken, c.credentials.Method,
			"an empty token must not install API-token credentials")
		assert.Nil(t, c.credentials.Config)
	})

	t.Run("non-empty token installs credentials", func(t *testing.T) {
		c := &Client{credentials: credentials.Credentials{}}
		WithToken("real-token")(c)
		assert.Equal(t, credentials.CredentialsMethodApiToken, c.credentials.Method)
		require.NotNil(t, c.credentials.Config)
		assert.Equal(t, "real-token", c.credentials.Config.ApiToken)
	})
}

func TestWithClientId(t *testing.T) {
	c := &Client{credentials: credentials.Credentials{}}
	WithClientId("client-id", "client-secret", "api-audience", "https://issuer.example.com")(c)

	assert.Equal(t, credentials.CredentialsMethodClientCredentials, c.credentials.Method)
	require.NotNil(t, c.credentials.Config)
	assert.Equal(t, "client-id", c.credentials.Config.ClientCredentialsClientId)
	assert.Equal(t, "client-secret", c.credentials.Config.ClientCredentialsClientSecret)
	assert.Equal(t, "api-audience", c.credentials.Config.ClientCredentialsApiAudience)
	assert.Equal(t, "https://issuer.example.com", c.credentials.Config.ClientCredentialsApiTokenIssuer)
}

// fakeOpenFga is a minimal OpenFga REST server used to prove that the
// constructor does not dial and that the first API call performs store
// discovery exactly as before.
type fakeOpenFga struct {
	mu       sync.Mutex
	requests int
	creates  int
}

func (f *fakeOpenFga) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests++
		f.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/stores":
			_, _ = w.Write([]byte(`{"stores": [], "continuation_token": ""}`))
		case r.Method == http.MethodPost && r.URL.Path == "/stores":
			f.mu.Lock()
			f.creates++
			f.mu.Unlock()
			_, _ = w.Write([]byte(`{"id": "01GRC27AM72M4SGK4VBHF3DY0F", "name": "fake"}`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}
}

// NewClient must construct without touching the network; store discovery
// (ListStores + CreateStore) is deferred to the first API call.
func TestNewClientDoesNotDialAndLazilyEnsuresStore(t *testing.T) {
	fake := &fakeOpenFga{}
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)

	cli := NewClient(WithApiUrl(srv.URL))
	require.NotNil(t, cli)
	require.NotNil(t, cli.fgaClient, "expected the SDK client to be created eagerly (no network involved)")
	assert.Empty(t, cli.storeId, "expected no store id before any API call")

	fake.mu.Lock()
	requests := fake.requests
	fake.mu.Unlock()
	assert.Zero(t, requests, "NewClient must not perform network I/O")

	// The first real call must ensure the store before proceeding.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stores, err := cli.ListStore(ctx)
	require.NoError(t, err)
	require.NotNil(t, stores)
	assert.Empty(t, *stores, "expected the fake server to report no existing stores")

	sdkStoreId, err := cli.fgaClient.GetStoreId()
	require.NoError(t, err)
	assert.Equal(t, "01GRC27AM72M4SGK4VBHF3DY0F", sdkStoreId, "expected store discovery to set the store id")

	fake.mu.Lock()
	defer fake.mu.Unlock()
	assert.Equal(t, 3, fake.requests, "expected discovery (ListStores + CreateStore) plus the actual ListStore call")
	assert.Equal(t, 1, fake.creates, "expected an empty server to trigger store creation")
}

// NewClient against an unreachable endpoint must still construct; the
// connection error must surface on the first real API call.
func TestNewClientUnreachableEndpointSurfacesErrorOnFirstCall(t *testing.T) {
	cli := NewClient(WithApiUrl("http://127.0.0.1:1"))
	require.NotNil(t, cli)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stores, err := cli.ListStore(ctx)
	assert.Error(t, err, "expected the first call to surface the connection error")
	assert.Nil(t, stores)
}

package openfga

import (
	"testing"

	"github.com/openfga/go-sdk/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Only the pure ClientOption functions are tested here. NewClient is
// intentionally NOT exercised: its init calls ensureStore, which performs a
// live ListStores HTTP request — that surface is covered by the KRATOS_IT-
// gated tests in openfga_test.go.

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

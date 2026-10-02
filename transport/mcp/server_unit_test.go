package mcp

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Construction, options, and endpoint derivation
// ---------------------------------------------------------------------------

func TestNewServerDefaults(t *testing.T) {
	srv := NewServer()

	assert.Equal(t, KindMCP, srv.Name())
	assert.Equal(t, DefaultMCPServerName, srv.serverName)
	assert.Equal(t, DefaultMCPServerVersion, srv.serverVersion)
	assert.Equal(t, ServerTypeStdio, srv.serverType)
	assert.Equal(t, DefaultMCPServerAddress, srv.serverAddr)
	assert.NotNil(t, srv.mcpServer, "mcpServer must be created during init")
	assert.NotNil(t, srv.endpoint, "endpoint must be derived during init")
	assert.Equal(t, "http://localhost:8080", srv.Endpoint())
	assert.Empty(t, srv.mcpOpts)
}

func TestServerOptions(t *testing.T) {
	srv := NewServer(
		WithServerName("unit-test"),
		WithServerVersion("9.9.9"),
		WithMCPServeType(ServerTypeHTTP),
		WithMCPServeAddress("127.0.0.1:9999"),
		WithMCPServerOptions(server.WithToolCapabilities(false)),
	)

	assert.Equal(t, "unit-test", srv.serverName)
	assert.Equal(t, "9.9.9", srv.serverVersion)
	assert.Equal(t, ServerTypeHTTP, srv.serverType)
	assert.Equal(t, "127.0.0.1:9999", srv.serverAddr)
	assert.Equal(t, "http://127.0.0.1:9999", srv.Endpoint())
	assert.Len(t, srv.mcpOpts, 1)
}

// TestEndpointDerivation covers the address-to-endpoint mapping rules.
func TestEndpointDerivation(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want string
	}{
		{"port only", ":8080", "http://localhost:8080"},
		{"empty falls back to default", "", "http://localhost:8080"},
		{"explicit host", "example.com:443", "http://example.com:443"},
		{"loopback", "127.0.0.1:9090", "http://127.0.0.1:9090"},
		{"ipv6 loopback", "[::1]:9090", "http://[::1]:9090"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := NewServer(WithMCPServeAddress(tc.addr))
			assert.Equal(t, tc.want, srv.Endpoint())
		})
	}
}

// TestUnsupportedServerTypeFallsBackToStdio verifies the default branch of
// init's server type switch.
func TestUnsupportedServerTypeFallsBackToStdio(t *testing.T) {
	srv := NewServer(WithMCPServeType(ServerType("BOGUS")))
	assert.Equal(t, ServerTypeStdio, srv.serverType, "unknown type must fall back to STDIO")
}

// ---------------------------------------------------------------------------
// Tool registration
// ---------------------------------------------------------------------------

func noopHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText("ok"), nil
}

const validEchoToolJSON = `{
  "name": "echo",
  "description": "Echoes the input",
  "inputSchema": {
    "type": "object",
    "properties": {
      "message": { "type": "string", "description": "The message to echo" }
    },
    "required": ["message"]
  }
}`

func TestRegisterHandler(t *testing.T) {
	srv := NewServer()
	tool := mcp.NewTool("add", mcp.WithDescription("Adds nothing"))
	assert.NoError(t, srv.RegisterHandler(tool, noopHandler))
}

func TestRegisterHandlerNilServer(t *testing.T) {
	srv := &Server{}
	assert.EqualError(t, srv.RegisterHandler(mcp.NewTool("x"), noopHandler), "mcp server is nil")
	assert.EqualError(t, srv.RegisterHandlerWithJsonString("{}", noopHandler), "mcp server is nil")
	assert.EqualError(t, srv.RegisterHandlerWithJsonSchema("x", "d", "{}", noopHandler), "mcp server is nil")
}

func TestRegisterHandlerWithJsonString(t *testing.T) {
	srv := NewServer()

	assert.NoError(t, srv.RegisterHandlerWithJsonString(validEchoToolJSON, noopHandler))

	// Invalid JSON and invalid schemas must be rejected before registration.
	assert.Error(t, srv.RegisterHandlerWithJsonString("{not json", noopHandler))
	assert.Error(t, srv.RegisterHandlerWithJsonString(`{"name":"bad"}`, noopHandler), "missing InputSchema")
}

func TestRegisterHandlerWithJsonSchema(t *testing.T) {
	srv := NewServer()

	schema := `{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`
	assert.NoError(t, srv.RegisterHandlerWithJsonSchema("reverse", "Reverses text", schema, noopHandler))

	// A schema that is not valid JSON still builds a raw-schema tool; the
	// registration itself succeeds because validation happens client-side.
	assert.NoError(t, srv.RegisterHandlerWithJsonSchema("raw", "raw", "{nope", noopHandler))
}

// ---------------------------------------------------------------------------
// utils.go
// ---------------------------------------------------------------------------

func TestValidateToolInputSchema(t *testing.T) {
	tests := []struct {
		name    string
		schema  mcp.ToolInputSchema
		wantErr string
	}{
		{"empty type", mcp.ToolInputSchema{}, "type"},
		{"non-object type", mcp.ToolInputSchema{Type: "string"}, "object"},
		{"nil properties", mcp.ToolInputSchema{Type: "object"}, "properties"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateToolInputSchema(tc.schema)
			if assert.Error(t, err) {
				assert.Contains(t, err.Error(), tc.wantErr)
			}
		})
	}

	t.Run("valid schema", func(t *testing.T) {
		schema := mcp.ToolInputSchema{Type: "object", Properties: make(map[string]any)}
		assert.NoError(t, ValidateToolInputSchema(schema))
	})
}

func TestLoadToolFromJsonString(t *testing.T) {
	tool, err := LoadToolFromJsonString(validEchoToolJSON)
	require.NoError(t, err)
	assert.Equal(t, "echo", tool.Name)
	assert.Equal(t, "object", tool.InputSchema.Type)

	_, err = LoadToolFromJsonString("{broken")
	assert.Error(t, err, "invalid JSON must fail")

	_, err = LoadToolFromJsonString(`{"name":"x","inputSchema":{"type":"array"}}`)
	assert.Error(t, err, "non-object schema must fail")
}

func TestLoadToolFromJsonFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tool.json")
	require.NoError(t, os.WriteFile(path, []byte(validEchoToolJSON), 0o600))

	tool, err := LoadToolFromJsonFile(path)
	require.NoError(t, err)
	assert.Equal(t, "echo", tool.Name)

	_, err = LoadToolFromJsonFile(filepath.Join(dir, "missing.json"))
	assert.Error(t, err, "missing file must fail")

	bad := filepath.Join(dir, "broken.json")
	require.NoError(t, os.WriteFile(bad, []byte("{broken"), 0o600))
	_, err = LoadToolFromJsonFile(bad)
	assert.Error(t, err, "invalid JSON must fail")
}

func TestToRawMessage(t *testing.T) {
	raw := toRawMessage(`{"a":1}`)
	assert.Equal(t, `{"a":1}`, string(raw))
}

// ---------------------------------------------------------------------------
// Lifecycle guards (in-process server type, fully hermetic)
// ---------------------------------------------------------------------------

func TestStartStopInProcess(t *testing.T) {
	srv := NewServer(WithMCPServeType(ServerTypeInProcess))
	ctx := context.Background()

	require.NoError(t, srv.Start(ctx))
	// A second Start is a no-op warning, not an error.
	assert.NoError(t, srv.Start(ctx))
	assert.NoError(t, srv.Stop(ctx))
	// Stop is idempotent once started state is cleared.
	assert.NoError(t, srv.Stop(ctx))
}

func TestStopBeforeStart(t *testing.T) {
	srv := NewServer()
	assert.NoError(t, srv.Stop(context.Background()))
}

func TestEndpointNil(t *testing.T) {
	srv := &Server{}
	assert.Empty(t, srv.Endpoint())
}

func TestSetErrJoinsAndIgnoresNil(t *testing.T) {
	srv := &Server{}

	srv.setErr(nil)
	assert.NoError(t, srv.err)

	err1 := assert.AnError
	srv.setErr(err1)
	srv.setErr(err1)
	assert.Error(t, srv.err)
}

func TestWaitGroupReturns(t *testing.T) {
	srv := &Server{}
	ctx := context.Background()

	// Already-released WaitGroup returns immediately with nil.
	var done sync.WaitGroup
	done.Add(1)
	done.Done()
	assert.NoError(t, srv.waitGroup(&done, ctx))

	// A canceled context must surface as ctx.Err instead of blocking.
	var pending sync.WaitGroup
	pending.Add(1)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	assert.ErrorIs(t, srv.waitGroup(&pending, canceled), context.Canceled)
	// Unblock the goroutine so it does not leak past the test.
	pending.Done()
}

// TestStartStopDeadlineBounded keeps any accidental blocking of the
// lifecycle bounded: Stop must return even with a tight context deadline.
func TestStopWithExpiredDeadline(t *testing.T) {
	srv := NewServer(WithMCPServeType(ServerTypeInProcess))
	ctx := context.Background()
	require.NoError(t, srv.Start(ctx))

	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	assert.NoError(t, srv.Stop(stopCtx))
}

package trpc

import (
	"context"
	"testing"
	"time"

	trpcClient "trpc.group/trpc-go/trpc-go/client"
	"trpc.group/trpc-go/trpc-go/filter"
	trpcServer "trpc.group/trpc-go/trpc-go/server"
)

// Aliases matching the wrapper's types keep the signatures short.
// (ServerFilter/ClientFilter already exist in the package itself.)
type (
	ServerHandleFunc = filter.ServerHandleFunc
	ClientHandleFunc = filter.ClientHandleFunc
)

// ---------------------------------------------------------------------------
// Server options and helpers (no trpc_go.yaml required)
// ---------------------------------------------------------------------------

func TestNewServerDefaults(t *testing.T) {
	srv := NewServer()

	if srv.address != ":8972" {
		t.Errorf("default address = %q, want %q", srv.address, ":8972")
	}
	if got := srv.Name(); got != KindTRPC {
		t.Errorf("Name() = %q, want %q", got, KindTRPC)
	}
	if got := srv.Endpoint(); got != srv.address {
		t.Errorf("Endpoint() = %q, want %q", got, srv.address)
	}
	if len(srv.trpcOptions) != 0 {
		t.Errorf("trpcOptions = %d, want 0 before any option", len(srv.trpcOptions))
	}
	if len(srv.filters) != 0 {
		t.Errorf("filters = %d, want 0 before any filter", len(srv.filters))
	}
	if srv.baseCtx == nil {
		t.Error("baseCtx is nil after NewServer")
	}
}

func TestServerOptions(t *testing.T) {
	srv := NewServer(
		WithNamespace("Development"),
		WithAddress("127.0.0.1:9999"),
		WithEnvName("test"),
		WithContainer("c1"),
		WithSetName("s1"),
		WithServiceName("svc"),
		WithNetwork("tcp"),
		WithProtocol("trpc"),
		WithTimeout(3*time.Second),
		WithTLS("", "", ""),
		WithIdleTimeout(time.Minute),
		WithMaxRoutines(8),
		WithCloseWaitTime(5*time.Second),
		WithDisableRequestTimeout(true),
		WithNamedFilter("named", ServerFilter(func(context.Context, interface{}, ServerHandleFunc) (interface{}, error) {
			return nil, nil
		})),
		WithTrpcOptions(trpcServer.WithTimeout(time.Second)),
	)

	if srv.address != "127.0.0.1:9999" {
		t.Errorf("address = %q, want %q", srv.address, "127.0.0.1:9999")
	}
	if got := srv.Endpoint(); got != "127.0.0.1:9999" {
		t.Errorf("Endpoint() = %q, want %q", got, "127.0.0.1:9999")
	}
	// Each With* appends exactly one underlying option.
	if len(srv.trpcOptions) != 16 {
		t.Errorf("trpcOptions = %d, want 16", len(srv.trpcOptions))
	}
}

func TestServerFilters(t *testing.T) {
	filter := ServerFilter(func(context.Context, interface{}, ServerHandleFunc) (interface{}, error) {
		return nil, nil
	})

	srv := NewServer()
	srv.Use(filter)
	WithFilter(filter)(srv)
	WithFilters([]ServerFilter{filter, filter})(srv)

	if len(srv.filters) != 4 {
		t.Errorf("filters = %d, want 4 after Use/WithFilter/WithFilters", len(srv.filters))
	}
}

// TestWithServerTLSConfigIsNoOp pins the documented no-op behavior: tRPC only
// supports certificate files via WithTLS, so a *tls.Config is ignored.
func TestWithServerTLSConfigIsNoOp(t *testing.T) {
	srv := NewServer(WithServerTLSConfig(nil))
	if len(srv.trpcOptions) != 0 {
		t.Errorf("WithServerTLSConfig appended %d options, want 0", len(srv.trpcOptions))
	}
}

func TestStopBeforeStart(t *testing.T) {
	srv := NewServer()
	if err := srv.Stop(context.Background()); err != nil {
		t.Errorf("Stop before Start returned error: %v", err)
	}
}

// TestRegisterServiceAfterStartRejected verifies the guard without touching
// the tRPC runtime: once started, registration must fail fast.
func TestRegisterServiceAfterStartRejected(t *testing.T) {
	srv := NewServer()
	srv.started.Store(true)

	err := srv.RegisterService("trpc.test.Service", nil, nil)
	if err == nil {
		t.Fatal("RegisterService after Start returned nil error")
	}
	if got := err.Error(); got != `cannot register service "trpc.test.Service" after server started` {
		t.Errorf("error = %q, want the after-start message", got)
	}
}

// ---------------------------------------------------------------------------
// Client options and helpers
// ---------------------------------------------------------------------------

func TestNewClient(t *testing.T) {
	c := NewClient()
	if c.Client == nil {
		t.Fatal("Client is nil after NewClient")
	}
	if got := c.timeout(); got != 0 {
		t.Errorf("timeout() = %v, want 0 (framework controls timeouts)", got)
	}
	if len(c.clientOptions) != 0 {
		t.Errorf("clientOptions = %d, want 0 before any option", len(c.clientOptions))
	}
}

func TestClientOptions(t *testing.T) {
	c := NewClient(
		WithTarget("127.0.0.1:9999"),
		WithClientServiceName("svc"),
		WithClientNamespace("Development"),
		WithClientEnvName("test"),
		WithClientSetName("s1"),
		WithClientTimeout(2*time.Second),
		WithClientNetwork("tcp"),
		WithClientProtocol("trpc"),
		WithClientTLS("", "", "", ""),
		WithMetaData("k", []byte("v")),
		WithDiscoveryName("disc"),
		WithBalancerName("bal"),
		WithCalleeMethod("method"),
		WithClientOptions(trpcClient.WithTimeout(time.Second)),
	)

	if len(c.clientOptions) != 14 {
		t.Errorf("clientOptions = %d, want 14", len(c.clientOptions))
	}
}

func TestClientFilters(t *testing.T) {
	filter := ClientFilter(func(context.Context, interface{}, interface{}, ClientHandleFunc) error {
		return nil
	})

	c := NewClient(WithClientFilter(filter))
	WithClientFilters([]ClientFilter{filter, filter})(c)
	c.Use(filter)

	if len(c.filters) != 4 {
		t.Errorf("filters = %d, want 4 after WithClientFilter/WithClientFilters/Use", len(c.filters))
	}
}

// TestWithClientTLSConfigIsNoOp pins the documented no-op behavior.
func TestWithClientTLSConfigIsNoOp(t *testing.T) {
	c := NewClient(WithClientTLSConfig(nil))
	if len(c.clientOptions) != 0 {
		t.Errorf("WithClientTLSConfig appended %d options, want 0", len(c.clientOptions))
	}
}

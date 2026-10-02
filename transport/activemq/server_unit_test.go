package activemq

import (
	"context"
	"crypto/tls"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	api "github.com/tx7do/go-wind-plugins/testing/api/manual"

	"github.com/tx7do/go-wind-plugins/broker"
)

func TestKind(t *testing.T) {
	assert.Equal(t, "activemq", KindActiveMQ)
}

func TestNewServer(t *testing.T) {
	srv := NewServer(
		WithAddress([]string{"127.0.0.1"}), WithCodec("json"),
	)
	assert.NotNil(t, srv)
	assert.Equal(t, "activemq", srv.Name())
	assert.False(t, srv.started.Load())
}

func TestEndpoint(t *testing.T) {
	srv := NewServer(
		WithAddress([]string{"127.0.0.1"}), WithCodec("json"),
	)
	assert.Equal(t, "", srv.Endpoint())
}

func TestStopBeforeStart(t *testing.T) {
	srv := NewServer(
		WithAddress([]string{"127.0.0.1"}), WithCodec("json"),
	)
	err := srv.Stop(context.Background())
	assert.Nil(t, err)
}

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

func TestServerOptionsAppendBrokerOptions(t *testing.T) {
	base := NewServer()
	baseCount := len(base.brokerOpts)

	srv := NewServer(
		WithBrokerOptions(broker.WithCodec("proto")),
		WithAddress([]string{"stomp://127.0.0.1:61613"}),
		WithCodec("json"),
		WithGlobalTracerProvider(),
		WithGlobalPropagator(),
		WithTracerProvider(otel.GetTracerProvider(), "svc"),
		WithPropagator(propagation.TraceContext{}),
	)

	assert.Greater(t, len(srv.brokerOpts), baseCount,
		"With* broker options must extend the broker option list")
}

func TestWithTLSConfig(t *testing.T) {
	// nil config: only the pass-through option is appended.
	nilSrv := NewServer(WithTLSConfig(nil))
	// non-nil config: an extra WithEnableSecure option is appended first.
	tlsSrv := NewServer(WithTLSConfig(&tls.Config{}))

	assert.Equal(t, 1, len(nilSrv.brokerOpts)-len(NewServer().brokerOpts))
	assert.Equal(t, 2, len(tlsSrv.brokerOpts)-len(NewServer().brokerOpts),
		"a non-nil TLS config must additionally append WithEnableSecure(true)")
}

func TestWithMetrics(t *testing.T) {
	m := &fakeMetrics{}
	srv := NewServer(WithMetrics(m))
	assert.Same(t, m, srv.m)
}

// ---------------------------------------------------------------------------
// Deferred subscriber registration (no broker connection required)
// ---------------------------------------------------------------------------

func TestRegisterSubscriberDefersUntilStart(t *testing.T) {
	srv := NewServer()

	handler := func(context.Context, broker.Event) error { return nil }
	binder := func() any { return &api.Hygrothermograph{} }

	assert.NoError(t, srv.RegisterSubscriber(context.Background(), testTopic, handler, binder))
	assert.NotNil(t, srv.subscriberOpts[testTopic],
		"registration before Start must be cached in subscriberOpts")
	assert.NotNil(t, srv.subscriberOpts[testTopic].Handler)
	assert.NotNil(t, srv.subscriberOpts[testTopic].Binder)
	assert.NotNil(t, srv.subscriberOpts[testTopic].Binder)
	assert.NotEmpty(t, srv.subscriberOpts[testTopic].SubscribeOptions,
		"a WithSubscribeContext option must be prepended")
}

// ---------------------------------------------------------------------------
// Generic typed subscriber wrapper
// ---------------------------------------------------------------------------

type fakeEvent struct {
	topic   string
	message *broker.Message
}

func (e *fakeEvent) Topic() string            { return e.topic }
func (e *fakeEvent) Message() *broker.Message { return e.message }
func (e *fakeEvent) RawMessage() any          { return nil }
func (e *fakeEvent) Ack() error               { return nil }
func (e *fakeEvent) Error() error             { return nil }

func TestRegisterSubscriberTypedDispatch(t *testing.T) {
	ctx := context.Background()

	called := make(chan *api.Hygrothermograph, 2)
	handler := func(_ context.Context, topic string, headers broker.Headers, msg *api.Hygrothermograph) error {
		assert.Equal(t, testTopic, topic)
		called <- msg
		return nil
	}

	require.NoError(t, RegisterSubscriber(srvUnused(), ctx, testTopic, handler))

	srv := NewServer()
	require.NoError(t, RegisterSubscriber(srv, ctx, testTopic, handler))

	typed := srv.subscriberOpts[testTopic]
	require.NotNil(t, typed)
	require.NotNil(t, typed.Handler)
	require.NotNil(t, typed.Binder)

	// *T body dispatches directly.
	want := &api.Hygrothermograph{Humidity: 1, Temperature: 2}
	assert.NoError(t, typed.Handler(ctx, &fakeEvent{topic: testTopic, message: &broker.Message{Body: want}}))
	select {
	case got := <-called:
		assert.Same(t, want, got)
	default:
		t.Fatal("*T handler not invoked")
	}

	// Plain T body also dispatches.
	assert.NoError(t, typed.Handler(ctx, &fakeEvent{topic: testTopic, message: &broker.Message{Body: api.Hygrothermograph{}}}))
	select {
	case <-called:
	default:
		t.Fatal("T handler not invoked")
	}

	// Unsupported bodies are rejected with a descriptive error.
	err := typed.Handler(ctx, &fakeEvent{topic: testTopic, message: &broker.Message{Body: "wrong"}})
	assert.ErrorContains(t, err, "unsupported type")

	// Nil events, messages, and bodies are rejected.
	assert.ErrorContains(t, typed.Handler(ctx, nil), "event or message body is nil")
	assert.ErrorContains(t, typed.Handler(ctx, &fakeEvent{message: nil}), "event or message body is nil")
	assert.ErrorContains(t, typed.Handler(ctx, &fakeEvent{message: &broker.Message{}}), "event or message body is nil")

	// Handler errors propagate unchanged through the typed wrapper.
	boom := errors.New("boom")
	failingSrv := NewServer()
	require.NoError(t, RegisterSubscriber(failingSrv, ctx, testTopic,
		func(context.Context, string, broker.Headers, *api.Hygrothermograph) error { return boom }))
	failing := failingSrv.subscriberOpts[testTopic]
	require.NotNil(t, failing)
	assert.ErrorIs(t, failing.Handler(ctx, &fakeEvent{
		topic:   testTopic,
		message: &broker.Message{Body: want},
	}), boom)
}

func srvUnused() *Server { return NewServer() }

// ---------------------------------------------------------------------------
// wrapHandler metrics path
// ---------------------------------------------------------------------------

type fakeMetrics struct {
	counters   map[string]float64
	histograms []string
}

func (m *fakeMetrics) Counter(_ context.Context, name string, value float64, _ map[string]string) {
	if m.counters == nil {
		m.counters = make(map[string]float64)
	}
	m.counters[name] += value
}

func (m *fakeMetrics) Histogram(_ context.Context, name string, _ float64, _ map[string]string) {
	m.histograms = append(m.histograms, name)
}

func (m *fakeMetrics) Gauge(_ context.Context, _ string, _ float64, _ map[string]string) {}

func TestWrapHandlerWithoutMetricsIsIdentity(t *testing.T) {
	srv := NewServer()
	inner := func(context.Context, broker.Event) error { return nil }

	// With no metrics injected the handler is passed through unchanged.
	assert.NotNil(t, srv.wrapHandler(testTopic, inner))
}

func TestWrapHandlerWithMetrics(t *testing.T) {
	m := &fakeMetrics{}
	srv := NewServer(WithMetrics(m))

	innerCalled := false
	inner := func(context.Context, broker.Event) error {
		innerCalled = true
		return nil
	}
	wrapped := srv.wrapHandler(testTopic, inner)

	assert.NoError(t, wrapped(context.Background(), &fakeEvent{
		topic:   testTopic,
		message: &broker.Message{Body: &api.Hygrothermograph{}},
	}))
	assert.True(t, innerCalled)
	assert.Equal(t, 1.0, m.counters["broker.messages.received"])
	assert.NotEmpty(t, m.histograms, "a duration sample must be recorded")
	assert.NotContains(t, m.counters, "broker.messages.errors")

	// Handler errors are counted on the error counter and still propagate.
	boom := errors.New("boom")
	failing := srv.wrapHandler(testTopic, func(context.Context, broker.Event) error { return boom })
	assert.ErrorIs(t, failing(context.Background(), &fakeEvent{
		topic:   testTopic,
		message: &broker.Message{Body: &api.Hygrothermograph{}},
	}), boom)
	assert.Equal(t, 1.0, m.counters["broker.messages.errors"])
}

package pulsar

import (
	"context"
	"crypto/tls"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"

	"github.com/tx7do/go-wind/log"

	"github.com/tx7do/go-wind-plugins/broker"
	"github.com/tx7do/go-wind-plugins/metrics"
	"github.com/tx7do/go-wind-plugins/transport"
)

///////////////////////////////////////////////////////////////////////////////
// Test fakes: no network, every call is recorded so the server lifecycle can
// be exercised hermetically by swapping the embedded broker out.
///////////////////////////////////////////////////////////////////////////////

// fakeSubscriber is a broker.Subscriber fake that counts unsubscribe calls.
type fakeSubscriber struct {
	topic        string
	opts         broker.SubscribeOptions
	unsubscribed int
}

func (s *fakeSubscriber) Options() broker.SubscribeOptions { return s.opts }
func (s *fakeSubscriber) Topic() string                    { return s.topic }
func (s *fakeSubscriber) Unsubscribe(bool) error {
	s.unsubscribed++
	return nil
}

// fakeBroker is a broker.Broker fake. Init/Connect/Disconnect/Subscribe are
// recorded; Publish/Request fail fast so tests never touch the network.
type fakeBroker struct {
	mu            sync.Mutex
	initErr       error
	connectErr    error
	disconnectErr error
	subscribeErr  error

	inits       int
	connects    int
	disconnects int
	published   []*broker.Message
	subs        map[string]*fakeSubscriber
}

func newFakeBroker() *fakeBroker {
	return &fakeBroker{subs: make(map[string]*fakeSubscriber)}
}

func (b *fakeBroker) Name() string            { return "fake" }
func (b *fakeBroker) Options() broker.Options { return broker.NewOptions() }
func (b *fakeBroker) Address() string         { return "fake://127.0.0.1:1" }

func (b *fakeBroker) Init(...broker.Option) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.inits++
	return b.initErr
}

func (b *fakeBroker) Connect() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.connects++
	return b.connectErr
}

func (b *fakeBroker) Disconnect() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.disconnects++
	return b.disconnectErr
}

func (b *fakeBroker) Publish(_ context.Context, _ string, msg *broker.Message, _ ...broker.PublishOption) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, msg)
	return nil
}

func (b *fakeBroker) Subscribe(topic string, _ broker.Handler, _ broker.Binder, opts ...broker.SubscribeOption) (broker.Subscriber, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subscribeErr != nil {
		return nil, b.subscribeErr
	}
	sub := &fakeSubscriber{topic: topic, opts: broker.NewSubscribeOptions(opts...)}
	b.subs[topic] = sub
	return sub, nil
}

func (b *fakeBroker) Request(_ context.Context, _ string, _ *broker.Message, _ ...broker.RequestOption) (*broker.Message, error) {
	return nil, errors.New("fakeBroker: Request not supported")
}

// fakeEvent is a broker.Event fake that counts acknowledgements.
type fakeEvent struct {
	topic  string
	msg    *broker.Message
	acked  int
	ackErr error
}

func (e *fakeEvent) Topic() string            { return e.topic }
func (e *fakeEvent) Message() *broker.Message { return e.msg }
func (e *fakeEvent) RawMessage() any          { return nil }
func (e *fakeEvent) Ack() error {
	e.acked++
	return e.ackErr
}
func (e *fakeEvent) Error() error { return nil }

// fakeMetrics is a metrics.Metrics fake that aggregates recorded samples.
type fakeMetrics struct {
	mu         sync.Mutex
	counters   map[string]float64
	histograms map[string]int
	gauges     map[string]float64
}

func newFakeMetrics() *fakeMetrics {
	return &fakeMetrics{
		counters:   make(map[string]float64),
		histograms: make(map[string]int),
		gauges:     make(map[string]float64),
	}
}

func (m *fakeMetrics) Counter(_ context.Context, name string, value float64, _ map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[name] += value
}

func (m *fakeMetrics) Histogram(_ context.Context, name string, _ float64, _ map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.histograms[name]++
}

func (m *fakeMetrics) Gauge(_ context.Context, name string, value float64, _ map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gauges[name] = value
}

// fakeLogger is a log.Logger fake that records messages per level.
type fakeLogger struct {
	mu      sync.Mutex
	records map[log.Level][]string
}

func newFakeLogger() *fakeLogger {
	return &fakeLogger{records: make(map[log.Level][]string)}
}

func (l *fakeLogger) record(level log.Level, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records[level] = append(l.records[level], msg)
}

func (l *fakeLogger) Debug(_ context.Context, msg string, _ ...any) { l.record(log.LevelDebug, msg) }
func (l *fakeLogger) Info(_ context.Context, msg string, _ ...any)  { l.record(log.LevelInfo, msg) }
func (l *fakeLogger) Warn(_ context.Context, msg string, _ ...any)  { l.record(log.LevelWarn, msg) }
func (l *fakeLogger) Error(_ context.Context, msg string, _ ...any) { l.record(log.LevelError, msg) }
func (l *fakeLogger) Enabled(log.Level) bool                        { return true }
func (l *fakeLogger) With(...any) log.Logger                        { return l }

// fakePropagator is a propagation.TextMapPropagator fake.
type fakePropagator struct{}

func (p *fakePropagator) Inject(context.Context, propagation.TextMapCarrier) {}
func (p *fakePropagator) Extract(context.Context, propagation.TextMapCarrier) context.Context {
	return context.Background()
}
func (p *fakePropagator) Fields() []string { return nil }

///////////////////////////////////////////////////////////////////////////////
// Interface compliance.
///////////////////////////////////////////////////////////////////////////////

var (
	_ broker.Broker     = (*fakeBroker)(nil)
	_ broker.Subscriber = (*fakeSubscriber)(nil)
	_ broker.Event      = (*fakeEvent)(nil)
	_ metrics.Metrics   = (*fakeMetrics)(nil)
	_ log.Logger        = (*fakeLogger)(nil)

	// Server embeds broker.Broker and therefore satisfies it.
	_ broker.Broker = (*Server)(nil)
)

func fakeNopHandler(_ context.Context, _ broker.Event) error { return nil }

///////////////////////////////////////////////////////////////////////////////
// Lifecycle with the fake broker.
///////////////////////////////////////////////////////////////////////////////

func TestServerLifecycleWithFakeBroker(t *testing.T) {
	srv := NewServer(WithAddress([]string{"pulsar://127.0.0.1:6650"}), WithCodec("json"))
	fb := newFakeBroker()
	srv.Broker = fb

	// Registering before start defers the subscriptions.
	require.NoError(t, srv.RegisterSubscriber(context.Background(), "topic.a", fakeNopHandler, nil))
	require.NoError(t, srv.RegisterSubscriber(context.Background(), "topic.b", fakeNopHandler, nil))
	assert.Len(t, srv.subscriberOpts, 2)
	assert.Empty(t, srv.subscribers)
	assert.Equal(t, 0, fb.connects)

	// Start connects and flushes the deferred subscriptions.
	require.NoError(t, srv.Start(context.Background()))
	assert.True(t, srv.started.Load())
	assert.Equal(t, 1, fb.inits)
	assert.Equal(t, 1, fb.connects)
	assert.Len(t, srv.subscribers, 2)
	assert.NotNil(t, srv.baseCtx)

	// Starting twice is a no-op.
	require.NoError(t, srv.Start(context.Background()))
	assert.Equal(t, 1, fb.connects)

	// Registering while started subscribes immediately.
	require.NoError(t, srv.RegisterSubscriber(context.Background(), "topic.c", fakeNopHandler, nil))
	assert.Len(t, srv.subscribers, 3)

	// Re-registering a topic replaces the old subscription.
	oldSub := srv.subscribers["topic.a"].(*fakeSubscriber)
	require.NoError(t, srv.RegisterSubscriber(context.Background(), "topic.a", fakeNopHandler, nil))
	assert.Len(t, srv.subscribers, 3)
	assert.Equal(t, 1, oldSub.unsubscribed)

	// Stop unsubscribes everything but keeps the registration parameters.
	require.NoError(t, srv.Stop(context.Background()))
	assert.False(t, srv.started.Load())
	assert.Equal(t, 1, fb.disconnects)
	assert.Empty(t, srv.subscribers)
	assert.Len(t, srv.subscriberOpts, 3)

	// Stop is idempotent while not started.
	require.NoError(t, srv.Stop(context.Background()))
	assert.Equal(t, 1, fb.disconnects)

	// Restart re-registers every cached subscription.
	require.NoError(t, srv.Start(context.Background()))
	assert.Len(t, srv.subscribers, 3)
	require.NoError(t, srv.Stop(context.Background()))
}

func TestServerStartErrorPaths(t *testing.T) {
	srv := NewServer()
	fb := newFakeBroker()
	srv.Broker = fb

	// Connect failure surfaces from Start and keeps the server stopped.
	fb.connectErr = errors.New("connect boom")
	err := srv.Start(context.Background())
	assert.ErrorIs(t, err, fb.connectErr)
	assert.False(t, srv.started.Load())

	// The failure is sticky: a repeated Start returns the same error
	// without touching the broker again.
	assert.Equal(t, err, srv.Start(context.Background()))
	assert.Equal(t, 1, fb.connects)

	// Stop after a failed Start clears the residual error state.
	require.NoError(t, srv.Stop(context.Background()))
	assert.NoError(t, srv.err)

	// Init failure surfaces as well.
	fb.initErr = errors.New("init boom")
	err = srv.Start(context.Background())
	assert.ErrorIs(t, err, fb.initErr)
	assert.False(t, srv.started.Load())
	require.NoError(t, srv.Stop(context.Background()))
}

func TestServerStartSubscribeError(t *testing.T) {
	srv := NewServer()
	fb := newFakeBroker()
	fb.subscribeErr = errors.New("subscribe boom")
	srv.Broker = fb

	require.NoError(t, srv.RegisterSubscriber(context.Background(), "topic.a", fakeNopHandler, nil))

	err := srv.Start(context.Background())
	assert.ErrorIs(t, err, fb.subscribeErr)
	// On subscribe failure Start unwinds via Stop: the server must not
	// stay half-started, and a retry must succeed.
	assert.False(t, srv.started.Load())

	// Retry with the broker healthy.
	fb.subscribeErr = nil
	require.NoError(t, srv.Start(context.Background()))
	assert.True(t, srv.started.Load())

	require.NoError(t, srv.Stop(context.Background()))
}

func TestServerPublishDelegatesToBroker(t *testing.T) {
	srv := NewServer()
	fb := newFakeBroker()
	srv.Broker = fb

	msg := broker.NewMessage("payload")
	require.NoError(t, srv.Publish(context.Background(), "topic.a", msg))
	assert.Len(t, fb.published, 1)
	assert.Equal(t, msg, fb.published[0])
}

///////////////////////////////////////////////////////////////////////////////
// Subscriber registration.
///////////////////////////////////////////////////////////////////////////////

func TestRegisterSubscriberDefersUntilStart(t *testing.T) {
	ctxKey := &struct{ key string }{}
	ctx := context.WithValue(context.Background(), ctxKey, "value")

	srv := NewServer()

	// The subscription context is recorded so it can be replayed on Start.
	require.NoError(t, srv.RegisterSubscriber(ctx, "topic.a", fakeNopHandler, nil,
		broker.WithSubscribeQueueName("queue.a")))
	opt := srv.subscriberOpts["topic.a"]
	require.NotNil(t, opt)
	require.NotNil(t, opt.Handler)

	// The context option is inserted at the head, so it wins over defaults
	// while the trailing options are preserved.
	opts := broker.NewSubscribeOptions(opt.SubscribeOptions...)
	assert.Equal(t, "queue.a", opts.Queue)
	assert.Equal(t, ctx, opts.Context)

	// A nil context falls back to the server base context.
	require.NoError(t, srv.RegisterSubscriber(nil, "topic.b", fakeNopHandler, nil))
	optsB := broker.NewSubscribeOptions(srv.subscriberOpts["topic.b"].SubscribeOptions...)
	assert.Equal(t, srv.baseCtx, optsB.Context)
}

func TestRegisterSubscriberTypedDispatch(t *testing.T) {
	type typedPayload struct {
		Value string
	}

	srv := NewServer()

	var (
		gotPayload *typedPayload
		gotTopic   string
		gotHeaders broker.Headers
	)
	require.NoError(t, RegisterSubscriber[typedPayload](srv, context.Background(), "topic.typed",
		func(_ context.Context, topic string, headers broker.Headers, msg *typedPayload) error {
			gotPayload, gotTopic, gotHeaders = msg, topic, headers
			return nil
		}))

	handler := srv.subscriberOpts["topic.typed"].Handler
	require.NotNil(t, handler)

	// Pointer body is passed through as-is.
	handler(context.Background(), &fakeEvent{
		topic: "topic.typed",
		msg:   &broker.Message{Headers: broker.Headers{"k": "v"}, Body: &typedPayload{Value: "ptr"}},
	})
	require.NotNil(t, gotPayload)
	assert.Equal(t, "ptr", gotPayload.Value)
	assert.Equal(t, "topic.typed", gotTopic)
	assert.Equal(t, broker.Headers{"k": "v"}, gotHeaders)

	// Value body is passed by pointer to a copy.
	gotPayload = nil
	handler(context.Background(), &fakeEvent{
		topic: "topic.typed",
		msg:   &broker.Message{Body: typedPayload{Value: "val"}},
	})
	require.NotNil(t, gotPayload)
	assert.Equal(t, "val", gotPayload.Value)

	// Unsupported body type is rejected.
	err := handler(context.Background(), &fakeEvent{
		topic: "topic.typed",
		msg:   &broker.Message{Body: 42},
	})
	assert.ErrorContains(t, err, "unsupported type")

	// Nil event, nil message, and nil body are all rejected.
	assert.Error(t, handler(context.Background(), nil))
	assert.Error(t, handler(context.Background(), &fakeEvent{topic: "topic.typed"}))
	assert.Error(t, handler(context.Background(), &fakeEvent{topic: "topic.typed", msg: &broker.Message{}}))

	// Handler errors propagate.
	innerErr := errors.New("handler boom")
	require.NoError(t, RegisterSubscriber[typedPayload](srv, context.Background(), "topic.err",
		func(_ context.Context, _ string, _ broker.Headers, _ *typedPayload) error {
			return innerErr
		}))
	err = srv.subscriberOpts["topic.err"].Handler(context.Background(), &fakeEvent{
		topic: "topic.err",
		msg:   &broker.Message{Body: &typedPayload{}},
	})
	assert.ErrorIs(t, err, innerErr)
}

func TestDoRegisterSubscriberDefersWhenStopped(t *testing.T) {
	srv := NewServer()
	fb := newFakeBroker()
	srv.Broker = fb
	assert.False(t, srv.started.Load())

	// Registering against a stopped server creates the subscription and
	// immediately tears it down; parameters stay cached for the next Start.
	require.NoError(t, srv.doRegisterSubscriber("topic.a", fakeNopHandler, nil))
	assert.Nil(t, srv.subscribers["topic.a"])
	require.NotNil(t, fb.subs["topic.a"])
	assert.Equal(t, 1, fb.subs["topic.a"].unsubscribed)
	assert.NotNil(t, srv.subscriberOpts["topic.a"])
}

func TestDoRegisterSubscriberMap(t *testing.T) {
	srv := NewServer()
	fb := newFakeBroker()
	srv.Broker = fb

	// An empty cache registers nothing and reports no error.
	assert.NoError(t, srv.doRegisterSubscriberMap())

	// A Subscribe failure is joined into a single error.
	fb.subscribeErr = errors.New("subscribe boom")
	srv.subscriberOpts["topic.a"] = &transport.SubscribeOption{Handler: fakeNopHandler}
	assert.Error(t, srv.doRegisterSubscriberMap())
}

///////////////////////////////////////////////////////////////////////////////
// Metrics wrapping.
///////////////////////////////////////////////////////////////////////////////

func TestWrapHandlerWithMetrics(t *testing.T) {
	fm := newFakeMetrics()
	srv := NewServer(WithMetrics(fm))

	// Success path: received counter and duration histogram only.
	wrapped := srv.wrapHandler("topic.a", fakeNopHandler)
	require.NoError(t, wrapped(context.Background(), &fakeEvent{topic: "topic.a"}))
	assert.Equal(t, float64(1), fm.counters["broker.messages.received"])
	assert.Equal(t, 1, fm.histograms["broker.message.duration"])
	assert.NotContains(t, fm.counters, "broker.messages.errors")

	// Failure path adds the error counter.
	failing := srv.wrapHandler("topic.a", func(_ context.Context, _ broker.Event) error {
		return errors.New("handler boom")
	})
	assert.Error(t, failing(context.Background(), &fakeEvent{topic: "topic.a"}))
	assert.Equal(t, float64(1), fm.counters["broker.messages.errors"])
}

///////////////////////////////////////////////////////////////////////////////
// Server options.
///////////////////////////////////////////////////////////////////////////////

func TestServerOptions(t *testing.T) {
	errorHandler := func(_ context.Context, _ broker.Event) error { return nil }
	propagator := &fakePropagator{}

	srv := NewServer(
		WithBrokerOptions(broker.WithErrorHandler(errorHandler)),
		WithAddress([]string{"pulsar://127.0.0.1:6650"}),
		WithTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12}),
		WithCodec("json"),
		WithGlobalTracerProvider(),
		WithGlobalPropagator(),
		// trace.TracerProvider has an unexported marker method, so a nil
		// provider is used; the applier only records it in the option list.
		WithTracerProvider(nil, "otlp"),
		WithPropagator(propagator),
		WithMetrics(newFakeMetrics()),
	)

	opts := srv.Options()
	assert.Equal(t, []string{"pulsar://127.0.0.1:6650"}, opts.Addrs)
	assert.NotNil(t, opts.Codec)
	assert.True(t, opts.Secure)
	assert.NotNil(t, opts.TLSConfig)
	// Functions cannot be compared for equality with assert.Equal.
	assert.NotNil(t, opts.ErrorHandler)
	assert.Len(t, opts.Tracings, 4)
	assert.NotNil(t, srv.m)
}

func TestServerTLSConfigNilKeepsInsecure(t *testing.T) {
	srv := NewServer(WithTLSConfig(nil))
	opts := srv.Options()
	assert.False(t, opts.Secure)
	assert.Nil(t, opts.TLSConfig)
}

///////////////////////////////////////////////////////////////////////////////
// Package logger.
///////////////////////////////////////////////////////////////////////////////

func TestLoggerRouting(t *testing.T) {
	fl := newFakeLogger()
	SetLogger(fl)
	defer SetLogger(nil)

	LogDebug("debug message")
	LogInfo("info message")
	LogWarn("warn message")
	LogError("error message")
	LogFatal("fatal message") // routes to the error level, must not exit
	LogDebugf("%s", "debug message")
	LogInfof("%s", "info message")
	LogWarnf("%s", "warn message")
	LogErrorf("%s", "error message")
	LogFatalf("%s", "fatal message")

	for _, level := range []log.Level{log.LevelDebug, log.LevelInfo, log.LevelWarn} {
		records := fl.records[level]
		require.Len(t, records, 2, "level %v", level)
		for _, msg := range records {
			assert.True(t, strings.HasPrefix(msg, logKey), "message %q should carry the package prefix", msg)
		}
	}
	// Error level: LogError/LogErrorf plus LogFatal/LogFatalf (which route
	// to the error level instead of exiting).
	assert.Len(t, fl.records[log.LevelError], 4)
}

func TestLoggerResetFallsBackToGlobal(t *testing.T) {
	SetLogger(newFakeLogger())
	SetLogger(nil)
	// With no package logger injected, the framework global logger is used.
	assert.NotNil(t, getLogger())
}

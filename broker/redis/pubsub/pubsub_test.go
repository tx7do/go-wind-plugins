package pubsub

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"

	"github.com/tx7do/go-wind-plugins/broker"
	redisOption "github.com/tx7do/go-wind-plugins/broker/redis/option"
)

// ---------------------------------------------------------------------------
// fake redis.Conn implementation — lets us exercise publish/subscribe paths
// without a real Redis server.
// ---------------------------------------------------------------------------

type fakeConn struct {
	mu        sync.Mutex
	commands  [][]any  // each entry: [commandName, arg1, arg2...]
	replies   []any    // replies returned by Do in order (last one repeats)
	receiveCh chan any // frames returned by Receive
	err       error    // sticky error
}

func newFakeConn(replies ...any) *fakeConn {
	return &fakeConn{
		receiveCh: make(chan any, 16),
		replies:   replies,
	}
}

func (c *fakeConn) Close() error { return nil }
func (c *fakeConn) Err() error   { return c.err }
func (c *fakeConn) Send(_ string, _ ...any) error {
	return nil
}
func (c *fakeConn) Flush() error { return nil }
func (c *fakeConn) Receive() (any, error) {
	return <-c.receiveCh, nil
}

func (c *fakeConn) Do(commandName string, args ...any) (any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if commandName == "" {
		// Empty command is the connection state probe issued by the pool —
		// not an application command. Answer with a clean reply.
		return nil, nil
	}
	cmd := append([]any{commandName}, args...)
	c.commands = append(c.commands, cmd)
	var reply any
	if len(c.replies) > 0 {
		idx := len(c.commands) - 1
		if idx >= len(c.replies) {
			// More commands than scripted replies: repeat the last one.
			idx = len(c.replies) - 1
		}
		reply = c.replies[idx]
	}
	if e, ok := reply.(error); ok {
		return nil, e
	}
	return reply, nil
}

func (c *fakeConn) recordedCommands() [][]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([][]any, len(c.commands))
	copy(out, c.commands)
	return out
}

// newFakePool returns a redigo pool that dials a fakeConn (no network).
func newFakePool(conn *fakeConn) *redis.Pool {
	return &redis.Pool{
		Dial: func() (redis.Conn, error) { return conn, nil },
	}
}

// ---------------------------------------------------------------------------
// NewBroker / basic accessors
// ---------------------------------------------------------------------------

func TestNewBroker(t *testing.T) {
	b := NewBroker()

	if b.Name() != "redis" {
		t.Errorf("Name() = %q, want %q", b.Name(), "redis")
	}
	if b.Address() != "" {
		t.Errorf("Address() = %q, want empty before Init", b.Address())
	}

	// Defaults must be present without any dialing.
	common := b.(*pubsubBroker).commonOpts
	if common.MaxIdle != redisOption.DefaultMaxIdle {
		t.Errorf("commonOpts.MaxIdle = %d, want %d", common.MaxIdle, redisOption.DefaultMaxIdle)
	}
	if common.ConnectTimeout != redisOption.DefaultConnectTimeout {
		t.Errorf("commonOpts.ConnectTimeout = %v, want %v", common.ConnectTimeout, redisOption.DefaultConnectTimeout)
	}

	// Options must have a default codec.
	if b.Options().Codec == nil {
		t.Error("Options().Codec = nil, want default codec")
	}
}

// ---------------------------------------------------------------------------
// normalizeAddr
// ---------------------------------------------------------------------------

func TestNormalizeAddr(t *testing.T) {
	tests := []struct {
		name  string
		addrs []string
		want  string
	}{
		{"empty list", nil, "redis://127.0.0.1:6379"},
		{"empty slice", []string{}, "redis://127.0.0.1:6379"},
		{"first empty", []string{""}, "redis://127.0.0.1:6379"},
		{"no scheme", []string{"127.0.0.1:6379"}, "redis://127.0.0.1:6379"},
		{"with scheme", []string{"redis://example.com:6380"}, "redis://example.com:6380"},
		{"only first addr used", []string{"a:1", "b:2"}, "redis://a:1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeAddr(tt.addrs); got != tt.want {
				t.Errorf("normalizeAddr(%v) = %q, want %q", tt.addrs, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Init / Connect / Disconnect lifecycle (no server needed)
// ---------------------------------------------------------------------------

func TestInit_AppliesOptionsAndAddress(t *testing.T) {
	b := NewBroker().(*pubsubBroker)

	err := b.Init(
		broker.WithAddress("127.0.0.1:7000"),
		redisOption.WithMaxIdle(11),
		redisOption.WithPassword("pw"),
	)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if b.Address() != "redis://127.0.0.1:7000" {
		t.Errorf("Address() = %q, want %q", b.Address(), "redis://127.0.0.1:7000")
	}
	if b.commonOpts.MaxIdle != 11 {
		t.Errorf("commonOpts.MaxIdle = %d, want 11", b.commonOpts.MaxIdle)
	}
	if b.commonOpts.Password != "pw" {
		t.Errorf("commonOpts.Password = %q, want %q", b.commonOpts.Password, "pw")
	}
	if b.pool != nil {
		t.Error("Init must not create the pool")
	}
}

func TestInit_ErrorWhileConnected(t *testing.T) {
	b := NewBroker().(*pubsubBroker)
	b.pool = &redis.Pool{} // simulate connected state

	// 无新选项的 Init 在已连接状态下幂等成功（Connect 先于 Init 是合法顺序，
	// 例如先 srv.Connect() 再 srv.Start()）。
	if err := b.Init(); err != nil {
		t.Fatalf("Init() without options while connected should be idempotent, got %v", err)
	}

	// 携带新选项的 Init 仍然拒绝，防止活跃连接池感知不到的配置漂移。
	err := b.Init(broker.WithAddress("127.0.0.1:9999"))
	if err == nil {
		t.Fatal("Init() with new options while connected should return an error")
	}
	if !strings.Contains(err.Error(), "cannot init while connected") {
		t.Errorf("Init() error = %v, want it to mention \"cannot init while connected\"", err)
	}
}

func TestConnect_CreatesPoolIdempotently(t *testing.T) {
	b := NewBroker().(*pubsubBroker)
	if err := b.Init(broker.WithAddress("127.0.0.1:6379")); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if err := b.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if b.pool == nil {
		t.Fatal("Connect() did not create the pool")
	}
	first := b.pool

	// Second connect must be a no-op.
	if err := b.Connect(); err != nil {
		t.Fatalf("second Connect() error = %v", err)
	}
	if b.pool != first {
		t.Error("second Connect() replaced the pool")
	}

	if err := b.Disconnect(); err != nil {
		t.Fatalf("Disconnect() error = %v", err)
	}
	if b.pool != nil {
		t.Error("Disconnect() did not clear the pool")
	}
}

func TestDisconnect_Idempotent(t *testing.T) {
	b := NewBroker().(*pubsubBroker)

	// Disconnect without connect must be a safe no-op.
	if err := b.Disconnect(); err != nil {
		t.Errorf("Disconnect() without pool error = %v, want nil", err)
	}
}

// ---------------------------------------------------------------------------
// Publish
// ---------------------------------------------------------------------------

func TestPublish_NotConnected(t *testing.T) {
	b := NewBroker().(*pubsubBroker)

	err := b.Publish(t.Context(), "topic", broker.NewMessage([]byte("x")))
	if err == nil {
		t.Fatal("Publish() without connection should fail")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("Publish() error = %v, want it to mention \"not connected\"", err)
	}
}

func TestPublish_SendsPUBLISHCommand(t *testing.T) {
	conn := newFakeConn(int64(1))
	b := NewBroker().(*pubsubBroker)
	b.pool = newFakePool(conn)

	msg := broker.NewMessage(map[string]string{"k": "v"})
	if err := b.Publish(t.Context(), "mytopic", msg); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	cmds := conn.recordedCommands()
	if len(cmds) != 1 {
		t.Fatalf("recorded %d commands, want 1: %v", len(cmds), cmds)
	}
	cmd := cmds[0]
	if cmd[0] != "PUBLISH" {
		t.Errorf("command = %v, want PUBLISH", cmd[0])
	}
	if cmd[1] != "mytopic" {
		t.Errorf("channel = %v, want mytopic", cmd[1])
	}
	body, ok := cmd[2].([]byte)
	if !ok {
		t.Fatalf("payload type = %T, want []byte", cmd[2])
	}
	if !strings.Contains(string(body), `"v"`) {
		t.Errorf("payload = %q, want marshalled JSON containing \"v\"", body)
	}
}

func TestPublish_WithMiddleware(t *testing.T) {
	conn := newFakeConn(int64(1))
	b := NewBroker(
		broker.WithPublishMiddlewares(func(p broker.PublishHandler) broker.PublishHandler {
			return func(ctx context.Context, topic string, msg *broker.Message, opts ...broker.PublishOption) error {
				return p(ctx, topic, msg, opts...)
			}
		}),
	).(*pubsubBroker)
	b.pool = newFakePool(conn)

	if err := b.Publish(t.Context(), "t", broker.NewMessage([]byte("x"))); err != nil {
		t.Fatalf("Publish() with middleware error = %v", err)
	}
	if len(conn.recordedCommands()) != 1 {
		t.Error("middleware chain did not reach internalPublish")
	}
}

// ---------------------------------------------------------------------------
// publication
// ---------------------------------------------------------------------------

func TestPublicationAccessors(t *testing.T) {
	msg := broker.NewMessage([]byte("body"))
	p := &publication{topic: "t", message: msg, err: errors.New("boom")}

	if p.Topic() != "t" {
		t.Errorf("Topic() = %q, want %q", p.Topic(), "t")
	}
	if p.Message() != msg {
		t.Error("Message() returned a different message")
	}
	if p.RawMessage() != msg {
		t.Error("RawMessage() should return the broker message")
	}
	if p.Error() == nil || p.Error().Error() != "boom" {
		t.Errorf("Error() = %v, want boom", p.Error())
	}
	// Pub/Sub has no ack semantics — Ack must always succeed.
	if err := p.Ack(); err != nil {
		t.Errorf("Ack() error = %v, want nil", err)
	}
}

// ---------------------------------------------------------------------------
// subscriber.onMessage
// ---------------------------------------------------------------------------

type testPayload struct {
	Name string `json:"name"`
}

func newTestSubscriber(t *testing.T, opts ...func(*subscriber)) *subscriber {
	t.Helper()
	o := broker.NewOptions() // default json codec
	b := &pubsubBroker{options: o}
	s := &subscriber{
		b:       b,
		topic:   "test-topic",
		options: broker.NewSubscribeOptions(),
	}
	for _, f := range opts {
		f(s)
	}
	return s
}

func TestOnMessage_WithoutBinder(t *testing.T) {
	var gotTopic string
	var gotBody []byte
	s := newTestSubscriber(t, func(s *subscriber) {
		s.handler = func(_ context.Context, evt broker.Event) error {
			gotTopic = evt.Topic()
			gotBody = evt.Message().Body.([]byte)
			return nil
		}
	})

	data := []byte(`{"name":"a"}`)
	if err := s.onMessage("chan-1", data); err != nil {
		t.Fatalf("onMessage() error = %v", err)
	}
	if gotTopic != "chan-1" {
		t.Errorf("handler topic = %q, want chan-1", gotTopic)
	}
	if string(gotBody) != string(data) {
		t.Errorf("handler body = %q, want raw data %q", gotBody, data)
	}
}

func TestOnMessage_WithBinderDecodes(t *testing.T) {
	var got *testPayload
	s := newTestSubscriber(t, func(s *subscriber) {
		s.binder = func() any { return &testPayload{} }
		s.handler = func(_ context.Context, evt broker.Event) error {
			got = evt.Message().Body.(*testPayload)
			return nil
		}
	})

	if err := s.onMessage("chan", []byte(`{"name":"decoded"}`)); err != nil {
		t.Fatalf("onMessage() error = %v", err)
	}
	if got == nil || got.Name != "decoded" {
		t.Errorf("handler payload = %+v, want {decoded}", got)
	}
}

func TestOnMessage_PoisonMessageCallsErrorHandler(t *testing.T) {
	handlerCalled := false
	var ehErr error
	s := newTestSubscriber(t, func(s *subscriber) {
		s.binder = func() any { return &testPayload{} }
		s.handler = func(_ context.Context, _ broker.Event) error {
			handlerCalled = true
			return nil
		}
		s.b.options.ErrorHandler = func(_ context.Context, evt broker.Event) error {
			ehErr = evt.Error()
			return nil
		}
	})

	// Malformed payload for the bound type must not reach the handler,
	// must notify the ErrorHandler, and must not fail the receive loop.
	if err := s.onMessage("chan", []byte(`not-json`)); err != nil {
		t.Fatalf("onMessage() error = %v, want nil for poison message", err)
	}
	if handlerCalled {
		t.Error("handler called for poison message")
	}
	if ehErr == nil {
		t.Error("ErrorHandler not invoked for poison message")
	}
}

func TestOnMessage_HandlerErrorNotifiesErrorHandler(t *testing.T) {
	handlerErr := errors.New("handler failed")
	var ehErr error
	s := newTestSubscriber(t, func(s *subscriber) {
		s.handler = func(_ context.Context, _ broker.Event) error { return handlerErr }
		s.b.options.ErrorHandler = func(_ context.Context, evt broker.Event) error {
			ehErr = evt.Error()
			return nil
		}
	})

	if err := s.onMessage("chan", []byte("x")); err == nil {
		t.Fatal("onMessage() should propagate handler error")
	} else if !errors.Is(err, handlerErr) {
		t.Errorf("onMessage() error = %v, want %v", err, handlerErr)
	}
	if !errors.Is(ehErr, handlerErr) {
		t.Errorf("ErrorHandler error = %v, want %v", ehErr, handlerErr)
	}
}

func TestOnMessage_AutoAck(t *testing.T) {
	s := newTestSubscriber(t, func(s *subscriber) {
		s.handler = func(_ context.Context, _ broker.Event) error { return nil }
		s.options = broker.NewSubscribeOptions(broker.WithSubscribeAutoAck(true))
	})

	if err := s.onMessage("chan", []byte("x")); err != nil {
		t.Errorf("onMessage() with AutoAck error = %v, want nil", err)
	}
}

// ---------------------------------------------------------------------------
// subscriber accessors / lifecycle
// ---------------------------------------------------------------------------

func TestSubscriber_UnsubscribeAndClose(t *testing.T) {
	s := newTestSubscriber(t)
	s.topic = "unsub-topic"

	if s.IsClosed() {
		t.Error("subscriber should start open")
	}
	if s.Topic() != "unsub-topic" {
		t.Errorf("Topic() = %q, want %q", s.Topic(), "unsub-topic")
	}

	// Unsubscribe with a nil conn must not panic and must mark closed.
	if err := s.Unsubscribe(true); err != nil {
		t.Errorf("Unsubscribe() error = %v, want nil", err)
	}
	if !s.IsClosed() {
		t.Error("IsClosed() = false after Unsubscribe")
	}
}

func TestSubscriber_RecvReturnsWhenClosed(t *testing.T) {
	s := newTestSubscriber(t)
	s.closed = true

	// recv() must return immediately for a closed subscriber.
	done := make(chan struct{})
	go func() {
		s.recv()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("recv() did not return for a closed subscriber")
	}
}

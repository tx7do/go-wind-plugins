package stream

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
// fake redis.Conn implementation — lets us exercise publish/ack paths
// without a real Redis server.
// ---------------------------------------------------------------------------

type fakeConn struct {
	mu       sync.Mutex
	commands [][]any // each entry: [commandName, arg1, arg2...]
	replies  []any   // replies returned by Do in order (last one repeats)
	err      error   // sticky error
}

func newFakeConn(replies ...any) *fakeConn {
	return &fakeConn{replies: replies}
}

func (c *fakeConn) Close() error { return nil }
func (c *fakeConn) Err() error   { return c.err }
func (c *fakeConn) Send(_ string, _ ...any) error {
	return nil
}
func (c *fakeConn) Flush() error { return nil }
func (c *fakeConn) Receive() (any, error) {
	return nil, nil
}

func (c *fakeConn) Do(commandName string, args ...any) (any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if commandName == "" {
		// Connection state probe issued by the pool — not an application command.
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

	if b.Name() != "redis-stream" {
		t.Errorf("Name() = %q, want %q", b.Name(), "redis-stream")
	}
	if b.Address() != "" {
		t.Errorf("Address() = %q, want empty before Init", b.Address())
	}

	common := b.(*streamBroker).commonOpts
	if common.MaxIdle != redisOption.DefaultMaxIdle {
		t.Errorf("commonOpts.MaxIdle = %d, want %d", common.MaxIdle, redisOption.DefaultMaxIdle)
	}
	if common.ReadTimeout != redisOption.DefaultReadTimeout {
		t.Errorf("commonOpts.ReadTimeout = %v, want %v", common.ReadTimeout, redisOption.DefaultReadTimeout)
	}
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
		{"nil list", nil, "redis://127.0.0.1:6379"},
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
	b := NewBroker().(*streamBroker)

	err := b.Init(
		broker.WithAddress("127.0.0.1:7000"),
		redisOption.WithMaxIdle(21),
	)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if b.Address() != "redis://127.0.0.1:7000" {
		t.Errorf("Address() = %q, want %q", b.Address(), "redis://127.0.0.1:7000")
	}
	if b.commonOpts.MaxIdle != 21 {
		t.Errorf("commonOpts.MaxIdle = %d, want 21", b.commonOpts.MaxIdle)
	}
	if b.pool != nil {
		t.Error("Init must not create the pool")
	}
}

func TestInit_ErrorWhileConnected(t *testing.T) {
	b := NewBroker().(*streamBroker)
	b.pool = &redis.Pool{} // simulate connected state

	err := b.Init()
	if err == nil {
		t.Fatal("Init() while connected should return an error")
	}
	if !strings.Contains(err.Error(), "cannot init while connected") {
		t.Errorf("Init() error = %v, want it to mention \"cannot init while connected\"", err)
	}
}

func TestConnect_CreatesPoolIdempotently(t *testing.T) {
	b := NewBroker().(*streamBroker)
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
	b := NewBroker().(*streamBroker)
	if err := b.Disconnect(); err != nil {
		t.Errorf("Disconnect() without pool error = %v, want nil", err)
	}
}

// ---------------------------------------------------------------------------
// publish (XADD argument building)
// ---------------------------------------------------------------------------

func TestPublish_NotConnected(t *testing.T) {
	b := NewBroker().(*streamBroker)

	err := b.Publish(t.Context(), "stream", broker.NewMessage([]byte("x")))
	if err == nil {
		t.Fatal("Publish() without connection should fail")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("Publish() error = %v, want it to mention \"not connected\"", err)
	}
}

func TestPublish_XADDArguments(t *testing.T) {
	conn := newFakeConn("1-1")
	b := NewBroker().(*streamBroker)
	b.pool = newFakePool(conn)

	msg := broker.NewMessage("payload")
	msg.SetHeader("trace", "abc")
	msg.SetHeader("kind", "evt")

	if err := b.Publish(t.Context(), "orders", msg); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	cmds := conn.recordedCommands()
	if len(cmds) != 1 {
		t.Fatalf("recorded %d commands, want 1: %v", len(cmds), cmds)
	}
	cmd := cmds[0]
	if cmd[0] != "XADD" {
		t.Fatalf("command = %v, want XADD", cmd[0])
	}

	// Without MAXLEN the args are: stream, "*", "body", payload, then headers.
	if len(cmd) < 5 {
		t.Fatalf("XADD args too short: %v", cmd)
	}
	if cmd[1] != "orders" {
		t.Errorf("stream = %v, want orders", cmd[1])
	}
	if cmd[2] != "*" {
		t.Errorf("id placeholder = %v, want \"*\"", cmd[2])
	}
	if cmd[3] != "body" {
		t.Errorf("body field name = %v, want body", cmd[3])
	}
	// The json codec marshals a string body to a JSON string literal.
	if body, ok := cmd[4].([]byte); !ok || string(body) != `"payload"` {
		t.Errorf("body value = %v (%T), want []byte(`\"payload\"`)", cmd[4], cmd[4])
	}

	// Headers are appended as key/value string pairs.
	gotHeaders := map[string]string{}
	for i := 5; i+1 < len(cmd); i += 2 {
		gotHeaders[cmd[i].(string)] = cmd[i+1].(string)
	}
	if len(gotHeaders) != 2 {
		t.Errorf("header args = %v, want 2 header pairs", cmd[5:])
	}
	if gotHeaders["trace"] != "abc" || gotHeaders["kind"] != "evt" {
		t.Errorf("headers = %v, want trace=abc kind=evt", gotHeaders)
	}
}

func TestPublish_XADDMaxLenOption(t *testing.T) {
	conn := newFakeConn("1-1")
	b := NewBroker().(*streamBroker)
	b.pool = newFakePool(conn)

	if err := b.Publish(t.Context(), "s", broker.NewMessage([]byte("x")),
		redisOption.WithStreamMaxLen(500)); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	cmd := conn.recordedCommands()[0]
	// MAXLEN must appear between the stream name and the id placeholder.
	if len(cmd) < 7 {
		t.Fatalf("XADD args too short: %v", cmd)
	}
	if cmd[1] != "s" || cmd[2] != "MAXLEN" || cmd[3] != "~" || cmd[4] != int64(500) {
		t.Errorf("stream/MAXLEN args = %v, want [s MAXLEN ~ 500]", cmd[1:5])
	}
	if cmd[5] != "*" {
		t.Errorf("id placeholder = %v, want \"*\"", cmd[5])
	}
}

func TestPublish_WithoutMaxLenOmitsArgument(t *testing.T) {
	conn := newFakeConn("1-1")
	b := NewBroker().(*streamBroker)
	b.pool = newFakePool(conn)

	// MAXLEN 0 means unlimited — the argument must be omitted entirely.
	if err := b.Publish(t.Context(), "s", broker.NewMessage([]byte("x")),
		redisOption.WithStreamMaxLen(0)); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	cmd := conn.recordedCommands()[0]
	for _, a := range cmd {
		if a == "MAXLEN" {
			t.Errorf("MAXLEN present with value 0: %v", cmd)
		}
	}
}

// ---------------------------------------------------------------------------
// ensureGroup
// ---------------------------------------------------------------------------

func TestEnsureGroup_Created(t *testing.T) {
	conn := newFakeConn("OK")
	b := NewBroker().(*streamBroker)
	b.pool = newFakePool(conn)

	if err := b.ensureGroup("s", "g"); err != nil {
		t.Fatalf("ensureGroup() error = %v", err)
	}

	cmd := conn.recordedCommands()[0]
	want := []any{"XGROUP", "CREATE", "s", "g", "$", "MKSTREAM"}
	if len(cmd) != len(want) {
		t.Fatalf("XGROUP args = %v, want %v", cmd, want)
	}
	for i := range want {
		if cmd[i] != want[i] {
			t.Errorf("XGROUP arg %d = %v, want %v", i, cmd[i], want[i])
		}
	}
}

func TestEnsureGroup_BusyGroupIsNotAnError(t *testing.T) {
	conn := newFakeConn(errors.New("BUSYGROUP Consumer Group name already exists"))
	b := NewBroker().(*streamBroker)
	b.pool = newFakePool(conn)

	if err := b.ensureGroup("s", "g"); err != nil {
		t.Errorf("ensureGroup() with BUSYGROUP error = %v, want nil", err)
	}
}

func TestEnsureGroup_PropagatesOtherErrors(t *testing.T) {
	conn := newFakeConn(errors.New("ERR something else"))
	b := NewBroker().(*streamBroker)
	b.pool = newFakePool(conn)

	if err := b.ensureGroup("s", "g"); err == nil {
		t.Error("ensureGroup() should propagate non-BUSYGROUP errors")
	}
}

// ---------------------------------------------------------------------------
// publication
// ---------------------------------------------------------------------------

func TestPublicationAccessors(t *testing.T) {
	msg := broker.NewMessage([]byte("body"))
	p := &publication{topic: "s", group: "g", msgID: "1-1", message: msg, err: errors.New("boom")}

	if p.Topic() != "s" {
		t.Errorf("Topic() = %q, want s", p.Topic())
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
}

func TestPublicationAck_WithoutPool(t *testing.T) {
	p := &publication{topic: "s", group: "g", msgID: "1-1"}

	// A nil pool must make Ack a no-op success (no panic).
	if err := p.Ack(); err != nil {
		t.Errorf("Ack() with nil pool error = %v, want nil", err)
	}
}

func TestPublicationAck_SendsXACKOnce(t *testing.T) {
	conn := newFakeConn(int64(1))
	p := &publication{
		topic: "s",
		group: "g",
		msgID: "42-0",
		pool:  newFakePool(conn),
	}

	if err := p.Ack(); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}
	// Idempotent: the second Ack must be a no-op.
	if err := p.Ack(); err != nil {
		t.Fatalf("second Ack() error = %v", err)
	}

	cmds := conn.recordedCommands()
	if len(cmds) != 1 {
		t.Fatalf("recorded %d commands, want 1 (Ack must be idempotent): %v", len(cmds), cmds)
	}
	want := []any{"XACK", "s", "g", "42-0"}
	if len(cmds[0]) != len(want) {
		t.Fatalf("XACK args = %v, want %v", cmds[0], want)
	}
	for i := range want {
		if cmds[0][i] != want[i] {
			t.Errorf("XACK arg %d = %v, want %v", i, cmds[0][i], want[i])
		}
	}
}

func TestPublicationAck_ErrorRecorded(t *testing.T) {
	ackErr := errors.New("ack failed")
	conn := newFakeConn(ackErr)
	p := &publication{topic: "s", group: "g", msgID: "1-1", pool: newFakePool(conn)}

	if err := p.Ack(); !errors.Is(err, ackErr) {
		t.Errorf("Ack() error = %v, want %v", err, ackErr)
	}
	if p.Error() == nil {
		t.Error("publication.Error() should record the ack failure")
	}
	// Failed ack is not remembered as acked — retry must hit the conn again.
	if err := p.Ack(); !errors.Is(err, ackErr) {
		t.Errorf("retry Ack() error = %v, want %v", err, ackErr)
	}
	if len(conn.recordedCommands()) != 2 {
		t.Error("failed ack should not mark the publication acked")
	}
}

// ---------------------------------------------------------------------------
// subscriber
// ---------------------------------------------------------------------------

type testPayload struct {
	Name string `json:"name"`
}

func newTestSubscriber(t *testing.T, opts ...func(*subscriber)) *subscriber {
	t.Helper()
	o := broker.NewOptions() // default json codec
	b := &streamBroker{options: o}
	s := &subscriber{
		b:         b,
		topic:     "test-stream",
		group:     "test-group",
		consumer:  "test-consumer",
		blockTime: 100 * time.Millisecond,
		count:     10,
		options:   broker.NewSubscribeOptions(),
	}
	for _, f := range opts {
		f(s)
	}
	return s
}

func TestSubscriberExtractField(t *testing.T) {
	s := newTestSubscriber(t)

	fields := []any{
		[]byte("body"), []byte("the-payload"),
		[]byte("extra"), []byte("v"),
	}

	got := s.extractField(fields, "body")
	if v, ok := got.([]byte); !ok || string(v) != "the-payload" {
		t.Errorf("extractField(body) = %v (%T), want \"the-payload\"", got, got)
	}

	got = s.extractField(fields, "extra")
	if v, ok := got.([]byte); !ok || string(v) != "v" {
		t.Errorf("extractField(extra) = %v (%T), want \"v\"", got, got)
	}

	if got := s.extractField(fields, "missing"); got != nil {
		t.Errorf("extractField(missing) = %v, want nil", got)
	}
	if got := s.extractField(nil, "body"); got != nil {
		t.Errorf("extractField(nil) = %v, want nil", got)
	}
}

func TestSubscriberOnMessage_WithoutBinder(t *testing.T) {
	var gotID string
	var gotBody []byte
	s := newTestSubscriber(t, func(s *subscriber) {
		s.handler = func(_ context.Context, evt broker.Event) error {
			gotID = evt.(*publication).msgID
			gotBody = evt.Message().Body.([]byte)
			return nil
		}
	})

	if err := s.onMessage("7-0", []byte("raw")); err != nil {
		t.Fatalf("onMessage() error = %v", err)
	}
	if gotID != "7-0" {
		t.Errorf("handler msgID = %q, want 7-0", gotID)
	}
	if string(gotBody) != "raw" {
		t.Errorf("handler body = %q, want raw", gotBody)
	}
}

func TestSubscriberOnMessage_WithBinderDecodes(t *testing.T) {
	var got *testPayload
	s := newTestSubscriber(t, func(s *subscriber) {
		s.binder = func() any { return &testPayload{} }
		s.handler = func(_ context.Context, evt broker.Event) error {
			got = evt.Message().Body.(*testPayload)
			return nil
		}
	})

	if err := s.onMessage("8-0", []byte(`{"name":"decoded"}`)); err != nil {
		t.Fatalf("onMessage() error = %v", err)
	}
	if got == nil || got.Name != "decoded" {
		t.Errorf("handler payload = %+v, want {decoded}", got)
	}
}

func TestSubscriberOnMessage_PoisonMessageReturnsError(t *testing.T) {
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

	// Unlike pub/sub, a poison message in a stream is reported as an error so
	// it stays in the PEL for later recovery.
	err := s.onMessage("9-0", []byte(`not-json`))
	if err == nil {
		t.Fatal("onMessage() should return the unmarshal error for a poison message")
	}
	if handlerCalled {
		t.Error("handler called for poison message")
	}
	if ehErr == nil {
		t.Error("ErrorHandler not invoked for poison message")
	}
}

func TestSubscriberOnMessage_HandlerErrorNotifiesErrorHandler(t *testing.T) {
	handlerErr := errors.New("handler failed")
	var ehErr error
	s := newTestSubscriber(t, func(s *subscriber) {
		s.handler = func(_ context.Context, _ broker.Event) error { return handlerErr }
		s.b.options.ErrorHandler = func(_ context.Context, evt broker.Event) error {
			ehErr = evt.Error()
			return nil
		}
	})

	if err := s.onMessage("10-0", []byte("x")); !errors.Is(err, handlerErr) {
		t.Errorf("onMessage() error = %v, want %v", err, handlerErr)
	}
	if !errors.Is(ehErr, handlerErr) {
		t.Errorf("ErrorHandler error = %v, want %v", ehErr, handlerErr)
	}
}

func TestSubscriberOnMessage_AutoAckWithoutPoolDoesNotPanic(t *testing.T) {
	s := newTestSubscriber(t)
	s.handler = func(_ context.Context, _ broker.Event) error { return nil }
	s.options = broker.NewSubscribeOptions(broker.WithSubscribeAutoAck(true))

	// AutoAck with a nil broker pool must be a silent no-op.
	if err := s.onMessage("11-0", []byte("x")); err != nil {
		t.Errorf("onMessage() with AutoAck error = %v, want nil", err)
	}
}

func TestSubscriberAccessorsAndUnsubscribe(t *testing.T) {
	s := newTestSubscriber(t)

	if s.Topic() != "test-stream" {
		t.Errorf("Topic() = %q, want test-stream", s.Topic())
	}
	opts := s.Options()
	if opts.Context == nil {
		t.Error("Options().Context = nil")
	}
	if s.IsClosed() {
		t.Error("subscriber should start open")
	}

	// Unsubscribe must be safe without a conn and not remove from the
	// manager when asked not to.
	if err := s.Unsubscribe(false); err != nil {
		t.Errorf("Unsubscribe() error = %v, want nil", err)
	}
	if !s.IsClosed() {
		t.Error("IsClosed() = false after Unsubscribe")
	}
}

func TestSubscriberReceiveLoop_ReturnsWhenClosedOrCancelled(t *testing.T) {
	// A closed subscriber must make receiveLoop return immediately.
	s := newTestSubscriber(t)
	s.closed = true
	if err := s.receiveLoop(); err != nil {
		t.Errorf("receiveLoop() for closed subscriber error = %v, want nil", err)
	}

	// A cancelled subscribe context must also stop the loop.
	s2 := newTestSubscriber(t)
	ctx, cancel := context.WithCancel(context.Background())
	s2.options.Context = ctx
	cancel()
	if err := s2.receiveLoop(); err != nil {
		t.Errorf("receiveLoop() with cancelled context error = %v, want nil", err)
	}
}

func TestSubscriberRecv_ReturnsWhenClosed(t *testing.T) {
	s := newTestSubscriber(t)
	s.closed = true

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

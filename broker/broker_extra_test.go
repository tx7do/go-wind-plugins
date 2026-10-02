package broker

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/tx7do/go-wind-plugins/encoding"
	_ "github.com/tx7do/go-wind-plugins/encoding/json"

	bLogger "github.com/tx7do/go-wind/log"
)

// ---------------------------------------------------------------------------
// Marshal / Unmarshal
// ---------------------------------------------------------------------------

type gobPayload struct {
	Name  string
	Count int
}

func TestMarshal_NilMessage(t *testing.T) {
	if _, err := Marshal(nil, nil); err == nil {
		t.Error("Marshal(nil, nil) should fail")
	}
}

func TestMarshal_NoCodec(t *testing.T) {
	// []byte passes through unchanged.
	raw := []byte("raw-bytes")
	got, err := Marshal(nil, raw)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !bytes.Equal(got, raw) {
		t.Errorf("Marshal([]byte) = %q, want %q", got, raw)
	}

	// string is converted.
	got, err = Marshal(nil, "a string")
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(got) != "a string" {
		t.Errorf("Marshal(string) = %q, want %q", got, "a string")
	}

	// Anything else falls back to gob.
	got, err = Marshal(nil, gobPayload{Name: "n", Count: 3})
	if err != nil {
		t.Fatalf("Marshal(gob): %v", err)
	}
	var out gobPayload
	if err := Unmarshal(nil, got, &out); err != nil {
		t.Fatalf("Unmarshal(gob): %v", err)
	}
	if out != (gobPayload{Name: "n", Count: 3}) {
		t.Errorf("gob round trip = %+v", out)
	}
}

func TestMarshal_WithCodec(t *testing.T) {
	codec := encoding.GetCodec("json")
	if codec == nil {
		t.Fatal("json codec not registered")
	}

	data, err := Marshal(codec, map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out map[string]string
	if err := Unmarshal(codec, data, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out["k"] != "v" {
		t.Errorf("codec round trip = %v", out)
	}
}

func TestUnmarshal_InputValidation(t *testing.T) {
	if err := Unmarshal(nil, nil, new([]byte)); err == nil {
		t.Error("Unmarshal with nil input should fail")
	}
	if err := Unmarshal(nil, []byte("x"), nil); err == nil {
		t.Error("Unmarshal with nil outValue should fail")
	}
}

func TestUnmarshal_NoCodec_TargetTypes(t *testing.T) {
	data := []byte("hello")

	var b []byte
	if err := Unmarshal(nil, data, &b); err != nil {
		t.Fatalf("Unmarshal(*[]byte): %v", err)
	}
	if !bytes.Equal(b, data) {
		t.Errorf("*[]byte = %q, want %q", b, data)
	}

	var s string
	if err := Unmarshal(nil, data, &s); err != nil {
		t.Fatalf("Unmarshal(*string): %v", err)
	}
	if s != "hello" {
		t.Errorf("*string = %q, want %q", s, "hello")
	}
}

// ---------------------------------------------------------------------------
// Generic Subscribe[T] wrapper
// ---------------------------------------------------------------------------

type fakeBroker struct {
	Broker

	capturedTopic   string
	capturedHandler Handler
	capturedBinder  Binder

	publishedTopic   string
	publishedMessage *Message
}

func (f *fakeBroker) Subscribe(topic string, handler Handler, binder Binder, _ ...SubscribeOption) (Subscriber, error) {
	f.capturedTopic = topic
	f.capturedHandler = handler
	f.capturedBinder = binder
	return &mockSubscriber{name: topic}, nil
}

func (f *fakeBroker) Publish(_ context.Context, topic string, msg *Message, _ ...PublishOption) error {
	f.publishedTopic = topic
	f.publishedMessage = msg
	return nil
}

type payload struct {
	Value string
}

// valueEvent wraps a fixed Message for driving captured handlers.
type valueEvent struct {
	msg *Message
}

func (e valueEvent) Topic() string     { return "topic" }
func (e valueEvent) Message() *Message { return e.msg }
func (e valueEvent) RawMessage() any   { return nil }
func (e valueEvent) Ack() error        { return nil }
func (e valueEvent) Error() error      { return nil }

func TestSubscribe_GenericWrapper(t *testing.T) {
	fb := &fakeBroker{}

	sub, err := Subscribe[payload](fb, "topic-1", func(_ context.Context, topic string, headers Headers, p *payload) error {
		p.Value = "handled:" + topic
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if sub == nil {
		t.Fatal("Subscribe returned a nil subscriber")
	}
	if fb.capturedTopic != "topic-1" {
		t.Errorf("topic = %q, want topic-1", fb.capturedTopic)
	}
	if fb.capturedBinder == nil {
		t.Fatal("binder should be non-nil")
	}
	if _, ok := fb.capturedBinder().(*payload); !ok {
		t.Errorf("binder should produce *payload, got %T", fb.capturedBinder())
	}

	t.Run("pointer body is handled", func(t *testing.T) {
		p := &payload{Value: "ptr"}
		err := fb.capturedHandler(context.Background(), valueEvent{msg: &Message{Body: p}})
		if err != nil {
			t.Fatalf("handler: %v", err)
		}
		if p.Value != "handled:topic" {
			t.Errorf("value = %q, want handled:topic", p.Value)
		}
	})

	t.Run("value body is handled via address-of copy", func(t *testing.T) {
		err := fb.capturedHandler(context.Background(), valueEvent{msg: &Message{Body: payload{Value: "val"}}})
		if err != nil {
			t.Fatalf("handler: %v", err)
		}
	})

	t.Run("nil event is rejected", func(t *testing.T) {
		err := fb.capturedHandler(context.Background(), nil)
		if err == nil {
			t.Error("nil event should be rejected")
		}
	})

	t.Run("nil message or body is rejected", func(t *testing.T) {
		if err := fb.capturedHandler(context.Background(), valueEvent{msg: nil}); err == nil {
			t.Error("nil message should be rejected")
		}
		if err := fb.capturedHandler(context.Background(), valueEvent{msg: &Message{}}); err == nil {
			t.Error("nil body should be rejected")
		}
	})

	t.Run("unsupported body type is rejected", func(t *testing.T) {
		err := fb.capturedHandler(context.Background(), valueEvent{msg: &Message{Body: "not-a-payload"}})
		if err == nil {
			t.Error("unsupported body type should be rejected")
		}
	})
}

// ---------------------------------------------------------------------------
// Publish middleware and legacy handler adapter
// ---------------------------------------------------------------------------

func TestChainPublishMiddleware_SkipNil(t *testing.T) {
	var seq []int

	base := PublishHandler(func(_ context.Context, _ string, _ *Message, _ ...PublishOption) error {
		seq = append(seq, 2)
		return nil
	})

	mw := func(next PublishHandler) PublishHandler {
		return func(ctx context.Context, topic string, msg *Message, opts ...PublishOption) error {
			seq = append(seq, 1)
			return next(ctx, topic, msg, opts...)
		}
	}

	chained := ChainPublishMiddleware(base, []PublishMiddleware{mw, nil})
	if err := chained(context.Background(), "t", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(seq, []int{1, 2}) {
		t.Errorf("order = %v, want [1 2]", seq)
	}

	// Empty middleware list returns the handler as-is.
	if ChainPublishMiddleware(base, nil) == nil {
		t.Error("empty middleware list should keep the handler")
	}
}

func TestWrapLegacyPublishHandler(t *testing.T) {
	var got any
	legacy := func(_ context.Context, _ string, msg any, _ ...PublishOption) error {
		got = msg
		return nil
	}
	wrapped := WrapLegacyPublishHandler(legacy)

	// nil message passes a nil payload.
	if err := wrapped(context.Background(), "t", nil); err != nil {
		t.Fatalf("wrapped(nil): %v", err)
	}
	if got != nil {
		t.Errorf("nil message should pass nil payload, got %v", got)
	}

	// Msg (raw) takes priority over Body.
	if err := wrapped(context.Background(), "t", &Message{Msg: "raw", Body: "body"}); err != nil {
		t.Fatalf("wrapped: %v", err)
	}
	if got != "raw" {
		t.Errorf("payload = %v, want raw", got)
	}

	// Without Msg, Body is passed.
	if err := wrapped(context.Background(), "t", &Message{Body: "body"}); err != nil {
		t.Fatalf("wrapped: %v", err)
	}
	if got != "body" {
		t.Errorf("payload = %v, want body", got)
	}
}

// ---------------------------------------------------------------------------
// Request/reply helpers — error paths
// ---------------------------------------------------------------------------

func TestGenericRequest_NilInputs(t *testing.T) {
	if _, err := GenericRequest(context.Background(), nil, "t", NewMessage("x")); err == nil {
		t.Error("nil broker should fail")
	}

	fb := &fakeBroker{}
	if _, err := GenericRequest(context.Background(), fb, "t", nil); !errors.Is(err, ErrRequestMessageNil) {
		t.Errorf("nil message should be ErrRequestMessageNil, got %v", err)
	}
}

func TestNewReply(t *testing.T) {
	// Without a request, the reply has no correlation id.
	reply := NewReply(nil, "body")
	if reply.GetHeader(HeaderCorrelationID) != "" {
		t.Error("reply to nil request should have no correlation id")
	}

	// With a request, the correlation id is echoed.
	req := NewMessage("q")
	req.SetHeader(HeaderCorrelationID, "corr-1")
	reply = NewReply(req, "body")
	if reply.GetHeader(HeaderCorrelationID) != "corr-1" {
		t.Errorf("correlation id = %q, want corr-1", reply.GetHeader(HeaderCorrelationID))
	}
	if reply.Body != "body" {
		t.Errorf("body = %v, want body", reply.Body)
	}
}

func TestReplyTo_ErrorPaths(t *testing.T) {
	if err := ReplyTo(context.Background(), nil, NewMessage("q"), "b"); err == nil {
		t.Error("nil broker should fail")
	}

	fb := &fakeBroker{}
	if err := ReplyTo(context.Background(), fb, nil, "b"); !errors.Is(err, ErrRequestMessageNil) {
		t.Errorf("nil request should be ErrRequestMessageNil, got %v", err)
	}

	// Request without Reply-To.
	if err := ReplyTo(context.Background(), fb, NewMessage("q"), "b"); !errors.Is(err, ErrRequestNoReplyTo) {
		t.Errorf("missing Reply-To should be ErrRequestNoReplyTo, got %v", err)
	}

	// Success path: publishes to the request's reply topic.
	req := NewMessage("q")
	req.SetHeader(HeaderReplyTo, "reply-topic")
	req.SetHeader(HeaderCorrelationID, "corr-9")
	if err := ReplyTo(context.Background(), fb, req, "body"); err != nil {
		t.Fatalf("ReplyTo: %v", err)
	}
	if fb.publishedTopic != "reply-topic" {
		t.Errorf("published topic = %q, want reply-topic", fb.publishedTopic)
	}
	if fb.publishedMessage == nil || fb.publishedMessage.GetHeader(HeaderCorrelationID) != "corr-9" {
		t.Errorf("reply should echo the correlation id: %+v", fb.publishedMessage)
	}
}

// ---------------------------------------------------------------------------
// Message helpers not covered by message_test.go
// ---------------------------------------------------------------------------

type stringerKey struct{}

func (stringerKey) String() string { return "stringer-key" }

type structKey struct{ A int }

func TestMessage_Chaining(t *testing.T) {
	m := NewMessage("body").
		WithHeader("h1", "v1").
		WithKey("key-1").
		WithMetadata("mk", "mv").
		WithBody([]byte("raw"))

	if m.Key != "key-1" {
		t.Errorf("Key = %q", m.Key)
	}
	if m.GetMetadata("mk") != "mv" {
		t.Errorf("Metadata mk = %v", m.GetMetadata("mk"))
	}
	if string(m.BodyBytes()) != "raw" {
		t.Errorf("BodyBytes = %q", m.BodyBytes())
	}

	m.SetDelay("5s")
	if m.GetMetadata("x-delay-level") != "5s" {
		t.Errorf("delay level = %v", m.GetMetadata("x-delay-level"))
	}
}

func TestNewMessage_Options(t *testing.T) {
	m := NewMessage("body",
		WithID("id-1"),
		WithKey("key-1"),
		WithMsg("raw-msg"),
		WithPartitionOffset(3, 42),
		WithHeaders(Headers{"a": "1"}),
		WithHeader("b", "2"),
		WithMetadata(Metadata{"x": 1}),
		WithMetadataKV("y", 2),
		nil, // nil options must be skipped
	)

	if m.ID != "id-1" || m.Key != "key-1" || m.Msg != "raw-msg" {
		t.Errorf("basic fields = %s/%s/%v", m.ID, m.Key, m.Msg)
	}
	if m.Partition != 3 || m.Offset != 42 {
		t.Errorf("partition/offset = %d/%d", m.Partition, m.Offset)
	}
	if m.GetHeader("a") != "1" || m.GetHeader("b") != "2" {
		t.Errorf("headers = %v", m.Headers)
	}
	if m.GetMetadata("x") != 1 || m.GetMetadata("y") != 2 {
		t.Errorf("metadata = %v", m.Metadata)
	}

	// A default message starts with Partition/Offset = -1.
	d := NewMessage("body")
	if d.Partition != -1 || d.Offset != -1 {
		t.Errorf("defaults partition/offset = %d/%d, want -1/-1", d.Partition, d.Offset)
	}
}

func TestNewMessage_WithHeadersAndMetadata_Nil(t *testing.T) {
	// nil maps must be no-ops.
	m := NewMessage("body", WithHeaders(nil), WithMetadata(nil))
	if len(m.Headers) != 0 || len(m.Metadata) != 0 {
		t.Errorf("nil header/metadata options should be no-ops: %v %v", m.Headers, m.Metadata)
	}
}

func TestNewMessageWithContext(t *testing.T) {
	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "cv")

	m := NewMessageWithContext(ctx, "body", ctxKey{})
	if m.GetHeader("broker.ctxKey") != "cv" {
		t.Errorf("header = %q, want %q", m.GetHeader("broker.ctxKey"), "cv")
	}

	// Context values that are absent are skipped.
	m2 := NewMessageWithContext(context.Background(), "body", ctxKey{})
	if len(m2.Headers) != 0 {
		t.Errorf("absent context values should be skipped: %v", m2.Headers)
	}
}

func TestMessage_ExtractAndMetadataContext(t *testing.T) {
	base := context.Background()

	// No metadata: base context passes through unchanged.
	m := &Message{}
	if got := m.ExtractContext(base); got != base {
		t.Error("nil metadata should return the base context unchanged")
	}
	if GetMetadataFromContext(base) != nil {
		t.Error("metadata should be absent from the base context")
	}

	// With metadata: retrievable via GetMetadataFromContext.
	m = &Message{Metadata: Metadata{"trace": "abc"}}
	ctx := m.ExtractContext(base)
	got := GetMetadataFromContext(ctx)
	if got["trace"] != "abc" {
		t.Errorf("metadata from context = %v", got)
	}
}

func TestMessage_AckSuccess_NoAcker(t *testing.T) {
	m := &Message{}
	if err := m.AckSuccess(); err != nil {
		t.Errorf("AckSuccess without Msg = %v, want nil", err)
	}
	m = &Message{Msg: "plain string"}
	if err := m.AckSuccess(); err != nil {
		t.Errorf("AckSuccess with non-acker Msg = %v, want nil", err)
	}
}

func TestFormatKey(t *testing.T) {
	tests := []struct {
		name string
		key  any
		want string
	}{
		{name: "nil", key: nil, want: ""},
		{name: "string", key: "plain", want: "plain"},
		{name: "stringer", key: stringerKey{}, want: "stringer-key"},
		{name: "struct", key: structKey{A: 1}, want: "broker.structKey"},
		{name: "pointer", key: &structKey{A: 1}, want: "*broker.structKey"},
		{name: "int", key: 42, want: "42"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatKey(tt.key); got != tt.want {
				t.Errorf("formatKey(%v) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Broker options
// ---------------------------------------------------------------------------

func TestNewOptions_Defaults(t *testing.T) {
	o := NewOptions()
	if o.Addrs == nil || len(o.Addrs) != 0 {
		t.Errorf("Addrs = %v, want empty non-nil", o.Addrs)
	}
	if o.Codec == nil {
		t.Error("default codec should be set")
	}
	if o.Context == nil {
		t.Error("Context should default to a background context")
	}
	if o.Secure {
		t.Error("Secure should default to false")
	}
	if o.Tracings == nil {
		t.Error("Tracings should default to an empty non-nil slice")
	}
}

func TestOptions_Apply(t *testing.T) {
	var nilOptions *Options
	nilOptions.Apply(WithEnableSecure(true)) // must not panic

	o := &Options{}
	o.Apply(
		WithAddress("nats://a:4222", "nats://b:4222"),
		WithCodec("json"),
		WithErrorHandler(func(_ context.Context, _ Event) error { return nil }),
		WithEnableSecure(true),
		WithSubscriberMiddlewares(func(next Handler) Handler { return next }),
		WithPublishMiddlewares(func(next PublishHandler) PublishHandler { return next }),
	)
	if len(o.Addrs) != 2 || o.Addrs[0] != "nats://a:4222" {
		t.Errorf("Addrs = %v", o.Addrs)
	}
	if o.Codec == nil {
		t.Error("codec should be set")
	}
	if o.ErrorHandler == nil {
		t.Error("error handler should be set")
	}
	if !o.Secure {
		t.Error("Secure should be true")
	}
	if len(o.SubscriberMiddlewares) != 1 || len(o.PublishMiddlewares) != 1 {
		t.Errorf("middlewares = %d/%d", len(o.SubscriberMiddlewares), len(o.PublishMiddlewares))
	}
}

func TestOptions_Context(t *testing.T) {
	type key struct{}
	o := NewOptionsAndApply(OptionContextWithValue(key{}, "v1"))
	if v := o.Context.Value(key{}); v != "v1" {
		t.Errorf("context value = %v, want v1", v)
	}

	ctx := context.WithValue(context.Background(), key{}, "v2")
	o = NewOptionsAndApply(WithOptionContext(ctx))
	if v := o.Context.Value(key{}); v != "v2" {
		t.Errorf("context value = %v, want v2", v)
	}
}

func TestOptions_Logger(t *testing.T) {
	if LoggerFromOptions(nil) != nil {
		t.Error("nil options should yield a nil logger")
	}
	o2 := NewOptions()
	if LoggerFromOptions(&o2) != nil {
		t.Error("options without an injected logger should yield nil")
	}

	o := NewOptionsAndApply(WithLogger(nopLogger{}))
	if LoggerFromOptions(&o) == nil {
		t.Error("injected logger should be retrievable")
	}
}

func TestOptions_TLS(t *testing.T) {
	o := &Options{}
	// A nil TLS config must not enable secure mode.
	o.Apply(WithTLSConfig(nil))
	if o.Secure {
		t.Error("nil TLSConfig should not enable Secure")
	}

	o.Apply(WithTLSConfig(&tlsConfigStub))
	if !o.Secure {
		t.Error("non-nil TLSConfig should enable Secure")
	}
	if o.TLSConfig != &tlsConfigStub {
		t.Error("TLSConfig should be stored")
	}
}

var tlsConfigStub = tls.Config{} //nolint:unused-check — used via pointer above

func TestOptions_Tracing(t *testing.T) {
	o := &Options{}
	o.Apply(
		WithTracerProvider(nil),
		WithPropagator(nil),
		WithGlobalTracerProvider(),
		WithGlobalPropagator(),
	)
	if len(o.Tracings) != 4 {
		t.Errorf("Tracings = %d options, want 4", len(o.Tracings))
	}
}

// ---------------------------------------------------------------------------
// Publish / subscribe / request option structs
// ---------------------------------------------------------------------------

func TestPublishOptions(t *testing.T) {
	var nilOptions *PublishOptions
	nilOptions.Apply(WithPublishTimeout(time.Second)) // must not panic

	defaults := NewPublishOptions()
	if defaults.Context == nil {
		t.Error("Context should default to a background context")
	}
	if defaults.Timeout != 0 {
		t.Errorf("Timeout = %v, want 0", defaults.Timeout)
	}

	called := false
	o := NewPublishOptions(
		WithPublishContext(context.Background()),
		WithPublishTimeout(3*time.Second),
		WithPublishAsync(func(error) {}),
		WithPublishRetries(5),
		WithPublishRequiredAcks(2),
		WithPublishBodyCodec("proto"),
		WithPublishCallback(func(error) { called = true }),
	)
	if o.Timeout != 3*time.Second || o.Retries != 5 || o.RequiredAcks != 2 || o.BodyCodec != "proto" {
		t.Errorf("publish options = %+v", o)
	}
	if !o.Async || o.Callback == nil {
		t.Error("async publish should be configured")
	}
	o.Callback(nil)
	if !called {
		t.Error("callback option should be reachable")
	}
}

func TestSubscribeOptions(t *testing.T) {
	var nilOptions *SubscribeOptions
	nilOptions.Apply(DisableAutoAck()) // must not panic

	defaults := NewSubscribeOptions()
	if !defaults.AutoAck {
		t.Error("AutoAck should default to true")
	}
	if defaults.Concurrency != 1 {
		t.Errorf("Concurrency = %d, want 1", defaults.Concurrency)
	}
	if defaults.Context == nil {
		t.Error("Context should default to a background context")
	}

	mw := func(next Handler) Handler { return next }
	o := NewSubscribeOptions(
		WithSubscribeAutoAck(false),
		WithSubscribeQueueName("queue-1"),
		WithSubscribeGroupID("group-1"),
		WithSubscribeContext(context.Background()),
		WithSubscribeConcurrency(4),
		WithSubscribeRetry(3, time.Second),
		WithSubscribeMiddlewares(mw),
	)
	if o.AutoAck {
		t.Error("AutoAck should be disabled")
	}
	if o.Queue != "group-1" {
		t.Errorf("Queue = %q, want group-1 (GroupID aliases QueueName)", o.Queue)
	}
	if o.Concurrency != 4 || o.MaxRetries != 3 || o.RetryDelay != time.Second {
		t.Errorf("subscribe options = %+v", o)
	}
	if len(o.Middlewares) != 1 {
		t.Errorf("middlewares = %d, want 1", len(o.Middlewares))
	}
}

func TestRequestOptions(t *testing.T) {
	var nilOptions *RequestOptions
	nilOptions.Apply(WithRequestTimeout(time.Second)) // must not panic

	defaults := NewRequestOptions()
	if defaults.Context == nil {
		t.Error("Context should default to a background context")
	}
	if defaults.Timeout != 10*time.Second {
		t.Errorf("Timeout = %v, want 10s", defaults.Timeout)
	}

	o := NewRequestOptions(
		WithRequestContext(context.Background()),
		WithRequestTimeout(2*time.Second),
		WithReplyTopic("reply-t"),
		WithRequestBodyCodec("proto"),
	)
	if o.Timeout != 2*time.Second || o.ReplyTopic != "reply-t" || o.BodyCodec != "proto" {
		t.Errorf("request options = %+v", o)
	}
}

// ---------------------------------------------------------------------------
// nopLogger for WithLogger
// ---------------------------------------------------------------------------

type nopLogger struct{}

func (nopLogger) Debug(_ context.Context, _ string, _ ...any) {}
func (nopLogger) Info(_ context.Context, _ string, _ ...any)  {}
func (nopLogger) Warn(_ context.Context, _ string, _ ...any)  {}
func (nopLogger) Error(_ context.Context, _ string, _ ...any) {}
func (nopLogger) Enabled(_ bLogger.Level) bool                { return false }
func (nopLogger) With(_ ...any) bLogger.Logger                { return nopLogger{} }

package websocket

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	ws "github.com/gorilla/websocket"

	_ "github.com/tx7do/go-wind-plugins/encoding/json"

	"github.com/tx7do/go-wind/log"
)

// ---------------------------------------------------------------------------
// test hooks + session helper
// ---------------------------------------------------------------------------

type testHooks struct {
	mu          sync.Mutex
	removed     []*Session
	rawData     []rawCall
	payloadType PayloadType
}

type rawCall struct {
	id   SessionID
	data []byte
}

func (h *testHooks) removeSession(s *Session) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.removed = append(h.removed, s)
}

func (h *testHooks) handleSocketRawData(id SessionID, buf []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rawData = append(h.rawData, rawCall{id, buf})
	return nil
}

func (h *testHooks) getPayloadType() PayloadType { return h.payloadType }

// newTestSession creates a Session backed by a real WebSocket connection over
// an httptest server (hermetic: 127.0.0.1 only). The returned cleanup closes
// both ends.
func newTestSession(t *testing.T, hooks SessionHooks, queries url.Values) (s *Session, client *ws.Conn, cleanup func()) {
	t.Helper()

	serverConnCh := make(chan *ws.Conn, 1)
	up := ws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConnCh <- conn
	}))
	wsURL := strings.Replace(srv.URL, "http", "ws", 1)
	clientConn, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		srv.Close()
		t.Fatalf("dial failed: %v", err)
	}

	select {
	case conn := <-serverConnCh:
		s = NewSession(hooks, conn, queries)
	case <-time.After(2 * time.Second):
		clientConn.Close()
		srv.Close()
		t.Fatal("server-side upgrade never completed")
	}

	return s, clientConn, func() {
		clientConn.Close()
		s.Close()
		srv.Close()
	}
}

// ---------------------------------------------------------------------------
// packet helpers
// ---------------------------------------------------------------------------

func TestBinaryNetPacket_Unmarshal_TooShort(t *testing.T) {
	var p BinaryNetPacket
	if err := p.Unmarshal([]byte{1, 2, 3}); err == nil {
		t.Error("Unmarshal() with fewer than 4 bytes should fail")
	}
}

func TestBinaryNetPacket_EmptyPayload(t *testing.T) {
	p := BinaryNetPacket{Type: 7}
	buf, err := p.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if len(buf) != 4 {
		t.Fatalf("Marshal() = %d bytes, want 4", len(buf))
	}
	var p2 BinaryNetPacket
	if err := p2.Unmarshal(buf); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if p2.Type != 7 || len(p2.Payload) != 0 {
		t.Errorf("Unmarshal() = %+v, want type 7 empty payload", p2)
	}
}

func TestExtractMessageType(t *testing.T) {
	// Binary layout: [4-byte LE type][payload]
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, 42)
	buf = append(buf, "data"...)

	if got := extractMessageType(buf, PayloadTypeBinary); got != 42 {
		t.Errorf("extractMessageType(binary) = %d, want 42", got)
	}

	text, _ := (&TextNetPacket{Type: 9, Payload: "x"}).Marshal()
	if got := extractMessageType(text, PayloadTypeText); got != 9 {
		t.Errorf("extractMessageType(text) = %d, want 9", got)
	}

	// Malformed input degrades to the zero message type (<4 bytes cannot
	// carry a type header).
	if got := extractMessageType([]byte("abc"), PayloadTypeBinary); got != 0 {
		t.Errorf("extractMessageType(short binary) = %d, want 0", got)
	}
	// A 7-byte buffer parses its first 4 bytes as the type header.
	want := NetMessageType(binary.LittleEndian.Uint32([]byte("garb")))
	if got := extractMessageType([]byte("garbage"), PayloadTypeBinary); got != want {
		t.Errorf("extractMessageType(7 bytes) = %d, want the LE header value %d", got, want)
	}
}

func TestMessageHandlerData_Create(t *testing.T) {
	// NOTE: Create() is not nil-receiver safe (it dereferences h.Creator
	// without a nil-receiver check and panics on a nil *MessageHandlerData);
	// this test only exercises non-nil receivers.
	h := &MessageHandlerData{}
	if got := h.Create(); got != nil {
		t.Errorf("Create() without Creator = %v, want nil", got)
	}

	h.Creator = func() any { return map[string]int{"n": 1} }
	got, ok := h.Create().(map[string]int)
	if !ok || got["n"] != 1 {
		t.Errorf("Create() with Creator = %#v, want the created payload", h.Create())
	}
}

// ---------------------------------------------------------------------------
// default marshal/unmarshal paths
// ---------------------------------------------------------------------------

func TestDefaultMarshalUnmarshal_Binary(t *testing.T) {
	srv := NewServer(":0", WithPayloadType(PayloadTypeBinary))

	const MsgTypeChat NetMessageType = 11
	type chatMsg struct {
		Text string `json:"text"`
	}

	received := make(chan *chatMsg, 1)
	srv.RegisterMessageHandler(MsgTypeChat,
		func(_ SessionID, payload MessagePayload) error {
			received <- payload.(*chatMsg)
			return nil
		},
		func() any { return &chatMsg{} },
	)

	buf, err := srv.marshalMessage(MsgTypeChat, chatMsg{Text: "hi"})
	if err != nil {
		t.Fatalf("marshalMessage() error = %v", err)
	}
	if binary.LittleEndian.Uint32(buf[:4]) != uint32(MsgTypeChat) {
		t.Errorf("binary header type = %d, want %d", binary.LittleEndian.Uint32(buf[:4]), MsgTypeChat)
	}

	handler, payload, err := srv.defaultUnmarshalNetPacket(buf)
	if err != nil {
		t.Fatalf("defaultUnmarshalNetPacket() error = %v", err)
	}
	if handler == nil {
		t.Fatal("handler not returned")
	}
	if _, isTyped := payload.(*chatMsg); !isTyped {
		t.Fatalf("payload type = %T, want *chatMsg", payload)
	}

	if err := handler.Handler("sid", payload); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	select {
	case msg := <-received:
		if msg.Text != "hi" {
			t.Errorf("received = %+v, want text hi", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("typed handler never received the payload")
	}
}

func TestDefaultMarshalUnmarshal_Text(t *testing.T) {
	srv := NewServer(":0", WithPayloadType(PayloadTypeText))

	const MsgTypeChat NetMessageType = 12
	srv.RegisterMessageHandler(MsgTypeChat, func(SessionID, MessagePayload) error { return nil }, nil)

	buf, err := srv.marshalMessage(MsgTypeChat, "payload-string")
	if err != nil {
		t.Fatalf("marshalMessage() error = %v", err)
	}

	var pkt TextNetPacket
	if err := pkt.Unmarshal(buf); err != nil {
		t.Fatalf("packet unmarshal error = %v", err)
	}
	if pkt.Type != MsgTypeChat {
		t.Errorf("packet type = %d, want %d", pkt.Type, MsgTypeChat)
	}
	// The payload is the JSON encoding of the string, so it arrives quoted.
	if pkt.Payload != `"payload-string"` {
		t.Errorf("packet payload = %q, want the quoted JSON string", pkt.Payload)
	}

	// Raw-bytes creator: payload comes back as the raw payload bytes.
	_, payload, err := srv.defaultUnmarshalNetPacket(buf)
	if err != nil {
		t.Fatalf("defaultUnmarshalNetPacket() error = %v", err)
	}
	if got, ok := payload.([]byte); !ok || string(got) != `"payload-string"` {
		t.Errorf("payload = %#v, want raw payload bytes", payload)
	}
}

func TestDefaultUnmarshal_UnknownHandler(t *testing.T) {
	srv := NewServer(":0")
	buf, _ := (&TextNetPacket{Type: 999, Payload: "x"}).Marshal()
	if _, _, err := srv.defaultUnmarshalNetPacket(buf); err == nil {
		t.Error("expected an error for an unregistered message type")
	}
}

func TestDefaultUnmarshal_Malformed(t *testing.T) {
	srv := NewServer(":0", WithPayloadType(PayloadTypeBinary))
	if _, _, err := srv.defaultUnmarshalNetPacket([]byte{1}); err == nil {
		t.Error("expected an error for a truncated binary packet")
	}

	srvText := NewServer(":0", WithPayloadType(PayloadTypeText))
	if _, _, err := srvText.defaultUnmarshalNetPacket([]byte("not json")); err == nil {
		t.Error("expected an error for a malformed text packet")
	}
}

func TestDefaultMarshal_UnknownPayloadType(t *testing.T) {
	srv := NewServer(":0")
	srv.payloadType = PayloadType(99)
	if _, err := srv.marshalMessage(1, "x"); err == nil {
		t.Error("expected an error for an unknown payload type")
	}
}

// ---------------------------------------------------------------------------
// raw data handler wiring
// ---------------------------------------------------------------------------

func TestDefaultHandleSocketRawData(t *testing.T) {
	srv := NewServer(":0", WithPayloadType(PayloadTypeText))

	const MsgTypeChat NetMessageType = 21
	handled := make(chan SessionID, 1)
	srv.RegisterMessageHandler(MsgTypeChat,
		func(sid SessionID, _ MessagePayload) error {
			handled <- sid
			return nil
		},
		nil,
	)

	buf, _ := srv.marshalMessage(MsgTypeChat, "x")
	if err := srv.handleSocketRawData("session-1", buf); err != nil {
		t.Fatalf("handleSocketRawData() error = %v", err)
	}
	select {
	case sid := <-handled:
		if sid != "session-1" {
			t.Errorf("handler session id = %q, want session-1", sid)
		}
	case <-time.After(time.Second):
		t.Fatal("handler not invoked")
	}

	// Unregistered type surfaces the dispatch error.
	if err := srv.handleSocketRawData("s", []byte(`{"type":1234,"payload":"x"}`)); err == nil {
		t.Error("expected an error for an unregistered message type")
	}
	// Malformed data surfaces the unmarshal error.
	if err := srv.handleSocketRawData("s", []byte("junk")); err == nil {
		t.Error("expected an error for malformed data")
	}
}

func TestWithSocketRawDataHandler_Override(t *testing.T) {
	called := make(chan []byte, 1)
	srv := NewServer(":0", WithSocketRawDataHandler(func(_ SessionID, buf []byte) error {
		called <- buf
		return nil
	}))

	// The override must bypass type dispatch entirely.
	if err := srv.handleSocketRawData("sid", []byte("anything")); err != nil {
		t.Fatalf("handleSocketRawData() error = %v", err)
	}
	select {
	case got := <-called:
		if string(got) != "anything" {
			t.Errorf("override got %q, want anything", got)
		}
	case <-time.After(time.Second):
		t.Fatal("override handler not invoked")
	}
}

// ---------------------------------------------------------------------------
// generic typed handler registration
// ---------------------------------------------------------------------------

type typedPayload struct {
	N int `json:"n"`
}

func TestRegisterServerMessageHandler_Typed(t *testing.T) {
	srv := NewServer(":0")

	const MsgTypeTyped NetMessageType = 31
	got := make(chan *typedPayload, 1)
	RegisterServerMessageHandler[typedPayload](srv, MsgTypeTyped,
		func(_ SessionID, p *typedPayload) error {
			got <- p
			return nil
		},
	)

	h := srv.GetMessageHandler(MsgTypeTyped)
	if h == nil || h.Creator == nil {
		t.Fatal("typed registration did not install a creator")
	}

	payload := h.Create()
	if _, ok := payload.(*typedPayload); !ok {
		t.Fatalf("Creator() = %T, want *typedPayload", payload)
	}
	if err := h.Handler("sid", payload); err != nil {
		t.Fatalf("Handler() error = %v", err)
	}
	select {
	case p := <-got:
		if p.N != 0 {
			t.Errorf("payload = %+v, want zero value", p)
		}
	case <-time.After(time.Second):
		t.Fatal("typed handler not invoked")
	}

	// Wrong payload type is rejected with an error.
	if err := h.Handler("sid", "wrong"); err == nil {
		t.Error("expected an error for a wrong payload type")
	}
}

// ---------------------------------------------------------------------------
// options
// ---------------------------------------------------------------------------

func TestOptions_Additional(t *testing.T) {
	srv := NewServer(":0",
		WithEnableCompression(true),
		WithHandshakeTimeout(3*time.Second),
		WithTimeout(30*time.Second),
		WithMiddleware(func(next http.Handler) http.Handler { return next }),
	)
	if !srv.upgrader.EnableCompression {
		t.Error("WithEnableCompression did not enable compression")
	}
	if srv.upgrader.HandshakeTimeout != 3*time.Second {
		t.Errorf("HandshakeTimeout = %v, want 3s", srv.upgrader.HandshakeTimeout)
	}
	if srv.timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", srv.timeout)
	}
	if len(srv.middlewares) != 1 {
		t.Errorf("middlewares = %d, want 1", len(srv.middlewares))
	}

	// WithPath ignores empty values.
	srvEmpty := NewServer(":0", WithPath(""))
	if srvEmpty.path != "/ws" {
		t.Errorf("path = %q, want the default /ws preserved for empty input", srvEmpty.path)
	}

	// WithCodec resolves codecs through the encoding registry.
	srvCodec := NewServer(":0", WithCodec("json"))
	if srvCodec.codec == nil {
		t.Error("WithCodec(json) did not set the codec")
	}
	srvEmptyCodec := NewServer(":0", WithCodec(""))
	if srvEmptyCodec.codec == nil {
		t.Error("empty codec name must keep the default json codec")
	}
}

func TestWithChannelBufferSize(t *testing.T) {
	orig := channelBufSize
	t.Cleanup(func() { channelBufSize = orig })

	NewServer(":0", WithChannelBufferSize(7))
	if channelBufSize != 7 {
		t.Errorf("channelBufSize = %d, want 7", channelBufSize)
	}
}

func TestWithCheckOrigin(t *testing.T) {
	srv := NewServer(":0", WithCheckOrigin("https://allowed.example.com"))

	req := &http.Request{Header: http.Header{}}
	req.Header.Set("Origin", "https://allowed.example.com")
	if !srv.upgrader.CheckOrigin(req) {
		t.Error("matching origin should be allowed")
	}

	req.Header.Set("Origin", "https://evil.example.com")
	if srv.upgrader.CheckOrigin(req) {
		t.Error("non-matching origin should be denied")
	}
}

func TestWithMessageMarshalerOverride(t *testing.T) {
	srv := NewServer(":0",
		WithMessageMarshaler(func(mt NetMessageType, _ MessagePayload) ([]byte, error) {
			return []byte{byte(mt)}, nil
		}),
	)

	buf, err := srv.marshalMessage(5, nil)
	if err != nil || len(buf) != 1 || buf[0] != 5 {
		t.Errorf("custom marshaler = %v (err %v), want [5]", buf, err)
	}
}

// TestWithMessageUnmarshaler_Consulted is the flipped regression test for the
// former wiring gap: the unmarshaler configured via WithMessageUnmarshaler is
// now consulted on the inbound decode path, with the default decoder only used
// as the fallback when none is configured.
func TestWithMessageUnmarshaler_Consulted(t *testing.T) {
	const raw = "not-a-valid-packet" // the default decoder cannot decode this

	received := make(chan MessagePayload, 1)
	srv := NewServer(":0",
		WithMessageUnmarshaler(func(buf []byte) (*MessageHandlerData, MessagePayload, error) {
			return &MessageHandlerData{
				Handler: func(_ SessionID, payload MessagePayload) error {
					received <- payload
					return nil
				},
			}, string(buf), nil
		}),
	)
	if srv.netPacketUnmarshaler == nil {
		t.Fatal("WithMessageUnmarshaler should store the unmarshaler")
	}

	// The custom unmarshaler owns the decode: it returns its own handler data
	// and payload, which must reach dispatch even though the default decoder
	// would reject the buffer.
	if err := srv.handleSocketRawData("sid", []byte(raw)); err != nil {
		t.Fatalf("custom unmarshaler was not consulted: %v", err)
	}

	select {
	case payload := <-received:
		if payload != MessagePayload(raw) {
			t.Errorf("payload = %v, want the custom unmarshaler output %q", payload, raw)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("handler not invoked; custom unmarshaler was not consulted")
	}

	// Without a custom unmarshaler the default decoder stays in charge: an
	// unregistered type must still be rejected.
	fallback := NewServer(":0")
	if err := fallback.handleSocketRawData("sid", []byte(`{"type":1234,"payload":"x"}`)); err == nil {
		t.Error("expected the default decode path to reject the unregistered type")
	}
}

func TestWithTLS_ValidPair(t *testing.T) {
	certPath, keyPath := writeTestCert(t, t.TempDir())

	var srv *Server
	assertNoPanic(t, func() { srv = NewServer(":0", WithTLS(certPath, keyPath)) })
	if srv.tlsConfig == nil || len(srv.tlsConfig.Certificates) != 1 {
		t.Fatal("WithTLS did not load the certificate pair")
	}
	if got := srv.Endpoint(); !strings.HasPrefix(got, "wss://") {
		t.Errorf("Endpoint() = %q, want wss scheme", got)
	}
}

func TestWithTLS_MissingFiles_Panics(t *testing.T) {
	assertPanic(t, func() {
		NewServer(":0", WithTLS("/nonexistent/cert.pem", "/nonexistent/key.pem"))
	})
}

// ---------------------------------------------------------------------------
// session lifecycle over a real (localhost) websocket
// ---------------------------------------------------------------------------

func TestNewSession_NilConn_Panics(t *testing.T) {
	assertPanic(t, func() { NewSession(&testHooks{}, nil, nil) })
}

func TestSession_SendReceiveAndClose(t *testing.T) {
	hooks := &testHooks{payloadType: PayloadTypeText}
	s, _, cleanup := newTestSession(t, hooks, url.Values{"user": {"alice"}})
	defer cleanup()

	if s.SessionID() == "" {
		t.Error("SessionID should be non-empty")
	}
	if got := s.Queries().Get("user"); got != "alice" {
		t.Errorf("Queries() user = %q, want alice", got)
	}
	if s.Conn() == nil {
		t.Error("Conn() should return the underlying connection")
	}

	s.Listen()
	s.Close()
	s.Wait()

	hooks.mu.Lock()
	removed := len(hooks.removed)
	hooks.mu.Unlock()
	if removed != 1 {
		t.Errorf("removeSession calls = %d, want 1", removed)
	}

	// Close is idempotent.
	s.Close()
	// Send after close is a no-op (must not block or panic).
	s.SendMessage([]byte("late"))
}

func TestSession_TextDelivery(t *testing.T) {
	hooks := &testHooks{payloadType: PayloadTypeText}
	s, client, cleanup := newTestSession(t, hooks, nil)
	defer cleanup()

	s.Listen()

	s.SendMessage([]byte(`{"n":1}`))
	_ = client.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, data, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if string(data) != `{"n":1}` {
		t.Errorf("client received %q, want {\"n\":1}", data)
	}
}

func TestSession_BinaryDelivery(t *testing.T) {
	hooks := &testHooks{payloadType: PayloadTypeBinary}
	s, client, cleanup := newTestSession(t, hooks, nil)
	defer cleanup()

	s.Listen()

	s.SendMessage([]byte{0xde, 0xad})
	_ = client.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, data, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if len(data) != 2 || data[0] != 0xde || data[1] != 0xad {
		t.Errorf("client received %v, want [222 173]", data)
	}
}

func TestSession_InboundRouting(t *testing.T) {
	hooks := &testHooks{payloadType: PayloadTypeText}
	s, client, cleanup := newTestSession(t, hooks, nil)
	defer cleanup()

	s.Listen()

	if err := client.WriteMessage(ws.TextMessage, []byte("inbound")); err != nil {
		t.Fatalf("client write failed: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		hooks.mu.Lock()
		n := len(hooks.rawData)
		hooks.mu.Unlock()
		if n > 0 {
			hooks.mu.Lock()
			call := hooks.rawData[0]
			hooks.mu.Unlock()
			if call.id != s.SessionID() {
				t.Errorf("routed session id = %q, want %q", call.id, s.SessionID())
			}
			if string(call.data) != "inbound" {
				t.Errorf("routed data = %q, want inbound", call.data)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("inbound data was never routed to the hooks")
}

func TestSession_PingControlFrames(t *testing.T) {
	hooks := &testHooks{payloadType: PayloadTypeText}
	s, _, cleanup := newTestSession(t, hooks, nil)
	defer cleanup()

	// Control frame senders must succeed against a live connection.
	if err := s.sendPingMessage("ping"); err != nil {
		t.Errorf("sendPingMessage() error = %v", err)
	}
	if err := s.sendPongMessage("pong"); err != nil {
		t.Errorf("sendPongMessage() error = %v", err)
	}
	if err := s.sendTextMessage("text"); err != nil {
		t.Errorf("sendTextMessage() error = %v", err)
	}
	if err := s.sendBinaryMessage([]byte{1}); err != nil {
		t.Errorf("sendBinaryMessage() error = %v", err)
	}

	// After the connection reference is cleared, the senders no-op.
	s.connMu.Lock()
	s.conn = nil
	s.connMu.Unlock()
	if err := s.sendPingMessage("x"); err != nil {
		t.Errorf("sendPingMessage() after close = %v, want nil", err)
	}
}

// ---------------------------------------------------------------------------
// session manager
// ---------------------------------------------------------------------------

type recordingObserver struct {
	mu        sync.Mutex
	added     []*Session
	removedOb []*Session
}

func (o *recordingObserver) OnSessionAdded(s *Session) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.added = append(o.added, s)
}

func (o *recordingObserver) OnSessionRemoved(s *Session) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.removedOb = append(o.removedOb, s)
}

func TestSessionManager_Lifecycle(t *testing.T) {
	obs := &recordingObserver{}
	sm := NewSessionManager(obs)
	if sm.Count() != 0 {
		t.Fatal("new manager should be empty")
	}

	hooks := &testHooks{payloadType: PayloadTypeText}
	s1, _, c1 := newTestSession(t, hooks, nil)
	defer c1()
	s2, _, c2 := newTestSession(t, hooks, nil)
	defer c2()

	// Nil adds/removes are no-ops.
	sm.AddSession(nil)
	sm.RemoveSession(nil)

	sm.AddSession(s1)
	sm.AddSession(s2)
	if sm.Count() != 2 {
		t.Fatalf("Count() = %d, want 2", sm.Count())
	}

	if sm.GetSession(s1.SessionID()) != s1 {
		t.Error("GetSession() did not return the stored session")
	}
	if sm.GetSession("missing") != nil {
		t.Error("GetSession() for an unknown id should be nil")
	}

	seen := map[SessionID]bool{}
	sm.RangeSessions(func(id SessionID, s *Session) bool {
		seen[id] = true
		return true
	})
	if !seen[s1.SessionID()] || !seen[s2.SessionID()] {
		t.Errorf("RangeSessions saw %v, want both sessions", seen)
	}

	// Early stop halts iteration.
	visited := 0
	sm.RangeSessions(func(SessionID, *Session) bool {
		visited++
		return false
	})
	if visited != 1 {
		t.Errorf("RangeSessions early stop visited %d, want 1", visited)
	}

	sm.RemoveSession(s1)
	if sm.Count() != 1 {
		t.Errorf("Count() after remove = %d, want 1", sm.Count())
	}

	sm.Clean()
	if sm.Count() != 0 {
		t.Errorf("Count() after Clean = %d, want 0", sm.Count())
	}

	obs.mu.Lock()
	added, removed := len(obs.added), len(obs.removedOb)
	obs.mu.Unlock()
	// Only RemoveSession notifies the observer: Session.Close() notifies the
	// session hooks, so Clean() above does not produce a second removed event.
	if added != 2 || removed != 1 {
		t.Errorf("observer added=%d removed=%d, want 2/1", added, removed)
	}
}

// ---------------------------------------------------------------------------
// server: wsHandler wiring (token injection, origin checks)
// ---------------------------------------------------------------------------

func TestServer_WSTokenInjection(t *testing.T) {
	queriesCh := make(chan url.Values, 1)
	srv := NewServer("127.0.0.1:0",
		WithPath("/ws"),
		WithSocketConnectHandler(func(_ SessionID, q url.Values, connect bool) {
			if connect {
				queriesCh <- q
			}
		}),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	addr := srv.listener.Addr().String()
	header := http.Header{}
	header.Set("Sec-WebSocket-Protocol", "secret-token")

	conn, _, err := ws.DefaultDialer.Dial("ws://"+addr+"/ws", header)
	if err != nil {
		t.Fatalf("Dial with protocol token failed: %v", err)
	}
	defer conn.Close()

	select {
	case q := <-queriesCh:
		if q.Get("token") != "secret-token" {
			t.Errorf("injected token = %q, want secret-token", q.Get("token"))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("connect handler never fired")
	}
}

func TestServer_WSNoTokenInjection(t *testing.T) {
	queriesCh := make(chan url.Values, 1)
	srv := NewServer("127.0.0.1:0",
		WithPath("/ws"),
		WithInjectTokenToQuery(false, "unused"),
		WithSocketConnectHandler(func(_ SessionID, q url.Values, connect bool) {
			if connect {
				queriesCh <- q
			}
		}),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	addr := srv.listener.Addr().String()
	header := http.Header{}
	header.Set("Sec-WebSocket-Protocol", "secret-token")

	conn, _, err := ws.DefaultDialer.Dial("ws://"+addr+"/ws", header)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	select {
	case q := <-queriesCh:
		if q.Get("token") != "" && q.Get("unused") != "" {
			t.Errorf("token unexpectedly injected: %v", q)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("connect handler never fired")
	}
}

func TestServer_WithCheckOrigin_Denied(t *testing.T) {
	srv := NewServer("127.0.0.1:0", WithCheckOrigin("https://allowed.example.com"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	addr := srv.listener.Addr().String()
	header := http.Header{}
	header.Set("Origin", "https://evil.example.com")

	if _, _, err := ws.DefaultDialer.Dial("ws://"+addr+"/ws", header); err == nil {
		t.Error("dialing with a denied Origin should fail the handshake")
	}
}

func TestServer_WithCheckOrigin_Allowed(t *testing.T) {
	srv := NewServer("127.0.0.1:0", WithCheckOrigin("https://allowed.example.com"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	addr := srv.listener.Addr().String()
	header := http.Header{}
	header.Set("Origin", "https://allowed.example.com")

	conn, _, err := ws.DefaultDialer.Dial("ws://"+addr+"/ws", header)
	if err != nil {
		t.Fatalf("dialing with the allowed Origin failed: %v", err)
	}
	conn.Close()
}

// ---------------------------------------------------------------------------
// server: errors and endpoints
// ---------------------------------------------------------------------------

func TestServer_Start_BadAddr(t *testing.T) {
	srv := NewServer("invalid-addr-no-port")
	if err := srv.Start(context.Background()); err == nil {
		t.Fatal("Start() expected an error for an unparseable address")
	}
}

func TestServer_Start_BusyPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listener setup failed: %v", err)
	}
	defer ln.Close()

	srv := NewServer(ln.Addr().String())
	if err := srv.Start(context.Background()); err == nil {
		t.Fatal("Start() expected an error when the port is already bound")
	}
}

func TestServer_StopRunning(t *testing.T) {
	srv := NewServer("127.0.0.1:0")

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	if err := srv.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after Stop")
	}
}

func TestServer_Endpoint_NoPort(t *testing.T) {
	srv := NewServer("kettle")
	if got := srv.Endpoint(); got != "ws://kettle" {
		t.Errorf("Endpoint() = %q, want ws://kettle", got)
	}
}

func TestServer_SendRawMessage_UnknownSession(t *testing.T) {
	srv := NewServer(":0")
	if err := srv.SendRawMessage("missing", []byte("x")); err == nil {
		t.Error("SendRawMessage() for an unknown session should fail")
	}
	if err := srv.SendMessage("missing", 1, "x"); err == nil {
		t.Error("SendMessage() for an unknown session should fail")
	}
}

func TestServer_BroadcastMarshalError(t *testing.T) {
	srv := NewServer(":0")
	srv.payloadType = PayloadType(99)
	// Must not panic; the marshal error is logged and swallowed.
	srv.Broadcast(1, "x")
}

func TestServer_SessionCount(t *testing.T) {
	srv := NewServer(":0")
	if srv.SessionCount() != 0 {
		t.Error("SessionCount() should start at 0")
	}
}

// ---------------------------------------------------------------------------
// logger
// ---------------------------------------------------------------------------

type wsRecordingLogger struct {
	mu      sync.Mutex
	entries []string
}

func (l *wsRecordingLogger) Debug(_ context.Context, msg string, _ ...any) { l.record(msg) }
func (l *wsRecordingLogger) Info(_ context.Context, msg string, _ ...any)  { l.record(msg) }
func (l *wsRecordingLogger) Warn(_ context.Context, msg string, _ ...any)  { l.record(msg) }
func (l *wsRecordingLogger) Error(_ context.Context, msg string, _ ...any) { l.record(msg) }
func (l *wsRecordingLogger) Enabled(_ log.Level) bool                      { return true }
func (l *wsRecordingLogger) With(_ ...any) log.Logger                      { return l }

func (l *wsRecordingLogger) record(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, msg)
}

func (l *wsRecordingLogger) messages() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.entries...)
}

func TestLogger_Injection(t *testing.T) {
	rec := &wsRecordingLogger{}
	SetLogger(rec)
	t.Cleanup(func() { SetLogger(nil) })

	LogDebug("d")
	LogDebugf("d%d", 0)
	LogInfo("i")
	LogInfof("i%d", 1)
	LogWarn("w")
	LogWarnf("w%d", 2)
	LogError("e")
	LogErrorf("e%v", 3)
	LogFatal("f")
	LogFatalf("f%d", 4)

	msgs := rec.messages()
	if len(msgs) != 10 {
		t.Fatalf("logged messages = %v, want 10", msgs)
	}
	for _, m := range msgs {
		if !strings.Contains(m, "[websocket]") {
			t.Errorf("message %q missing the [websocket] prefix", m)
		}
	}

	SetLogger(nil)
	LogInfo("after reset")
}

// ---------------------------------------------------------------------------
// shared helpers
// ---------------------------------------------------------------------------

func writeTestCert(t *testing.T, dir string) (certPath, keyPath string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("key generation failed: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("certificate creation failed: %v", err)
	}
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("key marshalling failed: %v", err)
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("writing cert failed: %v", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatalf("writing key failed: %v", err)
	}
	return certPath, keyPath
}

func assertNoPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()
	fn()
}

func assertPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected a panic")
		}
	}()
	fn()
}

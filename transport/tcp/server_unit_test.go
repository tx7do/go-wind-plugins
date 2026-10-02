package tcp

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used by the default codec
)

// ---------------------------------------------------------------------------
// Session
// ---------------------------------------------------------------------------

// recordingHooks captures the SessionHooks callbacks a Session invokes.
type recordingHooks struct {
	removed   chan *Session
	rawDataCh chan []byte
	failWith  error
}

func newRecordingHooks() *recordingHooks {
	return &recordingHooks{
		removed:   make(chan *Session, 1),
		rawDataCh: make(chan []byte, 1),
	}
}

func (h *recordingHooks) removeSession(s *Session) {
	select {
	case h.removed <- s:
	default:
	}
}

func (h *recordingHooks) handleSocketRawData(_ SessionID, buf []byte) error {
	select {
	case h.rawDataCh <- buf:
	default:
	}
	return h.failWith
}

func TestNewSession(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	sess := NewSession(server, nil)
	if sess == nil {
		t.Fatal("NewSession returned nil")
	}
	if sess.SessionID() == "" {
		t.Error("SessionID is empty, want a generated identifier")
	}
	if sess.Conn() != server {
		t.Error("Conn() did not return the wrapped connection")
	}

	// Every session must get a unique identifier.
	other := NewSession(client, nil)
	if other.SessionID() == sess.SessionID() {
		t.Error("two sessions share the same identifier")
	}
}

func TestNewSessionNilConnPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("NewSession with nil conn did not panic")
		}
	}()
	NewSession(nil, nil)
}

func TestSessionSendMessageRoundTrip(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	sess := NewSession(server, nil)
	sess.Listen()
	defer sess.Close()

	payload := []byte("ping")

	if err := client.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline failed: %v", err)
	}
	sess.SendMessage(payload)

	got, err := ReadFrame(client)
	if err != nil {
		t.Fatalf("ReadFrame on peer failed: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("peer received %q, want %q", got, payload)
	}
}

func TestSessionReadPumpDispatchesToHooks(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	hooks := newRecordingHooks()
	sess := NewSession(server, hooks)
	sess.Listen()
	defer sess.Close()

	payload := []byte(`{"hello":"world"}`)
	if err := client.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetWriteDeadline failed: %v", err)
	}
	if err := WriteFrame(client, payload); err != nil {
		t.Fatalf("WriteFrame failed: %v", err)
	}

	select {
	case got := <-hooks.rawDataCh:
		if !bytes.Equal(got, payload) {
			t.Errorf("hook received %q, want %q", got, payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handleSocketRawData not invoked within 2s")
	}
}

func TestSessionCloseNotifiesHooksOnce(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	hooks := newRecordingHooks()
	sess := NewSession(server, hooks)
	sess.Listen()

	sess.Close()
	sess.Close() // idempotent

	select {
	case got := <-hooks.removed:
		if got != sess {
			t.Error("removeSession hook received a different session")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("removeSession hook not invoked within 2s")
	}

	// After Close the connection is released and sending is a no-op.
	if sess.Conn() != nil {
		t.Error("Conn() not nil after Close")
	}
	// Must not block or panic on a closed session.
	sess.SendMessage([]byte("dropped"))
}

func TestSessionWait(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	sess := NewSession(server, nil)
	sess.Listen()

	sess.Close()

	done := make(chan struct{})
	go func() {
		sess.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Wait did not return within 2s after Close")
	}
}

// ---------------------------------------------------------------------------
// SessionManager
// ---------------------------------------------------------------------------

type recordingObserver struct {
	added   chan *Session
	removed chan *Session
}

func newRecordingObserver() *recordingObserver {
	return &recordingObserver{added: make(chan *Session, 4), removed: make(chan *Session, 4)}
}

func (o *recordingObserver) OnSessionAdded(s *Session)   { o.added <- s }
func (o *recordingObserver) OnSessionRemoved(s *Session) { o.removed <- s }

func TestSessionManagerLifecycle(t *testing.T) {
	obs := newRecordingObserver()
	sm := NewSessionManager(obs)

	if got := sm.count(); got != 0 {
		t.Fatalf("initial count = %d, want 0", got)
	}

	// nil sessions are ignored entirely.
	sm.addSession(nil)
	sm.removeSession(nil)
	if got := sm.count(); got != 0 {
		t.Fatalf("count after nil ops = %d, want 0", got)
	}

	c1, s1 := net.Pipe()
	defer c1.Close()
	sess1 := NewSession(s1, nil)
	sm.addSession(sess1)

	select {
	case got := <-obs.added:
		if got != sess1 {
			t.Error("observer received wrong session on add")
		}
	case <-time.After(time.Second):
		t.Fatal("OnSessionAdded not invoked")
	}

	if got := sm.count(); got != 1 {
		t.Fatalf("count after add = %d, want 1", got)
	}
	if got := sm.getSession(sess1.SessionID()); got != sess1 {
		t.Error("getSession did not return the stored session")
	}
	if got := sm.getSession("missing"); got != nil {
		t.Errorf("getSession for unknown id = %v, want nil", got)
	}

	visited := 0
	sm.rangeSessions(func(id SessionID, s *Session) bool {
		visited++
		if s != sess1 || id != sess1.SessionID() {
			t.Errorf("rangeSessions visited (%s, %v), want the stored session", id, s)
		}
		return false
	})
	if visited != 1 {
		t.Errorf("rangeSessions visited %d sessions, want 1", visited)
	}

	sm.removeSession(sess1)
	select {
	case got := <-obs.removed:
		if got != sess1 {
			t.Error("observer received wrong session on remove")
		}
	case <-time.After(time.Second):
		t.Fatal("OnSessionRemoved not invoked")
	}
	if got := sm.count(); got != 0 {
		t.Fatalf("count after remove = %d, want 0", got)
	}

	// Clean empties the map without touching the observer.
	sm.Clean()
	if got := sm.count(); got != 0 {
		t.Errorf("count after Clean = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// Server construction, handlers, and message marshaling
// ---------------------------------------------------------------------------

func TestNewServerDefaults(t *testing.T) {
	srv := NewServer()

	if srv.network != "tcp" {
		t.Errorf("default network = %q, want %q", srv.network, "tcp")
	}
	if srv.address != ":0" {
		t.Errorf("default address = %q, want %q", srv.address, ":0")
	}
	if srv.timeout != 0 {
		t.Errorf("default timeout = %v, want 0 (idle timeouts disabled)", srv.timeout)
	}
	if srv.codec == nil {
		t.Error("default codec is nil, want the JSON codec")
	}
	if srv.sessionManager == nil {
		t.Error("sessionManager is nil after NewServer")
	}
	if srv.netPacketMarshaler == nil || srv.netPacketUnmarshaler == nil {
		t.Error("default packet marshaler/unmarshaler not installed")
	}
	if srv.socketRawDataHandler == nil {
		t.Error("default raw data handler not installed")
	}
	if got := srv.SessionCount(); got != 0 {
		t.Errorf("initial SessionCount = %d, want 0", got)
	}
}

func TestServerOptions(t *testing.T) {
	srv := NewServer(
		WithAddress("127.0.0.1:9300"),
		WithTimeout(3*time.Second),
		WithCodec("json"),
		WithChannelBufferSize(64),
		WithReceiveBufferSize(4096),
		WithSocketConnectHandler(func(SessionID, bool) {}),
		WithSocketRawDataHandler(nil), // nil must be ignored, keeping the default
	)

	if srv.address != "127.0.0.1:9300" {
		t.Errorf("address = %q, want %q", srv.address, "127.0.0.1:9300")
	}
	if srv.timeout != 3*time.Second {
		t.Errorf("timeout = %v, want 3s", srv.timeout)
	}
	if srv.codec == nil {
		t.Error("codec is nil after WithCodec")
	}
	if srv.socketConnectHandler == nil {
		t.Error("socketConnectHandler not applied")
	}
	if srv.socketRawDataHandler == nil {
		t.Error("socketRawDataHandler became nil after WithSocketRawDataHandler(nil)")
	}
}

func TestServerMessageHandlerRegistry(t *testing.T) {
	srv := NewServer()

	handler := func(SessionID, NetMessagePayload) error { return nil }
	srv.RegisterMessageHandler(1, handler, nil)
	srv.RegisterMessageHandler(1, func(SessionID, NetMessagePayload) error {
		t.Error("duplicate registration replaced the original handler")
		return nil
	}, nil)

	got, err := srv.GetMessageHandler(1)
	if err != nil {
		t.Fatalf("GetMessageHandler(1) error: %v", err)
	}
	if got.Handler == nil {
		t.Error("registered handler is nil")
	}

	if _, err := srv.GetMessageHandler(999); err == nil {
		t.Error("GetMessageHandler for unknown type returned nil error")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want it to mention 'not found'", err)
	}

	srv.DeregisterMessageHandler(1)
	if _, err := srv.GetMessageHandler(1); err == nil {
		t.Error("GetMessageHandler after Deregister returned nil error")
	}
}

// TestServerPacketRoundTrip exercises the default marshal/unmarshal path with
// a typed handler registered through the generic helper.
func TestServerPacketRoundTrip(t *testing.T) {
	srv := NewServer()

	received := make(chan *ChatMessage, 1)
	RegisterServerMessageHandler(srv, MessageTypeChat, func(_ SessionID, msg *ChatMessage) error {
		received <- msg
		return nil
	})

	want := &ChatMessage{Type: 7, Sender: "alice", Message: "hello"}
	buf, err := srv.marshalNetPacket(MessageTypeChat, *want)
	if err != nil {
		t.Fatalf("marshalNetPacket error: %v", err)
	}

	handlerData, payload, err := srv.unmarshalNetPacket(buf)
	if err != nil {
		t.Fatalf("unmarshalNetPacket error: %v", err)
	}
	if handlerData == nil || handlerData.Handler == nil {
		t.Fatal("unmarshalNetPacket returned no handler")
	}
	if err := handlerData.Handler("session-1", payload); err != nil {
		t.Fatalf("dispatch error: %v", err)
	}

	select {
	case got := <-received:
		if got.Sender != want.Sender || got.Message != want.Message || got.Type != want.Type {
			t.Errorf("dispatched message = %+v, want %+v", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler not invoked within 2s")
	}

	// A payload of the wrong type must be rejected by the generic wrapper.
	badHandler, badPayload, err := srv.unmarshalNetPacket(buf)
	if err != nil {
		t.Fatalf("unmarshalNetPacket error: %v", err)
	}
	if err := badHandler.Handler("session-1", struct{}{}); err == nil {
		t.Error("handler accepted a mismatched payload type")
	} else if !strings.Contains(err.Error(), "invalid payload struct type") {
		t.Errorf("error = %q, want it to mention the invalid payload type", err)
	}
	_ = badPayload

	// Garbage on the wire fails at the packet header instead.
	if _, _, err := srv.unmarshalNetPacket([]byte{0x01}); err == nil {
		t.Error("unmarshalNetPacket on truncated input returned nil error")
	}
}

func TestServerSendWithoutSession(t *testing.T) {
	srv := NewServer()

	if err := srv.SendRawData("nobody", []byte("x")); err == nil {
		t.Error("SendRawData for unknown session returned nil error")
	} else if !strings.Contains(err.Error(), "session not found") {
		t.Errorf("error = %q, want it to mention the missing session", err)
	}

	if err := srv.SendMessage("nobody", 1, "payload"); err == nil {
		t.Error("SendMessage for unknown session returned nil error")
	}

	// Broadcasting to zero sessions is a valid no-op.
	srv.Broadcast(1, "payload")
	srv.BroadcastRawData([]byte("x"))
}

func TestServerEndpointWithoutListener(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    string
	}{
		{"host and port", "127.0.0.1:9300", "tcp://127.0.0.1:9300"},
		{"empty host becomes localhost", ":9300", "tcp://localhost:9300"},
		{"0.0.0.0 becomes localhost", "0.0.0.0:9300", "tcp://localhost:9300"},
		{"no port falls back to raw address", "localhost", "tcp://localhost"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := NewServer(WithAddress(tc.address))
			if got := srv.Endpoint(); got != tc.want {
				t.Errorf("Endpoint() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestServerStopBeforeStart(t *testing.T) {
	srv := NewServer()
	if err := srv.Stop(context.Background()); err != nil {
		t.Errorf("Stop before Start returned error: %v", err)
	}
}

func TestServerStartTwice(t *testing.T) {
	srv := NewServer(WithAddress("127.0.0.1:0"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start is blocking, so both calls run in goroutines: whichever installs
	// the listener blocks until cancellation, the other observes running and
	// returns immediately as a no-op.
	done := make(chan error, 2)
	go func() { done <- srv.Start(ctx) }()
	go func() { done <- srv.Start(ctx) }()

	// Let one Start install the listener and the other take the no-op path.
	time.Sleep(100 * time.Millisecond)

	cancel()
	for i := 0; i < 2; i++ {
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Start returned error after cancel: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Start did not return within 5s after cancellation")
		}
	}
}

// ---------------------------------------------------------------------------
// Loopback: server + raw net.Conn client exchange one message
// ---------------------------------------------------------------------------

// TestServerLoopbackExchange starts the server on a random local port, dials
// it with a plain net.Conn, exchanges one chat message through the frame
// protocol, and verifies the echo comes back. Every wait is deadline-bounded.
func TestServerLoopbackExchange(t *testing.T) {
	srv := NewServer(WithAddress("127.0.0.1:0"))

	echoed := make(chan string, 1)
	RegisterServerMessageHandler(srv, MessageTypeChat, func(sessionId SessionID, msg *ChatMessage) error {
		echoed <- msg.Message
		// Reply to the sender through the session.
		return srv.SendMessage(sessionId, MessageTypeChat, &ChatMessage{Message: "echo:" + msg.Message})
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- srv.Start(ctx) }()

	// Endpoint() takes the state lock, so polling it is race-free; it reports
	// the resolved port once the listener is bound.
	var addr string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		endpoint := srv.Endpoint()
		if strings.HasPrefix(endpoint, "tcp://127.0.0.1:") && !strings.HasSuffix(endpoint, ":0") {
			addr = strings.TrimPrefix(endpoint, "tcp://")
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if addr == "" || strings.HasSuffix(addr, ":0") {
		t.Fatal("server did not bind within 5s")
	}

	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial %s failed: %v", addr, err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("SetDeadline failed: %v", err)
	}

	// Wait for the server to register the session, then send one message.
	sessDeadline := time.Now().Add(2 * time.Second)
	for srv.SessionCount() == 0 {
		if time.Now().After(sessDeadline) {
			t.Fatal("server did not register the session within 2s")
		}
		time.Sleep(5 * time.Millisecond)
	}

	buf, err := srv.marshalNetPacket(MessageTypeChat, &ChatMessage{Message: "ping"})
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if err := WriteFrame(conn, buf); err != nil {
		t.Fatalf("WriteFrame failed: %v", err)
	}

	select {
	case got := <-echoed:
		if got != "ping" {
			t.Errorf("server handler received %q, want %q", got, "ping")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server handler not invoked within 3s")
	}

	frame, err := ReadFrame(conn)
	if err != nil {
		t.Fatalf("ReadFrame for echo failed: %v", err)
	}
	var reply NetPacket
	if err := reply.Unmarshal(frame); err != nil {
		t.Fatalf("reply Unmarshal failed: %v", err)
	}
	if reply.Type != MessageTypeChat {
		t.Errorf("reply type = %d, want %d", reply.Type, MessageTypeChat)
	}
	if !bytes.Contains(reply.Payload, []byte(`"echo:ping"`)) {
		t.Errorf("reply payload = %s, want it to contain the echo", reply.Payload)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Start returned error after cancel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return within 5s after cancellation")
	}
}

// TestClientLoopbackExchange connects a real Client to a live Server and
// verifies a bidirectional exchange through the client's handler dispatch.
func TestClientLoopbackExchange(t *testing.T) {
	srv := NewServer(WithAddress("127.0.0.1:0"))

	RegisterServerMessageHandler(srv, MessageTypeChat, func(sessionId SessionID, msg *ChatMessage) error {
		return srv.SendMessage(sessionId, MessageTypeChat, &ChatMessage{Message: "reply:" + msg.Message})
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srvDone := make(chan error, 1)
	go func() { srvDone <- srv.Start(ctx) }()

	var addr string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		endpoint := srv.Endpoint()
		if strings.HasPrefix(endpoint, "tcp://127.0.0.1:") && !strings.HasSuffix(endpoint, ":0") {
			addr = strings.TrimPrefix(endpoint, "tcp://")
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if addr == "" {
		t.Fatal("server did not bind within 5s")
	}

	// Note: the client endpoint handling is quirky (reported as a bug): it
	// parses the address as a URL but dials the same string back. A bare
	// IP:port fails url.Parse (endpoint nil) and a scheme-prefixed URL fails
	// net.Dial; only a letter-leading host:port round-trips, so dial via
	// localhost here.
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q) failed: %v", addr, err)
	}
	cli := NewClient(WithEndpoint(net.JoinHostPort("localhost", port)))
	defer cli.Disconnect()

	if cli.codec == nil {
		t.Error("client default codec is nil, want JSON")
	}
	if cli.timeout != time.Second {
		t.Errorf("client default timeout = %v, want 1s", cli.timeout)
	}

	got := make(chan string, 1)
	RegisterClientMessageHandler(cli, MessageTypeChat, func(msg *ChatMessage) error {
		got <- msg.Message
		return nil
	})

	if err := cli.Connect(); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Sending before Connect would fail; after Connect it must succeed.
	if err := cli.SendMessage(MessageTypeChat, &ChatMessage{Message: "hi"}); err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	select {
	case msg := <-got:
		if msg != "reply:hi" {
			t.Errorf("client received %q, want %q", msg, "reply:hi")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("client handler not invoked within 3s")
	}

	cli.Disconnect()

	cancel()
	select {
	case err := <-srvDone:
		if err != nil {
			t.Errorf("server Start returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server Start did not return within 5s after cancellation")
	}
}

func TestClientNotConnectedErrors(t *testing.T) {
	cli := NewClient()

	if err := cli.SendRawData([]byte("x")); err == nil {
		t.Error("SendRawData before Connect returned nil error")
	} else if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("error = %q, want it to mention the disconnected state", err)
	}
	if err := cli.SendMessage(1, "payload"); err == nil {
		t.Error("SendMessage before Connect returned nil error")
	}
}

func TestClientHandlerRegistry(t *testing.T) {
	cli := NewClient()

	called := false
	cli.RegisterMessageHandler(1, func(NetMessagePayload) error { called = true; return nil }, nil)
	cli.RegisterMessageHandler(1, func(NetMessagePayload) error {
		t.Error("duplicate registration replaced the original handler")
		return nil
	}, nil)

	// Marshal a packet with the client codec and dispatch it locally.
	buf, err := cli.codec.Marshal(&ChatMessage{Message: "x"})
	if err != nil {
		t.Fatalf("codec marshal failed: %v", err)
	}
	packet := NetPacket{Type: 1, Payload: buf}
	wire, err := packet.Marshal()
	if err != nil {
		t.Fatalf("packet marshal failed: %v", err)
	}
	if err := cli.messageHandler(wire); err != nil {
		t.Fatalf("messageHandler error: %v", err)
	}
	if !called {
		t.Error("registered handler was not invoked")
	}

	// Unknown type and undecodable input must surface errors.
	if err := cli.messageHandler([]byte{0x00}); err == nil {
		t.Error("messageHandler on truncated input returned nil error")
	}
	unknown := NetPacket{Type: 42, Payload: []byte("{}")}
	wire, err = unknown.Marshal()
	if err != nil {
		t.Fatalf("packet marshal failed: %v", err)
	}
	if err := cli.messageHandler(wire); err == nil {
		t.Error("messageHandler with unknown type returned nil error")
	}

	cli.DeregisterMessageHandler(1)
	if err := cli.messageHandler(wire); err == nil {
		t.Error("messageHandler after Deregister returned nil error")
	}
}

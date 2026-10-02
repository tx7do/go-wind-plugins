package webtransport

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used by the default codec
)

// ---------------------------------------------------------------------------
// Frame protocol
// ---------------------------------------------------------------------------

func TestWriteFrameReadFrameRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
	}{
		{"empty payload", []byte{}},
		{"text payload", []byte("hello frame")},
		{"binary payload", []byte{0x00, 0xFF, 0x10, 0x00, 0xDE, 0xAD, 0xBE, 0xEF}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteFrame(&buf, tc.payload); err != nil {
				t.Fatalf("WriteFrame() error: %v", err)
			}
			if buf.Len() != frameLengthSize+len(tc.payload) {
				t.Fatalf("frame length = %d, want %d", buf.Len(), frameLengthSize+len(tc.payload))
			}

			got, err := ReadFrame(&buf)
			if err != nil {
				t.Fatalf("ReadFrame() error: %v", err)
			}
			if !bytes.Equal(got, tc.payload) {
				t.Errorf("ReadFrame() = %v, want %v", got, tc.payload)
			}
		})
	}
}

func TestWriteFrameLengthPrefixIsLittleEndian(t *testing.T) {
	var buf bytes.Buffer
	payload := []byte("abc")
	if err := WriteFrame(&buf, payload); err != nil {
		t.Fatalf("WriteFrame() error: %v", err)
	}
	wire := buf.Bytes()
	if got := binary.LittleEndian.Uint32(wire); got != uint32(len(payload)) {
		t.Errorf("length prefix = %d, want %d", got, len(payload))
	}
}

func TestWriteFrameTooLarge(t *testing.T) {
	var buf bytes.Buffer
	err := WriteFrame(&buf, make([]byte, maxFrameSize+1))
	if err == nil {
		t.Fatal("WriteFrame with oversized payload returned nil error")
	}
	if buf.Len() != 0 {
		t.Errorf("WriteFrame wrote %d bytes before rejecting, want 0", buf.Len())
	}
}

func TestReadFrameErrors(t *testing.T) {
	oversized := make([]byte, 4)
	binary.LittleEndian.PutUint32(oversized, maxFrameSize+1)

	tests := []struct {
		name  string
		input []byte
	}{
		{"oversized length prefix", oversized},
		{"header cut short", []byte{0x02, 0x00}},
		{"payload cut short", []byte{0x05, 0x00, 0x00, 0x00, 'a', 'b'}},
		{"no data", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ReadFrame(bytes.NewReader(tc.input)); err == nil {
				t.Error("ReadFrame returned nil error, want failure")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Message envelope
// ---------------------------------------------------------------------------

func TestMessageMarshalUnmarshal(t *testing.T) {
	in := Message{Type: 42, Body: []byte(`{"k":"v"}`)}
	buf, err := in.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	if len(buf) != 4+len(in.Body) {
		t.Fatalf("wire length = %d, want %d", len(buf), 4+len(in.Body))
	}

	var out Message
	if err := out.Unmarshal(buf); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if out.Type != in.Type {
		t.Errorf("Type = %d, want %d", out.Type, in.Type)
	}
	if !bytes.Equal(out.Body, in.Body) {
		t.Errorf("Body = %s, want %s", out.Body, in.Body)
	}
}

func TestMessageUnmarshalTooShort(t *testing.T) {
	var msg Message
	if err := msg.Unmarshal([]byte{0x01}); err == nil {
		t.Error("Unmarshal on 1-byte input returned nil error, want failure")
	}
}

// ---------------------------------------------------------------------------
// Server: options, handlers, dispatch
// ---------------------------------------------------------------------------

// TestNewServerDefaults constructs the server with zero options (flipped
// regression test: bare NewServer() used to panic because the default s.path
// was empty and mux.HandleFunc("") is an invalid pattern). The default handler
// path is now "/webtransport", the path used across the module's examples.
func TestNewServerDefaults(t *testing.T) {
	srv := NewServer()

	if srv.Server == nil {
		t.Fatal("embedded http3.Server is nil after NewServer")
	}
	if srv.tlsConf == nil {
		t.Error("default TLS config not generated")
	} else if len(srv.tlsConf.Certificates) != 1 {
		t.Errorf("default TLS config has %d certificates, want 1", len(srv.tlsConf.Certificates))
	}
	if srv.timeout != 5*time.Second {
		t.Errorf("default timeout = %v, want 5s", srv.timeout)
	}
	if srv.codec == nil {
		t.Error("default codec is nil, want JSON")
	}
	if got, ok := srv.Server.AdditionalSettings[settingsEnableWebtransport]; !ok || got != 1 {
		t.Errorf("AdditionalSettings[%#x] = %d, want 1 (WebTransport advertised)", settingsEnableWebtransport, got)
	}
	if srv.path != defaultHandlerPath {
		t.Errorf("default handler path = %q, want %q", srv.path, defaultHandlerPath)
	}

	// An explicit WithPath must still win over the default.
	if withPath := NewServer(WithPath("/custom")); withPath.path != "/custom" {
		t.Errorf("WithPath path = %q, want %q", withPath.path, "/custom")
	}
}

func TestServerOptions(t *testing.T) {
	tlsConf := generateTLSConfig(alpnQuicTransport)

	srv := NewServer(
		WithTLSConfig(tlsConf),
		WithAddress("127.0.0.1:9400"),
		WithTimeout(7*time.Second),
		WithMaxIdleTimeout(11*time.Second),
		WithKeepAlivePeriod(3*time.Second),
		WithPath("/wt"),
		WithConnectHandle(func(SessionID, bool) {}),
		WithCodec("json"),
	)

	if srv.tlsConf != tlsConf {
		t.Error("WithTLSConfig was not applied")
	}
	if srv.Addr != "127.0.0.1:9400" {
		t.Errorf("Addr = %q, want %q", srv.Addr, "127.0.0.1:9400")
	}
	if srv.timeout != 7*time.Second {
		t.Errorf("timeout = %v, want 7s", srv.timeout)
	}
	if srv.Server.QUICConfig.MaxIdleTimeout != 11*time.Second {
		t.Errorf("MaxIdleTimeout = %v, want 11s", srv.Server.QUICConfig.MaxIdleTimeout)
	}
	if srv.Server.QUICConfig.KeepAlivePeriod != 3*time.Second {
		t.Errorf("KeepAlivePeriod = %v, want 3s", srv.Server.QUICConfig.KeepAlivePeriod)
	}
	if srv.connectHandler == nil {
		t.Error("connect handler not applied")
	}
	if srv.codec == nil {
		t.Error("codec is nil after WithCodec")
	}
}

func TestServerEndpoint(t *testing.T) {
	// Without WithAddress the embedded http3.Server keeps its ":443" default,
	// so the endpoint resolves to the https default port.
	srv0 := NewServer(WithPath("/webtransport"))
	if got := srv0.Endpoint(); got != "https://localhost:443" {
		t.Errorf("Endpoint with no address = %q, want %q", got, "https://localhost:443")
	}

	srv := NewServer(WithPath("/webtransport"), WithAddress(":9401"))
	if got := srv.Endpoint(); got != "https://localhost:9401" {
		t.Errorf("Endpoint() = %q, want %q", got, "https://localhost:9401")
	}

	srv2 := NewServer(WithPath("/webtransport"), WithAddress("127.0.0.1:9402"))
	if got := srv2.Endpoint(); got != "https://127.0.0.1:9402" {
		t.Errorf("Endpoint() = %q, want %q", got, "https://127.0.0.1:9402")
	}
}

func TestServerUseMiddleware(t *testing.T) {
	srv := NewServer(WithPath("/webtransport"))
	if len(srv.middlewares) != 0 {
		t.Fatalf("middlewares = %d, want 0 before Use", len(srv.middlewares))
	}
	srv.Use(func(next http.Handler) http.Handler { return next })
	srv.Use(func(next http.Handler) http.Handler { return next })
	if len(srv.middlewares) != 2 {
		t.Errorf("middlewares = %d, want 2", len(srv.middlewares))
	}
}

func TestServerMessageHandlerRegistry(t *testing.T) {
	srv := NewServer(WithPath("/webtransport"))

	called := 0
	srv.RegisterMessageHandler(1, func(SessionID, MessagePayload) error {
		called++
		return nil
	}, nil)
	// Duplicate registration is ignored.
	srv.RegisterMessageHandler(1, func(SessionID, MessagePayload) error {
		t.Error("duplicate registration replaced the original handler")
		return nil
	}, nil)

	buf, err := srv.marshalMessage(1, map[string]string{"x": "y"})
	if err != nil {
		t.Fatalf("marshalMessage error: %v", err)
	}
	if err := srv.messageHandler(SessionID(1), buf); err != nil {
		t.Fatalf("messageHandler error: %v", err)
	}
	if called != 1 {
		t.Errorf("handler called %d times, want 1", called)
	}

	// Unknown type must fail with the dedicated error.
	unknown, err := (&Message{Type: 99, Body: []byte("{}")}).Marshal()
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	if err := srv.messageHandler(SessionID(1), unknown); err == nil || err.Error() != "message handler not found" {
		t.Errorf("unknown type error = %v, want 'message handler not found'", err)
	}

	srv.DeregisterMessageHandler(1)
	if err := srv.messageHandler(SessionID(1), buf); err == nil {
		t.Error("messageHandler after Deregister returned nil error")
	}
}

// ---------------------------------------------------------------------------
// Server: CONNECT request validation (no QUIC needed)
// ---------------------------------------------------------------------------

func TestAddHandlerRejectsWrongMethod(t *testing.T) {
	srv := NewServer(WithPath("/webtransport"))

	req := httptest.NewRequest(http.MethodGet, "http://example.com/wt", nil)
	rec := httptest.NewRecorder()
	srv.addHandler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if !strings.Contains(rec.Body.String(), "expected CONNECT request") {
		t.Errorf("body = %q, want the CONNECT hint", rec.Body.String())
	}
}

func TestAddHandlerRejectsWrongProtocol(t *testing.T) {
	srv := NewServer(WithPath("/webtransport"))

	req := httptest.NewRequest(http.MethodConnect, "http://example.com/wt", nil)
	req.Proto = "not-webtransport"
	rec := httptest.NewRecorder()
	srv.addHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("CONNECT with wrong proto status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "invalid protocol") {
		t.Errorf("body = %q, want the invalid protocol hint", rec.Body.String())
	}
}

func TestAddHandlerRejectsNonHijackableWriter(t *testing.T) {
	srv := NewServer(WithPath("/webtransport"))

	// A recorder implements Flusher but not http3.HTTPStreamer, so the
	// request passes validation and fails at the hijack step.
	req := httptest.NewRequest(http.MethodConnect, "http://example.com/wt", nil)
	req.Proto = protocolHeader
	rec := httptest.NewRecorder()
	srv.addHandler(rec, req)

	if len(srv.sessions) != 0 {
		t.Errorf("sessions recorded %d entries, want 0 (hijack failed)", len(srv.sessions))
	}
}

// ---------------------------------------------------------------------------
// Server: send paths without live sessions
// ---------------------------------------------------------------------------

func TestServerSendWithoutSessions(t *testing.T) {
	srv := NewServer(WithPath("/webtransport"))

	if err := srv.SendRawData(SessionID(42), []byte("x")); err == nil {
		t.Error("SendRawData for unknown session returned nil error")
	}
	// Broadcasting to zero sessions is a no-op.
	if err := srv.BroadcastRawData([]byte("x")); err != nil {
		t.Errorf("BroadcastRawData with no sessions returned error: %v", err)
	}
}

// TestServerStopBeforeStart covers the shutdown path when the server was
// never listening; it must return promptly (bounded by the context).
func TestServerStopBeforeStart(t *testing.T) {
	srv := NewServer(WithPath("/webtransport"))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- srv.Stop(ctx) }()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Stop on unstarted server returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return within 5s")
	}
}

// ---------------------------------------------------------------------------
// utils.go
// ---------------------------------------------------------------------------

func TestEqualASCIIFold(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"", "", true},
		{"Example.COM", "example.com", true},
		{"example.com", "example.org", false},
		{"abc", "abcd", false},
		{"localhost", "LOCALHOST", true},
	}
	for _, tc := range tests {
		if got := equalASCIIFold(tc.a, tc.b); got != tc.want {
			t.Errorf("equalASCIIFold(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestCheckSameOrigin(t *testing.T) {
	tests := []struct {
		name   string
		origin string
		host   string
		want   bool
	}{
		{"no origin header", "", "example.com", true},
		{"same host", "http://example.com", "example.com", true},
		{"case-insensitive host", "http://EXAMPLE.com", "example.com", true},
		{"different host", "http://evil.com", "example.com", false},
		{"unparseable origin", "://bad", "example.com", false},
		{"origin with port vs no port", "http://example.com:8080", "example.com", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://receiver/", nil)
			req.Host = tc.host
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if got := checkSameOrigin(req); got != tc.want {
				t.Errorf("checkSameOrigin() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNewTlsConfig(t *testing.T) {
	// Without key/cert files the config is empty but valid.
	cfg := NewTlsConfig("", "", "")
	if cfg == nil {
		t.Fatal("NewTlsConfig with empty paths returned nil")
	}
	if len(cfg.Certificates) != 0 {
		t.Errorf("certificates = %d, want 0 without key/cert files", len(cfg.Certificates))
	}

	// Missing key/cert files on disk must return nil, not panic.
	if cfg := NewTlsConfig("./missing.key", "./missing.crt", ""); cfg != nil {
		t.Error("NewTlsConfig with unreadable files returned a config, want nil")
	}
}

func TestNewCertPool(t *testing.T) {
	if _, err := NewCertPool("./missing-ca.crt"); err == nil {
		t.Error("NewCertPool with a missing file returned nil error")
	}

	// Generate a real self-signed certificate and load it back.
	cfg := generateTLSConfig(alpnQuicTransport)
	if cfg == nil || len(cfg.Certificates) != 1 {
		t.Fatal("generateTLSConfig produced no certificate")
	}
	der := cfg.Certificates[0].Certificate[0]

	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.pem")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(caPath, pemBytes, 0o600); err != nil {
		t.Fatalf("write temp CA failed: %v", err)
	}

	pool, err := NewCertPool(caPath)
	if err != nil {
		t.Fatalf("NewCertPool error: %v", err)
	}
	// Subjects returns the DER-encoded subject names of the pooled certs.
	subjects := pool.Subjects() //nolint:staticcheck // only verification hook available
	if len(subjects) != 1 {
		t.Errorf("cert pool holds %d subjects, want 1", len(subjects))
		return
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("generated certificate does not parse: %v", err)
	}
	if !bytes.Equal(subjects[0], parsed.RawSubject) {
		t.Errorf("pool subject = %x, want %x", subjects[0], parsed.RawSubject)
	}
}

// ---------------------------------------------------------------------------
// Client: options and pure helpers
// ---------------------------------------------------------------------------

func TestNewClientDefaults(t *testing.T) {
	cli := NewClient()

	if cli.transport == nil {
		t.Fatal("client transport is nil")
	}
	if cli.timeout != 5*time.Second {
		t.Errorf("default timeout = %v, want 5s", cli.timeout)
	}
	if cli.codec == nil {
		t.Error("default codec is nil, want JSON")
	}
	if cli.tlsConf == nil || !cli.tlsConf.InsecureSkipVerify {
		t.Error("default TLS config must skip verification (self-signed servers)")
	}
	if len(cli.tlsConf.NextProtos) != 1 || cli.tlsConf.NextProtos[0] != alpnQuicTransport {
		t.Errorf("NextProtos = %v, want [%s]", cli.tlsConf.NextProtos, alpnQuicTransport)
	}
	if !cli.transport.EnableDatagrams {
		t.Error("transport datagrams not enabled")
	}
	if got, ok := cli.transport.AdditionalSettings[settingsEnableWebtransport]; !ok || got != 1 {
		t.Errorf("AdditionalSettings[%#x] = %d, want 1", settingsEnableWebtransport, got)
	}
	if cli.transport.QUICConfig == nil || cli.transport.QUICConfig.MaxIncomingStreams != 100 {
		t.Error("QUIC MaxIncomingStreams not defaulted to 100")
	}
}

func TestClientOptions(t *testing.T) {
	tlsConf := &tls.Config{}

	cli := NewClient(
		WithClientTLSConfig(tlsConf),
		WithEndpoint("https://localhost:9443/wt"),
		WithClientTimeout(9*time.Second),
		WithClientCodec("json"),
		WithClientMaxIdleTimeout(21*time.Second),
		WithClientKeepAlivePeriod(4*time.Second),
	)

	if cli.tlsConf != tlsConf {
		t.Error("WithClientTLSConfig was not applied")
	}
	if cli.url != "https://localhost:9443/wt" {
		t.Errorf("url = %q, want %q", cli.url, "https://localhost:9443/wt")
	}
	if cli.timeout != 9*time.Second {
		t.Errorf("timeout = %v, want 9s", cli.timeout)
	}
	if cli.transport.QUICConfig.MaxIdleTimeout != 21*time.Second {
		t.Errorf("MaxIdleTimeout = %v, want 21s", cli.transport.QUICConfig.MaxIdleTimeout)
	}
	if cli.transport.QUICConfig.KeepAlivePeriod != 4*time.Second {
		t.Errorf("KeepAlivePeriod = %v, want 4s", cli.transport.QUICConfig.KeepAlivePeriod)
	}
	// The custom TLS config has no ALPN; init must fill in the h3 proto.
	if len(cli.tlsConf.NextProtos) != 1 || cli.tlsConf.NextProtos[0] != alpnQuicTransport {
		t.Errorf("NextProtos = %v, want [%s]", cli.tlsConf.NextProtos, alpnQuicTransport)
	}
}

func TestClientNotConnectedErrors(t *testing.T) {
	cli := NewClient()

	if err := cli.SendRawData([]byte("x")); err == nil {
		t.Error("SendRawData before Connect returned nil error")
	}
	if err := cli.SendMessage(1, "payload"); err == nil {
		t.Error("SendMessage before Connect returned nil error")
	}
	if err := cli.Disconnect(); err != nil {
		t.Errorf("Disconnect before Connect returned error: %v", err)
	}
}

func TestClientMessageDispatch(t *testing.T) {
	cli := NewClient()

	received := make(chan string, 1)
	cli.RegisterMessageHandler(1, func(payload MessagePayload) error {
		received <- string(payload.([]byte))
		return nil
	}, nil)
	// Duplicate registration is ignored.
	cli.RegisterMessageHandler(1, func(MessagePayload) error {
		t.Error("duplicate registration replaced the original handler")
		return nil
	}, nil)

	buf, err := cli.codec.Marshal(map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("codec marshal failed: %v", err)
	}
	wire, err := (&Message{Type: 1, Body: buf}).Marshal()
	if err != nil {
		t.Fatalf("message marshal failed: %v", err)
	}
	if err := cli.messageHandler(wire); err != nil {
		t.Fatalf("messageHandler error: %v", err)
	}

	select {
	case got := <-received:
		if !bytes.Contains([]byte(got), []byte("k")) {
			t.Errorf("dispatched payload %q missing the map body", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler not invoked within 2s")
	}

	// Unknown type and broken envelope must fail.
	unknown, err := (&Message{Type: 77, Body: []byte("{}")}).Marshal()
	if err != nil {
		t.Fatalf("message marshal failed: %v", err)
	}
	if err := cli.messageHandler(unknown); err == nil {
		t.Error("messageHandler with unknown type returned nil error")
	}
	if err := cli.messageHandler([]byte{0x01}); err == nil {
		t.Error("messageHandler on truncated envelope returned nil error")
	}

	cli.DeregisterMessageHandler(1)
	if err := cli.messageHandler(wire); err == nil {
		t.Error("messageHandler after Deregister returned nil error")
	}
}

func TestClientNewWebTransportRequest(t *testing.T) {
	cli := NewClient(WithEndpoint("https://localhost:9443/wt"))

	u, err := url.Parse(cli.url)
	if err != nil {
		t.Fatalf("parse url failed: %v", err)
	}
	req, err := cli.newWebTransportRequest(u)
	if err != nil {
		t.Fatalf("newWebTransportRequest error: %v", err)
	}
	if req.Method != http.MethodConnect {
		t.Errorf("method = %q, want CONNECT", req.Method)
	}
	if req.Proto != protocolHeader {
		t.Errorf("proto = %q, want %q", req.Proto, protocolHeader)
	}
	if got := req.Header.Get(webTransportDraftOfferHeaderKey); got != "1" {
		t.Errorf("draft offer header = %q, want 1", got)
	}
	if req.Host != "localhost:9443" {
		t.Errorf("host = %q, want localhost:9443", req.Host)
	}
}

// ---------------------------------------------------------------------------
// HandlerData helper
// ---------------------------------------------------------------------------

func TestHandlerDataCreate(t *testing.T) {
	withBinder := HandlerData{Binder: func() any { return &struct{ A int }{} }}
	if withBinder.Create() == nil {
		t.Error("Create with binder returned nil")
	}

	withoutBinder := HandlerData{}
	if withoutBinder.Create() != nil {
		t.Error("Create without binder returned non-nil")
	}
}

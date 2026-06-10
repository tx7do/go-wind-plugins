package codec

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tx7do/go-wind-plugins/encoding"
	_ "github.com/tx7do/go-wind-plugins/encoding/json"
	_ "github.com/tx7do/go-wind-plugins/encoding/xml"
)

// ---------------------------------------------------------------------------
// MIME mapping
// ---------------------------------------------------------------------------

func TestCodecNameFromMIME_KnownTypes(t *testing.T) {
	tests := []struct {
		mime string
		want string
	}{
		{"application/json", "json"},
		{"application/xml", "xml"},
		{"text/xml", "xml"},
		{"application/protobuf", "proto"},
		{"application/x-yaml", "yaml"},
		{"application/cbor", "cbor"},
	}
	for _, tc := range tests {
		if got := codecNameFromMIME(tc.mime); got != tc.want {
			t.Errorf("codecNameFromMIME(%q) = %q, want %q", tc.mime, got, tc.want)
		}
	}
}

func TestCodecNameFromMIME_WithCharset(t *testing.T) {
	if got := codecNameFromMIME("application/json; charset=utf-8"); got != "json" {
		t.Errorf("got %q, want json", got)
	}
}

func TestCodecNameFromMIME_UnknownFallsBack(t *testing.T) {
	if got := codecNameFromMIME("application/x-weird"); got != "json" {
		t.Errorf("got %q, want json fallback", got)
	}
}

func TestRegisterMimeType(t *testing.T) {
	RegisterMimeType("application/x-custom", "json")
	if got := codecNameFromMIME("application/x-custom"); got != "json" {
		t.Errorf("got %q, want json", got)
	}
}

// ---------------------------------------------------------------------------
// Middleware — resolves codec from Content-Type
// ---------------------------------------------------------------------------

func TestMiddleware_ResolvesCodec(t *testing.T) {
	var resolved encoding.Codec
	mw := Middleware()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resolved = FromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if resolved == nil {
		t.Fatal("expected codec in context")
	}
	if resolved.Name() != "json" {
		t.Errorf("codec name = %q, want json", resolved.Name())
	}
}

func TestMiddleware_DefaultFallback(t *testing.T) {
	var resolved encoding.Codec
	mw := Middleware()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resolved = FromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// No Content-Type header
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if resolved == nil {
		t.Fatal("expected fallback codec")
	}
}

// ---------------------------------------------------------------------------
// ReadBody — unmarshal request body
// ---------------------------------------------------------------------------

type testPayload struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestReadBody_JSON(t *testing.T) {
	mw := Middleware()

	var got testPayload
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := ReadBody(r, &got); err != nil {
			t.Errorf("ReadBody: %v", err)
		}
	}))

	body := `{"name":"alice","age":30}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got.Name != "alice" || got.Age != 30 {
		t.Errorf("got %+v", got)
	}
}

func TestReadBody_BodyRestoredForDownstream(t *testing.T) {
	mw := Middleware()

	var bodyContent string
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p testPayload
		_ = ReadBody(r, &p)
		// Read body again — should still be available
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		bodyContent = string(buf[:n])
	}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"bob"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !strings.Contains(bodyContent, "bob") {
		t.Errorf("body should still be readable after ReadBody, got %q", bodyContent)
	}
}

func TestReadBody_WithoutMiddleware(t *testing.T) {
	var p testPayload
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := ReadBody(r, &p)
		if err == nil {
			t.Error("expected ErrCodecNotFound")
		}
	})

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
}

// ---------------------------------------------------------------------------
// Respond — marshal response with correct Content-Type
// ---------------------------------------------------------------------------

func TestRespond_JSON(t *testing.T) {
	mw := Middleware()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Respond(w, r, http.StatusOK, testPayload{Name: "alice", Age: 30})
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var resp testPayload
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Name != "alice" || resp.Age != 30 {
		t.Errorf("got %+v", resp)
	}
}

func TestRespond_XML(t *testing.T) {
	mw := Middleware()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Respond(w, r, http.StatusOK, testPayload{Name: "bob", Age: 25})
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Content-Type", "application/xml")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "application/xml" {
		t.Errorf("Content-Type = %q, want application/xml", ct)
	}

	// Body should be XML, not JSON
	bodyStr := rec.Body.String()
	if !strings.Contains(bodyStr, "<Name>bob</Name>") && !strings.Contains(bodyStr, "<testPayload") {
		// XML marshalling of struct may vary — just check it's not JSON
		if strings.HasPrefix(strings.TrimSpace(bodyStr), "{") {
			t.Errorf("expected XML, got JSON: %s", bodyStr)
		}
	}
}

// ---------------------------------------------------------------------------
// FromContext — nil safety
// ---------------------------------------------------------------------------

func TestFromContext_Nil(t *testing.T) {
	if c := FromContext(nil); c != nil {
		t.Error("expected nil from nil context")
	}
}

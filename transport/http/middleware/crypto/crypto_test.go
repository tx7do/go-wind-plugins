package crypto

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	utilsCrypto "github.com/tx7do/go-utils/crypto"
)

// newTestCipher returns an AES cipher for testing.
func newTestCipher(t *testing.T) *utilsCrypto.AESCipher {
	t.Helper()
	key := []byte("1234567890abcdef")
	return utilsCrypto.NewAESCipher(key, nil)
}

// ---------------------------------------------------------------------------
// Request decryption
// ---------------------------------------------------------------------------

func TestMiddleware_DecryptsRequest(t *testing.T) {
	cipher := newTestCipher(t)

	plaintext := []byte(`{"name":"alice"}`)
	ciphertext, err := cipher.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	var receivedBody string
	mw := Middleware(cipher)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])
	}))

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(ciphertext))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if receivedBody != string(plaintext) {
		t.Errorf("handler received %q, want %q", receivedBody, plaintext)
	}
}

func TestMiddleware_EmptyBody(t *testing.T) {
	cipher := newTestCipher(t)

	handlerCalled := false
	mw := Middleware(cipher)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !handlerCalled {
		t.Error("handler should be called for empty body")
	}
}

// ---------------------------------------------------------------------------
// Response encryption
// ---------------------------------------------------------------------------

func TestMiddleware_EncryptsResponse(t *testing.T) {
	cipher := newTestCipher(t)

	plaintext := []byte(`{"status":"ok"}`)

	mw := Middleware(cipher)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(plaintext)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Response body should be encrypted (different from plaintext)
	if rec.Body.String() == string(plaintext) {
		t.Error("response should be encrypted, got plaintext")
	}

	// Decrypt and verify
	decrypted, err := cipher.Decrypt(rec.Body.Bytes())
	if err != nil {
		t.Fatalf("Decrypt response: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

// ---------------------------------------------------------------------------
// Full round-trip: decrypt request + encrypt response
// ---------------------------------------------------------------------------

func TestMiddleware_RoundTrip(t *testing.T) {
	cipher := newTestCipher(t)

	reqPlain := []byte(`{"input":"hello"}`)
	respPlain := []byte(`{"output":"world"}`)
	reqCipher, _ := cipher.Encrypt(reqPlain)

	mw := Middleware(cipher)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read decrypted request
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		received := buf[:n]

		if string(received) != string(reqPlain) {
			t.Errorf("handler received %q, want %q", received, reqPlain)
		}

		w.WriteHeader(http.StatusOK)
		w.Write(respPlain)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(reqCipher))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Verify encrypted response decrypts correctly
	decrypted, err := cipher.Decrypt(rec.Body.Bytes())
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(decrypted) != string(respPlain) {
		t.Errorf("decrypted response = %q, want %q", decrypted, respPlain)
	}
}

// ---------------------------------------------------------------------------
// Skip function
// ---------------------------------------------------------------------------

func TestMiddleware_SkipFunc(t *testing.T) {
	cipher := newTestCipher(t)

	plaintext := []byte("should pass through unchanged")
	var received []byte

	mw := Middleware(cipher, WithSkipFunc(func(r *http.Request) bool {
		return r.URL.Path == "/health"
	}))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		received = buf[:n]
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", bytes.NewReader(plaintext))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Body should be plaintext (not decrypted)
	if string(received) != string(plaintext) {
		t.Errorf("skip path should not decrypt, got %q", received)
	}
}

// ---------------------------------------------------------------------------
// Disable encrypt / disable decrypt
// ---------------------------------------------------------------------------

func TestMiddleware_DecryptOnly(t *testing.T) {
	cipher := newTestCipher(t)

	plaintext := []byte(`{"data":"test"}`)
	ciphertext, _ := cipher.Encrypt(plaintext)

	mw := Middleware(cipher, WithEncryptResponse(false))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("plain response"))
	}))

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(ciphertext))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Response should be plaintext (not encrypted)
	if rec.Body.String() != "plain response" {
		t.Errorf("response should be plaintext, got %q", rec.Body.String())
	}
}

func TestMiddleware_EncryptOnly(t *testing.T) {
	cipher := newTestCipher(t)

	mw := Middleware(cipher, WithDecryptRequest(false))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		w.Write(buf[:n])
	}))

	plaintext := []byte("passthrough")
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(plaintext))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Request body should pass through as plaintext (not decrypted)
	// Response should be encrypted
	decrypted, err := cipher.Decrypt(rec.Body.Bytes())
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

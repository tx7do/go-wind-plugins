// Package crypto provides an HTTP middleware that transparently encrypts and
// decrypts request/response bodies using a [crypto.Cipher] from the
// [security/crypto] package.
//
// The middleware intercepts the request body, decrypts it before passing it
// to the next handler, and encrypts the response body before sending it to
// the client. This allows handlers to work with plaintext data while the
// transport layer handles encryption.
//
// It should be placed BEFORE the codec middleware so that the codec
// receives decrypted plaintext:
//
//	srv.Use(
//	    crypto.Middleware(cipher),   // decrypt body → encrypt response
//	    codec.Middleware(),           // unmarshal plaintext → marshal plaintext
//	)
//
// Usage:
//
//	import (
//	    "github.com/tx7do/go-wind-plugins/transport/http/middleware/crypto"
//	    utilsCrypto "github.com/tx7do/go-utils/crypto"
//	)
//
//	key := []byte("1234567890abcdef")
//	cipher := utilsCrypto.NewAESCipher(key, nil)
//	srv.Use(crypto.Middleware(cipher))
package crypto

import (
	"bytes"
	"io"
	"net/http"

	secCrypto "github.com/tx7do/go-wind-plugins/security/crypto"
	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
)

// Option configures the crypto middleware.
type Option func(*options)

type options struct {
	// skipFunc returns true to skip encryption/decryption for a request.
	// This is useful for health checks or public endpoints.
	skipFunc func(r *http.Request) bool

	// encryptResponse controls whether response bodies are encrypted.
	// Defaults to true.
	encryptResponse bool

	// decryptRequest controls whether request bodies are decrypted.
	// Defaults to true.
	decryptRequest bool
}

// WithSkipFunc sets a function that returns true to skip encryption/decryption
// for matching requests (e.g. health checks, public endpoints).
func WithSkipFunc(fn func(r *http.Request) bool) Option {
	return func(o *options) { o.skipFunc = fn }
}

// WithEncryptResponse enables or disables response body encryption.
func WithEncryptResponse(enabled bool) Option {
	return func(o *options) { o.encryptResponse = enabled }
}

// WithDecryptRequest enables or disables request body decryption.
func WithDecryptRequest(enabled bool) Option {
	return func(o *options) { o.decryptRequest = enabled }
}

// Middleware returns a [httpPlugin.Middleware] that transparently decrypts
// request bodies and encrypts response bodies.
//
// The cipher must implement [secCrypto.Cipher] (Encrypt + Decrypt + Name).
// Typically this is obtained from [github.com/tx7do/go-utils/crypto] or the
// registry [secCrypto.GetCipher].
func Middleware(cipher secCrypto.Cipher, opts ...Option) httpPlugin.Middleware {
	cfg := &options{
		encryptResponse: true,
		decryptRequest:  true,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.skipFunc != nil && cfg.skipFunc(r) {
				next.ServeHTTP(w, r)
				return
			}

			// Decrypt request body.
			if cfg.decryptRequest && r.Body != nil {
				ciphertext, err := io.ReadAll(r.Body)
				r.Body.Close()
				if err != nil {
					http.Error(w, "failed to read request body", http.StatusBadRequest)
					return
				}
				if len(ciphertext) > 0 {
					plaintext, err := cipher.Decrypt(ciphertext)
					if err != nil {
						http.Error(w, "failed to decrypt request body", http.StatusBadRequest)
						return
					}
					r.Body = io.NopCloser(bytes.NewReader(plaintext))
					r.ContentLength = int64(len(plaintext))
				}
			}

			// Encrypt response body.
			if cfg.encryptResponse {
				ew := &encryptingWriter{
					ResponseWriter: w,
					cipher:         cipher,
				}
				next.ServeHTTP(ew, r)
				ew.flush()
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ---------------------------------------------------------------------------
// encryptingWriter buffers response bytes and encrypts them on flush.
// ---------------------------------------------------------------------------

type encryptingWriter struct {
	http.ResponseWriter
	cipher secCrypto.Cipher
	buf    bytes.Buffer
	header bool
}

func (ew *encryptingWriter) Write(b []byte) (int, error) {
	return ew.buf.Write(b)
}

func (ew *encryptingWriter) WriteHeader(code int) {
	ew.header = true
	ew.ResponseWriter.WriteHeader(code)
}

func (ew *encryptingWriter) flush() {
	if ew.buf.Len() == 0 {
		return
	}
	encrypted, err := ew.cipher.Encrypt(ew.buf.Bytes())
	if err != nil {
		// Best-effort: write plaintext if encryption fails.
		ew.ResponseWriter.Write(ew.buf.Bytes())
		return
	}
	ew.ResponseWriter.Write(encrypted)
}

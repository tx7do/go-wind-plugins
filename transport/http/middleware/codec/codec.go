// Package codec provides an HTTP middleware that performs automatic content
// negotiation using the [encoding] registry.
//
// The middleware inspects the incoming Content-Type header, resolves it to a
// registered [encoding.Codec], and stores that codec in the request context.
// Handler code can then use the helper functions [FromContext], [ReadBody],
// and [Respond] to unmarshal requests and marshal responses without hardcoding
// a specific serialization format.
//
// Usage:
//
//	import (
//	    "github.com/tx7do/go-wind-plugins/transport/http/middleware/codec"
//	    _ "github.com/tx7do/go-wind-plugins/encoding/json"  // register JSON codec
//	    _ "github.com/tx7do/go-wind-plugins/encoding/xml"   // register XML codec
//	)
//
//	srv.Use(codec.Middleware())
//
//	// In a handler:
//	func(w http.ResponseWriter, r *http.Request) {
//	    var req MyRequest
//	    if err := codec.ReadBody(r, &req); err != nil {
//	        http.Error(w, err.Error(), http.StatusBadRequest)
//	        return
//	    }
//	    resp := process(&req)
//	    codec.Respond(w, r, http.StatusOK, resp)
//	}
package codec

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/tx7do/go-wind-plugins/encoding"
	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
)

// ---------------------------------------------------------------------------
// MIME type → codec name mapping
// ---------------------------------------------------------------------------

// mimeToCodec maps common MIME types to codec names registered via
// [encoding.RegisterCodec]. Extend this map with [WithMimeType] or
// [RegisterMimeType].
var mimeToCodec = map[string]string{
	"application/json":          "json",
	"application/xml":           "xml",
	"text/xml":                  "xml",
	"application/x-yaml":        "yaml",
	"text/yaml":                 "yaml",
	"application/protobuf":      "proto",
	"application/x-protobuf":    "proto",
	"application/avro":          "avro",
	"application/x-msgpack":     "msgpack",
	"application/x-toml":        "toml",
	"text/toml":                 "toml",
	"application/x-gob":         "gob",
	"application/cbor":          "cbor",
	"application/x-cbor":        "cbor",
	"application/x-thrift":      "thrift",
	"application/x-flatbuffers": "flatbuffers",
	"application/bson":          "bson",
}

var (
	mimeMu sync.RWMutex
)

// RegisterMimeType associates a MIME type with a codec name.
// This allows applications to add custom MIME type mappings.
func RegisterMimeType(mime, codecName string) {
	mimeMu.Lock()
	mimeToCodec[strings.ToLower(strings.TrimSpace(mime))] = codecName
	mimeMu.Unlock()
}

// codecNameFromMIME looks up the codec name for a given MIME type.
// Returns "json" as the fallback if the MIME type is not recognised.
func codecNameFromMIME(mime string) string {
	mime = strings.ToLower(strings.TrimSpace(mime))
	// Strip parameters like "; charset=utf-8"
	if idx := strings.IndexByte(mime, ';'); idx >= 0 {
		mime = strings.TrimSpace(mime[:idx])
	}
	mimeMu.RLock()
	name := mimeToCodec[mime]
	mimeMu.RUnlock()
	if name == "" {
		return "json" // safe default
	}
	return name
}

// codecToMime maps codec names to their canonical MIME types.
var codecToMime = map[string]string{
	"json":        "application/json",
	"xml":         "application/xml",
	"yaml":        "application/x-yaml",
	"proto":       "application/protobuf",
	"avro":        "application/avro",
	"msgpack":     "application/x-msgpack",
	"toml":        "application/x-toml",
	"gob":         "application/x-gob",
	"cbor":        "application/cbor",
	"thrift":      "application/x-thrift",
	"flatbuffers": "application/x-flatbuffers",
	"bson":        "application/bson",
}

// mimeFromCodecName returns the canonical MIME type for a codec name.
func mimeFromCodecName(name string) string {
	if mime, ok := codecToMime[name]; ok {
		return mime
	}
	return "application/json"
}

// ---------------------------------------------------------------------------
// Context
// ---------------------------------------------------------------------------

type ctxKey struct{}

// Option configures the codec middleware.
type Option func(*options)

type options struct {
	// defaultCodec overrides the fallback codec name when Content-Type is
	// absent or unrecognised. Defaults to "json".
	defaultCodec string
}

// WithDefaultCodec sets the fallback codec name used when Content-Type is
// absent or unrecognised.
func WithDefaultCodec(name string) Option {
	return func(o *options) { o.defaultCodec = name }
}

// Middleware returns a [httpPlugin.Middleware] that resolves the request's
// Content-Type header to an [encoding.Codec] and stores it in the request
// context.
//
// Handlers retrieve the codec with [FromContext] and use [ReadBody] /
// [Respond] for automatic (un)marshalling.
func Middleware(opts ...Option) httpPlugin.Middleware {
	cfg := &options{
		defaultCodec: "json",
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			codecName := codecNameFromMIME(r.Header.Get("Content-Type"))
			c := encoding.GetCodec(codecName)
			if c == nil {
				c = encoding.GetCodec(cfg.defaultCodec)
			}
			if c == nil {
				http.Error(w, "unsupported content type", http.StatusUnsupportedMediaType)
				return
			}
			ctx := context.WithValue(r.Context(), ctxKey{}, c)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// FromContext returns the [encoding.Codec] stored in the request context.
// Returns nil if the middleware was not registered.
func FromContext(ctx context.Context) encoding.Codec {
	if ctx == nil {
		return nil
	}
	c, _ := ctx.Value(ctxKey{}).(encoding.Codec)
	return c
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// ReadBody reads the request body and unmarshals it into target using the
// codec from the request context.
//
// The request body is consumed and replaced with a new reader so that
// downstream code can still access it if needed.
func ReadBody(r *http.Request, target any) error {
	c := FromContext(r.Context())
	if c == nil {
		return ErrCodecNotFound
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	r.Body.Close()
	// Restore body for potential downstream consumers.
	r.Body = io.NopCloser(bytes.NewReader(body))
	return c.Unmarshal(body, target)
}

// Respond marshals data with the codec from the request context and writes it
// to the response with an appropriate Content-Type header.
func Respond(w http.ResponseWriter, r *http.Request, statusCode int, data any) error {
	c := FromContext(r.Context())
	if c == nil {
		c = encoding.GetCodec("json")
	}

	body, err := c.Marshal(data)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", mimeFromCodecName(c.Name()))
	w.WriteHeader(statusCode)
	_, err = w.Write(body)
	return err
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// ErrCodecNotFound is returned when no codec is registered in the context.
var ErrCodecNotFound = errors.New("codec: no codec found in context (middleware not registered?)")

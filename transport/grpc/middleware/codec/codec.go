// Package codec bridges the framework's [encoding.Codec] registry with gRPC's
// native codec system.
//
// gRPC uses [google.golang.org/grpc/encoding.Codec] to serialize and
// deserialize messages. The [encoding.Codec] interface is structurally
// identical, so codecs registered with the framework registry can be
// seamlessly registered with gRPC's global codec registry.
//
// This enables gRPC services to accept non-protobuf wire formats such as JSON,
// XML, or YAML, negotiated via the Content-Subtype header
// (e.g. application/grpc+json).
//
// Usage:
//
//	import (
//	    grpccodec "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/codec"
//	    _ "github.com/tx7do/go-wind-plugins/encoding/json" // side-effect: registers JSON codec
//	)
//
//	func main() {
//	    // Register our JSON codec with gRPC's global registry.
//	    grpccodec.RegisterByName("json")
//
//	    // gRPC clients can now use JSON wire format:
//	    //   conn.Invoke(ctx, "/pkg.Svc/Method", req, reply,
//	    //       grpc.CallContentSubtype("json"))
//	}
package codec

import (
	"strings"

	"github.com/tx7do/go-wind-plugins/encoding"
	grpcEncoding "google.golang.org/grpc/encoding"
)

// grpcCodec wraps an [encoding.Codec] to satisfy gRPC's
// [grpcEncoding.Codec] interface.
//
// The two interfaces are structurally identical, so the wrapper is a simple
// pass-through. The only transformation is lowercasing the codec name, as gRPC
// requires lowercase content subtypes.
type grpcCodec struct {
	inner encoding.Codec
}

// Marshal delegates to the inner codec.
func (c *grpcCodec) Marshal(v any) ([]byte, error) {
	return c.inner.Marshal(v)
}

// Unmarshal delegates to the inner codec.
func (c *grpcCodec) Unmarshal(data []byte, v any) error {
	return c.inner.Unmarshal(data, v)
}

// Name returns the codec name, lowercased to match gRPC's convention.
func (c *grpcCodec) Name() string {
	return strings.ToLower(c.inner.Name())
}

// WrapCodec wraps an [encoding.Codec] to satisfy gRPC's
// [grpcEncoding.Codec] interface without registering it.
//
// This is useful when you need a gRPC codec instance but want to manage
// registration yourself.
func WrapCodec(c encoding.Codec) grpcEncoding.Codec {
	if c == nil {
		return nil
	}
	return &grpcCodec{inner: c}
}

// RegisterCodec wraps and registers an [encoding.Codec] with gRPC's global
// codec registry.
//
// After registration, gRPC clients can select this codec by specifying its name
// as the Content-Subtype:
//
//	conn.Invoke(ctx, method, req, reply, grpc.CallContentSubtype("json"))
//
// On the server side, gRPC automatically selects the codec based on the
// incoming Content-Type header.
func RegisterCodec(c encoding.Codec) {
	if c == nil {
		return
	}
	grpcEncoding.RegisterCodec(WrapCodec(c))
}

// RegisterByName looks up codecs from the framework's [encoding] registry by
// name and registers them with gRPC.
//
// Codecs that are not found in the registry are silently skipped. Returns the
// list of names that were successfully found and registered.
//
// Example:
//
//	// Register JSON and XML codecs with gRPC
//	grpccodec.RegisterByName("json", "xml")
func RegisterByName(names ...string) []string {
	registered := make([]string, 0, len(names))
	for _, name := range names {
		c := encoding.GetCodec(name)
		if c == nil {
			continue
		}
		RegisterCodec(c)
		registered = append(registered, strings.ToLower(c.Name()))
	}
	return registered
}

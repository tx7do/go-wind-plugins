// Package crypto provides transparent payload encryption for gRPC by wrapping
// any gRPC codec with an encryption layer.
//
// Unlike HTTP where encryption is applied to the request/response body via
// middleware, gRPC interceptors receive already-deserialized messages. Therefore,
// encryption must be applied at the codec level — after serialization (Marshal)
// and before deserialization (Unmarshal).
//
// The [EncryptedCodec] wraps any [grpcEncoding.Codec] (e.g. proto, json) with a
// [crypto.Cipher], transparently encrypting and decrypting message payloads:
//
//	Marshal flow:   v → inner.Marshal(v) → cipher.Encrypt(plaintext) → ciphertext
//	Unmarshal flow: ciphertext → cipher.Decrypt → inner.Unmarshal(plaintext, v)
//
// Both server and client use the same registered codec — no interceptor needed.
// The server automatically uses the codec based on the Content-Subtype in the
// incoming request; the client selects it via grpc.CallContentSubtype.
//
// Usage:
//
//	import (
//	    grpccrypto "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/crypto"
//	    utilsCrypto "github.com/tx7do/go-utils/crypto"
//	    grpcEncoding "google.golang.org/grpc/encoding"
//	)
//
//	key := []byte("1234567890abcdef")
//	aesCipher := utilsCrypto.NewAESCipher(key, nil)
//
//	// Wrap gRPC's built-in proto codec with AES encryption.
//	encCodec := grpccrypto.NewEncryptedCodec(
//	    grpcEncoding.GetCodec("proto"),
//	    aesCipher,
//	)
//	grpcEncoding.RegisterCodec(encCodec)
//
//	// Client side: select the encrypted codec by name.
//	conn.Invoke(ctx, method, req, reply,
//	    grpc.CallContentSubtype(encCodec.Name()))
package crypto

import (
	"strings"

	secCrypto "github.com/tx7do/go-wind-plugins/security/crypto"
	grpcEncoding "google.golang.org/grpc/encoding"
)

// EncryptedCodec wraps a [grpcEncoding.Codec] with transparent encryption and
// decryption. It implements [grpcEncoding.Codec], so it can be registered with
// gRPC's codec registry and selected by clients via Content-Subtype.
type EncryptedCodec struct {
	inner  grpcEncoding.Codec
	cipher secCrypto.Cipher
}

// NewEncryptedCodec creates an encrypted codec that wraps the given inner codec.
//
// The resulting codec name is "<cipher-name>-<inner-name>" (e.g. "aes-proto").
// This name is used as the gRPC Content-Subtype for codec negotiation.
func NewEncryptedCodec(inner grpcEncoding.Codec, cipher secCrypto.Cipher) *EncryptedCodec {
	return &EncryptedCodec{inner: inner, cipher: cipher}
}

// Marshal serializes the value with the inner codec, then encrypts the result.
func (c *EncryptedCodec) Marshal(v any) ([]byte, error) {
	plain, err := c.inner.Marshal(v)
	if err != nil {
		return nil, err
	}
	return c.cipher.Encrypt(plain)
}

// Unmarshal decrypts the data, then deserializes with the inner codec.
func (c *EncryptedCodec) Unmarshal(data []byte, v any) error {
	plain, err := c.cipher.Decrypt(data)
	if err != nil {
		return err
	}
	return c.inner.Unmarshal(plain, v)
}

// Name returns the codec name in the format "<cipher-name>-<inner-name>",
// lowercased for gRPC compatibility.
//
// For example, an AES cipher wrapping a proto codec yields "aes-proto".
func (c *EncryptedCodec) Name() string {
	return strings.ToLower(c.cipher.Name() + "-" + c.inner.Name())
}

// Register creates an EncryptedCodec and registers it with gRPC's global
// codec registry.
//
// This is a convenience wrapper around NewEncryptedCodec +
// grpcEncoding.RegisterCodec.
func Register(inner grpcEncoding.Codec, cipher secCrypto.Cipher) *EncryptedCodec {
	enc := NewEncryptedCodec(inner, cipher)
	grpcEncoding.RegisterCodec(enc)
	return enc
}

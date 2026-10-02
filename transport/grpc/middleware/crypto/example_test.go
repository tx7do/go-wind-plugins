package crypto_test

import (
	utilsCrypto "github.com/tx7do/go-utils/crypto"
	grpccrypto "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/crypto"
	grpcEncoding "google.golang.org/grpc/encoding"
)

// ExampleRegister creates an encrypted codec by wrapping gRPC's built-in
// proto codec with an AES cipher, and registers it with gRPC's global codec
// registry. The bridge operates at the codec layer: Marshal serializes with
// the inner codec and then encrypts the payload, while Unmarshal decrypts
// and then deserializes, so no interceptor is involved. Both peers select
// the codec by name through the Content-Subtype header; the server from the
// incoming Content-Type, the client via grpc.CallContentSubtype.
func ExampleRegister() {
	key := []byte("1234567890abcdef") // 16 bytes for AES-128
	aesCipher := utilsCrypto.NewAESCipher(key, nil)
	_ = grpccrypto.Register(grpcEncoding.GetCodec("proto"), aesCipher)
}

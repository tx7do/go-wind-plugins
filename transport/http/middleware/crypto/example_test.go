package crypto_test

import (
	utilsCrypto "github.com/tx7do/go-utils/crypto"
	windhttp "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/crypto"
)

// ExampleMiddleware attaches the body-encryption middleware with an AES
// cipher. Request bodies are transparently decrypted before they reach the
// handler, and responses written by the handler are encrypted on the way out.
//
// crypto must sit before codec in the chain, so bodies are decrypted before
// the codec parses them. In production the key must come from a secret
// manager; the literal here is a 16-byte AES-128 demo key.
func ExampleMiddleware() {
	cipher := utilsCrypto.NewAESCipher([]byte("1234567890abcdef"), nil)

	srv := windhttp.NewServer(":8080")
	srv.Use(crypto.Middleware(cipher))
}

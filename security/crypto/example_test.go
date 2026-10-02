package crypto_test

import (
	"fmt"

	utilsCrypto "github.com/tx7do/go-utils/crypto"
	"github.com/tx7do/go-wind-plugins/security/crypto"
)

// ExampleRegisterCipher registers an AES cipher in the global registry and
// resolves it back by name, the same lookup transport middleware uses to
// obtain the cipher it applies to payloads. The retrieved implementation
// round-trips a message to its original plaintext.
func ExampleRegisterCipher() {
	cipher := utilsCrypto.NewAESCipher([]byte("1234567890abcdef"), nil)
	crypto.RegisterCipher(cipher)

	retrieved := crypto.GetCipher(cipher.Name())
	if retrieved == nil {
		return
	}

	encrypted, err := retrieved.Encrypt([]byte("secret message"))
	if err != nil {
		return
	}
	decrypted, err := retrieved.Decrypt(encrypted)
	if err != nil {
		return
	}
	fmt.Println(string(decrypted))
	// Output: secret message
}

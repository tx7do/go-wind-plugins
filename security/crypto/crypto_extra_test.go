package crypto

import (
	"bytes"
	"testing"

	utilsCrypto "github.com/tx7do/go-utils/crypto"
)

// ---------------------------------------------------------------------------
// AES — key generation and round trips with generated keys
// ---------------------------------------------------------------------------

func TestGenerateAESKey(t *testing.T) {
	for _, size := range []int{16, 24, 32} {
		key, err := utilsCrypto.GenerateAESKey(size)
		if err != nil {
			t.Fatalf("GenerateAESKey(%d): %v", size, err)
		}
		if len(key) != size {
			t.Errorf("GenerateAESKey(%d) returned %d bytes", size, len(key))
		}
	}

	if _, err := utilsCrypto.GenerateAESKey(15); err == nil {
		t.Error("GenerateAESKey(15) should reject unsupported lengths")
	}
	if _, err := utilsCrypto.GenerateAESKey(0); err == nil {
		t.Error("GenerateAESKey(0) should reject unsupported lengths")
	}
}

func TestAES_RoundTrip_GeneratedKeys(t *testing.T) {
	for _, size := range []int{16, 24, 32} {
		t.Run(string(rune('0'+size/8))+"-byte key", func(t *testing.T) {
			key, err := utilsCrypto.GenerateAESKey(size)
			if err != nil {
				t.Fatalf("GenerateAESKey: %v", err)
			}
			c := utilsCrypto.NewAESCipher(key, nil)

			plain := []byte("the quick brown fox jumps over the lazy dog")
			encrypted, err := c.Encrypt(plain)
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}
			if bytes.Equal(encrypted, plain) {
				t.Fatal("ciphertext must differ from plaintext")
			}

			decrypted, err := c.Decrypt(encrypted)
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}
			if !bytes.Equal(decrypted, plain) {
				t.Errorf("round trip mismatch: got %q, want %q", decrypted, plain)
			}
		})
	}
}

func TestAES_WithExplicitIV(t *testing.T) {
	key := []byte("0123456789abcdef")
	iv := []byte("fedcba9876543210") // exactly one block
	c := utilsCrypto.NewAESCipher(key, iv)

	plain := []byte("explicit IV round trip")
	encrypted, err := c.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	decrypted, err := c.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(decrypted, plain) {
		t.Errorf("round trip mismatch: got %q, want %q", decrypted, plain)
	}

	// A wrong-size IV must be rejected.
	bad := utilsCrypto.NewAESCipher(key, []byte("short"))
	if _, err := bad.Encrypt(plain); err == nil {
		t.Error("Encrypt with a wrong-size IV should fail")
	}
}

func TestAES_EmptyPlaintextFails(t *testing.T) {
	c := utilsCrypto.NewAESCipher([]byte("1234567890abcdef"), nil)
	if _, err := c.Encrypt(nil); err == nil {
		t.Error("Encrypt(nil) should fail")
	}
	if _, err := c.Encrypt([]byte{}); err == nil {
		t.Error("Encrypt(empty) should fail")
	}
}

func TestAES_InvalidKeyLengthFails(t *testing.T) {
	// NewAESCipher falls back to a default key for empty keys, so use a
	// non-empty but unsupported length to exercise the validation.
	c := utilsCrypto.NewAESCipher([]byte("short-key"), nil)
	if _, err := c.Encrypt([]byte("data")); err == nil {
		t.Error("Encrypt with an 9-byte key should fail")
	}
	if _, err := c.Decrypt(make([]byte, 16)); err == nil {
		t.Error("Decrypt with an unsupported key should fail")
	}
}

func TestAES_WrongKeyDoesNotReturnPlaintext(t *testing.T) {
	enc, err := utilsCrypto.NewAESCipher([]byte("correct-key-16by"), nil).Encrypt([]byte("attack at dawn!!"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	dec, err := utilsCrypto.NewAESCipher([]byte("wrong--key-16byt"), nil).Decrypt(enc)
	if err != nil {
		return // CBC+PKCS5 usually rejects a wrong key outright
	}
	if bytes.Equal(dec, []byte("attack at dawn!!")) {
		t.Error("decryption with the wrong key unexpectedly returned the plaintext")
	}
}

func TestAES_TamperedCiphertext(t *testing.T) {
	key := []byte("1234567890abcdef")
	enc, err := utilsCrypto.NewAESCipher(key, nil).Encrypt([]byte("tamper test payload!!"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Flip one bit in the final block: this corrupts the padding.
	tampered := append([]byte(nil), enc...)
	tampered[len(tampered)-1] ^= 0xFF

	dec, err := utilsCrypto.NewAESCipher(key, nil).Decrypt(tampered)
	if err != nil {
		return
	}
	if bytes.Equal(dec, []byte("tamper test payload!!")) {
		t.Error("tampered ciphertext decrypted back to the original plaintext")
	}
}

func TestAES_DecryptInvalidInput(t *testing.T) {
	c := utilsCrypto.NewAESCipher([]byte("1234567890abcdef"), nil)
	if _, err := c.Decrypt(nil); err == nil {
		t.Error("Decrypt(nil) should fail")
	}
	// Not a multiple of the block size.
	if _, err := c.Decrypt([]byte("7 bytes")); err == nil {
		t.Error("Decrypt of a non-block-multiple should fail")
	}
}

// ---------------------------------------------------------------------------
// RSA — round trip, wrong key, tampering
// ---------------------------------------------------------------------------

func TestRSA_RoundTrip(t *testing.T) {
	r, err := utilsCrypto.NewRSACipher(1024) // small key keeps the test fast
	if err != nil {
		t.Fatalf("NewRSACipher: %v", err)
	}
	if r.Name() == "" {
		t.Error("expected a non-empty cipher name")
	}

	plain := []byte("rsa secret")
	encrypted, err := r.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	decrypted, err := r.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(decrypted, plain) {
		t.Errorf("round trip mismatch: got %q, want %q", decrypted, plain)
	}
}

func TestRSA_TamperedCiphertextFails(t *testing.T) {
	r, err := utilsCrypto.NewRSACipher(1024)
	if err != nil {
		t.Fatalf("NewRSACipher: %v", err)
	}
	encrypted, err := r.Encrypt([]byte("rsa secret"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	tampered := append([]byte(nil), encrypted...)
	tampered[0] ^= 0xFF
	if _, err := r.Decrypt(tampered); err == nil {
		t.Error("Decrypt of tampered RSA ciphertext should fail")
	}
}

func TestRSA_WrongKeyFails(t *testing.T) {
	r1, err := utilsCrypto.NewRSACipher(1024)
	if err != nil {
		t.Fatalf("NewRSACipher: %v", err)
	}
	r2, err := utilsCrypto.NewRSACipher(1024)
	if err != nil {
		t.Fatalf("NewRSACipher: %v", err)
	}

	encrypted, err := r1.Encrypt([]byte("cross-key"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := r2.Decrypt(encrypted); err == nil {
		t.Error("decrypting with a different key pair should fail")
	}
}

// ---------------------------------------------------------------------------
// SM4
// ---------------------------------------------------------------------------

func TestSM4_RoundTrip(t *testing.T) {
	key, err := utilsCrypto.GenerateSM4Key()
	if err != nil {
		t.Fatalf("GenerateSM4Key: %v", err)
	}
	s, err := utilsCrypto.NewSM4Cipher(key)
	if err != nil {
		t.Fatalf("NewSM4Cipher: %v", err)
	}

	plain := []byte("sm4 secret message")
	encrypted, err := s.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	decrypted, err := s.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(decrypted, plain) {
		t.Errorf("round trip mismatch: got %q, want %q", decrypted, plain)
	}

	// Tampering breaks the PKCS5 padding (or garbles the data).
	tampered := append([]byte(nil), encrypted...)
	tampered[0] ^= 0xFF
	if dec, err := s.Decrypt(tampered); err == nil && bytes.Equal(dec, plain) {
		t.Error("tampered SM4 ciphertext decrypted back to the original plaintext")
	}
}

func TestSM4_KeyValidation(t *testing.T) {
	if _, err := utilsCrypto.NewSM4Cipher([]byte("short")); err == nil {
		t.Error("SM4 requires a 16-byte key")
	}
}

// ---------------------------------------------------------------------------
// SM2 — encryption round trip and signatures
// ---------------------------------------------------------------------------

func TestSM2_RoundTripAndSignVerify(t *testing.T) {
	s, err := utilsCrypto.NewSM2Cipher()
	if err != nil {
		t.Fatalf("NewSM2Cipher: %v", err)
	}

	plain := []byte("sm2 secret")
	encrypted, err := s.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	decrypted, err := s.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(decrypted, plain) {
		t.Errorf("round trip mismatch: got %q, want %q", decrypted, plain)
	}

	sig, err := s.Sign(plain)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	ok, err := s.Verify(plain, sig)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !ok {
		t.Error("signature over the original data should verify")
	}

	ok, err = s.Verify([]byte("sm2 secret (tampered)"), sig)
	if err != nil {
		t.Fatalf("Verify over tampered data: %v", err)
	}
	if ok {
		t.Error("signature must not verify over modified data")
	}
}

// ---------------------------------------------------------------------------
// Hashers — SHA-256 / SHA-512 / SM3
// ---------------------------------------------------------------------------

func TestSHA256Hasher_KnownVector(t *testing.T) {
	h := &utilsCrypto.SHA256Hasher{}
	got, err := h.Sum([]byte("abc"))
	if err != nil {
		t.Fatalf("Sum: %v", err)
	}
	want := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if gotHex := toHex(got); gotHex != want {
		t.Errorf("SHA256(abc) = %s, want %s", gotHex, want)
	}

	// Deterministic.
	again, _ := h.Sum([]byte("abc"))
	if !bytes.Equal(got, again) {
		t.Error("SHA256 must be deterministic")
	}

	// Empty input is a valid, well-defined hash.
	if empty, err := h.Sum(nil); err != nil || len(empty) != 32 {
		t.Errorf("SHA256(nil) should yield a 32-byte digest, got %d bytes, err %v", len(empty), err)
	}
}

func TestSHA512Hasher_DigestLength(t *testing.T) {
	h := &utilsCrypto.SHA512Hasher{}
	got, err := h.Sum([]byte("abc"))
	if err != nil {
		t.Fatalf("Sum: %v", err)
	}
	if len(got) != 64 {
		t.Errorf("SHA512 digest length = %d, want 64", len(got))
	}
}

func TestSM3Hasher_KnownVector(t *testing.T) {
	h := utilsCrypto.NewSM3Hasher()
	got, err := h.Sum([]byte("abc"))
	if err != nil {
		t.Fatalf("SM3 Sum error: %v", err)
	}
	// Standard SM3 test vector for "abc".
	want := "66c7f0f462eeedd9d1f2d46bdc10e4e24167c4875cf2f7a2297da02b8f4ba8e0"
	if toHex(got) != want {
		t.Errorf("SM3(abc) = %s, want %s", toHex(got), want)
	}
}

// ---------------------------------------------------------------------------
// HMAC
// ---------------------------------------------------------------------------

func TestHMAC_SumAndVerify(t *testing.T) {
	h := utilsCrypto.NewHMAC([]byte("key"))

	sig := h.Sum([]byte("hello"))
	if sig == "" {
		t.Fatal("Sum returned an empty signature")
	}
	if !h.Verify([]byte("hello"), sig) {
		t.Error("Verify should accept the matching signature")
	}
	if h.Verify([]byte("hello!"), sig) {
		t.Error("Verify should reject a modified message")
	}
	if h.Verify([]byte("hello"), sig[:len(sig)-1]+"0") {
		t.Error("Verify should reject a modified signature")
	}

	// A different key must not produce the same signature.
	other := utilsCrypto.NewHMAC([]byte("other-key"))
	if other.Sum([]byte("hello")) == sig {
		t.Error("different keys must produce different signatures")
	}

	// Changing the key takes effect.
	h.SetKey([]byte("other-key"))
	if h.Sum([]byte("hello")) != other.Sum([]byte("hello")) {
		t.Error("SetKey should change the signing key")
	}
}

// ---------------------------------------------------------------------------
// ECDSA sign/verify and ECDH key agreement
// ---------------------------------------------------------------------------

func TestECDSA_SignAndVerify(t *testing.T) {
	e, err := utilsCrypto.NewECDSACipher()
	if err != nil {
		t.Fatalf("NewECDSACipher: %v", err)
	}
	// NOTE: ECDSACipher.PublicKeyBytes() currently returns nil because it
	// asn1-marshals an ecdsa.PublicKey (which has unexported fields) and
	// swallows the error — an upstream go-utils/crypto issue, not asserted
	// here.

	data := []byte("sign me")
	sig, err := e.Sign(data)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	ok, err := e.Verify(data, sig)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !ok {
		t.Error("valid signature should verify")
	}

	ok, err = e.Verify([]byte("sign me!"), sig)
	if err != nil {
		t.Fatalf("Verify over tampered data: %v", err)
	}
	if ok {
		t.Error("signature must not verify over modified data")
	}
}

func TestECDH_SharedSecretAgreement(t *testing.T) {
	a, err := utilsCrypto.NewECDHCipher()
	if err != nil {
		t.Fatalf("NewECDHCipher: %v", err)
	}
	b, err := utilsCrypto.NewECDHCipher()
	if err != nil {
		t.Fatalf("NewECDHCipher: %v", err)
	}

	secretA, err := a.DeriveSharedSecret(b.PublicKeyBytes())
	if err != nil {
		t.Fatalf("DeriveSharedSecret (A side): %v", err)
	}
	secretB, err := b.DeriveSharedSecret(a.PublicKeyBytes())
	if err != nil {
		t.Fatalf("DeriveSharedSecret (B side): %v", err)
	}

	if !bytes.Equal(secretA, secretB) {
		t.Error("both parties must derive the same shared secret")
	}
	if len(secretA) == 0 {
		t.Error("shared secret should not be empty")
	}

	// Garbage peer keys must be rejected.
	if _, err := a.DeriveSharedSecret([]byte{0x01, 0x02}); err == nil {
		t.Error("DeriveSharedSecret with an invalid peer key should fail")
	}
}

// ---------------------------------------------------------------------------
// Registry — hasher / signer / verifier variants
// ---------------------------------------------------------------------------

func TestRegisterHasher_SHA256(t *testing.T) {
	h := &utilsCrypto.SHA256Hasher{}
	RegisterHasher(h)

	got := GetHasher(h.Name())
	if got == nil {
		t.Fatal("GetHasher returned nil after registration")
	}
	digest, err := got.Sum([]byte("abc"))
	if err != nil {
		t.Fatalf("Sum: %v", err)
	}
	if len(digest) != 32 {
		t.Errorf("digest length = %d, want 32", len(digest))
	}

	if GetHasher("nonexistent") != nil {
		t.Error("unknown hasher should return nil")
	}
	if GetHasher("") != nil {
		t.Error("empty hasher name should return nil")
	}
}

func TestRegisterHasher_NilAndEmptyName(t *testing.T) {
	assertPanic(t, func() { RegisterHasher(nil) })
	assertPanic(t, func() { RegisterHasher(&emptyNameHasher{}) })
}

func TestRegisterSignerAndVerifier_ECDSA(t *testing.T) {
	e, err := utilsCrypto.NewECDSACipher()
	if err != nil {
		t.Fatalf("NewECDSACipher: %v", err)
	}

	RegisterSigner(e)
	gotSigner := GetSigner(e.Name())
	if gotSigner == nil {
		t.Fatal("GetSigner returned nil after registration")
	}

	RegisterVerifier(e)
	gotVerifier := GetVerifier(e.Name())
	if gotVerifier == nil {
		t.Fatal("GetVerifier returned nil after registration")
	}

	// The registry-looked-up pair must interoperate.
	data := []byte("registry round trip")
	sig, err := gotSigner.Sign(data)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	ok, err := gotVerifier.Verify(data, sig)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !ok {
		t.Error("registry-looked-up signer/verifier must interoperate")
	}

	if GetSigner("nonexistent") != nil {
		t.Error("unknown signer should return nil")
	}
	if GetVerifier("nonexistent") != nil {
		t.Error("unknown verifier should return nil")
	}
}

func TestRegisterSignerAndVerifier_NilAndEmptyName(t *testing.T) {
	assertPanic(t, func() { RegisterSigner(nil) })
	assertPanic(t, func() { RegisterSigner(&emptyNameSigner{}) })
	assertPanic(t, func() { RegisterVerifier(nil) })
	assertPanic(t, func() { RegisterVerifier(&emptyNameVerifier{}) })
}

// ---------------------------------------------------------------------------
// Small fakes for panic-path coverage
// ---------------------------------------------------------------------------

type emptyNameHasher struct{}

func (emptyNameHasher) Sum([]byte) ([]byte, error) { return nil, nil }
func (emptyNameHasher) Name() string               { return "" }

type emptyNameSigner struct{}

func (emptyNameSigner) Sign([]byte) (string, error) { return "", nil }
func (emptyNameSigner) Name() string                { return "" }

type emptyNameVerifier struct{}

func (emptyNameVerifier) Verify([]byte, string) (bool, error) { return false, nil }
func (emptyNameVerifier) Name() string                        { return "" }

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func assertPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic")
		}
	}()
	fn()
}

func toHex(b []byte) string {
	const hexDigits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, v := range b {
		out = append(out, hexDigits[v>>4], hexDigits[v&0x0F])
	}
	return string(out)
}

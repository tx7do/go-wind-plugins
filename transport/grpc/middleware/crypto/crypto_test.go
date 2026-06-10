package crypto

import (
	"encoding/json"
	"testing"

	utilsCrypto "github.com/tx7do/go-utils/crypto"
	grpcEncoding "google.golang.org/grpc/encoding"
)

// ---------------------------------------------------------------------------
// Test fixtures
// ---------------------------------------------------------------------------

// jsonCodec is a minimal gRPC codec for testing.
type jsonCodec struct{}

func (jsonCodec) Marshal(v any) ([]byte, error)      { return json.Marshal(v) }
func (jsonCodec) Unmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }
func (jsonCodec) Name() string                       { return "json" }

type payload struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

// newTestCipher returns an AES cipher for testing.
func newTestCipher() *utilsCrypto.AESCipher {
	key := []byte("1234567890abcdef") // 16 bytes for AES-128
	return utilsCrypto.NewAESCipher(key, nil)
}

// ---------------------------------------------------------------------------
// NewEncryptedCodec / Name
// ---------------------------------------------------------------------------

func TestEncryptedCodec_Name(t *testing.T) {
	c := NewEncryptedCodec(jsonCodec{}, newTestCipher())
	name := c.Name()
	if name == "" {
		t.Error("expected non-empty name")
	}
	// Name should be "<cipher-name>-json"
	if name[len(name)-5:] != "-json" {
		t.Errorf("name should end with -json, got %q", name)
	}
}

func TestEncryptedCodec_Name_Lowercased(t *testing.T) {
	c := NewEncryptedCodec(jsonCodec{}, newTestCipher())
	if c.Name() != toLower(c.Name()) {
		t.Errorf("Name() should be lowercase, got %q", c.Name())
	}
}

// ---------------------------------------------------------------------------
// Marshal / Unmarshal round-trip
// ---------------------------------------------------------------------------

func TestEncryptedCodec_RoundTrip(t *testing.T) {
	c := NewEncryptedCodec(jsonCodec{}, newTestCipher())

	original := &payload{Name: "alice", Value: 42}

	// Marshal (serialize + encrypt)
	ciphertext, err := c.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Ciphertext should differ from plaintext JSON
	plaintext, _ := json.Marshal(original)
	if string(ciphertext) == string(plaintext) {
		t.Error("ciphertext should differ from plaintext")
	}
	if len(ciphertext) == 0 {
		t.Error("ciphertext should not be empty")
	}

	// Unmarshal (decrypt + deserialize)
	var decoded payload
	if err := c.Unmarshal(ciphertext, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.Name != "alice" || decoded.Value != 42 {
		t.Errorf("decoded = %+v, want {alice 42}", decoded)
	}
}

func TestEncryptedCodec_MarshalProducesCiphertext(t *testing.T) {
	c := NewEncryptedCodec(jsonCodec{}, newTestCipher())

	original := &payload{Name: "bob", Value: 99}
	ciphertext, err := c.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Direct JSON unmarshal should fail (it's encrypted, not plain JSON)
	var direct payload
	if err := json.Unmarshal(ciphertext, &direct); err == nil {
		t.Error("ciphertext should not be valid JSON")
	}
}

func TestEncryptedCodec_UnmarshalInvalidData(t *testing.T) {
	c := NewEncryptedCodec(jsonCodec{}, newTestCipher())

	// Random garbage — should fail during decrypt
	var decoded payload
	err := c.Unmarshal([]byte("not-encrypted-garbage"), &decoded)
	if err == nil {
		t.Error("expected error for invalid ciphertext")
	}
}

// ---------------------------------------------------------------------------
// Register
// ---------------------------------------------------------------------------

func TestRegister_WithGRPCRegistry(t *testing.T) {
	inner := &namedInnerCodec{name: "regtest-inner"}
	cipher := newTestCipher()

	enc := Register(inner, cipher)

	got := grpcEncoding.GetCodec(enc.Name())
	if got == nil {
		t.Fatal("codec not found in gRPC registry after Register")
	}

	// Verify round-trip through gRPC registry lookup
	original := &payload{Name: "registry", Value: 7}
	data, err := got.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal via registry: %v", err)
	}

	var decoded payload
	if err := got.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal via registry: %v", err)
	}
	if decoded.Name != "registry" || decoded.Value != 7 {
		t.Errorf("decoded = %+v, want {registry 7}", decoded)
	}
}

// ---------------------------------------------------------------------------
// Multiple ciphers → unique names
// ---------------------------------------------------------------------------

func TestEncryptedCodec_UniqueNames(t *testing.T) {
	c1 := NewEncryptedCodec(jsonCodec{}, newTestCipher())
	c2 := NewEncryptedCodec(&namedInnerCodec{name: "xml"}, newTestCipher())

	if c1.Name() == c2.Name() {
		t.Errorf("different inner codecs should have different names: %q == %q",
			c1.Name(), c2.Name())
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// toLower is a simple ASCII lowercase for testing.
func toLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// namedInnerCodec wraps a name for testing unique registrations.
type namedInnerCodec struct {
	name string
}

func (n *namedInnerCodec) Marshal(v any) ([]byte, error)      { return json.Marshal(v) }
func (n *namedInnerCodec) Unmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }
func (n *namedInnerCodec) Name() string                       { return n.name }

package codec

import (
	"encoding/json"
	"testing"

	"github.com/tx7do/go-wind-plugins/encoding"
	grpcEncoding "google.golang.org/grpc/encoding"
)

// ---------------------------------------------------------------------------
// Test codec — a minimal encoding.Codec implementation
// ---------------------------------------------------------------------------

type testCodec struct{}

func (testCodec) Marshal(v any) ([]byte, error)      { return json.Marshal(v) }
func (testCodec) Unmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }
func (testCodec) Name() string                       { return "test-json" }

type greet struct {
	Hello string `json:"hello"`
}

// ---------------------------------------------------------------------------
// WrapCodec
// ---------------------------------------------------------------------------

func TestWrapCodec_SatisfiesGRPCInterface(t *testing.T) {
	c := WrapCodec(testCodec{})
	var _ grpcEncoding.Codec = c // compile-time check
	if c == nil {
		t.Fatal("WrapCodec returned nil for non-nil input")
	}
}

func TestWrapCodec_NilInput(t *testing.T) {
	if got := WrapCodec(nil); got != nil {
		t.Errorf("WrapCodec(nil) = %v, want nil", got)
	}
}

func TestWrapCodec_Name_Lowercased(t *testing.T) {
	c := WrapCodec(testCodec{})
	if got := c.Name(); got != "test-json" {
		t.Errorf("Name() = %q, want %q", got, "test-json")
	}
}

func TestWrapCodec_MarshalUnmarshal(t *testing.T) {
	c := WrapCodec(testCodec{})

	original := &greet{Hello: "world"}
	data, err := c.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded greet
	if err := c.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.Hello != "world" {
		t.Errorf("decoded.Hello = %q, want %q", decoded.Hello, "world")
	}
}

// ---------------------------------------------------------------------------
// RegisterCodec
// ---------------------------------------------------------------------------

func TestRegisterCodec_WithGRPCRegistry(t *testing.T) {
	// Use a unique name to avoid collision with other tests.
	uniqueCodec := &namedCodec{name: "regtest", inner: testCodec{}}
	RegisterCodec(uniqueCodec)

	got := grpcEncoding.GetCodec("regtest")
	if got == nil {
		t.Fatal("grpcEncoding.GetCodec returned nil after registration")
	}
	if got.Name() != "regtest" {
		t.Errorf("registered codec Name() = %q, want %q", got.Name(), "regtest")
	}
}

func TestRegisterCodec_NilSafe(t *testing.T) {
	// Should not panic.
	RegisterCodec(nil)
}

// ---------------------------------------------------------------------------
// RegisterByName
// ---------------------------------------------------------------------------

func TestRegisterByName_ExistingCodec(t *testing.T) {
	// Register a codec in our encoding registry.
	encoding.RegisterCodec(&namedCodec{name: "rbtest", inner: testCodec{}})

	registered := RegisterByName("rbtest")
	if len(registered) != 1 {
		t.Fatalf("expected 1 registered, got %d", len(registered))
	}
	if registered[0] != "rbtest" {
		t.Errorf("registered[0] = %q, want %q", registered[0], "rbtest")
	}

	// Verify it's in gRPC's registry.
	if got := grpcEncoding.GetCodec("rbtest"); got == nil {
		t.Error("codec not found in gRPC registry after RegisterByName")
	}
}

func TestRegisterByName_NonExistentSkipped(t *testing.T) {
	registered := RegisterByName("nonexistent-codec-xyz")
	if len(registered) != 0 {
		t.Errorf("expected 0 registered for nonexistent codec, got %d", len(registered))
	}
}

func TestRegisterByName_Mixed(t *testing.T) {
	encoding.RegisterCodec(&namedCodec{name: "mixed-exist", inner: testCodec{}})

	registered := RegisterByName("mixed-exist", "mixed-nonexist")
	if len(registered) != 1 {
		t.Fatalf("expected 1 registered, got %d", len(registered))
	}
}

// ---------------------------------------------------------------------------
// Round-trip through gRPC registry
// ---------------------------------------------------------------------------

func TestGRPCRegistry_RoundTrip(t *testing.T) {
	encoding.RegisterCodec(&namedCodec{name: "rttest", inner: testCodec{}})
	RegisterByName("rttest")

	c := grpcEncoding.GetCodec("rttest")
	if c == nil {
		t.Fatal("GetCodec returned nil")
	}

	original := &greet{Hello: "grpc"}
	data, err := c.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded greet
	if err := c.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.Hello != "grpc" {
		t.Errorf("decoded.Hello = %q, want %q", decoded.Hello, "grpc")
	}
}

// ---------------------------------------------------------------------------
// Helper: namedCodec wraps any encoding.Codec with a custom name
// ---------------------------------------------------------------------------

type namedCodec struct {
	name  string
	inner encoding.Codec
}

func (n *namedCodec) Marshal(v any) ([]byte, error) {
	return n.inner.Marshal(v)
}
func (n *namedCodec) Unmarshal(data []byte, v any) error {
	return n.inner.Unmarshal(data, v)
}
func (n *namedCodec) Name() string {
	return n.name
}

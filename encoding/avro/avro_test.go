package avro

import (
	"testing"

	"github.com/tx7do/go-wind-plugins/encoding"
)

// userSchema is a minimal Avro record schema used in the tests.
const userSchema = `{
	"type": "record",
	"name": "User",
	"fields": [
		{"name": "name", "type": "string"},
		{"name": "age",  "type": "int"}
	]
}`

// ---------------------------------------------------------------------------
// Name / registration
// ---------------------------------------------------------------------------

func TestName(t *testing.T) {
	var c codec
	if got := c.Name(); got != Name {
		t.Errorf("Name() = %q, want %q", got, Name)
	}
	if Name != "avro" {
		t.Errorf("Name constant = %q, want %q", Name, "avro")
	}
}

func TestCodecRegistered(t *testing.T) {
	c := encoding.GetCodec(Name)
	if c == nil {
		t.Fatalf("encoding.GetCodec(%q) returned nil after package init", Name)
	}
	if c.Name() != Name {
		t.Errorf("registered codec Name() = %q, want %q", c.Name(), Name)
	}
}

// ---------------------------------------------------------------------------
// NewCodec
// ---------------------------------------------------------------------------

func TestNewCodec_ValidSchema(t *testing.T) {
	c, err := NewCodec(userSchema)
	if err != nil {
		t.Fatalf("NewCodec() error = %v", err)
	}
	if c == nil {
		t.Fatal("NewCodec() returned nil codec without error")
	}
	if c.Name() != Name {
		t.Errorf("NewCodec().Name() = %q, want %q", c.Name(), Name)
	}
}

func TestNewCodec_InvalidSchema(t *testing.T) {
	c, err := NewCodec("this is not a json schema")
	if err == nil {
		t.Error("NewCodec with an invalid schema should return an error")
	}
	if c != nil {
		t.Error("NewCodec with an invalid schema should return a nil codec")
	}
}

// ---------------------------------------------------------------------------
// Marshal / Unmarshal roundtrip
// ---------------------------------------------------------------------------

func TestMarshalUnmarshalRoundtrip_Record(t *testing.T) {
	c, err := NewCodec(userSchema)
	if err != nil {
		t.Fatalf("NewCodec() error = %v", err)
	}

	want := map[string]any{"name": "Alice", "age": int32(30)}
	data, err := c.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal(%v) error = %v", want, err)
	}
	if len(data) == 0 {
		t.Fatal("Marshal() returned empty bytes")
	}

	t.Run("into map[string]any", func(t *testing.T) {
		got := map[string]any{}
		if err := c.Unmarshal(data, &got); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if got["name"] != "Alice" {
			t.Errorf("decoded name = %v, want %q", got["name"], "Alice")
		}
		if got["age"] != int32(30) {
			t.Errorf("decoded age = %v (%T), want int32(30)", got["age"], got["age"])
		}
	})

	t.Run("into any", func(t *testing.T) {
		var got any
		if err := c.Unmarshal(data, &got); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		m, ok := got.(map[string]any)
		if !ok {
			t.Fatalf("decoded value type = %T, want map[string]any", got)
		}
		if m["name"] != "Alice" {
			t.Errorf("decoded name = %v, want %q", m["name"], "Alice")
		}
	})
}

func TestMarshalUnmarshalRoundtrip_Primitive(t *testing.T) {
	c, err := NewCodec(`"int"`)
	if err != nil {
		t.Fatalf("NewCodec() error = %v", err)
	}

	data, err := c.Marshal(int32(7))
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var got any
	if err := c.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got != int32(7) {
		t.Errorf("decoded value = %v (%T), want int32(7)", got, got)
	}
}

func TestMarshalUnmarshal_EmptyValue(t *testing.T) {
	// The default registered codec uses the "null" schema: nil encodes to
	// zero bytes and decodes back to Go nil.
	c := codec{}

	data, err := c.Marshal(nil)
	if err != nil {
		t.Fatalf("Marshal(nil) error = %v", err)
	}
	if len(data) != 0 {
		t.Errorf("Marshal(nil) = %v, want zero bytes (null encodes to nothing)", data)
	}

	var got any
	if err := c.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got != nil {
		t.Errorf("decoded value = %v, want nil", got)
	}
}

// ---------------------------------------------------------------------------
// Error paths
// ---------------------------------------------------------------------------

func TestMarshal_SchemaMismatch(t *testing.T) {
	c, err := NewCodec(userSchema)
	if err != nil {
		t.Fatalf("NewCodec() error = %v", err)
	}
	// A plain string does not match the record schema.
	if _, err := c.Marshal("not a record"); err == nil {
		t.Error("Marshal of a value not matching the schema should return an error")
	}
}

func TestUnmarshal_GarbageData(t *testing.T) {
	c, err := NewCodec(userSchema)
	if err != nil {
		t.Fatalf("NewCodec() error = %v", err)
	}
	var got any
	if err := c.Unmarshal([]byte("garbage"), &got); err == nil {
		t.Error("Unmarshal of garbage bytes should return an error")
	}
}

func TestUnmarshal_UnsupportedTarget(t *testing.T) {
	c, err := NewCodec(userSchema)
	if err != nil {
		t.Fatalf("NewCodec() error = %v", err)
	}
	var s string
	if err := c.Unmarshal(nil, &s); err == nil {
		t.Error("Unmarshal into *string should return an error (only *any and *map[string]any supported)")
	}
}

func TestUnmarshal_DecodedPrimitiveIntoMapTarget(t *testing.T) {
	c, err := NewCodec(`"int"`)
	if err != nil {
		t.Fatalf("NewCodec() error = %v", err)
	}
	data, err := c.Marshal(int32(7))
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	got := map[string]any{}
	if err := c.Unmarshal(data, &got); err == nil {
		t.Error("Unmarshal of a non-map native value into *map[string]any should return an error")
	}
}

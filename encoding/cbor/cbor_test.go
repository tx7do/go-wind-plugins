package cbor

import (
	"reflect"
	"testing"

	"github.com/tx7do/go-wind-plugins/encoding"
)

// ---------------------------------------------------------------------------
// Name / registration
// ---------------------------------------------------------------------------

func TestName(t *testing.T) {
	var c codec
	if got := c.Name(); got != Name {
		t.Errorf("Name() = %q, want %q", got, Name)
	}
	if Name != "cbor" {
		t.Errorf("Name constant = %q, want %q", Name, "cbor")
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
// Marshal / Unmarshal roundtrip
// ---------------------------------------------------------------------------

type testPayload struct {
	Name   string
	Values []int
	Nested map[string]string
}

func TestMarshalUnmarshalRoundtrip_Struct(t *testing.T) {
	c := codec{}

	tests := []struct {
		name  string
		input testPayload
	}{
		{
			name:  "zero value struct",
			input: testPayload{},
		},
		{
			name: "populated struct",
			input: testPayload{
				Name:   "Alice",
				Values: []int{1, 2, 3},
				Nested: map[string]string{"city": "Berlin"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := c.Marshal(tt.input)
			if err != nil {
				t.Fatalf("Marshal(%+v) error = %v", tt.input, err)
			}
			if len(data) == 0 {
				t.Fatal("Marshal() returned empty bytes")
			}

			var got testPayload
			if err := c.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.input) {
				t.Errorf("roundtrip = %+v, want %+v", got, tt.input)
			}
		})
	}
}

func TestMarshalUnmarshalRoundtrip_Map(t *testing.T) {
	c := codec{}

	// CBOR decodes untyped positive integers as uint64.
	want := map[string]any{
		"name": "Alice",
		"age":  uint64(30),
	}

	data, err := c.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	got := map[string]any{}
	if err := c.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("roundtrip = %v, want %v", got, want)
	}
}

func TestMarshalUnmarshal_NilValue(t *testing.T) {
	c := codec{}

	data, err := c.Marshal(nil)
	if err != nil {
		t.Fatalf("Marshal(nil) error = %v", err)
	}
	// CBOR null is a single byte (0xf6).
	if len(data) != 1 {
		t.Errorf("Marshal(nil) = %v, want a single null byte", data)
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

func TestUnmarshal_EmptyInput(t *testing.T) {
	c := codec{}
	var got testPayload
	if err := c.Unmarshal(nil, &got); err == nil {
		t.Error("Unmarshal of empty input should return an error")
	}
}

func TestUnmarshal_GarbageData(t *testing.T) {
	c := codec{}
	var got testPayload
	if err := c.Unmarshal([]byte("garbage"), &got); err == nil {
		t.Error("Unmarshal of garbage bytes should return an error")
	}
}

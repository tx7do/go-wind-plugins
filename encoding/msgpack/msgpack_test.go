package msgpack

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
	if Name != "msgpack" {
		t.Errorf("Name constant = %q, want %q", Name, "msgpack")
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
	Count  int
	Values []float64
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
				Count:  42,
				Values: []float64{1.5, 2.5, 3.5},
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

	// MessagePack decodes untyped integers as int64.
	want := map[string]any{
		"name":   "Alice",
		"age":    int64(30),
		"nested": map[string]any{"city": "Berlin"},
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

// ---------------------------------------------------------------------------
// Error paths
// ---------------------------------------------------------------------------

func TestUnmarshal_EmptyInput(t *testing.T) {
	c := codec{}
	var got testPayload
	if err := c.Unmarshal(nil, &got); err == nil {
		t.Error("Unmarshal of empty input should return an error (unexpected EOF)")
	}
}

func TestUnmarshal_GarbageData(t *testing.T) {
	c := codec{}
	var got testPayload
	if err := c.Unmarshal([]byte("garbage"), &got); err == nil {
		t.Error("Unmarshal of garbage bytes into a struct should return an error")
	}
}

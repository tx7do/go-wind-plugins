package bson

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"

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
	if Name != "bson" {
		t.Errorf("Name constant = %q, want %q", Name, "bson")
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

type testUser struct {
	Name   string
	Age    int
	Active bool
	Tags   []string
}

func TestMarshalUnmarshalRoundtrip_Struct(t *testing.T) {
	c := codec{}

	tests := []struct {
		name  string
		input testUser
	}{
		{
			name:  "zero value struct",
			input: testUser{},
		},
		{
			name: "populated struct",
			input: testUser{
				Name:   "Alice",
				Age:    30,
				Active: true,
				Tags:   []string{"admin", "dev"},
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

			var got testUser
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

	// The mongo-driver decoder maps top-level arrays to primitive.A when
	// decoding into map[string]any, and stores small integers as int32.
	want := map[string]any{
		"name":    "Alice",
		"age":     int32(30),
		"aliases": primitive.A{"ali", "al"},
		"nested":  map[string]any{"city": "Berlin"},
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
	var got testUser
	if err := c.Unmarshal(nil, &got); err == nil {
		t.Error("Unmarshal of empty input should return an error (invalid document length)")
	}
}

func TestUnmarshal_GarbageData(t *testing.T) {
	c := codec{}
	var got testUser
	if err := c.Unmarshal([]byte("garbage"), &got); err == nil {
		t.Error("Unmarshal of garbage bytes should return an error")
	}
}

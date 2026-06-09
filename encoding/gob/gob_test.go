package gob

import (
	"testing"

	"github.com/tx7do/go-wind-plugins/encoding"
)

type testStruct struct {
	Name  string
	Value int
}

func TestCodec_Name(t *testing.T) {
	var c encoding.Codec = codec{}
	if c.Name() != "gob" {
		t.Errorf("Name() = %q, want %q", c.Name(), "gob")
	}
}

func TestCodec_RoundTrip(t *testing.T) {
	c := codec{}
	original := testStruct{Name: "hello", Value: 42}
	data, err := c.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded testStruct
	if err := c.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if decoded.Name != original.Name || decoded.Value != original.Value {
		t.Errorf("round-trip mismatch: got %+v, want %+v", decoded, original)
	}
}

func TestCodec_Registered(t *testing.T) {
	c := encoding.GetCodec("gob")
	if c == nil {
		t.Fatal("GetCodec(\"gob\") returned nil — init() should have registered it")
	}
}

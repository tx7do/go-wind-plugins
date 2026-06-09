package xml

import (
	"testing"

	"github.com/tx7do/go-wind-plugins/encoding"
)

type testStruct struct {
	XMLName struct{} `xml:"root"`
	Name    string   `xml:"name"`
	Value   int      `xml:"value"`
}

func TestCodec_Name(t *testing.T) {
	var c encoding.Codec = codec{}
	if c.Name() != "xml" {
		t.Errorf("Name() = %q, want %q", c.Name(), "xml")
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
		t.Errorf("round-trip mismatch: got Name=%q Value=%d, want Name=%q Value=%d",
			decoded.Name, decoded.Value, original.Name, original.Value)
	}
}

func TestCodec_Registered(t *testing.T) {
	c := encoding.GetCodec("xml")
	if c == nil {
		t.Fatal("GetCodec(\"xml\") returned nil — init() should have registered it")
	}
}

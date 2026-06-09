package encoding

import (
	"testing"
)

// mockCodec is a minimal Codec implementation for testing.
type mockCodec struct {
	name string
}

func (m mockCodec) Marshal(v any) ([]byte, error)      { return []byte("mock:" + m.name), nil }
func (m mockCodec) Unmarshal(data []byte, v any) error { return nil }
func (m mockCodec) Name() string                       { return m.name }

// ---------------------------------------------------------------------------
// RegisterCodec / GetCodec
// ---------------------------------------------------------------------------

func TestRegisterAndGetCodec(t *testing.T) {
	c := mockCodec{name: "testformat"}
	RegisterCodec(c)

	got := GetCodec("testformat")
	if got == nil {
		t.Fatal("GetCodec returned nil after RegisterCodec")
	}
	if got.Name() != "testformat" {
		t.Errorf("GetCodec().Name() = %q, want %q", got.Name(), "testformat")
	}
}

func TestGetCodec_CaseInsensitive(t *testing.T) {
	RegisterCodec(mockCodec{name: "CamelCase"})

	// Lookup should be case-insensitive
	if GetCodec("camelcase") == nil {
		t.Error("GetCodec(\"camelcase\") should find registered \"CamelCase\"")
	}
	if GetCodec("CAMELCASE") == nil {
		t.Error("GetCodec(\"CAMELCASE\") should find registered \"CamelCase\"")
	}
	if GetCodec("CamelCase") == nil {
		t.Error("GetCodec(\"CamelCase\") should find registered \"CamelCase\"")
	}
}

func TestGetCodec_NotFound(t *testing.T) {
	if got := GetCodec("nonexistent"); got != nil {
		t.Errorf("GetCodec(\"nonexistent\") = %v, want nil", got)
	}
}

func TestGetCodec_EmptyString(t *testing.T) {
	if got := GetCodec(""); got != nil {
		t.Errorf("GetCodec(\"\") = %v, want nil", got)
	}
}

func TestRegisterCodec_Overwrite(t *testing.T) {
	first := mockCodec{name: "overwrite-test"}
	second := mockCodec{name: "overwrite-test"}
	RegisterCodec(first)
	RegisterCodec(second)

	// The second registration should replace the first.
	got := GetCodec("overwrite-test")
	if got == nil {
		t.Fatal("GetCodec returned nil after overwrite")
	}
}

// ---------------------------------------------------------------------------
// Panic cases
// ---------------------------------------------------------------------------

func TestRegisterCodec_NilPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("RegisterCodec(nil) should panic")
		}
	}()
	RegisterCodec(nil)
}

func TestRegisterCodec_EmptyNamePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("RegisterCodec with empty name should panic")
		}
	}()
	RegisterCodec(mockCodec{name: ""})
}

// ---------------------------------------------------------------------------
// Codec interface compliance
// ---------------------------------------------------------------------------

var _ Codec = mockCodec{}

func TestCodecInterface(t *testing.T) {
	c := mockCodec{name: "interface-test"}

	data, err := c.Marshal("anything")
	if err != nil {
		t.Errorf("Marshal() error = %v", err)
	}
	if string(data) != "mock:interface-test" {
		t.Errorf("Marshal() = %q, want %q", data, "mock:interface-test")
	}

	if err := c.Unmarshal(nil, nil); err != nil {
		t.Errorf("Unmarshal() error = %v", err)
	}

	if c.Name() != "interface-test" {
		t.Errorf("Name() = %q, want %q", c.Name(), "interface-test")
	}
}

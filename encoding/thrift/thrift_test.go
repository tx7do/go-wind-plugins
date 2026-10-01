package thrift

import (
	"context"
	"testing"

	"github.com/apache/thrift/lib/go/thrift"

	"github.com/tx7do/go-wind-plugins/encoding"
)

// testMessage is a minimal hand-written stand-in for a Thrift-compiler
// generated struct. It implements thrift.TStruct by writing/reading two
// fields (a string and an i64) using the binary protocol, so no generated
// code or external files are needed.
type testMessage struct {
	Name   string
	Number int64
}

func (m *testMessage) Write(ctx context.Context, p thrift.TProtocol) error {
	if err := p.WriteStructBegin(ctx, "testMessage"); err != nil {
		return err
	}
	if err := p.WriteFieldBegin(ctx, "Name", thrift.STRING, 1); err != nil {
		return err
	}
	if err := p.WriteString(ctx, m.Name); err != nil {
		return err
	}
	if err := p.WriteFieldEnd(ctx); err != nil {
		return err
	}
	if err := p.WriteFieldBegin(ctx, "Number", thrift.I64, 2); err != nil {
		return err
	}
	if err := p.WriteI64(ctx, m.Number); err != nil {
		return err
	}
	if err := p.WriteFieldEnd(ctx); err != nil {
		return err
	}
	if err := p.WriteFieldStop(ctx); err != nil {
		return err
	}
	return p.WriteStructEnd(ctx)
}

func (m *testMessage) Read(ctx context.Context, p thrift.TProtocol) error {
	if _, err := p.ReadStructBegin(ctx); err != nil {
		return err
	}
	for {
		_, ttype, id, err := p.ReadFieldBegin(ctx)
		if err != nil {
			return err
		}
		if ttype == thrift.STOP {
			break
		}
		switch {
		case id == 1 && ttype == thrift.STRING:
			if m.Name, err = p.ReadString(ctx); err != nil {
				return err
			}
		case id == 2 && ttype == thrift.I64:
			if m.Number, err = p.ReadI64(ctx); err != nil {
				return err
			}
		}
		if err := p.ReadFieldEnd(ctx); err != nil {
			return err
		}
	}
	return p.ReadStructEnd(ctx)
}

// ---------------------------------------------------------------------------
// Name / registration
// ---------------------------------------------------------------------------

func TestName(t *testing.T) {
	var c codec
	if got := c.Name(); got != Name {
		t.Errorf("Name() = %q, want %q", got, Name)
	}
	if Name != "thrift" {
		t.Errorf("Name constant = %q, want %q", Name, "thrift")
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
// Interface compliance
// ---------------------------------------------------------------------------

var _ thrift.TStruct = (*testMessage)(nil)

// ---------------------------------------------------------------------------
// Marshal / Unmarshal roundtrip
// ---------------------------------------------------------------------------

func TestMarshalUnmarshalRoundtrip(t *testing.T) {
	c := codec{}

	src := &testMessage{Name: "Alice", Number: 42}

	data, err := c.Marshal(src)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Marshal() returned empty bytes")
	}

	dst := &testMessage{}
	if err := c.Unmarshal(data, dst); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if dst.Name != src.Name {
		t.Errorf("decoded Name = %q, want %q", dst.Name, src.Name)
	}
	if dst.Number != src.Number {
		t.Errorf("decoded Number = %d, want %d", dst.Number, src.Number)
	}
}

func TestMarshalUnmarshal_ZeroValue(t *testing.T) {
	c := codec{}

	src := &testMessage{}

	data, err := c.Marshal(src)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	dst := &testMessage{}
	if err := c.Unmarshal(data, dst); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if dst.Name != "" || dst.Number != 0 {
		t.Errorf("decoded = %+v, want zero value", dst)
	}
}

// ---------------------------------------------------------------------------
// Error paths
// ---------------------------------------------------------------------------

func TestMarshal_NonTStruct(t *testing.T) {
	c := codec{}
	// A plain struct does not implement thrift.TStruct.
	if _, err := c.Marshal(map[string]any{"Name": "Alice"}); err == nil {
		t.Error("Marshal of a value not implementing thrift.TStruct should return an error")
	}
}

func TestUnmarshal_NonTStructTarget(t *testing.T) {
	c := codec{}
	var got map[string]any
	if err := c.Unmarshal([]byte{0x00}, &got); err == nil {
		t.Error("Unmarshal into a target not implementing thrift.TStruct should return an error")
	}
}

func TestUnmarshal_GarbageData(t *testing.T) {
	c := codec{}
	got := &testMessage{}
	if err := c.Unmarshal([]byte("garbage"), got); err == nil {
		t.Error("Unmarshal of garbage bytes should return an error")
	}
}

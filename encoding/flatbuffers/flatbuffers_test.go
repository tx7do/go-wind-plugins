package flatbuffers

import (
	"errors"
	"testing"

	fb "github.com/google/flatbuffers/go"

	"github.com/tx7do/go-wind-plugins/encoding"
)

// testTable is a minimal hand-written stand-in for a flatc-generated table
// type. It satisfies fb.FlatBuffer (Init + Table) and exposes one int32 field
// (slot 0) and one string field (slot 1), mirroring the accessors flatc
// generates. No generated types or external files are required.
type testTable struct {
	tab fb.Table
}

func (t *testTable) Init(buf []byte, i fb.UOffsetT) {
	t.tab = fb.Table{
		Bytes: buf,
		Pos:   i,
	}
}

func (t *testTable) Table() fb.Table { return t.tab }

// ID reads the int32 stored in slot 0 (vtable offset 4).
func (t *testTable) ID() int32 { return t.tab.GetInt32Slot(0, 0) }

// Name reads the string stored in slot 1 (vtable offset 6).
func (t *testTable) Name() string {
	o := fb.UOffsetT(t.tab.Offset(6))
	if o != 0 {
		return t.tab.String(o + t.tab.Pos)
	}
	return ""
}

// testEntity is the source value used to produce a FlatBuffer. It implements
// FlatBufferMarshaler by building the buffer with the flatbuffers Builder API,
// the same way generated Pack() methods do.
type testEntity struct {
	id   int32
	name string
}

func (e *testEntity) PackFlatBuffer() ([]byte, error) {
	b := fb.NewBuilder(0)
	name := b.CreateString(e.name)
	b.StartObject(2)
	b.PrependInt32Slot(0, e.id, 0)
	b.PrependUOffsetTSlot(1, name, 0)
	root := b.EndObject()
	b.Finish(root)
	return b.FinishedBytes(), nil
}

// errPacked is returned by failingEntity to exercise the Marshal error path.
var errPacked = errors.New("flatbuffers: test pack failure")

// failingEntity always fails to pack, to exercise the Marshal error path.
type failingEntity struct{}

func (failingEntity) PackFlatBuffer() ([]byte, error) {
	return nil, errPacked
}

// ---------------------------------------------------------------------------
// Name / registration
// ---------------------------------------------------------------------------

func TestName(t *testing.T) {
	var c codec
	if got := c.Name(); got != Name {
		t.Errorf("Name() = %q, want %q", got, Name)
	}
	if Name != "flatbuffers" {
		t.Errorf("Name constant = %q, want %q", Name, "flatbuffers")
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

var (
	_ FlatBufferMarshaler = (*testEntity)(nil)
	_ fb.FlatBuffer       = (*testTable)(nil)
)

// ---------------------------------------------------------------------------
// Marshal / Unmarshal roundtrip
// ---------------------------------------------------------------------------

func TestMarshalUnmarshalRoundtrip(t *testing.T) {
	c := codec{}

	src := &testEntity{id: 42, name: "hello"}

	data, err := c.Marshal(src)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Marshal() returned empty bytes")
	}

	dst := &testTable{}
	if err := c.Unmarshal(data, dst); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got := dst.ID(); got != 42 {
		t.Errorf("decoded ID = %d, want %d", got, 42)
	}
	if got := dst.Name(); got != "hello" {
		t.Errorf("decoded Name = %q, want %q", got, "hello")
	}
}

func TestMarshalUnmarshal_ZeroValue(t *testing.T) {
	c := codec{}

	src := &testEntity{}

	data, err := c.Marshal(src)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	dst := &testTable{}
	if err := c.Unmarshal(data, dst); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got := dst.ID(); got != 0 {
		t.Errorf("decoded ID = %d, want %d", got, 0)
	}
	if got := dst.Name(); got != "" {
		t.Errorf("decoded Name = %q, want empty string", got)
	}
}

// ---------------------------------------------------------------------------
// Error paths
// ---------------------------------------------------------------------------

func TestMarshal_NonMarshaler(t *testing.T) {
	c := codec{}
	// A plain value does not implement FlatBufferMarshaler.
	if _, err := c.Marshal(map[string]any{"id": 42}); err == nil {
		t.Error("Marshal of a value not implementing FlatBufferMarshaler should return an error")
	}
}

func TestMarshal_PackError(t *testing.T) {
	c := codec{}
	if _, err := c.Marshal(failingEntity{}); err == nil {
		t.Error("Marshal should propagate the error returned by PackFlatBuffer")
	}
}

func TestUnmarshal_NonFlatBufferTarget(t *testing.T) {
	c := codec{}

	data, err := c.Marshal(&testEntity{id: 1, name: "x"})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	// A plain pointer does not implement fb.FlatBuffer.
	var got map[string]any
	if err := c.Unmarshal(data, &got); err == nil {
		t.Error("Unmarshal into a target not implementing fb.FlatBuffer should return an error")
	}
}

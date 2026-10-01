package proto

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/tx7do/go-wind-plugins/encoding"
)

// testMessage returns a representative structpb.Struct message. The
// google.golang.org/protobuf module is already a direct dependency of this
// package, and structpb is a hand-written (non-generated) message type from
// that module, so no protoc tooling or new go.mod entries are needed.
func testMessage() *structpb.Struct {
	return &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"name": structpb.NewStringValue("Alice"),
			"age":  structpb.NewNumberValue(30),
			"tags": structpb.NewListValue(&structpb.ListValue{
				Values: []*structpb.Value{
					structpb.NewStringValue("admin"),
					structpb.NewStringValue("dev"),
				},
			}),
		},
	}
}

// ---------------------------------------------------------------------------
// Name / registration
// ---------------------------------------------------------------------------

func TestName(t *testing.T) {
	var c codec
	if got := c.Name(); got != Name {
		t.Errorf("Name() = %q, want %q", got, Name)
	}
	if Name != "proto" {
		t.Errorf("Name constant = %q, want %q", Name, "proto")
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

func TestMarshalUnmarshalRoundtrip(t *testing.T) {
	c := codec{}

	want := testMessage()

	data, err := c.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Marshal() returned empty bytes")
	}

	got := &structpb.Struct{}
	if err := c.Unmarshal(data, got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !proto.Equal(want, got) {
		t.Errorf("roundtrip = %v, want %v", got, want)
	}
}

func TestMarshalUnmarshal_EmptyMessage(t *testing.T) {
	c := codec{}

	data, err := c.Marshal(&structpb.Struct{})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	got := &structpb.Struct{}
	if err := c.Unmarshal(data, got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(got.Fields) != 0 {
		t.Errorf("decoded fields = %v, want empty", got.Fields)
	}
}

// ---------------------------------------------------------------------------
// Error paths
// ---------------------------------------------------------------------------

func TestMarshal_NonMessage(t *testing.T) {
	c := codec{}
	if _, err := c.Marshal(map[string]any{"name": "Alice"}); err == nil {
		t.Error("Marshal of a non proto.Message value should return an error")
	}
}

func TestUnmarshal_NonMessageTarget(t *testing.T) {
	c := codec{}
	var got map[string]any
	if err := c.Unmarshal([]byte{0x0a}, &got); err == nil {
		t.Error("Unmarshal into a non proto.Message target should return an error")
	}
}

func TestUnmarshal_GarbageData(t *testing.T) {
	c := codec{}

	tests := []struct {
		name string
		data []byte
	}{
		{name: "field number zero", data: []byte{0x01}},
		{name: "invalid wire type", data: []byte{0xff, 0x01}},
		{name: "text garbage", data: []byte("garbage")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := &structpb.Struct{}
			if err := c.Unmarshal(tt.data, got); err == nil {
				t.Errorf("Unmarshal(%v) should return an error", tt.data)
			}
		})
	}
}

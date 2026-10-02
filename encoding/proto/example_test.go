package proto_test

import (
	"fmt"

	"google.golang.org/protobuf/types/known/structpb"

	"github.com/tx7do/go-wind-plugins/encoding"
	"github.com/tx7do/go-wind-plugins/encoding/proto"
)

// Example resolves the protobuf codec through the shared codec registry and
// round-trips a structpb.Struct message through it. The codec was registered
// under its name when this package was imported; messages must implement
// proto.Message, and structpb provides a hand-written one that needs no
// generated code.
func Example() {
	codec := encoding.GetCodec(proto.Name)
	if codec == nil {
		return
	}

	encoded, err := codec.Marshal(&structpb.Struct{
		Fields: map[string]*structpb.Value{
			"name": structpb.NewStringValue("Alice"),
		},
	})
	if err != nil {
		return
	}

	decoded := &structpb.Struct{}
	if err := codec.Unmarshal(encoded, decoded); err != nil {
		return
	}

	fmt.Println(decoded.GetFields()["name"].GetStringValue())
	// Output: Alice
}

package bson_test

import (
	"fmt"

	"github.com/tx7do/go-wind-plugins/encoding"
	"github.com/tx7do/go-wind-plugins/encoding/bson"
)

type sampleMessage struct {
	Name string
}

// Example resolves the BSON codec through the shared codec registry and
// round-trips a small document through it. The codec was registered under
// its name when this package was imported, so payload handling code depends
// only on that name, never on the codec's concrete type.
func Example() {
	codec := encoding.GetCodec(bson.Name)
	if codec == nil {
		return
	}

	encoded, err := codec.Marshal(sampleMessage{Name: "hello"})
	if err != nil {
		return
	}

	var decoded sampleMessage
	if err := codec.Unmarshal(encoded, &decoded); err != nil {
		return
	}

	fmt.Println(decoded.Name)
	// Output: hello
}

package msgpack_test

import (
	"fmt"

	"github.com/tx7do/go-wind-plugins/encoding"
	"github.com/tx7do/go-wind-plugins/encoding/msgpack"
)

type sampleMessage struct {
	Name string
}

// Example resolves the MessagePack codec through the shared codec registry
// and round-trips a small message through it. The codec was registered under
// its name when this package was imported, so components exchanging
// compact binary payloads stay format-agnostic: they ask for the codec by
// name and never link against it directly.
func Example() {
	codec := encoding.GetCodec(msgpack.Name)
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

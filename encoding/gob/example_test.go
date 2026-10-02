package gob_test

import (
	"fmt"

	"github.com/tx7do/go-wind-plugins/encoding"
	"github.com/tx7do/go-wind-plugins/encoding/gob"
)

type sampleMessage struct {
	Name string
}

// Example resolves the gob codec through the shared codec registry and
// round-trips a small message through it. The codec was registered under
// its name when this package was imported; gob only exchanges values
// between Go programs, which makes it a fit for internal links where both
// ends share the same types.
func Example() {
	codec := encoding.GetCodec(gob.Name)
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

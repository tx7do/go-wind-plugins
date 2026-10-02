package cbor_test

import (
	"fmt"

	"github.com/tx7do/go-wind-plugins/encoding"
	"github.com/tx7do/go-wind-plugins/encoding/cbor"
)

type sampleMessage struct {
	Name string
}

// Example resolves the CBOR codec through the shared codec registry and
// round-trips a small message through it. The codec was registered under
// its name when this package was imported, so components that exchange
// compact binary payloads reference the codec by that name alone and stay
// independent of the format behind it.
func Example() {
	codec := encoding.GetCodec(cbor.Name)
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

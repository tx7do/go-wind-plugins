package json_test

import (
	"fmt"

	"github.com/tx7do/go-wind-plugins/encoding"
	"github.com/tx7do/go-wind-plugins/encoding/json"
)

type sampleMessage struct {
	Name string `json:"name"`
}

// Example resolves the JSON codec through the shared codec registry and
// round-trips a small message through it. The codec was registered under
// its name when this package was imported, so components that move payloads
// refer to it by that name while the message itself remains a plain struct
// with JSON field tags.
func Example() {
	codec := encoding.GetCodec(json.Name)
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

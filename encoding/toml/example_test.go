package toml_test

import (
	"fmt"

	"github.com/tx7do/go-wind-plugins/encoding"
	"github.com/tx7do/go-wind-plugins/encoding/toml"
)

type sampleMessage struct {
	Name string `toml:"name"`
}

// Example resolves the TOML codec through the shared codec registry and
// round-trips a small message through it. The codec was registered under
// its name when this package was imported, so components that exchange
// configuration-style payloads depend only on that name, while the message
// itself remains a plain struct with TOML field tags.
func Example() {
	codec := encoding.GetCodec(toml.Name)
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

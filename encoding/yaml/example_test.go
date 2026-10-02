package yaml_test

import (
	"fmt"

	"github.com/tx7do/go-wind-plugins/encoding"
	"github.com/tx7do/go-wind-plugins/encoding/yaml"
)

type sampleMessage struct {
	Name string `yaml:"name"`
}

// Example resolves the YAML codec through the shared codec registry and
// round-trips a small message through it. The codec was registered under
// its name when this package was imported, so components that exchange
// YAML documents refer to it by that name alone, while the message itself
// remains a plain struct with YAML field tags.
func Example() {
	codec := encoding.GetCodec(yaml.Name)
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

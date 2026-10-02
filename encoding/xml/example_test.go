package xml_test

import (
	"fmt"

	"github.com/tx7do/go-wind-plugins/encoding"
	"github.com/tx7do/go-wind-plugins/encoding/xml"
)

type sampleMessage struct {
	XMLName struct{} `xml:"root"`
	Name    string   `xml:"name"`
}

// Example resolves the XML codec through the shared codec registry and
// round-trips a small message through it. The codec was registered under
// its name when this package was imported, so components that exchange
// XML documents refer to it by that name alone, while the message itself
// is an ordinary struct following the field-tag conventions of
// encoding/xml.
func Example() {
	codec := encoding.GetCodec(xml.Name)
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

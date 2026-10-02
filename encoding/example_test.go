package encoding_test

import (
	"fmt"

	"github.com/tx7do/go-wind-plugins/encoding"
)

// demoCodec is a minimal codec implementation used by the example below.
type demoCodec struct{}

func (demoCodec) Name() string {
	return "demo"
}

func (demoCodec) Marshal(v any) ([]byte, error) {
	return nil, nil
}

func (demoCodec) Unmarshal(data []byte, v any) error {
	return nil
}

// ExampleGetCodec registers a codec implementation under its name and then
// retrieves it through the registry. Codec packages register themselves this
// way when they are imported; afterwards any component that encodes or
// decodes payloads resolves the codec by that name, with lookups ignoring
// differences in letter case.
func ExampleGetCodec() {
	encoding.RegisterCodec(demoCodec{})

	codec := encoding.GetCodec("DEMO")
	if codec == nil {
		return
	}

	fmt.Println(codec.Name())
	// Output: demo
}

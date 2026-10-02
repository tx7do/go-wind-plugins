package avro_test

import (
	"fmt"

	"github.com/tx7do/go-wind-plugins/encoding/avro"
)

// sampleSchema describes a minimal Avro record with a single string field.
const sampleSchema = `{"type":"record","name":"User","fields":[{"name":"name","type":"string"}]}`

// ExampleNewCodec builds an Avro codec from a JSON-encoded record schema and
// round-trips a record that conforms to it. Avro binds each codec instance
// to the schema it was constructed with, so the encoding and the decoding
// side agree on the record's shape without any generated code.
func ExampleNewCodec() {
	codec, err := avro.NewCodec(sampleSchema)
	if err != nil {
		return
	}

	encoded, err := codec.Marshal(map[string]any{"name": "Alice"})
	if err != nil {
		return
	}

	decoded := map[string]any{}
	if err := codec.Unmarshal(encoded, &decoded); err != nil {
		return
	}

	fmt.Println(decoded["name"])
	// Output: Alice
}

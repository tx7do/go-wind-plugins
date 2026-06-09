// Package cbor provides a [encoding.Codec] implementation using
// CBOR (Concise Binary Object Representation, RFC 8949) via
// github.com/fxamacker/cbor/v2.
//
// CBOR is a binary data format that is semantically equivalent to JSON but
// significantly smaller and faster. It is used in COSE, WebAuthn, and other
// IETF standards.
//
// The codec self-registers under the name "cbor" via init().
package cbor

import (
	"github.com/fxamacker/cbor/v2"

	"github.com/tx7do/go-wind-plugins/encoding"
)

// Name is the name registered for the CBOR codec.
const Name = "cbor"

func init() {
	encoding.RegisterCodec(codec{})
}

// codec implements encoding.Codec using fxamacker/cbor/v2.
type codec struct{}

// Marshal encodes v into CBOR binary bytes.
func (codec) Marshal(v any) ([]byte, error) {
	return cbor.Marshal(v)
}

// Unmarshal decodes CBOR binary data into v.
func (codec) Unmarshal(data []byte, v any) error {
	return cbor.Unmarshal(data, v)
}

// Name returns the codec name.
func (codec) Name() string {
	return Name
}

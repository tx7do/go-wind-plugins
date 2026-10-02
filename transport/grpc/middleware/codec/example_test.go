package codec_test

import (
	grpccodec "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/codec"
)

// ExampleRegisterByName bridges codecs from the framework's encoding registry
// into gRPC's native codec registry, keyed by name, so that gRPC peers can
// negotiate a non-protobuf wire format through the Content-Subtype header;
// on the server side gRPC selects the codec from the incoming Content-Type
// header automatically. Names absent from the registry are skipped.
//
// Codec implementations are registered by importing the encoding packages
// for their side effects, e.g.
// _ "github.com/tx7do/go-wind-plugins/encoding/json".
func ExampleRegisterByName() {
	_ = grpccodec.RegisterByName("json")
}

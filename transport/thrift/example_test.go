package thrift_test

import (
	"fmt"

	thrifttransport "github.com/tx7do/go-wind-plugins/transport/thrift"
)

// ExampleNewServer constructs a Thrift RPC server with the binary protocol
// and the recovery and logging processor wrappers (the first wrapper
// registered is outermost). Thrift does not run over HTTP, so cross-cutting
// concerns are applied via these processor wrappers rather than HTTP
// middleware. The service processor itself — normally generated from a
// Thrift IDL, e.g. the echo service in testing/api/thrift — is mounted with
// WithProcessor.
func ExampleNewServer() {
	srv := thrifttransport.NewServer(":7700",
		thrifttransport.WithProtocol("binary"),
		thrifttransport.WithRecovery(nil), // outermost: catches panics in handlers
		thrifttransport.WithLogging(nil),  // logs every RPC call with duration
	)

	fmt.Println(srv.Endpoint())
}

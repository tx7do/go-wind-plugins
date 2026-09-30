package graphql_test

import (
	"github.com/99designs/gqlgen/graphql/playground"
	graphqltransport "github.com/tx7do/go-wind-plugins/transport/graphql"
)

// ExampleNewServer constructs a GraphQL server. GraphQL handlers are ordinary
// http.HandlerFunc, so the interactive playground mounts with HandleFunc
// without any adapter; the executable schema itself is mounted with
// srv.Handle("/query", schema). Because the server runs on top of net/http,
// Use also accepts the middleware instances from transport/http/middleware.
func ExampleNewServer() {
	srv := graphqltransport.NewServer(":8080")

	srv.HandleFunc("/", playground.Handler("GraphQL Playground", "/query"))
}

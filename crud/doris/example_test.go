package doris_test

import (
	"time"

	"github.com/tx7do/go-wind-plugins/crud/doris"
)

// ExampleNewClient constructs a Doris client from a MySQL-protocol DSN and
// wires the HTTP endpoint used for stream-load bulk imports, with
// connection-pool limits applied to the underlying handle. In the data
// access layer the client's query and exec helpers run statements against
// the connected database, and Repository layers typed CRUD operations on
// top of the same handle.
func ExampleNewClient() {
	client, err := doris.NewClient(
		doris.WithDSN("demo-user:demo-password@tcp(localhost:9030)/demo_db"),
		doris.WithMaxOpenConns(10),
		doris.WithMaxIdleConns(5),
		doris.WithConnMaxLifetime(5*time.Minute),
		doris.WithStreamLoadEndpoint("http://localhost:8030"),
		doris.WithStreamLoadAuth("demo-user", "demo-password"),
	)
	if err != nil {
		return
	}
	defer client.Close()
}

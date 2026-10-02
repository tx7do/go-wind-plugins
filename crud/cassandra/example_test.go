package cassandra_test

import (
	"time"

	"github.com/tx7do/go-wind-plugins/crud/cassandra"
)

// ExampleNewCassandraClient constructs a Cassandra client from connection
// options and releases the underlying session when the data access layer
// shuts down. The client's Query and Exec helpers execute row-returning and
// mutating CQL statements against the connected keyspace.
func ExampleNewCassandraClient() {
	client, err := cassandra.NewCassandraClient(
		cassandra.WithHosts("localhost:9042"),
		cassandra.WithUsername("demo-user"),
		cassandra.WithPassword("demo-password"),
		cassandra.WithKeyspace("demo_keyspace"),
		cassandra.WithConnectTimeout(5*time.Second),
		cassandra.WithTimeout(10*time.Second),
	)
	if err != nil {
		return
	}
	defer client.Close()
}

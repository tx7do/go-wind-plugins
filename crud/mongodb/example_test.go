package mongodb_test

import (
	"github.com/tx7do/go-wind-plugins/crud/mongodb"
)

// ExampleNewClient constructs a MongoDB data-access client from a connection
// URI and a database name. In an application's data access layer the client
// owns the database session; the document helpers built on top of it then
// perform the actual find, insert, update and delete operations against
// collections, which requires the live connection established here.
func ExampleNewClient() {
	client, err := mongodb.NewClient(
		mongodb.WithURI("mongodb://user:password@host:port/"),
		mongodb.WithDatabase("placeholder-database"),
	)
	if err != nil {
		return
	}
	defer client.Close()

	_ = client
}

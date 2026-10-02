package entgo_test

import (
	"time"

	"github.com/tx7do/go-wind-plugins/crud/entgo"
)

// demoClient stands in for the generated Ent client handle that the
// application's code generation produces.
type demoClient struct{}

func (demoClient) Close() error { return nil }

// ExampleNewEntClient wires an Ent client for the data access layer:
// CreateDriver opens the underlying SQL driver from a DSN, NewEntClient
// pairs it with the generated client handle, and SetConnectionOption tunes
// the connection pool. The container's query helpers and the Repository
// layer built on top of it execute CRUD statements through the wrapped
// driver.
func ExampleNewEntClient() {
	drv, err := entgo.CreateDriver(
		"mysql",
		"demo-user:demo-password@tcp(localhost:3306)/demo_db",
		true,
		false,
	)
	if err != nil {
		return
	}

	client := entgo.NewEntClient[demoClient](demoClient{}, drv)
	defer client.Close()

	client.SetConnectionOption(5, 10, 5*time.Minute)
}

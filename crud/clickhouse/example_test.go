package clickhouse_test

import (
	"time"

	"github.com/tx7do/go-wind-plugins/crud/clickhouse"
)

// ExampleNewClient constructs a ClickHouse client from endpoint, credential,
// and timeout options, and releases the underlying connection when the data
// access layer shuts down. The client's Query and Insert helpers run SELECT
// scans and writes against the configured database, and Repository layers
// typed CRUD operations on top of the same connection.
func ExampleNewClient() {
	client, err := clickhouse.NewClient(
		clickhouse.WithAddresses("localhost:9000"),
		clickhouse.WithUsername("demo-user"),
		clickhouse.WithPassword("demo-password"),
		clickhouse.WithDatabase("demo_db"),
		clickhouse.WithDialTimeout(5*time.Second),
		clickhouse.WithReadTimeout(30*time.Second),
	)
	if err != nil {
		return
	}
	defer client.Close()
}

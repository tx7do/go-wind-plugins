package influxdb_test

import (
	"github.com/tx7do/go-wind-plugins/crud/influxdb"
)

// ExampleNewClient constructs an InfluxDB data-access client from a host, an
// API token and a database name. In an application's data access layer the
// client owns the time-series connection; the insert and query helpers built
// on top of it then write points and run InfluxQL statements, which requires
// the live connection established here.
func ExampleNewClient() {
	client, err := influxdb.NewClient(
		influxdb.WithHost("http://host:port"),
		influxdb.WithToken("placeholder-token"),
		influxdb.WithDatabase("placeholder-database"),
	)
	if err != nil {
		return
	}
	defer client.Close()

	_ = client
}

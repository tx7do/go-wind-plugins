package gorm_test

import (
	"github.com/tx7do/go-wind-plugins/crud/gorm"
)

// ExampleNewClient constructs a GORM data-access client from a driver name and
// a DSN. In an application's data access layer the client owns the ORM
// session; the repository helpers built on top of it then issue the actual
// create, read, update and delete statements against the mapped entities,
// which requires the live connection established here.
func ExampleNewClient() {
	client, err := gorm.NewClient(
		gorm.WithDriverName("mysql"),
		gorm.WithDSN("user:password@tcp(host:port)/dbname"),
	)
	if err != nil {
		return
	}
	_ = client
}

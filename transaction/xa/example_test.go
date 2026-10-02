package xa_test

import (
	"context"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/tx7do/go-wind-plugins/transaction/xa"
)

// ExampleRun coordinates one business function across two databases inside
// a single XA transaction. Run binds a connection to each Resource and opens
// an XA branch on it; the Session routes each statement to the branch owning
// its resource by name, and afterwards every branch is ended, prepared, and
// — only if all prepares succeeded — committed, so the writes land
// everywhere or nowhere. In a real service the resources are the service's
// own database handles, the ones its gorm/ent repositories sit on, and the
// pattern is reserved for short, small-fanout writes that must be atomic; a
// crash between prepare and commit strands a prepared branch, which
// operators list with Pending and resolve with CommitPending or
// RollbackPending. The handles below are SQL mocks standing in for two MySQL
// instances.
func ExampleRun() {
	db, _, err := sqlmock.New()
	if err != nil {
		return
	}
	defer db.Close()

	err = xa.Run(context.Background(),
		func(ctx context.Context, s *xa.Session) error {
			if _, err := s.Exec(ctx, "orders", "UPDATE orders SET paid = 1 WHERE id = ?", "order-1"); err != nil {
				return err
			}
			_, err := s.Exec(ctx, "stocks", "UPDATE stocks SET locked = locked + 1 WHERE sku = ?", "sku-1")
			return err
		},
		xa.Resource{Name: "orders", DB: db},
		xa.Resource{Name: "stocks", DB: db},
	)
	if err != nil {
		return
	}
}

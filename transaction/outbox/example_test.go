package outbox_test

import (
	"context"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/tx7do/go-wind-plugins/broker"
	"github.com/tx7do/go-wind-plugins/transaction/outbox"
)

type orderCreated struct {
	OrderID string `json:"order_id"`
	Amount  int64  `json:"amount"`
}

// nopPublisher stands in for a real broker plugin (kafka, rabbitmq, ...);
// the relay only ever calls Publish on it.
type nopPublisher struct{}

func (nopPublisher) Publish(context.Context, string, *broker.Message, ...broker.PublishOption) error {
	return nil
}

// ExampleEnqueue records an outgoing event inside the database transaction
// that also carries the business writes: the outbox row commits atomically
// with them, and only committed rows are ever visible to a relay. In a real
// service this tx is the one the request handler opened around its gorm/ent
// repository writes, and the payload type is shared with the consuming
// service so its side can decode the event back into the struct.
func ExampleEnqueue() {
	db, _, err := sqlmock.New()
	if err != nil {
		return
	}
	defer db.Close()

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return
	}

	err = outbox.Enqueue(context.Background(), tx, outbox.Event[orderCreated]{
		Key:     "order-1",
		Payload: orderCreated{OrderID: "order-1", Amount: 100},
	})
	if err != nil {
		return
	}

	if err := tx.Commit(); err != nil {
		return
	}
}

// ExampleNewRelay builds the relay that drains the outbox table into a
// broker: the store binds to the producing service's database, and the
// publisher is any broker implementation — the stand-in type above
// represents one. The options set the poll interval, the batch size, and
// the sweep horizon for completed rows; Run drives the
// claim-publish-complete cycle behind them and belongs in a background
// goroutine with a cancelable context, typically inside the producing
// service or a dedicated dispatcher process. The single-cycle form, Once,
// is what Run polls.
func ExampleNewRelay() {
	db, _, err := sqlmock.New()
	if err != nil {
		return
	}
	defer db.Close()

	relay := outbox.NewRelay(
		outbox.NewStore(db),
		nopPublisher{},
		outbox.WithBatchSize(100),
		outbox.WithPollInterval(time.Second),
		outbox.WithSweepAfter(time.Hour),
	)

	published, err := relay.Once(context.Background())
	if err != nil {
		return
	}
	_ = published
}

// Package outbox implements the transactional outbox pattern on top of the
// broker package: business rows and outgoing events are written in one local
// database transaction, a relay publishes the events afterwards, and the
// consumer side suppresses redeliveries. It provides the same guarantees as
// DTM's 2-phase message pattern without running an external transaction
// server — the database IS the message store.
//
// Producer (inside any local transaction):
//
//	tx, _ := db.BeginTx(ctx, nil)
//	// ... business writes on tx ...
//	err := outbox.Enqueue(ctx, tx, outbox.Event[OrderCreated]{
//	    Key:     "order-123",
//	    Payload: OrderCreated{ID: "order-123", Amount: 100},
//	})
//	tx.Commit()
//
// Relay (publishes committed events to any broker implementation):
//
//	relay := outbox.NewRelay(outbox.NewStore(db), brokerImpl)
//	go relay.Run(ctx)
//
// Consumer (typed payload + dedupe):
//
//	outbox.Subscribe(brokerImpl, db, "billing", "OrderCreated",
//	    func(ctx context.Context, topic string, h broker.Headers, e *OrderCreated) error {
//	        // ...
//	        return nil
//	    })
//
// Outbox is at-least-once: consumers must be idempotent. For handlers whose
// effects are database writes, use Barrier to run the business writes and the
// dedupe record in one transaction — that is exactly-once for database state.
package outbox

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"time"
)

// DBTX is the subset of *sql.DB and *sql.Tx used to enqueue events. Passing
// the *sql.Tx running the business transaction is the normal case: the event
// becomes visible atomically with the business writes.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Event is a typed event envelope.
type Event[T any] struct {
	// ID uniquely identifies the event. When empty a UUID v4 is generated.
	// Consumers dedupe on it, so keep it stable across enqueue retries.
	ID string

	// Topic overrides the destination topic. When empty the payload type
	// name (Topic[T]()) is used.
	Topic string

	// Key optionally carries a partition/routing key (Kafka Key, RabbitMQ
	// RoutingKey, ...).
	Key string

	// Metadata is copied into the published message headers.
	Metadata map[string]string

	// Payload is the typed event body, JSON-serialized into the outbox row.
	Payload T
}

// Row is one outbox record as claimed by the Relay.
type Row struct {
	ID       string
	Topic    string
	Key      string
	Payload  []byte // JSON-encoded event payload
	Metadata []byte // JSON-encoded map[string]string, nil when empty
	Attempts int    // delivery attempts, starting at 1 for the current claim
}

// Header constants attached by the relay to every published message.
const (
	HeaderEventID = "outbox-event-id"
	HeaderTopic   = "outbox-topic"
)

// Topic returns the default topic name for payload type T: its Go type name.
// Producer and consumer agree on it by sharing the payload type.
func Topic[T any]() string {
	t := reflect.TypeOf((*T)(nil)).Elem()
	if name := t.Name(); name != "" {
		return name
	}
	return fmt.Sprintf("%T", *new(T))
}

// Enqueue writes the event into the outbox table on tx. Call it inside the
// same database transaction as the business writes; the event is then
// delivered by a Relay only if the transaction commits.
func Enqueue[T any](ctx context.Context, tx DBTX, e Event[T], opts ...Option) error {
	cfg := newConfig(opts...)

	payload, err := json.Marshal(e.Payload)
	if err != nil {
		return fmt.Errorf("outbox: marshal %s payload: %w", Topic[T](), err)
	}
	id := e.ID
	if id == "" {
		id = newID()
	}
	topic := e.Topic
	if topic == "" {
		topic = Topic[T]()
	}
	var metadata any
	if len(e.Metadata) > 0 {
		b, err := json.Marshal(e.Metadata)
		if err != nil {
			return fmt.Errorf("outbox: marshal metadata: %w", err)
		}
		metadata = b
	}

	_, err = tx.ExecContext(ctx,
		"INSERT INTO "+cfg.outboxTable+
			" (id, topic, event_key, payload, metadata, status, attempts, created_at)"+
			" VALUES (?, ?, ?, ?, ?, 'pending', 0, ?)",
		id, topic, e.Key, payload, metadata, cfg.now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("outbox: enqueue event %s: %w", id, err)
	}
	return nil
}

// newID returns a UUID v4 formatted string without external dependencies.
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand never fails on supported platforms
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// clock is injectable for tests.
func nowFunc() time.Time { return time.Now() }

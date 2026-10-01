package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/tx7do/go-wind-plugins/broker"
)

// Inbox returns a broker.Handler that decodes the event payload into T and
// invokes h at most once per (name, event ID) pair: the dedupe record is
// written before the handler runs, so broker redeliveries are suppressed.
//
// The handler itself is at-most-once: if the process crashes between the
// record and the handler, or h fails, the event is not retried. When the
// handler effects are database writes and the event must not be lost, use
// Barrier instead — it runs the writes and the record in one transaction.
//
// Subscribe the returned handler directly, or use Subscribe for the common
// wiring:
//
//	broker.Subscribe(topic, outbox.Inbox(db, "billing", handler), nil)
func Inbox[T any](db *sql.DB, name string, h broker.TypedHandler[T], opts ...Option) broker.Handler {
	cfg := newConfig(opts...)
	return func(ctx context.Context, evt broker.Event) error {
		payload, err := decodePayload[T](evt)
		if err != nil {
			return err
		}
		// Single INSERT ... WHERE NOT EXISTS is atomic on its own, so no
		// explicit transaction is needed for the record-first check.
		inserted, err := insertInboxRow(ctx, db, cfg, name, eventID(evt.Message()))
		if err != nil {
			return err
		}
		if !inserted {
			return nil
		}
		return h(ctx, evt.Topic(), msgHeaders(evt.Message()), payload)
	}
}

// Barrier is the transactional-consumer equivalent of DTM's branch barrier:
// the business handler runs inside a database transaction together with the
// inbox record, so either both commit or neither does. Database effects
// happen exactly once per event ID, even across crashes and redeliveries.
// Non-database side effects inside h are still at-least-once.
//
//	h := outbox.Barrier(db, "billing",
//	    func(ctx context.Context, tx *sql.Tx, e *OrderCreated) error {
//	        return charge(ctx, tx, e)
//	    })
func Barrier[T any](db *sql.DB, name string, h func(ctx context.Context, tx *sql.Tx, payload *T) error, opts ...Option) broker.Handler {
	cfg := newConfig(opts...)
	return func(ctx context.Context, evt broker.Event) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("outbox: begin barrier: %w", err)
		}
		defer func() { _ = tx.Rollback() }()

		// Insert the inbox record inside the business transaction. Zero rows
		// affected means the event was already processed: commit an empty
		// transaction and drop the duplicate.
		inserted, err := insertInboxRow(ctx, tx, cfg, name, eventID(evt.Message()))
		if err != nil {
			return err
		}
		if !inserted {
			return nil
		}

		payload, err := decodePayload[T](evt)
		if err != nil {
			return err
		}
		if err := h(ctx, tx, payload); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("outbox: commit barrier: %w", err)
		}
		return nil
	}
}

// Subscribe wires a typed, deduplicated handler to a broker topic in one
// call. The dedupe record lives in the inbox table keyed by name.
func Subscribe[T any](b broker.Broker, db *sql.DB, name, topic string, h broker.TypedHandler[T], opts ...Option) (broker.Subscriber, error) {
	return b.Subscribe(topic, Inbox[T](db, name, h, opts...), nil)
}

// EventID returns the dedupe key of an event: the message ID, the outbox
// event-id header, or an empty string when neither is present (the event is
// then processed every time).
func EventID(evt broker.Event) string {
	if evt == nil {
		return ""
	}
	return eventID(evt.Message())
}

// execer is satisfied by *sql.DB and *sql.Tx alike, so the inbox record can
// be written standalone (Inbox) or inside the business transaction (Barrier).
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func eventID(msg *broker.Message) string {
	if msg == nil {
		return ""
	}
	if msg.ID != "" {
		return msg.ID
	}
	if msg.Headers != nil {
		return msg.Headers[HeaderEventID]
	}
	return ""
}

func msgHeaders(msg *broker.Message) broker.Headers {
	if msg == nil {
		return nil
	}
	return msg.Headers
}

// insertInboxRow inserts the dedupe record unless it exists, using
// INSERT ... SELECT ... WHERE NOT EXISTS, which is portable across MySQL,
// PostgreSQL and SQLite. It returns false when the record already exists.
func insertInboxRow(ctx context.Context, ex execer, cfg config, name, id string) (bool, error) {
	res, err := ex.ExecContext(ctx,
		"INSERT INTO "+cfg.inboxTable+" (handler, event_id, processed_at)"+
			" SELECT * FROM (SELECT ?, ?, ?) AS v"+
			" WHERE NOT EXISTS (SELECT 1 FROM "+cfg.inboxTable+
			" WHERE handler = ? AND event_id = ?)",
		name, id, cfg.now().UTC(), name, id,
	)
	if err != nil {
		return false, fmt.Errorf("outbox: record inbox %s/%s: %w", name, id, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("outbox: record inbox %s/%s: %w", name, id, err)
	}
	return affected > 0, nil
}

// decodePayload converts the transport-specific message body into *T. Real
// brokers deliver JSON bytes or driver-decoded maps; in-process brokers may
// deliver the typed value itself.
func decodePayload[T any](evt broker.Event) (*T, error) {
	if evt == nil || evt.Message() == nil {
		return nil, fmt.Errorf("outbox: event or message is nil")
	}
	body := evt.Message().Body
	switch v := body.(type) {
	case nil:
		return nil, fmt.Errorf("outbox: event body is nil")
	case *T:
		return v, nil
	case T:
		return &v, nil
	case []byte:
		var out T
		if err := json.Unmarshal(v, &out); err != nil {
			return nil, fmt.Errorf("outbox: decode event body: %w", err)
		}
		return &out, nil
	case string:
		var out T
		if err := json.Unmarshal([]byte(v), &out); err != nil {
			return nil, fmt.Errorf("outbox: decode event body: %w", err)
		}
		return &out, nil
	default:
		// Driver-decoded values (e.g. map[string]any) round-trip through
		// JSON to recover the typed payload.
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("outbox: re-encode event body: %w", err)
		}
		var out T
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("outbox: decode event body: %w", err)
		}
		return &out, nil
	}
}

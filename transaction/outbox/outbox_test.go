package outbox

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/tx7do/go-wind-plugins/broker"
)

type orderCreated struct {
	OrderID string `json:"order_id"`
	Amount  int64  `json:"amount"`
}

// fakeEvent satisfies broker.Event for handler tests.
type fakeEvent struct {
	topic string
	msg   *broker.Message
}

func (f fakeEvent) Topic() string            { return f.topic }
func (f fakeEvent) Message() *broker.Message { return f.msg }
func (f fakeEvent) RawMessage() any          { return nil }
func (f fakeEvent) Ack() error               { return nil }
func (f fakeEvent) Error() error             { return nil }

func newFakeEvent(id string, body any) fakeEvent {
	return fakeEvent{
		topic: "orderCreated",
		msg:   &broker.Message{ID: id, Body: body},
	}
}

func TestTopic(t *testing.T) {
	if got := Topic[orderCreated](); got != "orderCreated" {
		t.Fatalf("Topic = %q, want orderCreated", got)
	}
}

func TestEnqueueDefaults(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectExec("INSERT INTO outbox_events").
		WithArgs(
			sqlmock.AnyArg(),
			"orderCreated",
			"order-1",
			[]byte(`{"order_id":"order-1","amount":100}`),
			nil,
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = Enqueue(context.Background(), db, Event[orderCreated]{
		Key:     "order-1",
		Payload: orderCreated{OrderID: "order-1", Amount: 100},
	}, WithNow(func() time.Time { return time.Unix(0, 0) }))
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestEnqueueExplicitFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectExec("INSERT INTO outbox_events").
		WithArgs(
			"evt-1",
			"order.placed",
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			[]byte(`{"tenant":"acme"}`),
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = Enqueue(context.Background(), db, Event[orderCreated]{
		ID:       "evt-1",
		Topic:    "order.placed",
		Metadata: map[string]string{"tenant": "acme"},
		Payload:  orderCreated{OrderID: "order-1"},
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestEnsureSchema(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectExec("CREATE TABLE IF NOT EXISTS outbox_events").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE INDEX IF NOT EXISTS").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS inbox_messages").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := EnsureSchema(context.Background(), db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestDecodePayload(t *testing.T) {
	want := &orderCreated{OrderID: "o-1", Amount: 7}

	cases := []struct {
		name string
		body any
	}{
		{"pointer", want},
		{"value", *want},
		{"bytes", []byte(`{"order_id":"o-1","amount":7}`)},
		{"string", `{"order_id":"o-1","amount":7}`},
		{"map", map[string]any{"order_id": "o-1", "amount": float64(7)}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodePayload[orderCreated](newFakeEvent("e1", tc.body))
			if err != nil {
				t.Fatalf("decodePayload: %v", err)
			}
			if got.OrderID != want.OrderID || got.Amount != want.Amount {
				t.Fatalf("got %+v, want %+v", got, want)
			}
		})
	}
}

func TestDecodePayloadErrors(t *testing.T) {
	if _, err := decodePayload[orderCreated](newFakeEvent("e1", nil)); err == nil {
		t.Fatal("nil body should fail")
	}
	if _, err := decodePayload[orderCreated](nil); err == nil {
		t.Fatal("nil event should fail")
	}
	if _, err := decodePayload[orderCreated](newFakeEvent("e1", "not-json")); err == nil {
		t.Fatal("malformed body should fail")
	}
}

func TestEventIDPrefersMessageID(t *testing.T) {
	evt := fakeEvent{msg: &broker.Message{
		ID:      "m1",
		Headers: broker.Headers{HeaderEventID: "h1"},
	}}
	if got := EventID(evt); got != "m1" {
		t.Fatalf("EventID = %q, want m1", got)
	}

	evt = fakeEvent{msg: &broker.Message{
		Headers: broker.Headers{HeaderEventID: "h1"},
	}}
	if got := EventID(evt); got != "h1" {
		t.Fatalf("EventID = %q, want h1", got)
	}

	evt = fakeEvent{msg: &broker.Message{}}
	if got := EventID(evt); got != "" {
		t.Fatalf("EventID = %q, want empty", got)
	}
	if got := EventID(nil); got != "" {
		t.Fatalf("EventID(nil) = %q, want empty", got)
	}
}

func TestNewIDUnique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := newID()
		if len(id) != 36 {
			t.Fatalf("newID = %q, want UUID-formatted 36 chars", id)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

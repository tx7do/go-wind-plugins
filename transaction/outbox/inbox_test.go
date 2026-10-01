package outbox

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/tx7do/go-wind-plugins/broker"
)

// fakeBroker captures the handler wiring done by Subscribe.
type fakeBroker struct {
	gotHandler broker.Handler
	gotTopic   string
}

func (f *fakeBroker) Name() string                { return "fake" }
func (f *fakeBroker) Options() broker.Options     { return broker.Options{} }
func (f *fakeBroker) Address() string             { return "" }
func (f *fakeBroker) Init(...broker.Option) error { return nil }
func (f *fakeBroker) Connect() error              { return nil }
func (f *fakeBroker) Disconnect() error           { return nil }
func (f *fakeBroker) Publish(context.Context, string, *broker.Message, ...broker.PublishOption) error {
	return nil
}
func (f *fakeBroker) Request(context.Context, string, *broker.Message, ...broker.RequestOption) (*broker.Message, error) {
	return nil, nil
}
func (f *fakeBroker) Subscribe(topic string, handler broker.Handler, _ broker.Binder, _ ...broker.SubscribeOption) (broker.Subscriber, error) {
	f.gotTopic = topic
	f.gotHandler = handler
	return &fakeSubscriber{topic: topic}, nil
}

type fakeSubscriber struct{ topic string }

func (s *fakeSubscriber) Options() broker.SubscribeOptions { return broker.SubscribeOptions{} }
func (s *fakeSubscriber) Topic() string                    { return s.topic }
func (s *fakeSubscriber) Unsubscribe(bool) error           { return nil }

// counter is a handler invocation recorder.
type counter struct {
	mu    sync.Mutex
	calls int
}

func (c *counter) hit() {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
}

func (c *counter) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func expectInboxRecord(mock sqlmock.Sqlmock, inserted bool) {
	rows := int64(0)
	if inserted {
		rows = 1
	}
	mock.ExpectExec("INSERT INTO inbox_messages").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, rows))
}

func TestInboxSuppressesDuplicates(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	var calls counter
	handler := func(_ context.Context, topic string, _ broker.Headers, e *orderCreated) error {
		calls.hit()
		if e.OrderID != "o-1" {
			t.Errorf("payload = %+v", e)
		}
		return nil
	}
	h := Inbox[orderCreated](db, "billing", handler)

	// First delivery: record written, handler runs.
	expectInboxRecord(mock, true)
	if err := h(context.Background(), newFakeEvent("e1", []byte(`{"order_id":"o-1"}`))); err != nil {
		t.Fatalf("first delivery: %v", err)
	}

	// Redelivery: record already present, handler skipped.
	expectInboxRecord(mock, false)
	if err := h(context.Background(), newFakeEvent("e1", []byte(`{"order_id":"o-1"}`))); err != nil {
		t.Fatalf("redelivery: %v", err)
	}

	if got := calls.count(); got != 1 {
		t.Fatalf("handler calls = %d, want 1", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestInboxMalformedBodyNotRecorded(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	h := Inbox[orderCreated](db, "billing", func(context.Context, string, broker.Headers, *orderCreated) error {
		return nil
	})

	// Decode happens before the record: no INSERT expected.
	if err := h(context.Background(), newFakeEvent("e1", "not-json")); err == nil {
		t.Fatal("expected decode error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestBarrierRunsBusinessAndRecordTogether(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	var got orderCreated
	h := Barrier[orderCreated](db, "billing",
		func(_ context.Context, tx *sql.Tx, e *orderCreated) error {
			got = *e
			_, err := tx.ExecContext(context.Background(), "UPDATE orders SET paid = 1")
			return err
		})

	// First delivery: record + business in one transaction, committed.
	mock.ExpectBegin()
	expectInboxRecord(mock, true)
	mock.ExpectExec("UPDATE orders SET paid = 1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := h(context.Background(), newFakeEvent("e1", []byte(`{"order_id":"o-9"}`))); err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	if got.OrderID != "o-9" {
		t.Fatalf("payload = %+v", got)
	}

	// Duplicate: empty transaction rolled back, handler skipped.
	mock.ExpectBegin()
	expectInboxRecord(mock, false)
	mock.ExpectRollback()
	if err := h(context.Background(), newFakeEvent("e1", []byte(`{"order_id":"o-9"}`))); err != nil {
		t.Fatalf("duplicate delivery: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestBarrierRollsBackOnHandlerError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	boom := errors.New("boom")
	h := Barrier[orderCreated](db, "billing",
		func(context.Context, *sql.Tx, *orderCreated) error { return boom })

	mock.ExpectBegin()
	expectInboxRecord(mock, true)
	mock.ExpectRollback()
	if err := h(context.Background(), newFakeEvent("e1", []byte(`{}`))); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestSubscribeWiresTypedInbox(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	fb := &fakeBroker{}
	sub, err := Subscribe[orderCreated](fb, db, "billing", "orderCreated",
		func(context.Context, string, broker.Headers, *orderCreated) error { return nil })
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if sub.Topic() != "orderCreated" {
		t.Fatalf("topic = %q", sub.Topic())
	}
	if fb.gotTopic != "orderCreated" || fb.gotHandler == nil {
		t.Fatalf("broker got topic=%q handler=%v", fb.gotTopic, fb.gotHandler)
	}

	// The wired handler dedupes and decodes end to end.
	expectInboxRecord(mock, true)
	if err := fb.gotHandler(context.Background(),
		newFakeEvent("e1", []byte(`{"order_id":"o-1"}`))); err != nil {
		t.Fatalf("wired handler: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

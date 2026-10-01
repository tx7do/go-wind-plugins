package outbox

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/tx7do/go-wind-plugins/broker"
)

// memRecord is an in-memory outbox row for Store-interface tests.
type memRecord struct {
	row         Row
	status      string
	createdAt   time.Time
	claimedAt   time.Time
	completedAt time.Time
	nextVisible time.Time
	lastErr     string
}

// memStore implements Store without a database.
type memStore struct {
	mu   sync.Mutex
	now  func() time.Time
	recs []*memRecord
}

func newMemStore(now func() time.Time) *memStore {
	return &memStore{now: now}
}

func (m *memStore) seed(t *testing.T, id, topic string) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recs = append(m.recs, &memRecord{
		row:       Row{ID: id, Topic: topic, Payload: []byte(`{}`)},
		status:    "pending",
		createdAt: m.now(),
	})
}

func (m *memStore) status(id string) (memRecord, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.recs {
		if r.row.ID == id {
			return *r, true
		}
	}
	return memRecord{}, false
}

func (m *memStore) EnsureSchema(ctx context.Context) error { return nil }

func (m *memStore) Claim(ctx context.Context, limit int) ([]Row, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	var claimed []Row
	for _, r := range m.recs {
		if len(claimed) >= limit {
			break
		}
		if r.status != "pending" {
			continue
		}
		if !r.nextVisible.IsZero() && r.nextVisible.After(now) {
			continue
		}
		r.status = "claimed"
		r.row.Attempts++
		r.claimedAt = now
		claimed = append(claimed, r.row)
	}
	return claimed, nil
}

func (m *memStore) Reclaim(ctx context.Context, olderThan time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.recs {
		if r.status == "claimed" && r.claimedAt.Before(olderThan) {
			r.status = "pending"
			r.nextVisible = time.Time{}
			r.claimedAt = time.Time{}
		}
	}
	return nil
}

func (m *memStore) Complete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.recs {
		if r.row.ID == id {
			r.status = "done"
			r.completedAt = m.now()
		}
	}
	return nil
}

func (m *memStore) Fail(ctx context.Context, id, lastErr string, nextVisible time.Time, dead bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.recs {
		if r.row.ID == id {
			r.lastErr = lastErr
			if dead {
				r.status = "dead"
				r.nextVisible = time.Time{}
			} else {
				r.status = "pending"
				r.nextVisible = nextVisible
			}
		}
	}
	return nil
}

func (m *memStore) Sweep(ctx context.Context, olderThan time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var kept []*memRecord
	var removed int64
	for _, r := range m.recs {
		if r.status == "done" && r.completedAt.Before(olderThan) {
			removed++
			continue
		}
		kept = append(kept, r)
	}
	m.recs = kept
	return removed, nil
}

// fakePublisher records published messages; publishes to failTopic fail.
type fakePublisher struct {
	mu        sync.Mutex
	msgs      []*broker.Message
	failTopic string
}

func (p *fakePublisher) Publish(_ context.Context, topic string, msg *broker.Message, _ ...broker.PublishOption) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if topic == p.failTopic {
		return context.DeadlineExceeded
	}
	p.msgs = append(p.msgs, msg)
	return nil
}

func clockAt(start *time.Time) func() time.Time {
	return func() time.Time { return *start }
}

func TestRelayPublishesAndCompletes(t *testing.T) {
	now := time.Unix(1700000000, 0)
	store := newMemStore(clockAt(&now))
	store.seed(t, "e1", "orderCreated")
	store.seed(t, "e2", "stockReserved")
	pub := &fakePublisher{}

	relay := NewRelay(store, pub, WithRelayNow(clockAt(&now)))
	published, err := relay.Once(context.Background())
	if err != nil {
		t.Fatalf("Once: %v", err)
	}
	if published != 2 {
		t.Fatalf("published = %d, want 2", published)
	}
	for _, id := range []string{"e1", "e2"} {
		rec, ok := store.status(id)
		if !ok || rec.status != "done" {
			t.Fatalf("row %s: status = %q, want done", id, rec.status)
		}
	}
	if len(pub.msgs) != 2 {
		t.Fatalf("published messages = %d, want 2", len(pub.msgs))
	}
	msg := pub.msgs[0]
	if msg.ID != "e1" || msg.Headers[HeaderEventID] != "e1" || msg.Headers[HeaderTopic] != "orderCreated" {
		t.Fatalf("message = %+v", msg)
	}
}

func TestRelayRetriesThenDeadLetters(t *testing.T) {
	now := time.Unix(1700000000, 0)
	tick := func() *time.Time { return &now }
	store := newMemStore(clockAt(tick()))
	store.seed(t, "e1", "broken")
	pub := &fakePublisher{failTopic: "broken"}

	relay := NewRelay(store, pub,
		WithRelayNow(clockAt(tick())),
		WithMaxAttempts(2),
		WithRetryBackoff(func(int) time.Duration { return 10 * time.Second }),
	)

	published, err := relay.Once(context.Background())
	if err != nil {
		t.Fatalf("Once: %v", err)
	}
	if published != 0 {
		t.Fatalf("published = %d, want 0", published)
	}
	rec, _ := store.status("e1")
	if rec.status != "pending" || rec.row.Attempts != 1 || rec.nextVisible.Equal(now) {
		t.Fatalf("after first failure: status=%q attempts=%d nextVisible=%v", rec.status, rec.row.Attempts, rec.nextVisible)
	}

	now = now.Add(11 * time.Second) // past nextVisible, attempts=2 == max
	if _, err := relay.Once(context.Background()); err != nil {
		t.Fatalf("second Once: %v", err)
	}
	rec, _ = store.status("e1")
	if rec.status != "dead" || rec.lastErr == "" {
		t.Fatalf("after second failure: status=%q lastErr=%q, want dead", rec.status, rec.lastErr)
	}
}

func TestRelaySkipsInvisibleRows(t *testing.T) {
	now := time.Unix(1700000000, 0)
	store := newMemStore(clockAt(&now))
	store.seed(t, "e1", "broken")
	pub := &fakePublisher{failTopic: "broken"}

	relay := NewRelay(store, pub,
		WithRelayNow(clockAt(&now)),
		WithRetryBackoff(func(int) time.Duration { return time.Hour }),
	)
	if _, err := relay.Once(context.Background()); err != nil {
		t.Fatalf("Once: %v", err)
	}
	// Backoff has not elapsed: the row must stay unclaimed.
	_, err := relay.Once(context.Background())
	if err != nil {
		t.Fatalf("second Once: %v", err)
	}
	if len(pub.msgs) != 0 {
		t.Fatalf("publishes = %d, want 0", len(pub.msgs))
	}
}

func TestRelayReclaimsStaleClaims(t *testing.T) {
	now := time.Unix(1700000000, 0)
	tick := func() *time.Time { return &now }
	store := newMemStore(clockAt(tick()))
	store.seed(t, "e1", "orderCreated")

	// Simulate a relay that claimed the row and died: claim directly.
	if _, err := store.Claim(context.Background(), 10); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	now = now.Add(10 * time.Minute) // claim older than the lease

	pub := &fakePublisher{}
	relay := NewRelay(store, pub, WithRelayNow(clockAt(tick())), WithLease(5*time.Minute))
	published, err := relay.Once(context.Background())
	if err != nil {
		t.Fatalf("Once: %v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1 after reclaim", published)
	}
}

func TestRelaySweepsCompletedRows(t *testing.T) {
	now := time.Unix(1700000000, 0)
	tick := func() *time.Time { return &now }
	store := newMemStore(clockAt(tick()))
	store.seed(t, "e1", "orderCreated")
	pub := &fakePublisher{}

	relay := NewRelay(store, pub, WithRelayNow(clockAt(tick())), WithSweepAfter(30*time.Minute))
	if _, err := relay.Once(context.Background()); err != nil {
		t.Fatalf("Once: %v", err)
	}
	if _, ok := store.status("e1"); !ok {
		t.Fatal("row removed before sweep horizon")
	}
	now = now.Add(time.Hour)
	if _, err := relay.Once(context.Background()); err != nil {
		t.Fatalf("second Once: %v", err)
	}
	if _, ok := store.status("e1"); ok {
		t.Fatal("completed row not swept")
	}
}

func TestSQLStoreClaim(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	store := NewStore(db, WithNow(func() time.Time { return time.Unix(0, 0) }))

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, topic, event_key, payload, metadata, attempts FROM outbox_events`).
		WithArgs(sqlmock.AnyArg(), 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "topic", "event_key", "payload", "metadata", "attempts"}).
			AddRow("e1", "orderCreated", "k1", []byte(`{}`), nil, 0))
	mock.ExpectExec("UPDATE outbox_events SET status = 'claimed'").
		WithArgs(1, sqlmock.AnyArg(), "e1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	rows, err := store.Claim(context.Background(), 100)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "e1" || rows[0].Topic != "orderCreated" ||
		rows[0].Key != "k1" || rows[0].Attempts != 1 {
		t.Fatalf("claimed rows = %+v", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestSQLStoreCompleteFailSweep(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	store := NewStore(db, WithNow(func() time.Time { return time.Unix(0, 0) }))

	mock.ExpectExec("UPDATE outbox_events SET status = 'done'").
		WithArgs(sqlmock.AnyArg(), "e1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := store.Complete(context.Background(), "e1"); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	mock.ExpectExec("UPDATE outbox_events").
		WithArgs("pending", "boom", sqlmock.AnyArg(), "e1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := store.Fail(context.Background(), "e1", "boom", time.Unix(1, 0), false); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	mock.ExpectExec("UPDATE outbox_events").
		WithArgs("dead", sqlmock.AnyArg(), nil, "e2").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := store.Fail(context.Background(), "e2", "boom", time.Time{}, true); err != nil {
		t.Fatalf("Fail(dead): %v", err)
	}

	mock.ExpectExec("DELETE FROM outbox_events").
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 3))
	removed, err := store.Sweep(context.Background(), time.Unix(2, 0))
	if err != nil || removed != 3 {
		t.Fatalf("Sweep = (%d, %v), want (3, nil)", removed, err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

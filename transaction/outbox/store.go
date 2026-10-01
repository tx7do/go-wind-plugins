package outbox

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Store persists outbox rows. NewStore returns the SQL implementation; tests
// may supply in-memory implementations to exercise Relay without a database.
type Store interface {
	// EnsureSchema creates the outbox table when missing. It is idempotent.
	EnsureSchema(ctx context.Context) error

	// Claim atomically moves up to limit pending rows to the claimed state
	// and returns them. Rows claimed by a concurrent relay are not returned.
	Claim(ctx context.Context, limit int) ([]Row, error)

	// Reclaim returns rows to the pending state whose claim is older than
	// the given instant (crash recovery for relays that died mid-delivery).
	Reclaim(ctx context.Context, olderThan time.Time) error

	// Complete marks a delivered row as done.
	Complete(ctx context.Context, id string) error

	// Fail records a delivery failure. When dead is true the row moves to
	// the dead state, otherwise it becomes pending again and visible at
	// nextVisible.
	Fail(ctx context.Context, id, lastErr string, nextVisible time.Time, dead bool) error

	// Sweep deletes completed rows older than the given instant and returns
	// the number of rows removed. A zero count with nil error is normal.
	Sweep(ctx context.Context, olderThan time.Time) (int64, error)
}

// SQLStore is the SQL implementation of Store, compatible with MySQL 8+,
// MariaDB, PostgreSQL and SQLite (subject to WithSkipLocked).
type SQLStore struct {
	db  *sql.DB
	cfg config
}

// NewStore returns a SQL-backed Store for the outbox table.
func NewStore(db *sql.DB, opts ...Option) *SQLStore {
	return &SQLStore{db: db, cfg: newConfig(opts...)}
}

// EnsureSchema creates the outbox and inbox tables when missing. The helper
// index on (status, next_visible_at) is best-effort: databases without
// CREATE INDEX IF NOT EXISTS (MySQL) simply keep the table unindexed.
func (s *SQLStore) EnsureSchema(ctx context.Context) error {
	return EnsureSchema(ctx, s.db,
		WithTable(s.cfg.outboxTable),
		WithInboxTable(s.cfg.inboxTable),
	)
}

// EnsureSchema creates the outbox and inbox tables when missing. Call it once
// at startup; Enqueue and the consumer middlewares assume the tables exist.
func EnsureSchema(ctx context.Context, db *sql.DB, opts ...Option) error {
	cfg := newConfig(opts...)

	_, err := db.ExecContext(ctx,
		"CREATE TABLE IF NOT EXISTS "+cfg.outboxTable+" ("+
			"id VARCHAR(36) NOT NULL PRIMARY KEY,"+
			" topic VARCHAR(255) NOT NULL,"+
			" event_key VARCHAR(255) NULL,"+
			" payload TEXT NOT NULL,"+
			" metadata TEXT NULL,"+
			" status VARCHAR(16) NOT NULL DEFAULT 'pending',"+
			" attempts INT NOT NULL DEFAULT 0,"+
			" last_error TEXT NULL,"+
			" created_at TIMESTAMP NOT NULL,"+
			" claimed_at TIMESTAMP NULL,"+
			" completed_at TIMESTAMP NULL,"+
			" next_visible_at TIMESTAMP NULL)")
	if err != nil {
		return fmt.Errorf("outbox: create table %s: %w", cfg.outboxTable, err)
	}

	// Best-effort: duplicate/malformed index errors are ignored because the
	// syntax of CREATE INDEX IF NOT EXISTS is not portable.
	_, _ = db.ExecContext(ctx,
		"CREATE INDEX IF NOT EXISTS idx_"+cfg.outboxTable+"_dispatch"+
			" ON "+cfg.outboxTable+" (status, next_visible_at)")

	_, err = db.ExecContext(ctx,
		"CREATE TABLE IF NOT EXISTS "+cfg.inboxTable+" ("+
			" handler VARCHAR(255) NOT NULL,"+
			" event_id VARCHAR(255) NOT NULL,"+
			" processed_at TIMESTAMP NOT NULL,"+
			" PRIMARY KEY (handler, event_id))")
	if err != nil {
		return fmt.Errorf("outbox: create table %s: %w", cfg.inboxTable, err)
	}
	return nil
}

// Claim selects pending rows, then flips them to claimed inside one
// transaction. With skipLocked enabled the SELECT carries FOR UPDATE SKIP
// LOCKED so concurrent relays never claim the same row.
func (s *SQLStore) Claim(ctx context.Context, limit int) ([]Row, error) {
	cfg := s.cfg
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("outbox: begin claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	selectQ := "SELECT id, topic, event_key, payload, metadata, attempts FROM " + cfg.outboxTable +
		" WHERE status = 'pending' AND (next_visible_at IS NULL OR next_visible_at <= ?)" +
		" ORDER BY created_at LIMIT ?"
	if cfg.skipLocked {
		selectQ += " FOR UPDATE SKIP LOCKED"
	}
	now := cfg.now().UTC()
	rows, err := tx.QueryContext(ctx, selectQ, now, limit)
	if err != nil {
		return nil, fmt.Errorf("outbox: select pending: %w", err)
	}
	var claimed []Row
	for rows.Next() {
		var r Row
		var metadata sql.NullString
		var key sql.NullString
		if err := rows.Scan(&r.ID, &r.Topic, &key, &r.Payload, &metadata, &r.Attempts); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("outbox: scan pending: %w", err)
		}
		r.Key = key.String
		r.Metadata = []byte(metadata.String)
		r.Attempts++
		claimed = append(claimed, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("outbox: iterate pending: %w", err)
	}
	_ = rows.Close()

	if len(claimed) == 0 {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("outbox: commit claim: %w", err)
		}
		return nil, nil
	}

	updateQ := "UPDATE " + cfg.outboxTable +
		" SET status = 'claimed', attempts = ?, claimed_at = ? WHERE id = ?"
	for _, r := range claimed {
		if _, err := tx.ExecContext(ctx, updateQ, r.Attempts, now, r.ID); err != nil {
			return nil, fmt.Errorf("outbox: claim row %s: %w", r.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("outbox: commit claim: %w", err)
	}
	return claimed, nil
}

// Reclaim returns rows whose claim expired back to the pending state.
func (s *SQLStore) Reclaim(ctx context.Context, olderThan time.Time) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE "+s.cfg.outboxTable+
			" SET status = 'pending', next_visible_at = NULL, claimed_at = NULL"+
			" WHERE status = 'claimed' AND claimed_at < ?",
		olderThan.UTC(),
	)
	if err != nil {
		return fmt.Errorf("outbox: reclaim stale claims: %w", err)
	}
	return nil
}

// Complete marks a row as done.
func (s *SQLStore) Complete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE "+s.cfg.outboxTable+" SET status = 'done', completed_at = ? WHERE id = ?",
		s.cfg.now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("outbox: complete row %s: %w", id, err)
	}
	return nil
}

// Fail records a delivery failure: dead-letter after the configured maximum,
// otherwise schedule the next attempt.
func (s *SQLStore) Fail(ctx context.Context, id, lastErr string, nextVisible time.Time, dead bool) error {
	status := "pending"
	var next any = nextVisible.UTC()
	if dead {
		status = "dead"
		next = nil
	}
	_, err := s.db.ExecContext(ctx,
		"UPDATE "+s.cfg.outboxTable+
			" SET status = ?, last_error = ?, next_visible_at = ?, claimed_at = NULL WHERE id = ?",
		status, lastErr, next, id,
	)
	if err != nil {
		return fmt.Errorf("outbox: fail row %s: %w", id, err)
	}
	return nil
}

// Sweep deletes completed rows older than the given instant.
func (s *SQLStore) Sweep(ctx context.Context, olderThan time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		"DELETE FROM "+s.cfg.outboxTable+" WHERE status = 'done' AND completed_at < ?",
		olderThan.UTC(),
	)
	if err != nil {
		return 0, fmt.Errorf("outbox: sweep completed: %w", err)
	}
	return res.RowsAffected()
}

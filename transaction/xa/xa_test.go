package xa_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/tx7do/go-wind-plugins/transaction/xa"
)

func newMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, mock
}

func TestRunCommitsAll(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec(`XA START 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`XA START 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectExec("UPDATE orders").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE stocks").WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectExec(`XA END 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`XA END 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`XA PREPARE 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`XA PREPARE 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`XA COMMIT 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`XA COMMIT 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))

	err := xa.Run(context.Background(),
		func(ctx context.Context, s *xa.Session) error {
			if _, err := s.Exec(ctx, "orders", "UPDATE orders SET paid = 1"); err != nil {
				return err
			}
			_, err := s.Exec(ctx, "stocks", "UPDATE stocks SET locked = locked + 1")
			return err
		},
		xa.Resource{Name: "orders", DB: db},
		xa.Resource{Name: "stocks", DB: db},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestRunRollsBackOnBusinessError(t *testing.T) {
	db, mock := newMockDB(t)
	boom := errors.New("stock exhausted")

	mock.ExpectExec(`XA START 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`XA START 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectExec("UPDATE orders").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE stocks").WillReturnError(boom)

	mock.ExpectExec(`XA END 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`XA ROLLBACK 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`XA END 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`XA ROLLBACK 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))

	err := xa.Run(context.Background(),
		func(ctx context.Context, s *xa.Session) error {
			if _, err := s.Exec(ctx, "orders", "UPDATE orders SET paid = 1"); err != nil {
				return err
			}
			_, err := s.Exec(ctx, "stocks", "UPDATE stocks SET locked = locked + 1")
			return err
		},
		xa.Resource{Name: "orders", DB: db},
		xa.Resource{Name: "stocks", DB: db},
	)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want wrapped boom", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestRunRollsBackWhenPrepareRefused(t *testing.T) {
	db, mock := newMockDB(t)
	prepareErr := errors.New("resource manager refuses")

	mock.ExpectExec(`XA START 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`XA START 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`XA END 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`XA END 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`XA PREPARE 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`XA PREPARE 'windxa-`).WillReturnError(prepareErr)
	// Both branches — the prepared one included — are rolled back.
	mock.ExpectExec(`XA ROLLBACK 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`XA ROLLBACK 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))

	err := xa.Run(context.Background(),
		func(context.Context, *xa.Session) error { return nil },
		xa.Resource{Name: "orders", DB: db},
		xa.Resource{Name: "stocks", DB: db},
	)
	if !errors.Is(err, prepareErr) {
		t.Fatalf("err = %v, want wrapped prepare error", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestRunReportsInDoubtCommit(t *testing.T) {
	db, mock := newMockDB(t)
	commitErr := errors.New("coordinator lost")

	mock.ExpectExec(`XA START 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`XA START 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`XA END 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`XA END 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`XA PREPARE 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`XA PREPARE 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`XA COMMIT 'windxa-`).WillReturnError(commitErr)
	mock.ExpectExec(`XA COMMIT 'windxa-`).WillReturnResult(sqlmock.NewResult(0, 0))

	err := xa.Run(context.Background(),
		func(context.Context, *xa.Session) error { return nil },
		xa.Resource{Name: "orders", DB: db},
		xa.Resource{Name: "stocks", DB: db},
	)
	if !errors.Is(err, commitErr) {
		t.Fatalf("err = %v, want wrapped commit error", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestRunValidatesResources(t *testing.T) {
	db, _ := newMockDB(t)

	called := false
	err := xa.Run(context.Background(),
		func(context.Context, *xa.Session) error { called = true; return nil },
		xa.Resource{Name: "a", DB: db},
		xa.Resource{Name: "a", DB: db},
	)
	if err == nil {
		t.Fatalf("duplicate resources must be rejected, got nil")
	}
	if called {
		t.Fatal("business function ran on invalid resources")
	}

	err = xa.Run(context.Background(), func(context.Context, *xa.Session) error { return nil })
	if err == nil {
		t.Fatal("zero resources must be rejected")
	}
}

func TestPendingAndResolve(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("XA RECOVER").
		WillReturnRows(sqlmock.NewRows([]string{"formatID", "gtrid_length", "bqual_length", "data"}).
			AddRow(1, 43, 0, []byte("windxa-0f0e0d0c-0a0b-4c0d-8e0f-010203040506")).
			AddRow(1, 10, 0, []byte("foreigntxn")))
	xids, err := xa.Pending(context.Background(), db)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if len(xids) != 1 {
		t.Fatalf("xids = %v, want only the windxa entry", xids)
	}

	mock.ExpectExec(`XA COMMIT 'windxa-0f0e0d0c-0a0b-4c0d-8e0f-010203040506'`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := xa.CommitPending(context.Background(), db, xids[0]); err != nil {
		t.Fatalf("CommitPending: %v", err)
	}

	if err := xa.CommitPending(context.Background(), db, "'; DROP TABLE x"); err == nil {
		t.Fatal("foreign XID must be refused")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestSessionUnknownResource(t *testing.T) {
	db, _ := newMockDB(t)
	err := xa.Run(context.Background(),
		func(_ context.Context, s *xa.Session) error {
			_, err := s.Exec(context.Background(), "nope", "SELECT 1")
			return err
		},
		xa.Resource{Name: "orders", DB: db},
	)
	if err == nil {
		t.Fatal("unknown resource must fail")
	}
}

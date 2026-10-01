package doris

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

// TestOptions_ApplyToClient verifies every Option mutates the expected field.
// Applying options only sets struct fields and never touches a server.
func TestOptions_ApplyToClient(t *testing.T) {
	c := &Client{}
	hc := &http.Client{Timeout: 5 * time.Second}

	WithDSN("dsn")(c)
	if c.dsn != "dsn" {
		t.Error("WithDSN did not set dsn")
	}

	WithMaxOpenConns(3)(c)
	if c.maxOpenConns != 3 {
		t.Error("WithMaxOpenConns did not set maxOpenConns")
	}

	WithMaxIdleConns(2)(c)
	if c.maxIdleConns != 2 {
		t.Error("WithMaxIdleConns did not set maxIdleConns")
	}

	WithConnMaxLifetime(time.Minute)(c)
	if c.connMaxLifetime != time.Minute {
		t.Error("WithConnMaxLifetime did not set connMaxLifetime")
	}

	WithStreamLoadEndpoint("http://fe:8030/")(c)
	if c.streamLoadEndpoint != "http://fe:8030/" {
		t.Error("WithStreamLoadEndpoint did not set endpoint")
	}

	WithStreamLoadAuth("user", "pass")(c)
	if c.streamLoadUser != "user" || c.streamLoadPass != "pass" {
		t.Error("WithStreamLoadAuth did not set credentials")
	}

	WithHTTPClient(hc)(c)
	if c.httpClient != hc {
		t.Error("WithHTTPClient did not inject client")
	}

	WithStreamLoadTimeout(10 * time.Second)(c)
	if c.streamLoadTimeout != 10*time.Second {
		t.Error("WithStreamLoadTimeout did not set timeout")
	}

	WithStreamLoadMethod("POST")(c)
	if c.streamLoadMethod != "POST" {
		t.Error("WithStreamLoadMethod did not set method")
	}
}

// TestNewClient_RequiresDSN verifies the fail-fast path when neither a DB nor
// a DSN is provided.
func TestNewClient_RequiresDSN(t *testing.T) {
	if _, err := NewClient(); err == nil {
		t.Fatal("expected error when no db and no dsn")
	}
}

// ---------------------------------------------------------------------------
// Client accessors and passthrough helpers (sqlmock-backed, no server)
// ---------------------------------------------------------------------------

func TestClient_AccessorsAndClose(t *testing.T) {
	c, mock, cleanup := newMockClient(t)
	defer cleanup()

	if c.DB() == nil {
		t.Fatal("DB() must return the injected sqlx handle")
	}

	// closing is safe and returns no error
	mock.ExpectClose()
	if err := c.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	// Close on a client without a DB is a no-op
	if err := (&Client{}).Close(); err != nil {
		t.Fatalf("Close on nil db must be a no-op, got %v", err)
	}
}

func TestClient_ExecGetSelect(t *testing.T) {
	c, mock, cleanup := newMockClient(t)
	defer cleanup()
	ctx := context.Background()

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO t (id) VALUES (?)")).
		WithArgs(1).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if _, err := c.Exec("INSERT INTO t (id) VALUES (?)", 1); err != nil {
		t.Fatalf("Exec error: %v", err)
	}
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO t (id) VALUES (?)")).
		WithArgs(1).
		WillReturnResult(sqlmock.NewResult(2, 1))
	if _, err := c.ExecContext(ctx, "INSERT INTO t (id) VALUES (?)", 1); err != nil {
		t.Fatalf("ExecContext error: %v", err)
	}

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM t")).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM t")).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(8))

	var one struct {
		ID int `db:"id"`
	}
	if err := c.Get(&one, "SELECT id FROM t"); err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if one.ID != 7 {
		t.Fatalf("Get scanned %d, want 7", one.ID)
	}

	var two struct {
		ID int `db:"id"`
	}
	if err := c.GetContext(ctx, &two, "SELECT id FROM t"); err != nil {
		t.Fatalf("GetContext error: %v", err)
	}
	if two.ID != 8 {
		t.Fatalf("GetContext scanned %d, want 8", two.ID)
	}

	// Select / SelectContext into slices
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM t")).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1).AddRow(2))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM t")).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))

	var list []struct {
		ID int `db:"id"`
	}
	if err := c.Select(&list, "SELECT id FROM t"); err != nil {
		t.Fatalf("Select error: %v", err)
	}
	if len(list) != 2 || list[0].ID != 1 || list[1].ID != 2 {
		t.Fatalf("Select scanned %+v", list)
	}

	var list2 []struct {
		ID int `db:"id"`
	}
	if err := c.SelectContext(ctx, &list2, "SELECT id FROM t"); err != nil {
		t.Fatalf("SelectContext error: %v", err)
	}
	if len(list2) != 1 || list2[0].ID != 3 {
		t.Fatalf("SelectContext scanned %+v", list2)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestClient_Query_NilDB covers the guard branch of Query.
func TestClient_Query_NilDB(t *testing.T) {
	c := &Client{}
	results := []any{}
	if err := c.Query(context.Background(), func() any { return &struct{}{} }, &results, "SELECT 1"); err == nil {
		t.Fatal("Query without a DB must fail")
	}
}

func TestClient_BatchInsertGuards(t *testing.T) {
	c, mock, cleanup := newMockClient(t)
	defer cleanup()
	ctx := context.Background()

	// no rows
	if _, err := c.BatchInsert(ctx, "t", []string{"id"}, nil); err == nil {
		t.Fatal("BatchInsert without rows must fail")
	}

	// column count mismatch
	if _, err := c.BatchInsert(ctx, "t", []string{"id"}, [][]any{{1, "extra"}}); err == nil {
		t.Fatal("BatchInsert with mismatched row must fail")
	}

	// empty table / columns surface BuildInsertSQL errors
	if _, err := c.BatchInsert(ctx, "", []string{"id"}, [][]any{{1}}); err == nil {
		t.Fatal("BatchInsert with empty table must fail")
	}
	if _, err := c.BatchInsert(ctx, "t", nil, [][]any{{1}}); err == nil {
		t.Fatal("BatchInsert with empty columns must fail")
	}

	// BatchInsertProto without data fails during extraction
	if _, err := c.BatchInsertProto(ctx, "t", nil); err == nil {
		t.Fatal("BatchInsertProto without data must fail")
	}

	// success path via struct extraction
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "demo" ("id","name") VALUES (?,?),(?,?)`)).
		WithArgs(1, "a", 2, "b").
		WillReturnResult(sqlmock.NewResult(0, 2))
	if _, err := c.BatchInsertStruct(ctx, "demo", []any{
		&struct {
			ID   int    `db:"id"`
			Name string `db:"name"`
		}{ID: 1, Name: "a"},
		&struct {
			ID   int    `db:"id"`
			Name string `db:"name"`
		}{ID: 2, Name: "b"},
	}); err != nil {
		t.Fatalf("BatchInsertStruct error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestClient_SetSessionStatements(t *testing.T) {
	c, mock, cleanup := newMockClient(t)
	defer cleanup()
	ctx := context.Background()

	mock.ExpectExec(regexp.QuoteMeta("SET SESSION query_timeout = ?")).
		WithArgs(5).
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := c.SetSession(ctx, "SET SESSION query_timeout = ?", 5); err != nil {
		t.Fatalf("SetSession error: %v", err)
	}

	mock.ExpectExec(regexp.QuoteMeta("SET SESSION sql_mode = ?")).
		WithArgs("STRICT_TRANS_TABLES").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := c.SetSQLMode(ctx, "STRICT_TRANS_TABLES"); err != nil {
		t.Fatalf("SetSQLMode error: %v", err)
	}

	// SetSessionVars with mixed value shapes (numeric+unit, quoted, plain);
	// statements are issued in sorted key order.
	mock.ExpectExec(regexp.QuoteMeta("SET SESSION enable_profile = true")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("SET SESSION sql_mode = 'STRICT'")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("SET SESSION wait_timeout = 10")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := c.SetSessionVars(ctx, map[string]string{
		"sql_mode":       "STRICT",
		"wait_timeout":   "10",
		"enable_profile": "true",
	}); err != nil {
		t.Fatalf("SetSessionVars error: %v", err)
	}

	// invalid variable names fail closed without touching the server
	if err := c.SetSessionVars(ctx, map[string]string{"bad name": "1"}); err == nil {
		t.Fatal("invalid session variable name must fail")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestClient_WithSessionConn(t *testing.T) {
	ctx := context.Background()

	// nil db fails fast before acquiring a connection
	if err := (&Client{}).WithSessionConn(ctx, nil, func(context.Context, *sql.Conn) error { return nil }); err == nil {
		t.Fatal("WithSessionConn without a DB must fail")
	}

	c, mock, cleanup := newMockClient(t)
	defer cleanup()

	// blank statements are skipped, the rest run on the dedicated connection
	mock.ExpectExec(regexp.QuoteMeta("SET SESSION a = 1")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("SET SESSION b = 2")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("SELECT 1")).
		WillReturnResult(sqlmock.NewResult(0, 0))

	ran := false
	err := c.WithSessionConn(ctx, []string{"SET SESSION a = 1", "   ", "SET SESSION b = 2"}, func(ctx context.Context, conn *sql.Conn) error {
		ran = true
		_, err := conn.ExecContext(ctx, "SELECT 1")
		return err
	})
	if err != nil {
		t.Fatalf("WithSessionConn error: %v", err)
	}
	if !ran {
		t.Fatal("callback must run on the dedicated connection")
	}

	// callback errors propagate
	mock.ExpectExec(regexp.QuoteMeta("SET SESSION a = 1")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	wantErr := errFake
	if err := c.WithSessionConn(ctx, []string{"SET SESSION a = 1"}, func(context.Context, *sql.Conn) error {
		return wantErr
	}); err == nil {
		t.Fatal("callback error must propagate")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// errFake is a sentinel error for propagation assertions.
var errFake = errors.New("fake error")

func TestClient_WithTxPaths(t *testing.T) {
	ctx := context.Background()

	// BeginTx without a DB fails fast
	if _, err := (&Client{}).BeginTx(ctx, nil); err == nil {
		t.Fatal("BeginTx without a DB must fail")
	}

	c, mock, cleanup := newMockClient(t)
	defer cleanup()

	// success commits
	mock.ExpectBegin()
	mock.ExpectCommit()
	if err := c.WithTx(ctx, nil, func(tx *sqlx.Tx) error { return nil }); err != nil {
		t.Fatalf("WithTx(commit) error: %v", err)
	}

	// function error rolls back
	mock.ExpectBegin()
	mock.ExpectRollback()
	if err := c.WithTx(ctx, nil, func(tx *sqlx.Tx) error { return errFake }); err == nil {
		t.Fatal("WithTx(fn error) must propagate the error")
	}

	// RunInTx is an alias of WithTx
	mock.ExpectBegin()
	mock.ExpectCommit()
	if err := c.RunInTx(ctx, nil, func(tx *sqlx.Tx) error { return nil }); err != nil {
		t.Fatalf("RunInTx error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestClient_BeginTxWithSessionPaths(t *testing.T) {
	ctx := context.Background()

	// nil db fails fast
	if _, err := (&Client{}).BeginTxWithSession(ctx, nil, nil); err == nil {
		t.Fatal("BeginTxWithSession without a DB must fail")
	}

	c, mock, cleanup := newMockClient(t)
	defer cleanup()

	// invalid variable name closes the connection and fails
	if _, err := c.BeginTxWithSession(ctx, map[string]string{"bad name": "1"}, nil); err == nil {
		t.Fatal("invalid session variable name must fail")
	}

	// success path: session var applied, tx committed
	mock.ExpectExec(regexp.QuoteMeta("SET SESSION wait_timeout = 10")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectBegin()
	mock.ExpectCommit()
	tx, err := c.BeginTxWithSession(ctx, map[string]string{"wait_timeout": "10"}, nil)
	if err != nil {
		t.Fatalf("BeginTxWithSession error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit error: %v", err)
	}

	// WithTxWithSession rollback path
	mock.ExpectExec(regexp.QuoteMeta("SET SESSION wait_timeout = 10")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectBegin()
	mock.ExpectRollback()
	if err := c.WithTxWithSession(ctx, map[string]string{"wait_timeout": "10"}, nil, func(tx *sql.Tx) error {
		return errFake
	}); err == nil {
		t.Fatal("WithTxWithSession(fn error) must propagate the error")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestTxWithConn_NilTx(t *testing.T) {
	tx := &TxWithConn{}
	if err := tx.Commit(); err == nil || !strings.Contains(err.Error(), "nil tx") {
		t.Fatalf("Commit on nil tx must fail with nil tx, got %v", err)
	}
	if err := tx.Rollback(); err == nil || !strings.Contains(err.Error(), "nil tx") {
		t.Fatalf("Rollback on nil tx must fail with nil tx, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// StreamLoad against a loopback httptest server (fully local, no external net)
// ---------------------------------------------------------------------------

func TestStreamLoad_Validation(t *testing.T) {
	c := &Client{}
	// missing endpoint
	if _, _, err := c.StreamLoad(context.Background(), "db", "t", nil, nil); err == nil {
		t.Fatal("missing endpoint must fail")
	}

	// missing db/table
	c2 := &Client{streamLoadEndpoint: "http://127.0.0.1:1", httpClient: &http.Client{Timeout: time.Second}}
	if _, _, err := c2.StreamLoad(context.Background(), "", "t", nil, nil); err == nil {
		t.Fatal("missing db must fail")
	}
	if _, _, err := c2.StreamLoad(context.Background(), "db", "", nil, nil); err == nil {
		t.Fatal("missing table must fail")
	}
}

func TestStreamLoad_HTTP(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotQuery  string
		gotUser   string
		gotPass   string
		gotBody   string
	)
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.EscapedPath()
		gotQuery = r.URL.RawQuery
		gotUser, gotPass, _ = r.BasicAuth()
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"Status":"Success"}`))
	}))
	defer srv.Close()

	c := &Client{}
	WithStreamLoadEndpoint(srv.URL)(c)
	WithHTTPClient(srv.Client())(c)
	WithStreamLoadMethod("POST")(c)
	WithStreamLoadAuth("u", "p")(c)

	body, code, err := c.StreamLoad(context.Background(), "my db", "tbl", map[string]string{"format": "json"}, strings.NewReader("payload"))
	if err != nil {
		t.Fatalf("StreamLoad error: %v", err)
	}
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if string(body) != `{"Status":"Success"}` {
		t.Fatalf("unexpected body %q", body)
	}
	if gotMethod != "POST" {
		t.Fatalf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/api/my%20db/tbl/_stream_load" {
		t.Fatalf("path = %s", gotPath)
	}
	if gotQuery != "format=json" {
		t.Fatalf("query = %s", gotQuery)
	}
	if gotUser != "u" || gotPass != "p" {
		t.Fatalf("basic auth = %q/%q", gotUser, gotPass)
	}
	if gotBody != "payload" {
		t.Fatalf("body = %q", gotBody)
	}

	// non-2xx status yields an error while still returning the body
	status = http.StatusInternalServerError
	_, code, err = c.StreamLoad(context.Background(), "db", "tbl", nil, strings.NewReader("x"))
	if err == nil || code != http.StatusInternalServerError {
		t.Fatalf("expected error with status 500, got %d, %v", code, err)
	}
}

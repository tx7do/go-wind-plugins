package influxdb

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/InfluxCommunity/influxdb3-go/v2/influxdb3"
	paginationV1 "github.com/tx7do/go-wind-plugins/crud/api/gen/go/pagination/v1"
	"github.com/tx7do/go-wind/log"
)

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

// TestOptions_All applies every Option and asserts the ClientConfig mutation.
// Applying options only prepares the client config; nothing dials a server.
func TestOptions_All(t *testing.T) {
	c := &Client{options: &influxdb3.ClientConfig{}}

	WithOptions(nil)(c) // nil config is ignored
	if c.options.Host != "" {
		t.Error("WithOptions(nil) must be ignored")
	}

	cfg := &influxdb3.ClientConfig{Host: "http://other:8181"}
	WithOptions(cfg)(c)
	if c.options.Host != "http://other:8181" {
		t.Error("WithOptions did not replace the config")
	}

	WithHost("http://localhost:8181")(c)
	if c.options.Host != "http://localhost:8181" {
		t.Error("WithHost did not set host")
	}

	WithToken("token")(c)
	if c.options.Token != "token" {
		t.Error("WithToken did not set token")
	}

	WithOrganization("org")(c)
	if c.options.Organization != "org" {
		t.Error("WithOrganization did not set organization")
	}

	WithDatabase("db")(c)
	if c.options.Database != "db" {
		t.Error("WithDatabase did not set database")
	}

	WithTLSConfig(nil)(c) // nil tls config is ignored
	if c.options.HTTPClient != nil {
		t.Error("WithTLSConfig(nil) must be ignored")
	}
	tlsCfg := &tls.Config{InsecureSkipVerify: true} //nolint:gosec // test fixture
	WithTLSConfig(tlsCfg)(c)
	if c.options.HTTPClient == nil {
		t.Fatal("WithTLSConfig did not install an HTTP client")
	}
	if transport, ok := c.options.HTTPClient.Transport.(*http.Transport); !ok || transport.TLSClientConfig != tlsCfg {
		t.Error("WithTLSConfig did not install the caller TLS config on the transport")
	}

	WithWriteTimeout(1 * time.Second)(c)
	if c.options.WriteTimeout != time.Second {
		t.Error("WithWriteTimeout did not set timeout")
	}

	WithQueryTimeout(2 * time.Second)(c)
	if c.options.QueryTimeout != 2*time.Second {
		t.Error("WithQueryTimeout did not set timeout")
	}

	WithIdleConnectionTimeout(3 * time.Second)(c)
	if c.options.IdleConnectionTimeout != 3*time.Second {
		t.Error("WithIdleConnectionTimeout did not set timeout")
	}

	WithMaxIdleConnections(7)(c)
	if c.options.MaxIdleConnections != 7 {
		t.Error("WithMaxIdleConnections did not set value")
	}

	WithAuthScheme("Bearer")(c)
	if c.options.AuthScheme != "Bearer" {
		t.Error("WithAuthScheme did not set scheme")
	}

	// WithLogger only sets the package-level logger
	WithLogger(log.GetLogger())(c)
}

// ---------------------------------------------------------------------------
// Nil-client guard branches (no server, no dial)
// ---------------------------------------------------------------------------

// TestClient_NilClientGuards walks every method guard for a client that was
// never connected.
func TestClient_NilClientGuards(t *testing.T) {
	c := &Client{}
	ctx := context.Background()

	if _, err := c.Query(ctx, "SELECT 1"); !errors.Is(err, ErrInfluxDBClientNotInitialized) {
		t.Errorf("Query guard = %v", err)
	}
	if _, err := c.QueryWithParams(ctx, "m", nil, nil, nil); !errors.Is(err, ErrInfluxDBClientNotInitialized) {
		t.Errorf("QueryWithParams guard = %v", err)
	}
	if err := c.Insert(ctx, nil); !errors.Is(err, ErrInfluxDBClientNotInitialized) {
		t.Errorf("Insert guard = %v", err)
	}
	if err := c.BatchInsert(ctx, nil); !errors.Is(err, ErrInfluxDBClientNotInitialized) {
		t.Errorf("BatchInsert guard = %v", err)
	}
	if _, err := c.Count(ctx, "SELECT COUNT(*) FROM m"); !errors.Is(err, ErrInfluxDBClientNotInitialized) {
		t.Errorf("Count guard = %v", err)
	}
	if _, err := c.Exist(ctx, "SELECT 1"); !errors.Is(err, ErrInfluxDBClientNotInitialized) {
		t.Errorf("Exist guard = %v", err)
	}
	if _, err := c.ExecInfluxQLQuery(ctx, "SELECT 1"); !errors.Is(err, ErrInfluxDBClientNotInitialized) {
		t.Errorf("ExecInfluxQLQuery guard = %v", err)
	}
	if _, err := c.ExecSQLQuery(ctx, "SELECT 1"); !errors.Is(err, ErrInfluxDBClientNotInitialized) {
		t.Errorf("ExecSQLQuery guard = %v", err)
	}
	if err := c.WritePointsStrict(ctx, nil); !errors.Is(err, ErrInfluxDBClientNotInitialized) {
		t.Errorf("WritePointsStrict guard = %v", err)
	}
	if err := c.WritePoints(ctx, nil); !errors.Is(err, ErrInfluxDBClientNotInitialized) {
		t.Errorf("WritePoints guard = %v", err)
	}
	if v := c.ServerVersion(); v != "" {
		t.Errorf("ServerVersion on nil client = %q, want empty", v)
	}
	c.Close() // must not panic
}

// TestClient_LazyClientGuards uses a lazily-created client (no connection is
// established by NewClient) to cover argument validation before any I/O.
func TestClient_LazyClientGuards(t *testing.T) {
	c, err := NewClient(
		WithHost("http://127.0.0.1:1"),
		WithToken("token"),
		WithOrganization("org"),
		WithDatabase("db"),
	)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	ctx := context.Background()

	// nil point is rejected before any write
	if err := c.Insert(ctx, nil); !errors.Is(err, ErrInvalidPoint) {
		t.Errorf("Insert(nil point) = %v", err)
	}
	// empty batch is rejected before any write
	if err := c.BatchInsert(ctx, nil); !errors.Is(err, ErrNoPointsToInsert) {
		t.Errorf("BatchInsert(empty) = %v", err)
	}
	// empty strict batch is a no-op success
	if err := c.WritePointsStrict(ctx, nil); err != nil {
		t.Errorf("WritePointsStrict(empty) = %v", err)
	}
	// unsupported element types are rejected by the safe converter
	if err := c.WritePoints(ctx, []any{42}); err == nil {
		t.Error("WritePoints with unsupported element must fail")
	}
	pt := influxdb3.NewPoint("m", map[string]string{"t": "v"}, map[string]any{"f": 1}, time.Now())
	if err := c.WritePoints(ctx, []any{pt, 42}); err == nil {
		t.Error("WritePoints with a later unsupported element must fail")
	}
}

// ---------------------------------------------------------------------------
// Pure helpers
// ---------------------------------------------------------------------------

func TestConvertAnyToPointsSafe(t *testing.T) {
	if pts, err := ConvertAnyToPointsSafe(nil); err != nil || len(pts) != 0 {
		t.Fatalf("nil input = %v, %v", pts, err)
	}
	pt := influxdb3.NewPoint("m", nil, map[string]any{"f": 1}, time.Now())
	pts, err := ConvertAnyToPointsSafe([]any{pt})
	if err != nil || len(pts) != 1 {
		t.Fatalf("valid input = %v, %v", pts, err)
	}
	if _, err := ConvertAnyToPointsSafe([]any{"nope"}); err == nil {
		t.Fatal("unsupported element must fail")
	}
	if _, err := ConvertAnyToPointsSafe([]any{pt, "nope"}); err == nil {
		t.Fatal("later unsupported element must fail")
	}
}

func TestNumericToInt64(t *testing.T) {
	cases := []struct {
		in   any
		want int64
		ok   bool
	}{
		{int64(1), 1, true},
		{int(2), 2, true},
		{uint64(3), 3, true},
		{float64(4.9), 4, true},
		{float32(5), 5, true},
		{uint32(6), 6, true},
		{int32(7), 7, true},
		{uint(8), 8, true},
		{"9", 0, false},
		{nil, 0, false},
	}
	for _, tc := range cases {
		got, ok := numericToInt64(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("numericToInt64(%v) = %d, %v; want %d, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestPointTagHelpers(t *testing.T) {
	now := time.Now()
	pt := influxdb3.NewPoint(
		"m",
		map[string]string{"name": "x", "flag": "true", "u32": "42", "u64": "43", "env": "prod"},
		map[string]any{"ts": now, "count": uint64(7)},
		now,
	)

	if got := GetPointTag(nil, "name"); got != nil {
		t.Error("GetPointTag(nil) must be nil")
	}
	if got := GetPointTag(pt, "name"); got == nil || *got != "x" {
		t.Errorf("GetPointTag = %v", got)
	}
	if got := GetPointTag(pt, "missing"); got != nil {
		t.Errorf("GetPointTag(missing) = %v", got)
	}

	if got := GetBoolPointTag(nil, "flag"); got != nil {
		t.Error("GetBoolPointTag(nil) must be nil")
	}
	if got := GetBoolPointTag(pt, "flag"); got == nil || !*got {
		t.Errorf("GetBoolPointTag = %v", got)
	}
	if got := GetBoolPointTag(pt, "name"); got == nil || *got {
		t.Errorf("GetBoolPointTag(non-bool) = %v", got)
	}

	if got := GetUint32PointTag(nil, "u32"); got != nil {
		t.Error("GetUint32PointTag(nil) must be nil")
	}
	if got := GetUint32PointTag(pt, "u32"); got == nil || *got != 42 {
		t.Errorf("GetUint32PointTag = %v", got)
	}
	if got := GetUint32PointTag(pt, "name"); got != nil {
		t.Errorf("GetUint32PointTag(non-numeric) = %v", got)
	}

	if got := GetUint64PointTag(nil, "u64"); got != nil {
		t.Error("GetUint64PointTag(nil) must be nil")
	}
	if got := GetUint64PointTag(pt, "u64"); got == nil || *got != 43 {
		t.Errorf("GetUint64PointTag = %v", got)
	}
	if got := GetUint64PointTag(pt, "name"); got != nil {
		t.Errorf("GetUint64PointTag(non-numeric) = %v", got)
	}

	enums := map[string]int32{"prod": 1, "dev": 2}
	if got := GetEnumPointTag[int32](nil, "env", enums); got != nil {
		t.Error("GetEnumPointTag(nil) must be nil")
	}
	if got := GetEnumPointTag[int32](pt, "env", enums); got == nil || *got != 1 {
		t.Errorf("GetEnumPointTag = %v", got)
	}
	if got := GetEnumPointTag[int32](pt, "name", enums); got != nil {
		t.Errorf("GetEnumPointTag(unknown value) = %v", got)
	}

	if got := GetTimestampField(nil, "ts"); got != nil {
		t.Error("GetTimestampField(nil) must be nil")
	}
	if got := GetTimestampField(pt, "ts"); got == nil || got.AsTime().Unix() != now.Unix() {
		t.Errorf("GetTimestampField = %v", got)
	}
	if got := GetTimestampField(pt, "count"); got != nil {
		t.Errorf("GetTimestampField(non-timestamp) = %v", got)
	}

	if got := GetUint32Field(nil, "count"); got != nil {
		t.Error("GetUint32Field(nil) must be nil")
	}
	if got := GetUint32Field(pt, "count"); got == nil || *got != 7 {
		t.Errorf("GetUint32Field = %v", got)
	}
}

func TestScalarToStringHelpers(t *testing.T) {
	tr := true
	fa := false
	if BoolToString(nil) != "false" || BoolToString(&tr) != "true" || BoolToString(&fa) != "false" {
		t.Error("BoolToString misbehaves")
	}
	var u uint64 = 12
	if Uint64ToString(nil) != "0" || Uint64ToString(&u) != "12" {
		t.Error("Uint64ToString misbehaves")
	}
}

// TestStructToPoint_FieldValidation covers the per-field error branches of the
// generic struct converter (the happy path is covered by utils_test.go).
func TestStructToPoint_FieldValidation(t *testing.T) {
	if _, err := StructToPoint(42); err == nil {
		t.Fatal("non-struct input must fail")
	}

	type badTag struct {
		DeviceID int `influx:"tag"`
	}
	if _, err := StructToPoint(badTag{}); err == nil {
		t.Fatal("non-string tag field must fail")
	}

	type badField struct {
		Measurement string    `influx:"measurement"`
		Extra       []string  `influx:"field"`
		When        time.Time `influx:"time"`
	}
	if _, err := StructToPoint(badField{When: time.Now()}); err == nil {
		t.Fatal("unsupported field kind must fail")
	}

	type badTime struct {
		Measurement string `influx:"measurement"`
		When        int64  `influx:"time"`
	}
	if _, err := StructToPoint(badTime{}); err == nil {
		t.Fatal("non-time 'time' field must fail")
	}

	// a pointer to struct is dereferenced; a zero timestamp defaults to now
	type hasMeasurement struct {
		Measurement string `influx:"measurement"`
	}
	pt, err := StructToPoint(&hasMeasurement{Measurement: "m"})
	if err != nil {
		t.Fatalf("pointer input with measurement failed: %v", err)
	}
	if pt == nil || pt.GetMeasurement() != "m" {
		t.Fatalf("unexpected point: %v", pt)
	}
}

// ---------------------------------------------------------------------------
// Mapper and repository guard branches (no server I/O)
// ---------------------------------------------------------------------------

// TestMapperGuards covers the argument validation of the package-level mapper
// helpers using a lazily-created client (no request is issued).
func TestMapperGuards(t *testing.T) {
	ctx := context.Background()
	lazy, err := NewClient(WithHost("http://127.0.0.1:1"), WithToken("token"))
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	nilCli := &Client{}
	m := &candleMapper

	// nil client guards
	if err := Insert[Candle](ctx, nilCli, &Candle{}, m); !errors.Is(err, ErrClientNotConnected) {
		t.Errorf("Insert(nil client) = %v", err)
	}
	if err := BatchInsert[Candle](ctx, nilCli, []*Candle{{}}, m); !errors.Is(err, ErrClientNotConnected) {
		t.Errorf("BatchInsert(nil client) = %v", err)
	}
	if _, err := Query[Candle](ctx, nilCli, "SELECT 1", m); !errors.Is(err, ErrClientNotConnected) {
		t.Errorf("Query(nil client) = %v", err)
	}

	// nil data guards (client is connected lazily, nothing is sent)
	if err := Insert[Candle](ctx, lazy, nil, m); !errors.Is(err, ErrEmptyData) {
		t.Errorf("Insert(nil data) = %v", err)
	}
	if err := BatchInsert[Candle](ctx, lazy, nil, m); !errors.Is(err, ErrEmptyData) {
		t.Errorf("BatchInsert(nil data) = %v", err)
	}

	// a mapper that yields a nil point is rejected before any I/O
	if err := Insert[Candle](ctx, lazy, &Candle{}, nilPointMapper{}); !errors.Is(err, ErrInvalidPoint) {
		t.Errorf("Insert(nil point) = %v", err)
	}
	if err := BatchInsert[Candle](ctx, lazy, []*Candle{{}}, nilPointMapper{}); !errors.Is(err, ErrInvalidPoint) {
		t.Errorf("BatchInsert(nil point) = %v", err)
	}
}

// nilPointMapper is a Mapper that never produces a point.
type nilPointMapper struct{}

func (nilPointMapper) ToPoint(*Candle) *influxdb3.Point { return nil }
func (nilPointMapper) ToData(*influxdb3.Point) *Candle  { return nil }

// TestRepositoryGuards covers the repository validation branches that return
// before touching the client.
func TestRepositoryGuards(t *testing.T) {
	ctx := context.Background()

	// nil client
	repoNil := NewRepository[Candle, Candle](nil, "candles")
	if _, _, err := repoNil.ListWithPaging(ctx, &paginationV1.PagingRequest{}); err == nil {
		t.Error("ListWithPaging(nil client) must fail")
	}
	if _, _, err := repoNil.ListWithPagination(ctx, &paginationV1.PaginationRequest{}); err == nil {
		t.Error("ListWithPagination(nil client) must fail")
	}
	if _, err := repoNil.Create(ctx, nil); err == nil {
		t.Error("Create(nil client) must fail")
	}
	if _, err := repoNil.BatchCreate(ctx, nil); err == nil {
		t.Error("BatchCreate(nil client) must fail")
	}
	if _, err := repoNil.Count(ctx, ""); err == nil {
		t.Error("Count(nil client) must fail")
	}
	if _, err := repoNil.Exists(ctx, ""); err == nil {
		t.Error("Exists(nil client) must fail")
	}

	// empty collection
	repoEmpty := NewRepository[Candle, Candle](&Client{}, "")
	if _, _, err := repoEmpty.ListWithPaging(ctx, &paginationV1.PagingRequest{}); err == nil {
		t.Error("ListWithPaging(empty collection) must fail")
	}
	if _, _, err := repoEmpty.ListWithPagination(ctx, &paginationV1.PaginationRequest{}); err == nil {
		t.Error("ListWithPagination(empty collection) must fail")
	}
	if _, err := repoEmpty.Create(ctx, nil); err == nil {
		t.Error("Create(empty collection) must fail")
	}
	if _, err := repoEmpty.BatchCreate(ctx, nil); err == nil {
		t.Error("BatchCreate(empty collection) must fail")
	}
	if _, err := repoEmpty.Count(ctx, ""); err == nil {
		t.Error("Count(empty collection) must fail")
	}
	if _, err := repoEmpty.Exists(ctx, ""); err == nil {
		t.Error("Exists(empty collection) must fail")
	}

	// dto validation with a connected-but-lazy client (no I/O performed)
	repo := NewRepository[Candle, Candle](mustLazyClient(t), "candles")
	if _, err := repo.Create(ctx, nil); err == nil {
		t.Error("Create(nil dto) must fail")
	}
	if res, err := repo.BatchCreate(ctx, nil); err != nil || res != nil {
		t.Errorf("BatchCreate(empty dtos) = %v, %v", res, err)
	}
}

// mustLazyClient creates a lazily-initialized client for argument validation.
func mustLazyClient(t *testing.T) *Client {
	t.Helper()
	c, err := NewClient(WithHost("http://127.0.0.1:1"), WithToken("token"))
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	return c
}

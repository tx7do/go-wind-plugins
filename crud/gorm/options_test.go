package gorm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"go.opentelemetry.io/otel/attribute"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/prometheus"

	"github.com/tx7do/go-wind/log"
)

// prometheusConfigFixture returns a recognizable prometheus config used to
// verify WithPrometheusConfig copies the whole struct.
func prometheusConfigFixture() prometheus.Config {
	return prometheus.Config{DBName: "fixture"}
}

// recordingLogger is a minimal log.Logger that records emitted messages so
// tests can assert which gormLogger branches fired.
type recordingLogger struct {
	msgs []string
}

func (r *recordingLogger) Debug(_ context.Context, msg string, _ ...any) {
	r.msgs = append(r.msgs, "debug:"+msg)
}
func (r *recordingLogger) Info(_ context.Context, msg string, _ ...any) {
	r.msgs = append(r.msgs, "info:"+msg)
}
func (r *recordingLogger) Warn(_ context.Context, msg string, _ ...any) {
	r.msgs = append(r.msgs, "warn:"+msg)
}
func (r *recordingLogger) Error(_ context.Context, msg string, _ ...any) {
	r.msgs = append(r.msgs, "error:"+msg)
}
func (r *recordingLogger) Enabled(_ log.Level) bool { return true }
func (r *recordingLogger) With(_ ...any) log.Logger { return r }

func (r *recordingLogger) contains(substr string) bool {
	for _, m := range r.msgs {
		if strings.Contains(m, substr) {
			return true
		}
	}
	return false
}

// TestOptions_ApplyAll exercises every Option constructor and asserts the
// resulting Client field mutation. Each case is hermetic: applying an option
// only sets fields, it never dials a database.
func TestOptions_ApplyAll(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	attrs := []attribute.KeyValue{attribute.String("k", "v")}

	// The Client under test starts from a pristine zero value; each option is
	// applied and the touched field verified directly (same package access).
	c := &Client{}

	optList := []struct {
		name  string
		apply func(*Client)
		check func(t *testing.T, c *Client)
	}{
		{"WithGormDB", WithGormDB(db), func(t *testing.T, c *Client) {
			if c.DB != db {
				t.Error("WithGormDB did not set DB")
			}
		}},
		{"WithDriverName", WithDriverName("go_sqlite"), func(t *testing.T, c *Client) {
			if c.driverName != "go_sqlite" {
				t.Error("WithDriverName did not set driverName")
			}
		}},
		{"WithDSN", WithDSN(":memory:"), func(t *testing.T, c *Client) {
			if c.masterDSN != ":memory:" {
				t.Error("WithDSN did not set masterDSN")
			}
		}},
		{"WithReplicaDsns", WithReplicaDsns([]string{"a", "b"}), func(t *testing.T, c *Client) {
			if len(c.replicaDsns) != 2 {
				t.Error("WithReplicaDsns did not set replicaDsns")
			}
		}},
		{"WithEnableTrace", WithEnableTrace(true), func(t *testing.T, c *Client) {
			if !c.enableTrace {
				t.Error("WithEnableTrace did not set enableTrace")
			}
		}},
		{"WithEnableMigrate", WithEnableMigrate(true), func(t *testing.T, c *Client) {
			if !c.enableMigrate {
				t.Error("WithEnableMigrate did not set enableMigrate")
			}
		}},
		{"WithEnableMetrics", WithEnableMetrics(true), func(t *testing.T, c *Client) {
			if !c.enableMetrics {
				t.Error("WithEnableMetrics did not set enableMetrics")
			}
		}},
		{"WithEnableDbResolver", WithEnableDbResolver(true), func(t *testing.T, c *Client) {
			if !c.enableDbResolver {
				t.Error("WithEnableDbResolver did not set enableDbResolver")
			}
		}},
		{"WithGormConfig", WithGormConfig(&gorm.Config{SkipDefaultTransaction: true}), func(t *testing.T, c *Client) {
			if c.gormCfg == nil || !c.gormCfg.SkipDefaultTransaction {
				t.Error("WithGormConfig did not set gormCfg")
			}
		}},
		{"WithGormConfig_NilIgnored", WithGormConfig(nil), func(t *testing.T, c *Client) {
			// nil config must not clobber the previously set one
			if c.gormCfg == nil || !c.gormCfg.SkipDefaultTransaction {
				t.Error("WithGormConfig(nil) overwrote existing config")
			}
		}},
		{"WithMixin", WithMixin(func(*gorm.DB) error { return nil }), func(t *testing.T, c *Client) {
			if len(c.mixins) != 1 {
				t.Errorf("WithMixin expected 1 mixin, got %d", len(c.mixins))
			}
		}},
		{"WithMixins", WithMixins(func(*gorm.DB) error { return nil }, nil), func(t *testing.T, c *Client) {
			if len(c.mixins) != 3 {
				t.Errorf("WithMixins expected 3 mixins total, got %d", len(c.mixins))
			}
		}},
		{"WithAutoMigrate", WithAutoMigrate(&testUserEntity{}), func(t *testing.T, c *Client) {
			if len(c.mixins) != 4 {
				t.Errorf("WithAutoMigrate expected appended mixin, got %d", len(c.mixins))
			}
		}},
		{"WithGetMigrateModels", WithGetMigrateModels(func() []any { return []any{&testUserEntity{}} }), func(t *testing.T, c *Client) {
			if c.getMigrateModels == nil {
				t.Error("WithGetMigrateModels did not set getMigrateModels")
			}
		}},
		{"WithLogger", WithLogger(&recordingLogger{}), func(t *testing.T, c *Client) {
			if c.gormCfg.Logger == nil {
				t.Error("WithLogger did not set gorm logger")
			}
		}},
		{"WithLoggerLevel", WithLoggerLevel(&recordingLogger{}, logger.Error), func(t *testing.T, c *Client) {
			if c.gormCfg.Logger == nil {
				t.Error("WithLoggerLevel did not set gorm logger")
			}
		}},
		{"WithContext", WithContext(context.Background()), func(t *testing.T, c *Client) {
			if c.ctx == nil {
				t.Error("WithContext did not set ctx")
			}
		}},
		{"WithBeforeOpen", WithBeforeOpen(func(*gorm.DB) error { return nil }), func(t *testing.T, c *Client) {
			if len(c.beforeOpen) != 1 {
				t.Error("WithBeforeOpen did not append hook")
			}
		}},
		{"WithAfterOpen", WithAfterOpen(func(*gorm.DB) error { return nil }), func(t *testing.T, c *Client) {
			if len(c.afterOpen) != 1 {
				t.Error("WithAfterOpen did not append hook")
			}
		}},
		{"WithPrometheusConfig", WithPrometheusConfig(prometheusConfigFixture()), func(t *testing.T, c *Client) {
			if c.prometheusConfig.DBName != "fixture" {
				t.Error("WithPrometheusConfig did not copy config")
			}
		}},
		{"WithPrometheusDbName", WithPrometheusDbName("metrics-db"), func(t *testing.T, c *Client) {
			if c.prometheusConfig.DBName != "metrics-db" {
				t.Error("WithPrometheusDbName did not set DBName")
			}
		}},
		{"WithPrometheusPushAddr", WithPrometheusPushAddr("127.0.0.1:9091"), func(t *testing.T, c *Client) {
			if c.prometheusConfig.PushAddr != "127.0.0.1:9091" {
				t.Error("WithPrometheusPushAddr did not set PushAddr")
			}
		}},
		{"WithPrometheusHTTPServerPort", WithPrometheusHTTPServerPort(8081), func(t *testing.T, c *Client) {
			if c.prometheusConfig.HTTPServerPort != 8081 {
				t.Error("WithPrometheusHTTPServerPort did not set port")
			}
		}},
		{"WithPrometheusRefreshInterval", WithPrometheusRefreshInterval(30), func(t *testing.T, c *Client) {
			if c.prometheusConfig.RefreshInterval != 30 {
				t.Error("WithPrometheusRefreshInterval did not set interval")
			}
		}},
		{"WithPrometheusPushAuth", WithPrometheusPushAuth("u", "p"), func(t *testing.T, c *Client) {
			if c.prometheusConfig.PushUser != "u" || c.prometheusConfig.PushPassword != "p" {
				t.Error("WithPrometheusPushAuth did not set credentials")
			}
		}},
		{"WithPrometheusStartServer", WithPrometheusStartServer(true), func(t *testing.T, c *Client) {
			if !c.prometheusConfig.StartServer {
				t.Error("WithPrometheusStartServer did not set StartServer")
			}
		}},
		{"WithPrometheusLabels", WithPrometheusLabels(map[string]string{"a": "b"}), func(t *testing.T, c *Client) {
			if c.prometheusConfig.Labels["a"] != "b" {
				t.Error("WithPrometheusLabels did not set labels")
			}
		}},
		{"WithMaxIdleConns", WithMaxIdleConns(7), func(t *testing.T, c *Client) {
			if c.maxIdleConns == nil || *c.maxIdleConns != 7 {
				t.Error("WithMaxIdleConns did not set maxIdleConns")
			}
		}},
		{"WithMaxOpenConns", WithMaxOpenConns(9), func(t *testing.T, c *Client) {
			if c.maxOpenConns == nil || *c.maxOpenConns != 9 {
				t.Error("WithMaxOpenConns did not set maxOpenConns")
			}
		}},
		{"WithConnMaxLifetime", WithConnMaxLifetime(time.Minute), func(t *testing.T, c *Client) {
			if c.connMaxLifetime == nil || *c.connMaxLifetime != time.Minute {
				t.Error("WithConnMaxLifetime did not set connMaxLifetime")
			}
		}},
		{"WithTracingOptions", WithTracingOptions(), func(t *testing.T, c *Client) {
			// zero options append nothing but must not panic
			_ = c
		}},
		{"WithTracingAttributes", WithTracingAttributes(attrs...), func(t *testing.T, c *Client) {
			if len(c.tracingOption) == 0 {
				t.Error("WithTracingAttributes did not append tracing options")
			}
		}},
		{"WithTracingDBSystem", WithTracingDBSystem("sqlite"), func(t *testing.T, c *Client) {
			if len(c.tracingOption) == 0 {
				t.Error("WithTracingDBSystem did not append tracing options")
			}
		}},
		{"WithTracingWithoutMetrics", WithTracingWithoutMetrics(), func(t *testing.T, c *Client) {
			if len(c.tracingOption) == 0 {
				t.Error("WithTracingWithoutMetrics did not append tracing options")
			}
		}},
		{"WithTracingWithoutServerAddress", WithTracingWithoutServerAddress(), func(t *testing.T, c *Client) {
			if len(c.tracingOption) == 0 {
				t.Error("WithTracingWithoutServerAddress did not append tracing options")
			}
		}},
	}

	for _, tc := range optList {
		t.Run(tc.name, func(t *testing.T) {
			tc.apply(c)
			tc.check(t, c)
		})
	}
}

// TestNewClient_NoDBAndNoDriver verifies the fail-fast path when neither an
// external DB nor driverName/masterDSN is provided.
func TestNewClient_NoDBAndNoDriver(t *testing.T) {
	c, err := NewClient()
	if err == nil {
		t.Fatal("expected error when no DB and no driver provided")
	}
	if c != nil {
		t.Fatal("expected nil client on error")
	}
	if !strings.Contains(err.Error(), "gorm DB not provided") {
		t.Fatalf("unexpected error text: %v", err)
	}
}

// TestNewClient_NilDBOptionStillErrors verifies WithGormDB(nil) does not count
// as a provided DB (the nil check happens before use).
func TestNewClient_NilDBOptionStillErrors(t *testing.T) {
	c, err := NewClient(WithGormDB(nil))
	if err == nil {
		t.Fatal("expected error for nil DB option")
	}
	if c != nil {
		t.Fatal("expected nil client on error")
	}
}

// TestNewClient_MixinErrorPropagates verifies a failing mixin aborts NewClient.
func TestNewClient_MixinErrorPropagates(t *testing.T) {
	wantErr := errors.New("boom")
	_, err := NewClient(
		WithGormDB(mustOpenClientDB(t)),
		WithMixin(func(*gorm.DB) error { return wantErr }),
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected mixin error, got %v", err)
	}
}

// TestNewClient_BeforeOpenErrorPropagates verifies a failing beforeOpen hook
// aborts NewClient before mixins run.
func TestNewClient_BeforeOpenErrorPropagates(t *testing.T) {
	wantErr := errors.New("before-open failed")
	mixinRan := false
	_, err := NewClient(
		WithGormDB(mustOpenClientDB(t)),
		WithBeforeOpen(func(*gorm.DB) error { return wantErr }),
		WithMixin(func(*gorm.DB) error { mixinRan = true; return nil }),
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected beforeOpen error, got %v", err)
	}
	if mixinRan {
		t.Fatal("mixins must not run when beforeOpen fails")
	}
}

// TestNewClient_AfterOpenErrorPropagates verifies a failing afterOpen hook
// aborts NewClient.
func TestNewClient_AfterOpenErrorPropagates(t *testing.T) {
	wantErr := errors.New("after-open failed")
	_, err := NewClient(
		WithGormDB(mustOpenClientDB(t)),
		WithAfterOpen(func(*gorm.DB) error { return wantErr }),
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected afterOpen error, got %v", err)
	}
}

// TestClient_UseAndResolveMigrateModels covers Client.Use plus all three
// sources merged by resolveMigrateModels.
func TestClient_UseAndResolveMigrateModels(t *testing.T) {
	c := &Client{}

	if got := c.resolveMigrateModels(); got != nil {
		t.Fatalf("expected nil model list on empty client, got %v", got)
	}

	c.Use(func(*gorm.DB) error { return nil })
	c.Use(nil) // nil mixin is skipped at run time but still appended by Use
	if len(c.mixins) != 2 {
		t.Fatalf("Use should append mixins, got %d", len(c.mixins))
	}

	RegisterMigrateModel(nil) // ignored
	RegisterMigrateModel(&testUserEntity{})
	RegisterMigrateModels()
	RegisterMigrateModels(&testUserEntity{}, &CacheTestUser{})

	c.getMigrateModels = func() []any { return []any{CacheTestUser{}} }
	c.migrateModels = []any{&struct{ ID uint }{}}

	got := c.resolveMigrateModels()
	// 3 globally registered (1 + 2) + 1 injected + 1 instance-level
	if len(got) != 5 {
		t.Fatalf("expected 5 resolved models, got %d", len(got))
	}
}

// TestGormLogger_LevelsAndTrace exercises the gorm logger adapter level
// gating and the Trace slow/error/not-found branches.
func TestGormLogger_LevelsAndTrace(t *testing.T) {
	t.Run("nil logger source keeps global logger untouched", func(t *testing.T) {
		l := NewGormLogger(nil)
		if l == nil {
			t.Fatal("expected non-nil logger")
		}
	})

	t.Run("default level is warn", func(t *testing.T) {
		rec := &recordingLogger{}
		defer log.SetLogger(nil)
		l := NewGormLogger(rec)
		gl, ok := l.(*gormLogger)
		if !ok {
			t.Fatalf("expected *gormLogger, got %T", l)
		}
		if gl.cfg.LogLevel != logger.Warn {
			t.Fatalf("expected default Warn level, got %v", gl.cfg.LogLevel)
		}
	})

	t.Run("LogMode clones and does not mutate receiver", func(t *testing.T) {
		l := NewGormLoggerWithLevel(nil, logger.Warn)
		cloned := l.LogMode(logger.Info)
		if l == cloned {
			t.Fatal("LogMode must return a new instance")
		}
		if l.(*gormLogger).cfg.LogLevel != logger.Warn {
			t.Fatal("receiver level must stay warn")
		}
		if cloned.(*gormLogger).cfg.LogLevel != logger.Info {
			t.Fatal("clone level must be info")
		}
	})

	t.Run("level gating for Info Warn Error", func(t *testing.T) {
		rec := &recordingLogger{}
		l := NewGormLoggerWithLevel(rec, logger.Error)

		ctx := context.Background()
		l.Info(ctx, "info %d", 1)
		l.Warn(ctx, "warn %d", 2)
		if len(rec.msgs) != 0 {
			t.Fatalf("error-level logger must drop info/warn, got %v", rec.msgs)
		}
		l.Error(ctx, "err %d", 3)
		if !rec.contains("error:err 3") {
			t.Fatalf("expected error message recorded, got %v", rec.msgs)
		}

		// Raise level and confirm info/warn now pass through.
		l = l.LogMode(logger.Info)
		l.Info(ctx, "info-on")
		l.Warn(ctx, "warn-on")
		if !rec.contains("info:info-on") || !rec.contains("warn:warn-on") {
			t.Fatalf("expected info/warn recorded after LogMode, got %v", rec.msgs)
		}
	})

	t.Run("trace branches", func(t *testing.T) {
		ctx := context.Background()
		fc := func() (string, int64) { return "SELECT 1", 1 }

		// Silent: fc must not even be invoked.
		called := false
		silentFC := func() (string, int64) { called = true; return "SELECT 1", 1 }
		NewGormLoggerWithLevel(nil, logger.Silent).Trace(ctx, time.Now(), silentFC, nil)
		if called {
			t.Fatal("silent logger must not call fc")
		}

		// Ignored record-not-found: no output.
		rec := &recordingLogger{}
		notFound := NewGormLoggerWithLevel(rec, logger.Error)
		notFound.Trace(ctx, time.Now(), fc, gorm.ErrRecordNotFound)
		if len(rec.msgs) != 0 {
			t.Fatalf("ErrRecordNotFound must be ignored, got %v", rec.msgs)
		}

		// Real error: logged at error level.
		rec2 := &recordingLogger{}
		errL := NewGormLoggerWithLevel(rec2, logger.Error)
		errL.Trace(ctx, time.Now(), fc, errors.New("db exploded"))
		if !rec2.contains("db exploded") {
			t.Fatalf("expected error logged, got %v", rec2.msgs)
		}

		// Slow query above threshold: logged at warn.
		rec3 := &recordingLogger{}
		slowL := NewGormLoggerWithLevel(rec3, logger.Warn)
		slowL.Trace(ctx, time.Now().Add(-time.Second), fc, nil)
		if !rec3.contains("[GORM][SLOW]") {
			t.Fatalf("expected slow query warning, got %v", rec3.msgs)
		}

		// Fast query at info level: logged at info.
		rec4 := &recordingLogger{}
		infoL := NewGormLoggerWithLevel(rec4, logger.Info)
		infoL.Trace(ctx, time.Now(), fc, nil)
		if !rec4.contains("[GORM]") || rec4.contains("[GORM][SLOW]") {
			t.Fatalf("expected plain info trace, got %v", rec4.msgs)
		}
	})
}

// TestEscapeScanPattern verifies Redis MATCH wildcard escaping.
func TestEscapeScanPattern(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"plain", "plain"},
		{"star*", `star\*`},
		{"quest?ion", `quest\?ion`},
		{"brack[et]s", `brack\[et\]s`},
		{`back\slash`, `back\\slash`},
		{"", ""},
	}
	for _, tc := range cases {
		if got := escapeScanPattern(tc.in); got != tc.want {
			t.Errorf("escapeScanPattern(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// mustOpenClientDB opens a throwaway in-memory sqlite DB used as an externally
// provided gorm.DB in client tests (existing tests in this module already use
// in-memory sqlite via the glebarez driver).
func mustOpenClientDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

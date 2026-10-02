package machinery

import (
	"context"
	"crypto/tls"
	"strings"
	"testing"
	"time"

	"github.com/RichardKnop/machinery/v2/config"
	"github.com/RichardKnop/machinery/v2/tasks"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/tx7do/go-wind/log"
)

// ---------------------------------------------------------------------------
// MessageCarrier
// ---------------------------------------------------------------------------

func TestMessageCarrierSetGetKeys(t *testing.T) {
	// Regression test: the carrier used to panic on a nil tasks.Headers map
	// ("assignment to entry in nil map"); Set must lazily initialize the map
	// behind the pointer instead.
	var headers tasks.Headers
	carrier := NewMessageCarrier(&headers)

	assert.Empty(t, carrier.Keys())

	carrier.Set("trace-id", "abc-123")
	carrier.Set("span-id", "span-1")

	assert.Equal(t, "abc-123", carrier.Get("trace-id"))
	assert.Equal(t, "span-1", carrier.Get("span-id"))
	assert.Equal(t, "", carrier.Get("missing"))

	keys := carrier.Keys()
	assert.Len(t, keys, 2)
	assert.Contains(t, keys, "trace-id")
	assert.Contains(t, keys, "span-id")

	// The map must have been initialized in place, visible to the owner.
	assert.NotNil(t, headers)
	assert.Equal(t, "abc-123", headers["trace-id"])
}

// TestMessageCarrierNilPointerDoesNotPanic covers the nil *tasks.Headers
// pointer: Get/Keys read from nothing and Set has nowhere to write, so they
// must degrade gracefully instead of dereferencing the nil pointer.
func TestMessageCarrierNilPointerDoesNotPanic(t *testing.T) {
	carrier := NewMessageCarrier(nil)

	assert.NotPanics(t, func() {
		assert.Empty(t, carrier.Get("k"))
		assert.Empty(t, carrier.Keys())
		carrier.Set("k", "v")
	})
}

// TestMessageCarrierGetNumericTypes verifies the numeric formatting branches:
// the machinery Headers map stores any values, and Get must render them all
// as strings without losing precision.
func TestMessageCarrierGetNumericTypes(t *testing.T) {
	headers := tasks.Headers{
		"float64":  1.5,
		"float32":  float32(2.5),
		"int":      -3,
		"uint":     uint(4),
		"int8":     int8(-5),
		"uint8":    uint8(6),
		"int16":    int16(-7),
		"uint16":   uint16(8),
		"int32":    int32(-9),
		"uint32":   uint32(10),
		"int64":    int64(-11),
		"uint64":   uint64(12),
		"string":   "plain",
		"bytes":    []byte("raw"),
		"nil":      nil,
		"bool":     true,
		"structty": struct{ A int }{A: 1},
	}

	carrier := NewMessageCarrier(&headers)

	assert.Equal(t, "1.5", carrier.Get("float64"))
	assert.Equal(t, "2.5", carrier.Get("float32"))
	assert.Equal(t, "-3", carrier.Get("int"))
	assert.Equal(t, "4", carrier.Get("uint"))
	assert.Equal(t, "-5", carrier.Get("int8"))
	assert.Equal(t, "6", carrier.Get("uint8"))
	assert.Equal(t, "-7", carrier.Get("int16"))
	assert.Equal(t, "8", carrier.Get("uint16"))
	assert.Equal(t, "-9", carrier.Get("int32"))
	assert.Equal(t, "10", carrier.Get("uint32"))
	assert.Equal(t, "-11", carrier.Get("int64"))
	assert.Equal(t, "12", carrier.Get("uint64"))
	assert.Equal(t, "plain", carrier.Get("string"))
	assert.Equal(t, "raw", carrier.Get("bytes"))
	assert.Equal(t, "", carrier.Get("nil"))
	assert.Equal(t, "", carrier.Get("bool"))     // unsupported kind falls back to ""
	assert.Equal(t, "", carrier.Get("structty")) // unsupported kind falls back to ""
}

// TestMessageCarrierImplementsTextMapCarrier pins the otel carrier contract
// so tracing injection/extraction keeps working against machinery headers.
func TestMessageCarrierImplementsTextMapCarrier(t *testing.T) {
	// Seed a valid span context; TraceContext only injects for valid ones.
	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		SpanID:  trace.SpanID{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18},
		Remote:  true,
	})
	baseCtx := trace.ContextWithSpanContext(context.Background(), spanCtx)

	// Regression test: the carrier used to require an initialized map up
	// front; Inject now lazily initializes the map behind the pointer.
	var headers tasks.Headers
	carrier := NewMessageCarrier(&headers)

	propagator := propagation.TraceContext{}
	propagator.Inject(baseCtx, carrier)
	assert.NotEmpty(t, carrier.Keys(), "TraceContext inject must write traceparent")
	assert.NotEmpty(t, headers, "the injected map must be initialized in place")

	got := trace.SpanContextFromContext(propagator.Extract(baseCtx, carrier))
	assert.Equal(t, spanCtx.TraceID(), got.TraceID())
	assert.Equal(t, spanCtx.SpanID(), got.SpanID())
	assert.True(t, got.IsRemote())
}

// ---------------------------------------------------------------------------
// Task options
// ---------------------------------------------------------------------------

func TestTaskOptions(t *testing.T) {
	delay := time.Now().UTC().Add(time.Minute)

	sig := &tasks.Signature{Name: "task"}
	WithDelayTime(delay)(sig)
	WithRetryCount(3)(sig)
	WithRetryTimeout(30)(sig)
	WithHeaders(tasks.Headers{"h1": "v1"})(sig)
	WithRoutingKey("rk")(sig)
	WithPriority(7)(sig)
	WithArgument("int64", 5)(sig)
	WithArgument("string", "x")(sig)
	WithHeader("h2", "v2")(sig) // appends into the existing headers map

	assert.Equal(t, delay.Format(time.RFC3339Nano), sig.ETA.Format(time.RFC3339Nano))
	assert.Equal(t, 3, sig.RetryCount)
	assert.Equal(t, 30, sig.RetryTimeout)
	assert.Equal(t, "rk", sig.RoutingKey)
	assert.Equal(t, uint8(7), sig.Priority)
	assert.Len(t, sig.Args, 2)
	assert.Equal(t, "int64", sig.Args[0].Type)
	assert.Equal(t, 5, sig.Args[0].Value)
	assert.Equal(t, "string", sig.Args[1].Type)
	assert.Equal(t, "x", sig.Args[1].Value)

	// WithHeaders replaced the map first, WithHeader then appended into it.
	assert.Equal(t, "v1", sig.Headers["h1"])
	assert.Equal(t, "v2", sig.Headers["h2"])
}

func TestWithHeaderCreatesMapWhenNil(t *testing.T) {
	sig := &tasks.Signature{Name: "task"}
	WithHeader("k", "v")(sig)
	assert.Equal(t, "v", sig.Headers["k"])
}

func TestWithTaskBuildsSignatures(t *testing.T) {
	var sigs []*tasks.Signature

	WithTask("add", WithArgument("int64", 1), WithArgument("int64", 2))(&sigs)
	WithTask("noop")(&sigs)

	if assert.Len(t, sigs, 2) {
		assert.Equal(t, "add", sigs[0].Name)
		assert.Len(t, sigs[0].Args, 2)
		assert.Equal(t, "noop", sigs[1].Name)
		assert.Empty(t, sigs[1].Args)
	}
}

// ---------------------------------------------------------------------------
// Server options and defaults
// ---------------------------------------------------------------------------

func TestNewServerDefaults(t *testing.T) {
	srv := NewServer()

	assert.Equal(t, KindMachinery, srv.Name())
	assert.Equal(t, "machinery", srv.Name())
	assert.NotNil(t, srv.cfg)
	assert.Equal(t, "wind_machinery_queue", srv.cfg.DefaultQueue)
	assert.Equal(t, 3600, srv.cfg.ResultsExpireIn)
	assert.True(t, srv.cfg.NoUnixSignals, "worker must not capture unix signals itself")
	assert.Equal(t, "wind_machinery_worker", srv.consumerOption.consumerTag)
	assert.Equal(t, 1, srv.consumerOption.concurrency)
	assert.Equal(t, "wind_machinery_queue", srv.consumerOption.queue)
	assert.Equal(t, BrokerTypeRedis, srv.brokerOption.brokerType)
	assert.Equal(t, BackendTypeRedis, srv.backendOption.backendType)
	assert.Equal(t, LockTypeRedis, srv.lockOption.lockType)

	// No broker configured: the server must fall back to the eager in-memory
	// broker, leaving the machinery server usable for task registration.
	assert.NotNil(t, srv.machineryServer)
	assert.NoError(t, srv.err)
	assert.Empty(t, srv.Endpoint())
}

func TestServerOptions(t *testing.T) {
	redisCfg := &config.RedisConfig{MaxIdle: 9}
	amqpCfg := &config.AMQPConfig{Exchange: "ex"}
	sqsCfg := &config.SQSConfig{}
	gcpCfg := &config.GCPPubSubConfig{}
	mongoCfg := &config.MongoDBConfig{}
	dynamoCfg := &config.DynamoDBConfig{}

	srv := NewServer(
		WithBrokerAddress("redis.example:6379", 3, BrokerTypeRedis),
		WithResultBackendAddress("backend.example:6379", 2, BackendTypeRedis),
		WithLockAddress("lock.example:6379", 1, 5, LockTypeRedis),
		WithRedisConfig(redisCfg),
		WithAMQPConfig(amqpCfg),
		WithSQSConfig(sqsCfg),
		WithGCPPubSubConfig(gcpCfg),
		WithMongoDBConfig(mongoCfg),
		WithDynamoDBConfig(dynamoCfg),
		WithConsumerOption("custom_tag", 4, "custom_queue"),
		WithDefaultQueue("custom_queue"),
		WithResultsExpireIn(60),
		WithNoUnixSignals(false),
	)

	assert.Equal(t, "redis.example:6379", srv.cfg.Broker)
	assert.Equal(t, 3, srv.brokerOption.db)
	assert.Equal(t, "backend.example:6379", srv.cfg.ResultBackend)
	assert.Equal(t, 2, srv.backendOption.db)
	assert.Equal(t, "lock.example:6379", srv.cfg.Lock)
	assert.Equal(t, 1, srv.lockOption.db)
	assert.Equal(t, 5, srv.lockOption.retries)
	assert.Same(t, redisCfg, srv.cfg.Redis)
	assert.Same(t, amqpCfg, srv.cfg.AMQP)
	assert.Same(t, sqsCfg, srv.cfg.SQS)
	assert.Same(t, gcpCfg, srv.cfg.GCPPubSub)
	assert.Same(t, mongoCfg, srv.cfg.MongoDB)
	assert.Same(t, dynamoCfg, srv.cfg.DynamoDB)
	assert.Equal(t, "custom_tag", srv.consumerOption.consumerTag)
	assert.Equal(t, 4, srv.consumerOption.concurrency)
	assert.Equal(t, "custom_queue", srv.consumerOption.queue)
	assert.Equal(t, "custom_queue", srv.cfg.DefaultQueue)
	assert.Equal(t, 60, srv.cfg.ResultsExpireIn)
	assert.False(t, srv.cfg.NoUnixSignals)
}

func TestWithTLSConfig(t *testing.T) {
	tlsConf := &tls.Config{}

	// WithTLSConfig must lazily create the config object when missing.
	srv := NewServer(WithTLSConfig(tlsConf))
	assert.Same(t, tlsConf, srv.cfg.TLSConfig)
}

func TestWithYamlConfig(t *testing.T) {
	srv := NewServer(WithYamlConfig("./testconfig.yml", false))

	assert.Equal(t, "127.0.0.1:6379", srv.cfg.Broker)
	assert.Equal(t, "127.0.0.1:6379", srv.cfg.ResultBackend)
	assert.NotNil(t, srv.cfg.Redis)
	assert.Equal(t, 12, srv.cfg.Redis.MaxIdle)
}

// TestWithYamlConfigMissingFile is the flipped regression test for the former
// defect: a missing yaml file used to leave Server.cfg nil and NewServer
// nil-panicked inside createMachineryServer. The load error is now captured in
// Server.err (Start returns it) while NewServer completes with the default
// config.
func TestWithYamlConfigMissingFile(t *testing.T) {
	var srv *Server
	assert.NotPanics(t, func() {
		srv = NewServer(WithYamlConfig("./does_not_exist.yml", false))
	})

	assert.NotNil(t, srv.cfg, "the default config must be kept so initialization can finish")
	assert.ErrorContains(t, srv.err, "does_not_exist.yml")

	assert.Error(t, srv.Start(context.Background()), "Start must surface the captured config load error")
}

// ---------------------------------------------------------------------------
// Task registration and workflow validation (eager broker, no external deps)
// ---------------------------------------------------------------------------

func noopTask() error { return nil }

func TestHandleFuncRegistersTask(t *testing.T) {
	srv := NewServer()

	assert.NoError(t, srv.HandleFunc("hermetic_noop", noopTask))
	// Duplicate registration silently overwrites (machinery behavior), so it
	// must not error either.
	assert.NoError(t, srv.HandleFunc("hermetic_noop", noopTask))
	// Non-function handlers must be rejected by ValidateTask.
	assert.Error(t, srv.HandleFunc("hermetic_bad", 42))
}

func TestHandleFuncWithoutMachineryServer(t *testing.T) {
	srv := &Server{}
	assert.EqualError(t, srv.HandleFunc("task", noopTask), "machinery server not initialized (broker creation may have failed)")
}

func TestWorkflowValidationErrors(t *testing.T) {
	srv := NewServer()

	assert.EqualError(t, srv.NewGroup(), "group task is empty")
	assert.EqualError(t, srv.NewChain(), "chain task is empty")
	assert.EqualError(t, srv.NewChord(WithTask("add")), "chord task is empty")
}

func TestRegisterPeriodicTaskEager(t *testing.T) {
	srv := NewServer()
	assert.NoError(t, srv.HandleFunc("hermetic_periodic", noopTask))

	// Registering a periodic task only touches the eager broker's in-memory
	// cron table; no broker connection is needed.
	assert.NoError(t, srv.NewPeriodicTask(context.Background(), "*/1 * * * *", "hermetic_periodic"))
}

// ---------------------------------------------------------------------------
// Lifecycle guards
// ---------------------------------------------------------------------------

func TestStopWithoutStart(t *testing.T) {
	srv := NewServer()
	assert.NoError(t, srv.Stop(context.Background()))
	assert.NoError(t, srv.Stop(context.Background()), "Stop must be idempotent")
}

func TestBrokerAndBackendTypeConstants(t *testing.T) {
	assert.Equal(t, BrokerType(0), BrokerTypeRedis)
	assert.Equal(t, BrokerType(1), BrokerTypeAmqp)
	assert.Equal(t, BrokerType(2), BrokerTypeGcpPubSub)
	assert.Equal(t, BrokerType(3), BrokerTypeSQS)

	assert.Equal(t, BackendType(0), BackendTypeRedis)
	assert.Equal(t, BackendType(1), BackendTypeAmqp)
	assert.Equal(t, BackendType(2), BackendTypeMemcache)
	assert.Equal(t, BackendType(3), BackendTypeMongoDB)
	assert.Equal(t, BackendType(4), BackendTypeDynamoDB)

	assert.Equal(t, LockType(0), LockTypeRedis)
}

// TestSetLoggerNilResets ensures the package logger override can be cleared.
func TestSetLoggerNilResets(t *testing.T) {
	assert.NotPanics(t, func() {
		SetLogger(nil)
		getLogger() // falls back to the framework logger
	})
}

// TestLoggerAdaptersExerciseAllLevels covers the machinery logging adapter so
// none of its methods can panic; output goes to the nop framework logger.
func TestLoggerAdaptersExerciseAllLevels(t *testing.T) {
	l := newLogger(log.LevelDebug)

	assert.NotPanics(t, func() {
		l.Print("print")
		l.Printf("printf %d", 1)
		l.Println("println")
		l.Fatal("fatal")
		l.Fatalf("fatalf %d", 1)
		l.Fatalln("fatalln")
		l.Panic("panic")
		l.Panicf("panicf %d", 1)
		l.Panicln("panicln")
	})
}

// ---------------------------------------------------------------------------
// Log helper functions
// ---------------------------------------------------------------------------

func TestLogHelpersDoNotPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		LogDebug("debug")
		LogInfo("info")
		LogWarn("warn")
		LogError("error")
		LogFatal("fatal")
		LogDebugf("debug %s", "f")
		LogInfof("info %s", "f")
		LogWarnf("warn %s", "f")
		LogErrorf("error %s", "f")
		LogFatalf("fatal %s", "f")
	})
	assert.True(t, strings.HasPrefix(logKey, "[machinery"))
}

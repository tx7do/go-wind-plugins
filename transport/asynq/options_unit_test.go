package asynq

import (
	"context"
	"crypto/tls"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Redis connection option plumbing
// ---------------------------------------------------------------------------

type unknownConnOpt struct{}

func (unknownConnOpt) MakeRedisClient() interface{} { return nil }

func TestNewServerDefaults(t *testing.T) {
	srv := NewServer()

	assert.Equal(t, "asynq", KindAsynq)
	assert.NotNil(t, srv.redisConnOpt)
	assert.Equal(t, defaultConcurrency, srv.asynqConfig.Concurrency)
	assert.NotNil(t, srv.mux)
	assert.NotNil(t, srv.schedulerOpts)
	assert.NotNil(t, srv.codec, "default codec must be JSON")
	assert.True(t, srv.serverEnabled)
	assert.True(t, srv.clientEnabled)
	assert.True(t, srv.schedulerEnabled)

	// The default single-node option points at the documented default.
	opt, ok := srv.redisConnOpt.(*asynq.RedisClientOpt)
	require.True(t, ok)
	assert.Equal(t, defaultRedisAddress, opt.Addr)
	assert.Equal(t, defaultRedisDB, opt.DB)

	assert.NotNil(t, srv.server, "init must create the consumer server")
	assert.NotNil(t, srv.client, "init must create the producer client")
	assert.NotNil(t, srv.scheduler, "init must create the scheduler")
}

func TestEndpointVariants(t *testing.T) {
	assert.Equal(t, "asynq://127.0.0.1:6379", NewServer().Endpoint())

	cluster := NewServer(WithRedisType(RedisTypeCluster))
	assert.Contains(t, cluster.Endpoint(), "asynq://cluster:")

	failover := NewServer(WithRedisType(RedisTypeSentinel))
	assert.Contains(t, failover.Endpoint(), "asynq://mymaster@")

	// An unrecognized connection option falls back to the unknown sentinel.
	// The components must be disabled here: the asynq library itself panics
	// when asked to build a server/client with an unknown option type.
	unknown := NewServer(
		WithRedisConnOpt(unknownConnOpt{}),
		WithServerEnabled(false),
		WithClientEnabled(false),
		WithSchedulerEnabled(false),
	)
	assert.Equal(t, "asynq://unknown", unknown.Endpoint())
}

func TestWithRedisType(t *testing.T) {
	single := NewServer(WithRedisType(RedisTypeSingle))
	_, ok := single.redisConnOpt.(*asynq.RedisClientOpt)
	assert.True(t, ok, "single mode must build RedisClientOpt")

	cluster := NewServer(WithRedisType(RedisTypeCluster))
	_, ok = cluster.redisConnOpt.(*asynq.RedisClusterClientOpt)
	assert.True(t, ok, "cluster mode must build RedisClusterClientOpt")

	failover := NewServer(WithRedisType(RedisTypeSentinel))
	_, ok = failover.redisConnOpt.(*asynq.RedisFailoverClientOpt)
	assert.True(t, ok, "sentinel mode must build RedisFailoverClientOpt")

	assert.Panics(t, func() {
		WithRedisType("nonsense")(NewServer())
	}, "unknown redis type must panic")
}

// TestWithRedisURI pins the schemes the WithRedisURI option (via
// asynq.ParseRedisURI) actually accepts: "redis", "rediss", "redis-socket",
// and "redis-sentinel". The "redis+cluster://" and "redis+sentinel://" forms
// are NOT supported by the parser (cluster mode goes through
// WithRedisType/WithRedisConnOpt instead), and the option's doc comment
// matches that. (The former "value-type overrides are dropped" defect is
// fixed; see TestApplyRedisOptionsValueTypes and TestWithRedisURIThenOverrides.)
func TestWithRedisURI(t *testing.T) {
	single := NewServer(WithRedisURI("redis://127.0.0.1:6379"))
	opt, ok := single.redisConnOpt.(asynq.RedisClientOpt)
	require.True(t, ok, "the single-node URI must parse into a value RedisClientOpt")
	assert.Equal(t, "127.0.0.1:6379", opt.Addr)

	failover := NewServer(WithRedisURI("redis-sentinel://127.0.0.1:26379?master=mymaster"))
	fopt, ok := failover.redisConnOpt.(asynq.RedisFailoverClientOpt)
	require.True(t, ok, "the sentinel URI must parse into a value RedisFailoverClientOpt")
	assert.Equal(t, "mymaster", fopt.MasterName)

	assert.Panics(t, func() {
		WithRedisURI("redis+cluster://127.0.0.1:7000,127.0.0.1:7001")(NewServer())
	}, "redis+cluster is not a parser-supported scheme (the doc comment matches)")

	assert.Panics(t, func() {
		WithRedisURI("://not-a-uri")(NewServer())
	}, "an unparsable URI must panic")
}

func TestApplyRedisOptionsSingle(t *testing.T) {
	addr := "10.0.0.1:6379"
	user := "svc"
	pass := "secret"
	db := int32(3)
	pool := int32(11)
	dial := 5 * time.Second
	read := 6 * time.Second
	write := 7 * time.Second
	network := "tcp"

	srv := NewServer(
		WithRedisAddress(addr),
		WithRedisUsername(user),
		WithRedisPassword(pass),
		WithRedisDB(db),
		WithRedisPoolSize(pool),
		WithDialTimeout(dial),
		WithReadTimeout(read),
		WithWriteTimeout(write),
		WithTLSConfig(&tls.Config{}),
		WithNetwork(&network),
	)

	opt, ok := srv.redisConnOpt.(*asynq.RedisClientOpt)
	require.True(t, ok)
	assert.Equal(t, addr, opt.Addr)
	assert.Equal(t, user, opt.Username)
	assert.Equal(t, pass, opt.Password)
	assert.Equal(t, int(db), opt.DB)
	assert.Equal(t, int(pool), opt.PoolSize)
	assert.Equal(t, dial, opt.DialTimeout)
	assert.Equal(t, read, opt.ReadTimeout)
	assert.Equal(t, write, opt.WriteTimeout)
	assert.NotNil(t, opt.TLSConfig)
	assert.Equal(t, network, opt.Network)
}

func TestApplyRedisOptionsCluster(t *testing.T) {
	addrs := []string{"10.0.0.1:7000", "10.0.0.2:7000"}
	redirects := int32(7)

	srv := NewServer(WithRedisType(RedisTypeCluster))
	srv.addresses = addrs
	srv.username = strPtr("svc")
	srv.password = strPtr("secret")
	srv.dialTimeout = durPtr(3 * time.Second)
	srv.readTimeout = durPtr(4 * time.Second)
	srv.writeTimeout = durPtr(5 * time.Second)
	srv.tlsConfig = &tls.Config{}
	srv.maxRedirects = &redirects
	srv.applyRedisOptions()

	opt, ok := srv.redisConnOpt.(*asynq.RedisClusterClientOpt)
	require.True(t, ok)
	assert.Equal(t, addrs, opt.Addrs)
	assert.Equal(t, "svc", opt.Username)
	assert.Equal(t, "secret", opt.Password)
	assert.Equal(t, 3*time.Second, opt.DialTimeout)
	assert.Equal(t, 4*time.Second, opt.ReadTimeout)
	assert.Equal(t, 5*time.Second, opt.WriteTimeout)
	assert.NotNil(t, opt.TLSConfig)
	assert.Equal(t, int(redirects), opt.MaxRedirects)
}

func TestApplyRedisOptionsFailover(t *testing.T) {
	addrs := []string{"10.0.0.1:26379"}
	master := "mymaster"

	srv := NewServer(WithRedisType(RedisTypeSentinel))
	srv.addresses = addrs
	srv.username = strPtr("svc")
	srv.password = strPtr("secret")
	srv.db = int32Ptr(2)
	srv.poolSize = int32Ptr(9)
	srv.masterName = &master
	srv.sentinelUsername = strPtr("suser")
	srv.sentinelPassword = strPtr("spass")
	srv.applyRedisOptions()

	opt, ok := srv.redisConnOpt.(*asynq.RedisFailoverClientOpt)
	require.True(t, ok)
	assert.Equal(t, addrs, opt.SentinelAddrs)
	assert.Equal(t, "svc", opt.Username)
	assert.Equal(t, "secret", opt.Password)
	assert.Equal(t, 2, opt.DB)
	assert.Equal(t, 9, opt.PoolSize)
	assert.Equal(t, master, opt.MasterName)
	assert.Equal(t, "suser", opt.SentinelUsername)
	assert.Equal(t, "spass", opt.SentinelPassword)
}

// TestApplyRedisOptionsValueTypes is the flipped regression test for the
// former defect: applyRedisOptions handled value-typed connection options by
// mutating a local copy, silently dropping every update. The mutated struct is
// now written back (still as a value type, matching what the option carried).
func TestApplyRedisOptionsValueTypes(t *testing.T) {
	srv := NewServer()
	srv.redisConnOpt = asynq.RedisClientOpt{Addr: "127.0.0.1:6379"}
	srv.password = strPtr("pw")
	srv.db = int32Ptr(2)
	srv.applyRedisOptions()

	opt, ok := srv.redisConnOpt.(asynq.RedisClientOpt)
	require.True(t, ok, "the value branch must keep the value type")
	assert.Equal(t, "127.0.0.1:6379", opt.Addr)
	assert.Equal(t, "pw", opt.Password, "value-branch updates must persist")
	assert.Equal(t, 2, opt.DB)

	// The other value branches persist their updates the same way.
	srv.redisConnOpt = asynq.RedisClusterClientOpt{}
	srv.username = strPtr("svc")
	srv.applyRedisOptions()
	cluster, ok := srv.redisConnOpt.(asynq.RedisClusterClientOpt)
	require.True(t, ok)
	assert.Equal(t, "svc", cluster.Username)

	srv.redisConnOpt = asynq.RedisFailoverClientOpt{}
	srv.applyRedisOptions()
	assert.IsType(t, asynq.RedisFailoverClientOpt{}, srv.redisConnOpt)
}

// TestWithRedisURIThenOverrides is the end-to-end regression for the value
// branch: overrides such as WithRedisPassword must survive when the connection
// option came from WithRedisURI (whose parser returns value types).
func TestWithRedisURIThenOverrides(t *testing.T) {
	srv := NewServer(
		WithRedisURI("redis://127.0.0.1:6379/2"),
		WithRedisPassword("pw"),
		WithRedisUsername("svc"),
	)

	opt, ok := srv.redisConnOpt.(asynq.RedisClientOpt)
	require.True(t, ok, "the URI-parsed single-node option is a value type")
	assert.Equal(t, "pw", opt.Password)
	assert.Equal(t, "svc", opt.Username)
}

func strPtr(s string) *string               { return &s }
func int32Ptr(v int32) *int32               { return &v }
func durPtr(d time.Duration) *time.Duration { return &d }

// ---------------------------------------------------------------------------
// asynq.Config options
// ---------------------------------------------------------------------------

func TestAsynqConfigOptions(t *testing.T) {
	srv := NewServer(
		WithConcurrency(42),
		WithStrictPriority(true),
		WithShutdownTimeout(9*time.Second),
		WithHealthCheckInterval(time.Minute),
		WithDelayedTaskCheckInterval(2*time.Minute),
		WithGroupGracePeriod(3*time.Second),
		WithGroupMaxDelay(4*time.Second),
		WithGroupMaxSize(55),
		WithTaskCheckInterval(6*time.Second),
		WithJanitorInterval(7*time.Second),
		WithJanitorBatchSize(88),
		WithGracefullyShutdown(true),
	)

	cfg := srv.asynqConfig
	assert.Equal(t, 42, cfg.Concurrency)
	assert.True(t, cfg.StrictPriority)
	assert.Equal(t, 9*time.Second, cfg.ShutdownTimeout)
	assert.Equal(t, time.Minute, cfg.HealthCheckInterval)
	assert.Equal(t, 2*time.Minute, cfg.DelayedTaskCheckInterval)
	assert.Equal(t, 3*time.Second, cfg.GroupGracePeriod)
	assert.Equal(t, 4*time.Second, cfg.GroupMaxDelay)
	assert.Equal(t, 55, cfg.GroupMaxSize)
	assert.Equal(t, 6*time.Second, cfg.TaskCheckInterval)
	assert.Equal(t, 7*time.Second, cfg.JanitorInterval)
	assert.Equal(t, 88, cfg.JanitorBatchSize)
	assert.True(t, srv.gracefullyShutdown)
}

func TestWithQueues(t *testing.T) {
	t.Run("explicit queues", func(t *testing.T) {
		srv := NewServer(WithQueues(map[string]int32{"critical": 5, "low": 1}))
		assert.Equal(t, 5, srv.asynqConfig.Queues["critical"])
		assert.Equal(t, 1, srv.asynqConfig.Queues["low"])
	})

	t.Run("empty map creates the default queue", func(t *testing.T) {
		srv := NewServer(WithQueues(nil))
		assert.Equal(t, map[string]int{"default": defaultConcurrency}, srv.asynqConfig.Queues)

		concurrent := NewServer(WithConcurrency(8), WithQueues(map[string]int32{}))
		assert.Equal(t, map[string]int{"default": 8}, concurrent.asynqConfig.Queues,
			"an empty map must size the default queue by concurrency")
	})
}

func TestWithConfigReplacesConfig(t *testing.T) {
	custom := asynq.Config{Concurrency: 3, StrictPriority: true}
	srv := NewServer(WithConfig(custom))
	assert.Equal(t, 3, srv.asynqConfig.Concurrency)
	assert.True(t, srv.asynqConfig.StrictPriority)
}

func TestWithIsFailureCopiesPredicate(t *testing.T) {
	source := asynq.Config{IsFailure: func(err error) bool { return err != nil }}
	srv := NewServer(WithIsFailure(source))
	assert.NotNil(t, srv.asynqConfig.IsFailure)
	assert.False(t, srv.asynqConfig.IsFailure(nil))
	assert.True(t, srv.asynqConfig.IsFailure(assert.AnError))
}

func TestComponentToggles(t *testing.T) {
	srv := NewServer(
		WithServerEnabled(false),
		WithClientEnabled(false),
		WithSchedulerEnabled(false),
	)

	assert.False(t, srv.serverEnabled)
	assert.False(t, srv.clientEnabled)
	assert.False(t, srv.schedulerEnabled)
	assert.Nil(t, srv.server, "disabled consumer must not be created")
	assert.Nil(t, srv.client, "disabled producer must not be created")
	assert.Nil(t, srv.scheduler, "disabled scheduler must not be created")

	// Re-enabling later lazily creates the components on demand.
	srv.serverEnabled = true
	srv.clientEnabled = true
	srv.schedulerEnabled = true
	assert.NoError(t, srv.createAsynqServer())
	assert.NoError(t, srv.createAsynqClient())
	assert.NoError(t, srv.createAsynqScheduler())
	assert.NotNil(t, srv.server)
	assert.NotNil(t, srv.client)
	assert.NotNil(t, srv.scheduler)
}

func TestWithCodecEmptyKeepsDefault(t *testing.T) {
	srv := NewServer(WithCodec(""))
	assert.NotNil(t, srv.codec, "an empty codec name must keep the JSON default")
}

// ---------------------------------------------------------------------------
// Handler registration
// ---------------------------------------------------------------------------

func TestRegisterSubscriberAndTypeRegistry(t *testing.T) {
	srv := NewServer()

	require.NoError(t, srv.RegisterSubscriber("task:a", func(string, MessagePayload) error { return nil }, nil))
	require.NoError(t, srv.RegisterSubscriber("task:b", func(string, MessagePayload) error { return nil }, nil))

	assert.True(t, srv.TaskTypeExists("task:a"))
	assert.True(t, srv.TaskTypeExists("task:b"))
	assert.False(t, srv.TaskTypeExists("task:missing"))

	types := srv.GetRegisteredTaskTypes()
	assert.Len(t, types, 2)
	assert.Contains(t, types, "task:a")
	assert.Contains(t, types, "task:b")
}

func TestRegisterWithCtxAndGenerics(t *testing.T) {
	srv := NewServer()

	require.NoError(t, srv.RegisterSubscriberWithCtx("ctx:task",
		func(context.Context, string, MessagePayload) error { return nil }, nil))
	assert.True(t, srv.TaskTypeExists("ctx:task"))

	require.NoError(t, RegisterSubscriber(srv, "generic:task", func(string, *TaskPayload) error { return nil }))
	assert.True(t, srv.TaskTypeExists("generic:task"))

	require.NoError(t, RegisterSubscriberWithCtx(srv, "genericctx:task",
		func(context.Context, string, *TaskPayload) error { return nil }))
	assert.True(t, srv.TaskTypeExists("genericctx:task"))
}

func TestRegisterAfterStartRejected(t *testing.T) {
	srv := NewServer()
	srv.started.Store(true)

	assert.Error(t, srv.RegisterSubscriber("late", func(string, MessagePayload) error { return nil }, nil))
	assert.Error(t, srv.RegisterSubscriberWithCtx("late", nil, nil))
	assert.False(t, srv.TaskTypeExists("late"))
}

// ---------------------------------------------------------------------------
// Task enqueue and periodic task guards (no broker round trips)
// ---------------------------------------------------------------------------

func TestNewTaskGuards(t *testing.T) {
	srv := NewServer()

	assert.Error(t, srv.NewTask("", &TaskPayload{}), "empty typeName must be rejected")
}

func TestNewPeriodicTaskGuards(t *testing.T) {
	srv := NewServer()

	_, err := srv.NewPeriodicTask("", "task", &TaskPayload{})
	assert.Error(t, err, "empty cron spec must be rejected")

	_, err = srv.NewPeriodicTask("*/5 * * * *", "", &TaskPayload{})
	assert.Error(t, err, "empty typeName must be rejected")
}

func TestRemovePeriodicTaskGuards(t *testing.T) {
	srv := NewServer()

	assert.Error(t, srv.RemovePeriodicTask("never-registered"))
	assert.Error(t, srv.RemovePeriodicTaskByID(""))

	// Removing everything with an empty registry is a safe no-op.
	srv.RemoveAllPeriodicTask()
	assert.NoError(t, srv.Stop(t.Context()))
}

func TestQueryPeriodicTaskEntryID(t *testing.T) {
	srv := NewServer()
	assert.Empty(t, srv.QueryPeriodicTaskEntryID("missing"))

	srv.addPeriodicTaskEntryID("task", "entry-1")
	assert.Equal(t, "entry-1", srv.QueryPeriodicTaskEntryID("task"))
}

// ---------------------------------------------------------------------------
// types.go helper
// ---------------------------------------------------------------------------

func TestMessageHandlerDataCreate(t *testing.T) {
	withCreator := MessageHandlerData{Creator: func() any { return &TaskPayload{} }}
	assert.NotNil(t, withCreator.Create())

	withoutCreator := MessageHandlerData{}
	assert.Nil(t, withoutCreator.Create())
}

// ---------------------------------------------------------------------------
// Lifecycle guards
// ---------------------------------------------------------------------------

// TestStopLifecycleWithoutStart verifies the shutdown path without Redis.
// TestStopLifecycleWithoutStart verifies the shutdown path without Redis and
// pins that Stop is idempotent: only the first Stop closes the components,
// subsequent Stops are no-ops returning nil.
func TestStopLifecycleWithoutStart(t *testing.T) {
	srv := NewServer()
	assert.NoError(t, srv.Stop(t.Context()))
	assert.NoError(t, srv.Stop(t.Context()),
		"Stop must be idempotent: the second Stop is a no-op")
	assert.NoError(t, srv.Stop(t.Context()),
		"Stop must be idempotent: the third Stop is a no-op")
}

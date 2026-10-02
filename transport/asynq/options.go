package asynq

import (
	"crypto/tls"
	"time"

	"github.com/hibiken/asynq"

	"github.com/tx7do/go-wind-plugins/encoding"
)

const (
	RedisTypeSingle   = "single"
	RedisTypeCluster  = "cluster"
	RedisTypeSentinel = "sentinel"
)

const (
	defaultRedisAddress = "127.0.0.1:6379"
	defaultRedisDB      = 0
)

func newRedisClientOpt() asynq.RedisConnOpt {
	return &asynq.RedisClientOpt{
		Addr: defaultRedisAddress,
		DB:   defaultRedisDB,
	}
}

func newRedisClusterClientOpt() asynq.RedisConnOpt {
	return &asynq.RedisClusterClientOpt{
		Addrs: []string{defaultRedisAddress},
	}
}

func newRedisFailoverClientOpt() asynq.RedisConnOpt {
	return &asynq.RedisFailoverClientOpt{
		MasterName:    "mymaster",
		SentinelAddrs: []string{defaultRedisAddress},
		DB:            defaultRedisDB,
	}
}

// Option 是 Asynq 服务器的配置选项。
type Option func(*Server)

// WithRedisType 设置 Redis 部署模式（single、cluster、sentinel）。
func WithRedisType(redisType string) Option {
	return func(s *Server) {
		switch redisType {
		case RedisTypeSingle:
			s.redisConnOpt = newRedisClientOpt()
		case RedisTypeCluster:
			s.redisConnOpt = newRedisClusterClientOpt()
		case RedisTypeSentinel:
			s.redisConnOpt = newRedisFailoverClientOpt()
		default:
			panic("asynq: unknown redis type " + redisType)
		}
	}
}

// WithRedisConnOpt 直接设置 asynq.RedisConnOpt。
func WithRedisConnOpt(redisConnOpt asynq.RedisConnOpt) Option {
	return func(s *Server) {
		s.redisConnOpt = redisConnOpt
	}
}

// WithRedisURI 通过 URI 字符串设置 Redis 连接。
// 支持格式（与 asynq.ParseRedisURI 一致；集群模式请改用
// WithRedisType(RedisTypeCluster) 或 WithRedisConnOpt）：
//   - 单节点:      redis://[:password@]host:port[/db]
//   - 单节点 TLS:  rediss://[:password@]host:port[/db]
//   - Unix 套接字: redis-socket:///path/to/socket[?db=N]
//   - 哨兵:        redis-sentinel://[:password@]host1:port1,host2:port2?master=mastername
//
// URI 无法解析时 panic。
func WithRedisURI(uri string) Option {
	return func(s *Server) {
		redisConnOpt, err := asynq.ParseRedisURI(uri)
		if err != nil {
			panic("asynq: parse redis URI failed: " + err.Error())
		}
		s.redisConnOpt = redisConnOpt
	}
}

// WithRedisAddress 设置 Redis 地址（"host:port"）。
func WithRedisAddress(address string) Option {
	return func(s *Server) {
		s.addresses = []string{address}
	}
}

// WithRedisAddresses 设置 Redis 地址列表（集群或哨兵模式）。
func WithRedisAddresses(addresses []string) Option {
	return func(s *Server) {
		s.addresses = addresses
	}
}

// WithRedisUsername 设置 Redis 用户名。
func WithRedisUsername(username string) Option {
	return func(s *Server) {
		s.username = &username
	}
}

// WithRedisPassword 设置 Redis 密码。
func WithRedisPassword(password string) Option {
	return func(s *Server) {
		s.password = &password
	}
}

// WithRedisAuth 同时设置 Redis 用户名和密码。
func WithRedisAuth(username, password string) Option {
	return func(s *Server) {
		s.username = &username
		s.password = &password
	}
}

// WithRedisDB 设置 Redis 数据库编号。
func WithRedisDB(db int32) Option {
	return func(s *Server) {
		s.db = &db
	}
}

// WithRedisPoolSize 设置 Redis 连接池大小。
func WithRedisPoolSize(size int32) Option {
	return func(s *Server) {
		s.poolSize = &size
	}
}

// WithDialTimeout 设置 Redis 拨号超时。
func WithDialTimeout(timeout time.Duration) Option {
	return func(s *Server) {
		s.dialTimeout = &timeout
	}
}

// WithReadTimeout 设置 Redis 读超时。
func WithReadTimeout(timeout time.Duration) Option {
	return func(s *Server) {
		s.readTimeout = &timeout
	}
}

// WithWriteTimeout 设置 Redis 写超时。
func WithWriteTimeout(timeout time.Duration) Option {
	return func(s *Server) {
		s.writeTimeout = &timeout
	}
}

// WithTLSConfig 设置 TLS 配置。
func WithTLSConfig(tlsConfig *tls.Config) Option {
	return func(s *Server) {
		s.tlsConfig = tlsConfig
	}
}

// WithMaxRedirects 设置集群模式下的最大重定向次数。
func WithMaxRedirects(maxRedirects *int32) Option {
	return func(s *Server) {
		s.maxRedirects = maxRedirects
	}
}

// WithMasterName 设置哨兵模式下的 master 名称。
func WithMasterName(masterName *string) Option {
	return func(s *Server) {
		s.masterName = masterName
	}
}

// WithSentinelUsername 设置哨兵模式下的用户名。
func WithSentinelUsername(sentinelUsername *string) Option {
	return func(s *Server) {
		s.sentinelUsername = sentinelUsername
	}
}

// WithSentinelPassword 设置哨兵模式下的密码。
func WithSentinelPassword(sentinelPassword *string) Option {
	return func(s *Server) {
		s.sentinelPassword = sentinelPassword
	}
}

// WithSentinelAuth 同时设置哨兵模式的用户名和密码。
func WithSentinelAuth(username, password *string) Option {
	return func(s *Server) {
		s.sentinelUsername = username
		s.sentinelPassword = password
	}
}

// WithNetwork 设置网络类型（如 "tcp"）。
func WithNetwork(network *string) Option {
	return func(s *Server) {
		s.network = network
	}
}

// ---------------------------------------------------------------------------
// Asynq Server 配置选项
// ---------------------------------------------------------------------------

// WithConcurrency 设置并发工作协程数。
func WithConcurrency(concurrency int32) Option {
	return func(s *Server) {
		s.asynqConfig.Concurrency = int(concurrency)
	}
}

// WithConfig 直接设置完整的 asynq.Config。
func WithConfig(cfg asynq.Config) Option {
	return func(s *Server) {
		s.asynqConfig = cfg
	}
}

// WithQueues 设置队列及权重。
func WithQueues(queues map[string]int32) Option {
	return func(s *Server) {
		if s.asynqConfig.Queues == nil {
			s.asynqConfig.Queues = make(map[string]int)
		}

		if len(queues) == 0 {
			if len(s.asynqConfig.Queues) == 0 {
				weight := 1
				if s.asynqConfig.Concurrency > 0 {
					weight = s.asynqConfig.Concurrency
				}
				s.asynqConfig.Queues["default"] = weight
			}
			return
		}

		for k, v := range queues {
			s.asynqConfig.Queues[k] = int(v)
		}
	}
}

// WithRetryDelayFunc 设置重试延迟函数。
func WithRetryDelayFunc(fn asynq.RetryDelayFunc) Option {
	return func(s *Server) {
		s.asynqConfig.RetryDelayFunc = fn
	}
}

// WithStrictPriority 设置是否严格按优先级处理。
func WithStrictPriority(val bool) Option {
	return func(s *Server) {
		s.asynqConfig.StrictPriority = val
	}
}

// WithErrorHandler 设置错误处理器。
func WithErrorHandler(fn asynq.ErrorHandler) Option {
	return func(s *Server) {
		s.asynqConfig.ErrorHandler = fn
	}
}

// WithHealthCheckFunc 设置健康检查回调。
func WithHealthCheckFunc(fn func(error)) Option {
	return func(s *Server) {
		s.asynqConfig.HealthCheckFunc = fn
	}
}

// WithHealthCheckInterval 设置健康检查间隔。
func WithHealthCheckInterval(tm time.Duration) Option {
	return func(s *Server) {
		s.asynqConfig.HealthCheckInterval = tm
	}
}

// WithDelayedTaskCheckInterval 设置延迟任务检查间隔。
func WithDelayedTaskCheckInterval(tm time.Duration) Option {
	return func(s *Server) {
		s.asynqConfig.DelayedTaskCheckInterval = tm
	}
}

// WithGroupGracePeriod 设置组聚合的宽限期。
func WithGroupGracePeriod(tm time.Duration) Option {
	return func(s *Server) {
		s.asynqConfig.GroupGracePeriod = tm
	}
}

// WithGroupMaxDelay 设置组聚合的最大延迟。
func WithGroupMaxDelay(tm time.Duration) Option {
	return func(s *Server) {
		s.asynqConfig.GroupMaxDelay = tm
	}
}

// WithGroupMaxSize 设置组聚合的最大大小。
func WithGroupMaxSize(sz int32) Option {
	return func(s *Server) {
		s.asynqConfig.GroupMaxSize = int(sz)
	}
}

// WithMiddleware 添加 asynq 中间件。
func WithMiddleware(m ...asynq.MiddlewareFunc) Option {
	return func(s *Server) {
		s.mux.Use(m...)
	}
}

// WithShutdownTimeout 设置优雅关闭超时时间。
func WithShutdownTimeout(t time.Duration) Option {
	return func(s *Server) {
		s.asynqConfig.ShutdownTimeout = t
	}
}

// WithGracefullyShutdown 启用或禁用优雅关闭模式。
func WithGracefullyShutdown(enable bool) Option {
	return func(s *Server) {
		s.gracefullyShutdown = enable
	}
}

// WithTaskCheckInterval 设置任务检查间隔。
func WithTaskCheckInterval(t time.Duration) Option {
	return func(s *Server) {
		s.asynqConfig.TaskCheckInterval = t
	}
}

// WithJanitorInterval 设置清理器间隔。
func WithJanitorInterval(t time.Duration) Option {
	return func(s *Server) {
		s.asynqConfig.JanitorInterval = t
	}
}

// WithJanitorBatchSize 设置清理器批量大小。
func WithJanitorBatchSize(sz int32) Option {
	return func(s *Server) {
		s.asynqConfig.JanitorBatchSize = int(sz)
	}
}

// WithIsFailure 设置失败判断函数。
func WithIsFailure(c asynq.Config) Option {
	return func(s *Server) {
		s.asynqConfig.IsFailure = c.IsFailure
	}
}

// ---------------------------------------------------------------------------
// Scheduler 配置选项
// ---------------------------------------------------------------------------

// WithLocation 设置定时任务的时区。
func WithLocation(name string) Option {
	return func(s *Server) {
		loc, err := time.LoadLocation(name)
		if err != nil {
			panic("asynq: invalid timezone name: " + name + ", error: " + err.Error())
		}
		s.schedulerOpts.Location = loc
	}
}

// WithPreEnqueueFunc 设置任务入队前的回调。
func WithPreEnqueueFunc(fn func(task *asynq.Task, opts []asynq.Option)) Option {
	return func(s *Server) {
		s.schedulerOpts.PreEnqueueFunc = fn
	}
}

// WithPostEnqueueFunc 设置任务入队后的回调。
func WithPostEnqueueFunc(fn func(info *asynq.TaskInfo, err error)) Option {
	return func(s *Server) {
		s.schedulerOpts.PostEnqueueFunc = fn
	}
}

// ---------------------------------------------------------------------------
// 编解码器选项
// ---------------------------------------------------------------------------

// WithCodec 设置编解码器名称（如 "json"、"proto" 等）。
// codec 会通过 encoding.GetCodec(name) 获取。
func WithCodec(name string) Option {
	return func(s *Server) {
		if name != "" {
			s.codec = encoding.GetCodec(name)
		}
	}
}

// ---------------------------------------------------------------------------
// 组件开关
// ---------------------------------------------------------------------------

// WithServerEnabled 启用或禁用消费者服务器。
func WithServerEnabled(enabled bool) Option {
	return func(s *Server) {
		s.serverEnabled = enabled
	}
}

// WithClientEnabled 启用或禁用生产者客户端。
func WithClientEnabled(enabled bool) Option {
	return func(s *Server) {
		s.clientEnabled = enabled
	}
}

// WithSchedulerEnabled 启用或禁用定时调度器。
func WithSchedulerEnabled(enabled bool) Option {
	return func(s *Server) {
		s.schedulerEnabled = enabled
	}
}

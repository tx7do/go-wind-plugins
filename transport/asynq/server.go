// Package asynq provides an async task queue server based on [hibiken/asynq]
// that implements the [transport.Server] interface.
//
// Asynq uses Redis as its backend and supports:
//   - Task enqueue with retry, timeout, scheduling (ProcessIn/ProcessAt)
//   - Task handler registration with type-based dispatch
//   - Periodic (cron) tasks via a built-in scheduler
//   - Result inspection (wait for completion)
//   - Redis single-node, cluster, and sentinel modes
//
// The server wraps asynq.Server + asynq.Scheduler + asynq.Client into a single
// lifecycle managed by Start/Stop, making it compatible with [wind.App].
//
// Usage:
//
//	import asynqServer "github.com/tx7do/go-wind-plugins/transport/asynq"
//
//	srv := asynqServer.NewServer(
//	    asynqServer.WithRedisURI("redis://127.0.0.1:6379"),
//	)
//
//	// Register a typed handler.
//	asynqServer.RegisterSubscriber(srv, "email:send", func(taskType string, msg *EmailPayload) error {
//	    // send email...
//	    return nil
//	})
//
//	// Enqueue a task.
//	srv.NewTask("email:send", &EmailPayload{To: "user@example.com"})
//
//	if err := srv.Start(ctx); err != nil { ... }
package asynq

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hibiken/asynq"

	"github.com/tx7do/go-wind-plugins/encoding"
	"github.com/tx7do/go-wind/transport"
)

// KindAsynq 是 Asynq 传输类型标识。
const KindAsynq = "asynq"

// 确保 Server 实现了 wind transport.Server 接口。
var _ transport.Server = (*Server)(nil)

const (
	defaultConcurrency = 20
)

// ---------------------------------------------------------------------------
// Server
// ---------------------------------------------------------------------------

// Server 是基于 asynq 的异步任务队列服务器，实现 transport.Server 接口。
// 它集成了 asynq.Server（消费者）、asynq.Client（生产者）、asynq.Scheduler（定时任务）
// 和 asynq.Inspector（任务巡检）于一体。
type Server struct {
	mu sync.Mutex

	started atomic.Bool

	server    *asynq.Server
	client    *asynq.Client
	scheduler *asynq.Scheduler
	inspector *asynq.Inspector

	serverEnabled    bool
	clientEnabled    bool
	schedulerEnabled bool

	mux           *asynq.ServeMux
	asynqConfig   asynq.Config
	redisConnOpt  asynq.RedisConnOpt
	schedulerOpts *asynq.SchedulerOpts

	// Redis connection parameters
	addresses        []string
	username         *string
	password         *string
	db               *int32
	poolSize         *int32
	dialTimeout      *time.Duration
	readTimeout      *time.Duration
	writeTimeout     *time.Duration
	tlsConfig        *tls.Config
	maxRedirects     *int32
	masterName       *string
	sentinelUsername *string
	sentinelPassword *string
	network          *string

	gracefullyShutdown bool

	codec encoding.Codec

	entryIDs    map[string]string
	mtxEntryIDs sync.RWMutex

	typeNameMap sync.Map
}

// NewServer 创建一个 Asynq 服务器实例。
func NewServer(opts ...Option) *Server {
	srv := &Server{
		redisConnOpt: newRedisClientOpt(),
		asynqConfig: asynq.Config{
			Concurrency: defaultConcurrency,
		},
		schedulerOpts: &asynq.SchedulerOpts{},
		mux:           asynq.NewServeMux(),

		codec: encoding.GetCodec("json"),

		entryIDs:    make(map[string]string),
		mtxEntryIDs: sync.RWMutex{},

		serverEnabled:    true,
		clientEnabled:    true,
		schedulerEnabled: true,
	}

	srv.init(opts...)

	return srv
}

func (s *Server) init(opts ...Option) {
	for _, o := range opts {
		o(s)
	}

	s.applyRedisOptions()

	if err := s.createAsynqServer(); err != nil {
		log.Printf("[asynq] create server failed: %v", err)
	}
	if err := s.createAsynqClient(); err != nil {
		log.Printf("[asynq] create client failed: %v", err)
	}
	if err := s.createAsynqScheduler(); err != nil {
		log.Printf("[asynq] create scheduler failed: %v", err)
	}
}

// applyRedisOptions applies the stored Redis connection parameters to the
// redisConnOpt based on its concrete type.
func (s *Server) applyRedisOptions() {
	switch v := s.redisConnOpt.(type) {
	case *asynq.RedisClientOpt:
		s.updateRedisClientOpt(v)
	case asynq.RedisClientOpt:
		s.updateRedisClientOpt(&v)
	case *asynq.RedisClusterClientOpt:
		s.updateRedisClusterClientOpt(v)
	case asynq.RedisClusterClientOpt:
		s.updateRedisClusterClientOpt(&v)
	case *asynq.RedisFailoverClientOpt:
		s.updateRedisFailoverClientOpt(v)
	case asynq.RedisFailoverClientOpt:
		s.updateRedisFailoverClientOpt(&v)
	}
}

func (s *Server) updateRedisClientOpt(opt *asynq.RedisClientOpt) {
	if s.username != nil {
		opt.Username = *s.username
	}
	if s.password != nil {
		opt.Password = *s.password
	}
	if s.db != nil {
		opt.DB = int(*s.db)
	}
	if len(s.addresses) > 0 {
		opt.Addr = s.addresses[0]
	}
	if s.poolSize != nil {
		opt.PoolSize = int(*s.poolSize)
	}
	if s.dialTimeout != nil {
		opt.DialTimeout = *s.dialTimeout
	}
	if s.readTimeout != nil {
		opt.ReadTimeout = *s.readTimeout
	}
	if s.writeTimeout != nil {
		opt.WriteTimeout = *s.writeTimeout
	}
	if s.tlsConfig != nil {
		opt.TLSConfig = s.tlsConfig
	}
	if s.network != nil {
		opt.Network = *s.network
	}
}

func (s *Server) updateRedisClusterClientOpt(opt *asynq.RedisClusterClientOpt) {
	if s.username != nil {
		opt.Username = *s.username
	}
	if s.password != nil {
		opt.Password = *s.password
	}
	if len(s.addresses) > 0 {
		opt.Addrs = s.addresses
	}
	if s.dialTimeout != nil {
		opt.DialTimeout = *s.dialTimeout
	}
	if s.readTimeout != nil {
		opt.ReadTimeout = *s.readTimeout
	}
	if s.writeTimeout != nil {
		opt.WriteTimeout = *s.writeTimeout
	}
	if s.tlsConfig != nil {
		opt.TLSConfig = s.tlsConfig
	}
	if s.maxRedirects != nil {
		opt.MaxRedirects = int(*s.maxRedirects)
	}
}

func (s *Server) updateRedisFailoverClientOpt(opt *asynq.RedisFailoverClientOpt) {
	if s.username != nil {
		opt.Username = *s.username
	}
	if s.password != nil {
		opt.Password = *s.password
	}
	if s.db != nil {
		opt.DB = int(*s.db)
	}
	if len(s.addresses) > 0 {
		opt.SentinelAddrs = s.addresses
	}
	if s.poolSize != nil {
		opt.PoolSize = int(*s.poolSize)
	}
	if s.dialTimeout != nil {
		opt.DialTimeout = *s.dialTimeout
	}
	if s.readTimeout != nil {
		opt.ReadTimeout = *s.readTimeout
	}
	if s.writeTimeout != nil {
		opt.WriteTimeout = *s.writeTimeout
	}
	if s.tlsConfig != nil {
		opt.TLSConfig = s.tlsConfig
	}
	if s.masterName != nil {
		opt.MasterName = *s.masterName
	}
	if s.sentinelUsername != nil {
		opt.SentinelUsername = *s.sentinelUsername
	}
	if s.sentinelPassword != nil {
		opt.SentinelPassword = *s.sentinelPassword
	}
}

// ---------------------------------------------------------------------------
// transport.Server 实现
// ---------------------------------------------------------------------------

// Start 启动 Asynq 服务器（消费者、调度器），阻塞直到 ctx 被取消。
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started.Load() {
		return nil
	}

	if s.server == nil {
		if err := s.createAsynqServer(); err != nil {
			return err
		}
	}
	if s.scheduler == nil {
		if err := s.createAsynqScheduler(); err != nil {
			return err
		}
	}

	s.started.Store(true)

	// 启动调度器
	if s.schedulerEnabled && s.scheduler != nil {
		if err := s.scheduler.Start(); err != nil {
			s.started.Store(false)
			return fmt.Errorf("asynq scheduler start failed: %w", err)
		}
		log.Println("[asynq] scheduler started")
	}

	// 启动消费者服务器（非阻塞 goroutine）
	if s.serverEnabled && s.server != nil {
		go func() {
			if err := s.server.Run(s.mux); err != nil {
				if s.started.Load() {
					log.Printf("[asynq] server run error: %v", err)
				}
			}
		}()
		log.Println("[asynq] server started")
	}

	// 阻塞等待 ctx 取消
	<-ctx.Done()
	s.started.Store(false)

	return s.stopInternal(context.Background())
}

// Stop 优雅关闭服务器及其所有组件。
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started.Load() {
		return s.stopInternal(ctx)
	}

	s.started.Store(false)
	return s.stopInternal(ctx)
}

func (s *Server) stopInternal(ctx context.Context) error {
	var stopErr error

	// 1. 清理定时任务注册表
	s.removeAllPeriodicTaskInternal()

	// 2. 关闭调度器
	if s.scheduler != nil {
		s.scheduler.Shutdown()
		log.Println("[asynq] scheduler stopped")
	}

	// 3. 关闭消费者
	if s.server != nil {
		if s.gracefullyShutdown {
			done := make(chan struct{})
			go func() {
				s.server.Shutdown()
				close(done)
			}()
			select {
			case <-done:
				log.Println("[asynq] server gracefully stopped")
			case <-ctx.Done():
				log.Println("[asynq] graceful shutdown timeout, force stopping")
				s.server.Stop()
			}
		} else {
			s.server.Stop()
			log.Println("[asynq] server force stopped")
		}
	}

	// 4. 关闭客户端
	if s.client != nil {
		if err := s.client.Close(); err != nil {
			log.Printf("[asynq] client close failed: %v", err)
			stopErr = err
		}
	}

	// 5. 关闭巡检器
	if s.inspector != nil {
		if err := s.inspector.Close(); err != nil {
			log.Printf("[asynq] inspector close failed: %v", err)
			stopErr = err
		}
	}

	return stopErr
}

// Endpoint 返回服务器的访问端点描述。
func (s *Server) Endpoint() string {
	switch v := s.redisConnOpt.(type) {
	case *asynq.RedisClientOpt:
		return fmt.Sprintf("%s://%s", KindAsynq, v.Addr)
	case asynq.RedisClientOpt:
		return fmt.Sprintf("%s://%s", KindAsynq, v.Addr)
	case *asynq.RedisClusterClientOpt:
		return fmt.Sprintf("%s://cluster:%v", KindAsynq, v.Addrs)
	case asynq.RedisClusterClientOpt:
		return fmt.Sprintf("%s://cluster:%v", KindAsynq, v.Addrs)
	case *asynq.RedisFailoverClientOpt:
		return fmt.Sprintf("%s://%s@%v", KindAsynq, v.MasterName, v.SentinelAddrs)
	case asynq.RedisFailoverClientOpt:
		return fmt.Sprintf("%s://%s@%v", KindAsynq, v.MasterName, v.SentinelAddrs)
	default:
		return KindAsynq + "://unknown"
	}
}

// ---------------------------------------------------------------------------
// 任务处理器注册
// ---------------------------------------------------------------------------

// RegisterSubscriber 注册指定任务类型的处理器。
// creator 用于创建载荷实例以进行反序列化；如果为 nil 则直接传递原始字节。
func (s *Server) RegisterSubscriber(taskType string, handler MessageHandler, creator Creator) error {
	if s.started.Load() {
		return errors.New("cannot register handler, server already started")
	}

	s.mux.HandleFunc(taskType, func(ctx context.Context, task *asynq.Task) error {
		var payload MessagePayload

		if creator != nil {
			payload = creator()
			if err := s.codec.Unmarshal(task.Payload(), payload); err != nil {
				log.Printf("[asynq] unmarshal payload failed: %v", err)
				return err
			}
		} else {
			payload = task.Payload()
		}

		if err := handler(task.Type(), payload); err != nil {
			log.Printf("[asynq] handler error: %v", err)
			return err
		}
		return nil
	})

	s.typeNameMap.Store(taskType, true)
	return nil
}

// RegisterSubscriber 是 RegisterSubscriber 的泛型便捷方法。
// 它自动创建 T 类型的载荷实例并调用 handler。
func RegisterSubscriber[T any](srv *Server, taskType string, handler func(string, *T) error) error {
	return srv.RegisterSubscriber(taskType,
		func(taskType string, payload MessagePayload) error {
			switch t := payload.(type) {
			case *T:
				return handler(taskType, t)
			default:
				return fmt.Errorf("invalid payload struct type: %T", t)
			}
		},
		func() any {
			var t T
			return &t
		},
	)
}

// RegisterSubscriberWithCtx 注册带 context 的任务处理器。
func (s *Server) RegisterSubscriberWithCtx(
	taskType string,
	handler func(context.Context, string, MessagePayload) error,
	creator Creator,
) error {
	if s.started.Load() {
		return errors.New("cannot register handler, server already started")
	}

	s.mux.HandleFunc(taskType, func(ctx context.Context, task *asynq.Task) error {
		var payload MessagePayload

		if creator != nil {
			payload = creator()
			if err := s.codec.Unmarshal(task.Payload(), payload); err != nil {
				log.Printf("[asynq] unmarshal payload failed: %v", err)
				return err
			}
		} else {
			payload = task.Payload()
		}

		if err := handler(ctx, task.Type(), payload); err != nil {
			log.Printf("[asynq] handler error: %v", err)
			return err
		}
		return nil
	})

	s.typeNameMap.Store(taskType, true)
	return nil
}

// RegisterSubscriberWithCtx 是带 context 的泛型注册方法。
func RegisterSubscriberWithCtx[T any](srv *Server, taskType string,
	handler func(context.Context, string, *T) error,
) error {
	return srv.RegisterSubscriberWithCtx(taskType,
		func(ctx context.Context, taskType string, payload MessagePayload) error {
			switch t := payload.(type) {
			case *T:
				return handler(ctx, taskType, t)
			default:
				return fmt.Errorf("invalid payload struct type: %T", t)
			}
		},
		func() any {
			var t T
			return &t
		},
	)
}

// TaskTypeExists 检查任务类型是否已注册。
func (s *Server) TaskTypeExists(taskType string) bool {
	_, ok := s.typeNameMap.Load(taskType)
	return ok
}

// GetRegisteredTaskTypes 返回所有已注册的任务类型。
func (s *Server) GetRegisteredTaskTypes() []string {
	var types []string
	s.typeNameMap.Range(func(key, _ any) bool {
		if typeName, ok := key.(string); ok {
			types = append(types, typeName)
		}
		return true
	})
	return types
}

// ---------------------------------------------------------------------------
// 任务发布
// ---------------------------------------------------------------------------

// NewTask 将一个新任务入队。
func (s *Server) NewTask(typeName string, msg any, opts ...asynq.Option) error {
	if typeName == "" {
		return errors.New("typeName cannot be empty")
	}

	if s.client == nil {
		if err := s.createAsynqClient(); err != nil {
			return err
		}
	}

	payload, err := s.codec.Marshal(msg)
	if err != nil {
		return err
	}

	task := asynq.NewTask(typeName, payload, opts...)
	if task == nil {
		return errors.New("new task failed")
	}

	taskInfo, err := s.client.Enqueue(task, opts...)
	if err != nil {
		return fmt.Errorf("[%s] enqueue failed: %w", typeName, err)
	}

	log.Printf("[asynq] [%s] enqueued task: id=%s queue=%s", typeName, taskInfo.ID, taskInfo.Queue)
	return nil
}

// NewWaitResultTask 将一个新任务入队，并等待结果。
func (s *Server) NewWaitResultTask(typeName string, msg any, opts ...asynq.Option) error {
	if typeName == "" {
		return errors.New("typeName cannot be empty")
	}

	if s.client == nil {
		if err := s.createAsynqClient(); err != nil {
			return err
		}
	}

	payload, err := s.codec.Marshal(msg)
	if err != nil {
		return err
	}

	task := asynq.NewTask(typeName, payload, opts...)
	if task == nil {
		return errors.New("new task failed")
	}

	taskInfo, err := s.client.Enqueue(task, opts...)
	if err != nil {
		return fmt.Errorf("[%s] enqueue failed: %w", typeName, err)
	}

	if s.inspector == nil {
		if err = s.createAsynqInspector(); err != nil {
			return err
		}
	}

	if _, err = waitResult(s.inspector, taskInfo); err != nil {
		return fmt.Errorf("[%s] wait result failed: %w", typeName, err)
	}

	log.Printf("[asynq] [%s] task completed: id=%s queue=%s", typeName, taskInfo.ID, taskInfo.Queue)
	return nil
}

// NewPeriodicTask 注册一个定时（cron）任务。
// 返回 entryID，可用于后续移除。
func (s *Server) NewPeriodicTask(cronSpec, typeName string, msg any, opts ...asynq.Option) (string, error) {
	if cronSpec == "" {
		return "", errors.New("cronSpec cannot be empty")
	}
	if typeName == "" {
		return "", errors.New("typeName cannot be empty")
	}

	if s.scheduler == nil {
		if err := s.createAsynqScheduler(); err != nil {
			return "", err
		}
		if !s.schedulerEnabled {
			s.schedulerEnabled = true
		}
	}

	payload, err := s.codec.Marshal(msg)
	if err != nil {
		return "", err
	}

	task := asynq.NewTask(typeName, payload, opts...)
	if task == nil {
		return "", errors.New("new task failed")
	}

	entryID, err := s.scheduler.Register(cronSpec, task, opts...)
	if err != nil {
		return "", fmt.Errorf("[%s] register periodic task failed: %w", typeName, err)
	}

	s.addPeriodicTaskEntryID(typeName, entryID)
	log.Printf("[asynq] [%s] registered periodic entry: id=%q", typeName, entryID)

	return entryID, nil
}

// RemovePeriodicTask 按 typeName 移除定时任务。
// 多个定时任务共用同一 typeName 时，只能移除最近注册的一条；
// 其余的请使用 NewPeriodicTask 返回的 entryID 调用 RemovePeriodicTaskByID 移除。
func (s *Server) RemovePeriodicTask(taskId string) error {
	entryId := s.QueryPeriodicTaskEntryID(taskId)
	if entryId == "" {
		return fmt.Errorf("[%s] periodic task not found", taskId)
	}

	if err := s.unregisterPeriodicTask(entryId); err != nil {
		return err
	}

	s.removePeriodicTaskEntryID(taskId)
	return nil
}

// RemovePeriodicTaskByID 按 NewPeriodicTask 返回的 entryID 移除定时任务。
// 与按 typeName 查找的 RemovePeriodicTask 不同，它不受 typeName 撞键影响：
// 共用同一 typeName 的多条定时任务可以各自独立移除。
func (s *Server) RemovePeriodicTaskByID(entryId string) error {
	if entryId == "" {
		return errors.New("entryId cannot be empty")
	}

	if err := s.unregisterPeriodicTask(entryId); err != nil {
		log.Printf("[asynq] [%s] dequeue periodic task failed: %s", entryId, err.Error())
		return err
	}

	s.removePeriodicTaskEntryIDByEntryID(entryId)

	return nil
}

// RemoveAllPeriodicTask 移除所有定时任务。
func (s *Server) RemoveAllPeriodicTask() {
	s.removeAllPeriodicTaskInternal()
}

func (s *Server) removeAllPeriodicTaskInternal() {
	s.mtxEntryIDs.Lock()
	ids := s.entryIDs
	s.entryIDs = make(map[string]string)
	s.mtxEntryIDs.Unlock()

	for _, v := range ids {
		_ = s.unregisterPeriodicTask(v)
	}
}

func (s *Server) unregisterPeriodicTask(entryId string) error {
	if s.scheduler == nil {
		return nil
	}
	return s.scheduler.Unregister(entryId)
}

func (s *Server) addPeriodicTaskEntryID(taskId, entryId string) {
	s.mtxEntryIDs.Lock()
	defer s.mtxEntryIDs.Unlock()
	s.entryIDs[taskId] = entryId
}

func (s *Server) removePeriodicTaskEntryID(taskId string) {
	s.mtxEntryIDs.Lock()
	defer s.mtxEntryIDs.Unlock()
	delete(s.entryIDs, taskId)
}

// removePeriodicTaskEntryIDByEntryID 按 entryID 反查并移除其所在的映射项
// （entryIDs 以 typeName 为键，因此需要遍历值匹配）。
func (s *Server) removePeriodicTaskEntryIDByEntryID(entryId string) {
	s.mtxEntryIDs.Lock()
	defer s.mtxEntryIDs.Unlock()

	for taskId, id := range s.entryIDs {
		if id == entryId {
			delete(s.entryIDs, taskId)
		}
	}
}

// QueryPeriodicTaskEntryID 查询定时任务的 entry ID。
func (s *Server) QueryPeriodicTaskEntryID(taskId string) string {
	s.mtxEntryIDs.RLock()
	defer s.mtxEntryIDs.RUnlock()
	entryID, ok := s.entryIDs[taskId]
	if !ok {
		return ""
	}
	return entryID
}

// ---------------------------------------------------------------------------
// 内部方法
// ---------------------------------------------------------------------------

func (s *Server) createAsynqServer() error {
	if !s.serverEnabled || s.server != nil {
		return nil
	}
	s.server = asynq.NewServer(s.redisConnOpt, s.asynqConfig)
	if s.server == nil {
		return errors.New("create asynq server failed")
	}
	return nil
}

func (s *Server) createAsynqClient() error {
	if !s.clientEnabled || s.client != nil {
		return nil
	}
	s.client = asynq.NewClient(s.redisConnOpt)
	if s.client == nil {
		return errors.New("create asynq client failed")
	}
	return nil
}

func (s *Server) createAsynqScheduler() error {
	if !s.schedulerEnabled || s.scheduler != nil {
		return nil
	}
	s.scheduler = asynq.NewScheduler(s.redisConnOpt, s.schedulerOpts)
	if s.scheduler == nil {
		return errors.New("create asynq scheduler failed")
	}
	return nil
}

func (s *Server) createAsynqInspector() error {
	if s.inspector != nil {
		return nil
	}
	s.inspector = asynq.NewInspector(s.redisConnOpt)
	if s.inspector == nil {
		return errors.New("create asynq inspector failed")
	}
	return nil
}

// ---------------------------------------------------------------------------
// waitResult 等待任务完成
// ---------------------------------------------------------------------------

const (
	defaultWaitResultPollInterval = 1 * time.Second
	defaultWaitResultTimeout      = 5 * time.Minute
)

func waitResult(intor *asynq.Inspector, info *asynq.TaskInfo) (*asynq.TaskInfo, error) {
	deadline := time.Now().Add(defaultWaitResultTimeout)

	for {
		taskInfo, err := intor.GetTaskInfo(info.Queue, info.ID)
		if err != nil {
			return nil, err
		}

		switch taskInfo.State {
		case asynq.TaskStateCompleted:
			return taskInfo, nil
		case asynq.TaskStateArchived:
			return nil, fmt.Errorf("task state is %s", taskInfo.State.String())
		case asynq.TaskStateRetry:
			return nil, fmt.Errorf("task state is %s", taskInfo.State.String())
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("wait result timed out after %v (task state: %s)",
				defaultWaitResultTimeout, taskInfo.State.String())
		}

		time.Sleep(defaultWaitResultPollInterval)
	}
}

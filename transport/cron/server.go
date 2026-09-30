// Package cron provides a cron-based timer server that implements the
// [transport.Server] interface.
//
// It wraps [robfig/cron/v3] with support for second-level expressions
// (second minute hour day month weekday) and cron descriptors (e.g. "@every 5s",
// "@daily"). The server lifecycle is managed via the standard Start/Stop
// pattern, making it compatible with [wind.App].
//
// Usage:
//
//	import cronServer "github.com/tx7do/go-wind-plugins/transport/cron"
//
//	srv := cronServer.NewServer()
//
//	// Register jobs BEFORE Start (recommended) or after.
//	srv.NewTimerJob("*/10 * * * * *", func() {
//	    log.Println("runs every 10 seconds")
//	})
//
//	if err := srv.Start(ctx); err != nil { ... }
package cron

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"

	"github.com/robfig/cron/v3"

	"github.com/tx7do/go-wind/transport"
)

// KindCron 是 Cron 传输类型标识。
const KindCron = "cron"

// 确保 Server 实现了 wind transport.Server 接口。
var _ transport.Server = (*Server)(nil)

// ---------------------------------------------------------------------------
// Server
// ---------------------------------------------------------------------------

// Server 是基于 robfig/cron 的定时任务服务器，实现 transport.Server 接口。
// 它管理 cron 调度器的完整生命周期，支持动态添加/移除定时任务。
type Server struct {
	mu sync.Mutex

	started  atomic.Bool
	stopping atomic.Bool

	cronScheduler *cron.Cron

	entryIDs sync.Map // cron.EntryID -> spec string
	cronMu   sync.Mutex

	gracefullyShutdown bool
}

// NewServer 创建一个 Cron 服务器实例。
// 默认使用支持秒级表达式和描述符的解析器。
func NewServer(opts ...Option) *Server {
	cronParser := cron.NewParser(
		cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)

	srv := &Server{
		cronScheduler: cron.New(
			cron.WithParser(cronParser),
			cron.WithChain(
				cron.Recover(cron.PrintfLogger(log.New(log.Writer(), "[cron] ", log.LstdFlags))),
			),
		),
		entryIDs:           sync.Map{},
		gracefullyShutdown: true,
	}

	srv.init(opts...)

	return srv
}

func (s *Server) init(opts ...Option) {
	for _, o := range opts {
		o(s)
	}
}

// ---------------------------------------------------------------------------
// transport.Server 实现
// ---------------------------------------------------------------------------

// Start 启动 Cron 调度器，阻塞直到 ctx 被取消。
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started.Load() {
		return nil
	}

	LogInfof("scheduler starting...")
	s.cronScheduler.Start()
	s.started.Store(true)
	LogInfof("scheduler started, %d job(s) registered", s.GetJobCount())

	// 阻塞等待 ctx 取消
	<-ctx.Done()
	s.started.Store(false)

	return s.stopInternal(context.Background())
}

// Stop 优雅关闭 Cron 调度器。
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started.Load() && !s.stopping.Load() {
		return s.stopInternal(ctx)
	}

	s.started.Store(false)
	return s.stopInternal(ctx)
}

func (s *Server) stopInternal(ctx context.Context) error {
	if s.stopping.Load() {
		return nil
	}
	s.stopping.Store(true)
	defer func() {
		s.stopping.Store(false)
	}()

	LogInfof("scheduler stopping...")

	stopCh := make(chan struct{})
	go func() {
		// Stop 返回一个 context，等待正在运行的任务执行完成
		stopCtx := s.cronScheduler.Stop()
		<-stopCtx.Done()
		close(stopCh)
	}()

	select {
	case <-stopCh:
		LogInfof("all jobs stopped gracefully")
	case <-ctx.Done():
		LogWarnf("shutdown timeout, force stopped")
	}

	return nil
}

// Endpoint 返回服务器的访问端点描述。
func (s *Server) Endpoint() string {
	return fmt.Sprintf("%s://scheduler", KindCron)
}

// ---------------------------------------------------------------------------
// 定时任务管理
// ---------------------------------------------------------------------------

// NewTimerJob 添加一个 cron 定时任务。
// spec 支持：
//   - 秒级表达式: "*/10 * * * * *"（每 10 秒）
//   - 描述符: "@every 5s", "@daily", "@hourly" 等
//
// 可以在 Start 前或 Start 后调用。Start 前注册的任务会在调度器启动时立即调度。
func (s *Server) NewTimerJob(spec string, cmd func()) (cron.EntryID, error) {
	s.cronMu.Lock()
	defer s.cronMu.Unlock()

	entryID, err := s.cronScheduler.AddFunc(spec, cmd)
	if err != nil {
		return 0, fmt.Errorf("cron: add job failed (spec=%q): %w", spec, err)
	}

	s.entryIDs.Store(entryID, spec)
	LogInfof("job added: id=%d, spec=%s", entryID, spec)
	return entryID, nil
}

// RemoveTimerJob 根据任务 ID 移除单个定时任务。
func (s *Server) RemoveTimerJob(entryID cron.EntryID) {
	s.cronMu.Lock()
	defer s.cronMu.Unlock()

	s.cronScheduler.Remove(entryID)
	if spec, ok := s.entryIDs.LoadAndDelete(entryID); ok {
		LogInfof("job removed: id=%d, spec=%s", entryID, spec)
	}
}

// RemoveAllJobs 移除所有定时任务。
func (s *Server) RemoveAllJobs() {
	s.cronMu.Lock()
	defer s.cronMu.Unlock()

	count := 0
	s.entryIDs.Range(func(key, _ any) bool {
		if entryID, ok := key.(cron.EntryID); ok {
			s.cronScheduler.Remove(entryID)
			count++
		}
		return true
	})

	s.entryIDs = sync.Map{}
	LogInfof("all jobs removed, total: %d", count)
}

// GetJobCount 获取当前注册的任务数量。
func (s *Server) GetJobCount() int {
	count := 0
	s.entryIDs.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// GetEntries 返回当前调度器中的所有任务条目。
func (s *Server) GetEntries() []cron.Entry {
	return s.cronScheduler.Entries()
}

// Scheduler 返回底层的 cron.Cron 实例。
// 注意：直接操作底层调度器时需自行管理并发安全。
func (s *Server) Scheduler() *cron.Cron {
	return s.cronScheduler
}

// ---------------------------------------------------------------------------
// 错误定义
// ---------------------------------------------------------------------------

var ErrServerNotStarted = errors.New("cron server not started")

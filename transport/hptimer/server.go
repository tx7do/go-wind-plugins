// Package hptimer provides a high-precision timer server based on a min-heap
// scheduler that implements the [transport.Server] interface.
//
// It uses [gorhill/cronexpr] for cron expression parsing and a goroutine-driven
// event loop for sub-millisecond scheduling precision. Supports:
//   - One-shot tasks (At time)
//   - Interval-based repeating tasks (Interval > 0)
//   - Cron-based repeating tasks (Cron expression)
//   - Task cancellation via context
//   - Thread-safe concurrent Add/Remove
//
// The server lifecycle is managed via the standard Start/Stop pattern, making
// it compatible with [wind.App].
//
// Usage:
//
//	import hptimer "github.com/tx7do/go-wind-plugins/transport/hptimer"
//
//	srv := hptimer.NewServer()
//
//	srv.AddTask(hptimer.NewTimerTask("my-task",
//	    time.Now().Add(5*time.Second),
//	    hptimer.WithCallback(func(ctx context.Context) error {
//	        log.Println("task fired!")
//	        return nil
//	    }),
//	))
//
//	if err := srv.Start(ctx); err != nil { ... }
package hptimer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tx7do/go-wind/transport"
)

// KindHighPrecisionTimer 是高精度定时器传输类型标识。
const KindHighPrecisionTimer = "hptimer"

// 确保 Server 实现了 wind transport.Server 接口。
var _ transport.Server = (*Server)(nil)

// ---------------------------------------------------------------------------
// Server
// ---------------------------------------------------------------------------

// Server 是基于 HighPrecisionTimer 的定时任务服务器，实现 transport.Server 接口。
// 它管理高精度定时器引擎的完整生命周期。
type Server struct {
	mu sync.Mutex

	started  atomic.Bool
	stopping atomic.Bool

	gracefullyShutdown bool

	// 高精度定时器引擎
	hpTimer *HighPrecisionTimer

	timerObserver TimerObserver
}

// NewServer 创建一个高精度定时器服务器实例。
func NewServer(opts ...Option) *Server {
	srv := &Server{
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

// Start 启动高精度定时器引擎，阻塞直到 ctx 被取消。
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started.Load() {
		return nil
	}

	LogInfof("server starting...")

	// 创建并启动定时器引擎
	s.hpTimer = NewHighPrecisionTimer(s.timerObserver)
	s.hpTimer.Start()

	s.started.Store(true)
	LogInfof("server started successfully")

	// 阻塞等待 ctx 取消
	<-ctx.Done()
	s.started.Store(false)

	return s.stopInternal(context.Background())
}

// Stop 优雅关闭高精度定时器引擎。
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

	LogInfof("server stopping...")

	// 停止定时器引擎
	// WithGracefullyShutdown(false) 时跳过排空等待，立即停止
	if !s.gracefullyShutdown {
		if s.hpTimer != nil {
			s.hpTimer.Stop()
		}
		LogInfof("timer engine stopped (immediate)")
	} else {
		stopCtx, stopCancel := context.WithTimeout(ctx, 10*time.Second)
		defer stopCancel()

		wait := make(chan struct{})
		go func() {
			if s.hpTimer != nil {
				s.hpTimer.Stop()
			}
			close(wait)
		}()

		select {
		case <-wait:
			LogInfof("timer engine stopped gracefully")
		case <-stopCtx.Done():
			LogWarnf("shutdown timeout, force stopped")
		}
	}

	LogInfof("server stopped successfully")
	return nil
}

// Endpoint 返回服务器的访问端点描述。
func (s *Server) Endpoint() string {
	return fmt.Sprintf("%s://scheduler", KindHighPrecisionTimer)
}

// ---------------------------------------------------------------------------
// 任务管理
// ---------------------------------------------------------------------------

// AddTask 添加定时任务，返回任务 ID。
func (s *Server) AddTask(task *TimerTask) TimerTaskID {
	if !s.started.Load() {
		return ""
	}
	if s.hpTimer == nil {
		return ""
	}
	return s.hpTimer.AddTask(task)
}

// RemoveTask 删除定时任务。
func (s *Server) RemoveTask(taskID TimerTaskID) bool {
	if !s.started.Load() {
		return false
	}
	if s.hpTimer == nil {
		return false
	}
	return s.hpTimer.RemoveTask(taskID)
}

// ---------------------------------------------------------------------------
// 错误定义
// ---------------------------------------------------------------------------

var ErrServerNotStarted = errors.New("hptimer server not started")

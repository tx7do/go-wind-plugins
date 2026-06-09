// Package health defines health-checking abstractions for the go-wind framework.
//
// It provides a minimal, composable interface for liveness and readiness probes
// suitable for Kubernetes, load balancers, and orchestration platforms.
//
// Core concepts:
//   - [Status]   — the health state of a component (up / down / unknown).
//   - [Checker]  — a named health-check function returning a [Result].
//   - [Result]   — the outcome of a single check (status + message + details).
//   - [Health]   — an aggregator that runs multiple [Checker]s and produces a
//     combined [Result] with per-check breakdowns.
//
// The [Handler] in this package exposes the aggregated health status as a JSON
// HTTP endpoint suitable for Kubernetes liveness/readiness probes.
//
// Example:
//
//	h := health.New()
//	h.Register("redis", health.PingFunc(redisClient.Ping))
//	h.Register("tcp-db", health.TCP("db.internal:5432", 2*time.Second))
//
//	srv.GET("/healthz", health.NewHandler(h).ServeHTTP)
package health

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Status 表示组件的健康状态。
type Status int

const (
	// StatusUnknown 是默认零值，表示尚未检查或检查结果不确定。
	StatusUnknown Status = iota
	// StatusUp 表示组件正常运行。
	StatusUp
	// StatusDown 表示组件不可用。
	StatusDown
)

// String 返回状态的可读字符串。
func (s Status) String() string {
	switch s {
	case StatusUp:
		return "up"
	case StatusDown:
		return "down"
	default:
		return "unknown"
	}
}

// Result 是单次健康检查的结果。
type Result struct {
	// Status 是检查结果状态。
	Status Status `json:"status"`
	// Message 是可选的人类可读消息（例如错误描述）。
	Message string `json:"message,omitempty"`
	// Details 是可选的键值对详情，用于传递额外的检查信息。
	Details map[string]any `json:"details,omitempty"`
}

// Checker 是健康检查的统一接口。
//
// 实现可以是：
//   - [PingFunc] — 任意返回 error 的函数。
//   - [TCPChecker] — TCP 端口拨号检查。
//   - 用户自定义的任何实现了 Check 方法的类型。
type Checker interface {
	// Check 执行健康检查并返回结果。
	// 实现应尽快返回，通常使用 ctx 来控制超时。
	Check(ctx context.Context) Result
}

// PingFunc 是返回 error 的简单检查函数，适配为 Checker。
//
// 返回 nil error 表示 StatusUp，返回非 nil error 表示 StatusDown。
type PingFunc func(ctx context.Context) error

// Check 实现 Checker 接口。
func (f PingFunc) Check(ctx context.Context) Result {
	if f == nil {
		return Result{Status: StatusUnknown, Message: "checker is nil"}
	}
	if err := f(ctx); err != nil {
		return Result{Status: StatusDown, Message: err.Error()}
	}
	return Result{Status: StatusUp}
}

// Health 是健康检查聚合器，管理多个命名检查器并聚合结果。
//
// Health 是并发安全的。Register 可在运行时调用（动态添加检查器），
// 但通常在应用初始化阶段完成所有注册。
type Health struct {
	mu       sync.RWMutex
	checkers map[string]Checker
	timeout  time.Duration
}

// Option 配置 Health 聚合器。
type Option func(*Health)

// WithTimeout 设置每次健康检查的全局超时时间。
// 默认 5 秒。每次 Check 调用时，每个检查器都会被此超时约束。
func WithTimeout(d time.Duration) Option {
	return func(h *Health) { h.timeout = d }
}

// New 创建一个 Health 聚合器实例。
func New(opts ...Option) *Health {
	h := &Health{
		checkers: make(map[string]Checker),
		timeout:  5 * time.Second,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Register 注册或替换一个命名健康检查器。
// 如果 name 已存在，其检查器将被替换。
func (h *Health) Register(name string, c Checker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checkers[name] = c
}

// Deregister 移除一个命名健康检查器。
// 如果 name 不存在则为空操作。
func (h *Health) Deregister(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.checkers, name)
}

// Check 执行所有已注册的检查器，返回聚合结果。
//
// 聚合规则：
//   - 如果任一检查器返回 StatusDown，整体状态为 StatusDown。
//   - 否则如果任一检查器返回 StatusUnknown，整体状态为 StatusUnknown。
//   - 否则（全部 StatusUp）整体状态为 StatusUp。
//
// 每个检查器在独立的 goroutine 中执行，受 [Health.timeout] 约束。
func (h *Health) Check(ctx context.Context) Result {
	h.mu.RLock()
	names := make([]string, 0, len(h.checkers))
	snapshot := make(map[string]Checker, len(h.checkers))
	for name, c := range h.checkers {
		names = append(names, name)
		snapshot[name] = c
	}
	timeout := h.timeout
	h.mu.RUnlock()

	if len(names) == 0 {
		return Result{Status: StatusUp, Message: "no checkers registered"}
	}

	type checkResult struct {
		name string
		res  Result
	}

	// 为每次 Check 创建带超时的子 context。
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resultCh := make(chan checkResult, len(names))
	for _, name := range names {
		go func(n string, c Checker) {
			done := make(chan Result, 1)
			go func() {
				done <- c.Check(checkCtx)
			}()
			select {
			case res := <-done:
				resultCh <- checkResult{n, res}
			case <-checkCtx.Done():
				resultCh <- checkResult{n, Result{
					Status:  StatusDown,
					Message: fmt.Sprintf("checker %q timed out", n),
				}}
			}
		}(name, snapshot[name])
	}

	overall := StatusUp
	details := make(map[string]any, len(names))
	for i := 0; i < len(names); i++ {
		cr := <-resultCh
		// 使用 map 结构，方便 JSON 序列化
		details[cr.name] = map[string]any{
			"status":  cr.res.Status.String(),
			"message": cr.res.Message,
		}
		// 合并 cr.res.Details
		for k, v := range cr.res.Details {
			details[cr.name].(map[string]any)[k] = v
		}
		if cr.res.Status == StatusDown {
			overall = StatusDown
		} else if cr.res.Status == StatusUnknown && overall != StatusDown {
			overall = StatusUnknown
		}
	}

	return Result{
		Status:  overall,
		Details: details,
	}
}

// Names 返回所有已注册检查器的名称。
func (h *Health) Names() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	names := make([]string, 0, len(h.checkers))
	for name := range h.checkers {
		names = append(names, name)
	}
	return names
}

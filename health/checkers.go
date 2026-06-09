package health

import (
	"context"
	"fmt"
	"net"
	"time"
)

// --- TCP 检查器 ---

// TCPChecker 通过建立 TCP 连接来检查目标地址是否可达。
//
// 适用于检查数据库、缓存、消息队列等基于 TCP 的依赖。
//
// Example:
//
//	h.Register("db", health.TCP("db.internal:5432", 2*time.Second))
type TCPChecker struct {
	addr    string
	timeout time.Duration
}

// TCP 创建一个 TCP 拨号检查器。
//
// addr 是目标地址（host:port）。
// timeout 是单次拨号的超时时间。如果为 0，使用 Health 的全局超时。
func TCP(addr string, timeout time.Duration) *TCPChecker {
	return &TCPChecker{addr: addr, timeout: timeout}
}

// Check 实现 Checker 接口。
func (t *TCPChecker) Check(ctx context.Context) Result {
	d := net.Dialer{}
	timeout := t.timeout
	if timeout == 0 {
		timeout = 3 * time.Second
	}

	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := d.DialContext(dialCtx, "tcp", t.addr)
	if err != nil {
		return Result{
			Status:  StatusDown,
			Message: fmt.Sprintf("tcp dial %s: %v", t.addr, err),
		}
	}
	_ = conn.Close()
	return Result{Status: StatusUp}
}

// --- HTTP 检查器 ---

// HTTPChecker 通过发送 HTTP GET 请求来检查目标端点是否健康。
//
// 适用于检查下游微服务或外部 API 的可用性。
//
// Example:
//
//	h.Register("payment-svc", health.HTTP("http://payment-svc:8080/healthz", 2*time.Second))
type HTTPChecker struct {
	url     string
	timeout time.Duration
}

// HTTP 创建一个 HTTP 检查器。
//
// url 是目标 URL。
// timeout 是单次请求的超时时间。如果为 0，使用默认 3 秒。
func HTTP(url string, timeout time.Duration) *HTTPChecker {
	return &HTTPChecker{url: url, timeout: timeout}
}

// Check 实现 Checker 接口。
func (h *HTTPChecker) Check(ctx context.Context) Result {
	timeout := h.timeout
	if timeout == 0 {
		timeout = 3 * time.Second
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := newHTTPRequest(reqCtx, h.url)
	if err != nil {
		return Result{
			Status:  StatusDown,
			Message: fmt.Sprintf("http request build: %v", err),
		}
	}

	resp, err := httpClient().Do(req)
	if err != nil {
		return Result{
			Status:  StatusDown,
			Message: fmt.Sprintf("http request %s: %v", h.url, err),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return Result{Status: StatusUp}
	}

	return Result{
		Status:  StatusDown,
		Message: fmt.Sprintf("http %s returned status %d", h.url, resp.StatusCode),
	}
}

// --- 组合检查器 ---

// All 是一个组合检查器，要求所有子检查器都通过才算健康。
//
// 短路求值：遇到第一个失败立即返回，不继续检查剩余的子检查器。
//
// Example:
//
//	h.Register("storage", health.All(
//	    health.TCP("db:5432", 2*time.Second),
//	    health.PingFunc(redisClient.Ping),
//	))
type All struct {
	checkers []Checker
}

// AllCheckers 创建一个 "全部通过" 组合检查器。
func AllCheckers(checkers ...Checker) *All {
	return &All{checkers: checkers}
}

// Check 实现 Checker 接口。
func (a *All) Check(ctx context.Context) Result {
	for i, c := range a.checkers {
		r := c.Check(ctx)
		if r.Status == StatusDown {
			return Result{
				Status:  StatusDown,
				Message: fmt.Sprintf("checker[%d] failed: %s", i, r.Message),
			}
		}
	}
	return Result{Status: StatusUp}
}

// Any 是一个组合检查器，只要任一子检查器通过就算健康。
//
// 适用于高可用场景（如多副本只要求一个可达即可）。
//
// Example:
//
//	h.Register("redis-ha", health.Any(
//	    health.TCP("redis-1:6379", 2*time.Second),
//	    health.TCP("redis-2:6379", 2*time.Second),
//	))
type Any struct {
	checkers []Checker
}

// AnyCheckers 创建一个 "任一通过" 组合检查器。
func AnyCheckers(checkers ...Checker) *Any {
	return &Any{checkers: checkers}
}

// Check 实现 Checker 接口。
func (a *Any) Check(ctx context.Context) Result {
	var lastErr string
	for _, c := range a.checkers {
		r := c.Check(ctx)
		if r.Status == StatusUp {
			return Result{Status: StatusUp}
		}
		lastErr = r.Message
	}
	return Result{
		Status:  StatusDown,
		Message: fmt.Sprintf("all checkers failed, last error: %s", lastErr),
	}
}

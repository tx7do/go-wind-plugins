// Package benchmark 对 transport/http 的各 driver 做端到端性能基准测试。
//
// 设计说明：
//
// fiber 基于 fasthttp，与 net/http 不兼容；各 driver 的内部 handler 字段均为私有。
// 因此采用端到端 loopback 黑盒测试：每个 driver 绑定随机端口 (:0)，
// 用统一的 keep-alive http.Client 通过真实 TCP 连接发起请求。
//
// 网络/HTTP 客户端开销对所有 driver 是相同常量，因此相对差异即反映了
// 各 driver 路由层 + 适配层（如 fiber 的 net/http 适配）的真实开销。
package benchmark

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/driver/chi"
	"github.com/tx7do/go-wind-plugins/transport/http/driver/fiber"
	"github.com/tx7do/go-wind-plugins/transport/http/driver/gin"
	"github.com/tx7do/go-wind-plugins/transport/http/driver/std"
)

// driverFactories 列出所有参与基准测试的 driver 工厂，按名称字典序排列以保证输出稳定。
var driverFactories = map[string]func() httpPlugin.Driver{
	"chi":   chi.NewDriver,
	"fiber": func() httpPlugin.Driver { return fiber.NewDriver(fiber.WithDisableStartupMessage()) },
	"gin":   gin.NewDriver,
	"std":   std.NewDriver,
}

// driverNames 是遍历时使用的稳定顺序。
var driverNames = []string{"std", "chi", "gin", "fiber"}

// JSON 响应固定负载（避免每次构造，确保各 driver 测的是序列化/写入开销而非构造开销）。
const userJSON = `{"id":1,"name":"Alice","email":"alice@example.com","active":true}`

// echo 测试用的请求体。
var echoBody = bytes.Repeat([]byte(`{"data":"benchmark payload for echo"}`), 1)

// registerRoutes 向 driver 注册统一的路由表，确保各 driver 跑同样的工作负载。
// 返回的 cleanup 在 driver 停止后调用（目前无资源需释放，预留扩展）。
func registerRoutes(d httpPlugin.Driver) {
	d.Handle(http.MethodGet, "/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	d.Handle(http.MethodGet, "/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(userJSON))
	})

	d.Handle(http.MethodPost, "/echo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// 显式读取并回写，测请求体解析 + 响应体写入的完整路径。
		_, _ = io.Copy(w, r.Body)
	})

	d.Handle(http.MethodGet, "/api/v1/users/123/posts/456/comments", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("comment"))
	})
}

// newClient 创建一个连接复用的 keep-alive HTTP 客户端。
// MaxIdleConnsPerHost 设大以避免连接复用不充分带来的抖动。
func newClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        1000,
			MaxIdleConnsPerHost: 1000,
			IdleConnTimeout:     30 * time.Second,
			DialContext: (&net.Dialer{
				Timeout:   2 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
		},
	}
}

// benchEnv 为单个 driver 启动一个绑定随机端口的服务器实例，
// 注册统一路由，并返回 baseURL 与 cleanup。
//
// b.Fatal 会在启动失败时终止整个 benchmark（属预期：环境问题不应静默）。
func benchEnv(b *testing.B, driverName string) (baseURL string, client *http.Client) {
	b.Helper()

	factory, ok := driverFactories[driverName]
	if !ok {
		b.Fatalf("unknown driver: %s", driverName)
	}

	d := factory()
	registerRoutes(d)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("listen: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errChan := make(chan error, 1)
	go func() {
		errChan <- d.Start(ctx, ln)
	}()

	// 获取实际端口并轮询就绪，避免冷启动污染首轮计时。
	addr := ln.Addr().String()
	baseURL = "http://" + addr
	if err := waitForReady(baseURL, 3*time.Second); err != nil {
		cancel()
		_ = d.Stop(context.Background())
		b.Fatalf("driver %s not ready: %v", driverName, err)
	}

	client = newClient()

	b.Cleanup(func() {
		cancel()
		// Start 在 ctx 取消时已 Shutdown，Stop 兜底确保关闭。
		_ = d.Stop(context.Background())
		// 若 Start 返回了非空错误（非正常关闭），让 benchmark 感知。
		select {
		case err := <-errChan:
			if err != nil {
				b.Logf("driver %s Start returned error on shutdown: %v", driverName, err)
			}
		case <-time.After(2 * time.Second):
			b.Logf("driver %s Start did not return within 2s on shutdown", driverName)
		}
	})

	return baseURL, client
}

// waitForReady 轮询 GET / 直到收到 200 或超时。
func waitForReady(baseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	probe := newClient()
	for {
		resp, err := probe.Get(baseURL + "/")
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("probe never succeeded: %w", err)
			}
			return fmt.Errorf("probe timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// drain 安全读取并丢弃响应体后关闭，确保连接可复用。
// io.Discard 实现了 io.ReaderFrom，io.Copy 会走零拷贝路径，不会分配 32KB buffer。
func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

// requestBody 是一个带 WriteTo 的请求体 reader，用于客户端发送请求体。
//
// 为什么要单独定义：net/http 客户端发送请求体时，内部用 io.Copy 把 body 写入连接。
// 若 body 的具体类型不实现 io.WriterTo（如 io.NopCloser 包裹的 bytes.Reader），
// io.copyBuffer 会每次分配一个 32KB 中间 buffer，严重污染基准测试结果。
//
// 本类型透传内部 bytes.Reader 的 WriteTo，使客户端走零拷贝路径；
// 同时实现 Close 以满足 io.ReadCloser，并通过 sync.Pool 复用。
type requestBody struct {
	r      *bytes.Reader
	closed bool
}

var requestBodyPool = sync.Pool{
	New: func() any { return &requestBody{r: &bytes.Reader{}} },
}

func newRequestBody(b []byte) *requestBody {
	rb := requestBodyPool.Get().(*requestBody)
	rb.r.Reset(b)
	rb.closed = false
	return rb
}

func (rb *requestBody) Read(p []byte) (int, error) { return rb.r.Read(p) }

// WriteTo 透传 bytes.Reader.WriteTo，避免客户端 io.copyBuffer 的 32KB 分配。
func (rb *requestBody) WriteTo(w io.Writer) (int64, error) { return rb.r.WriteTo(w) }

func (rb *requestBody) Close() error {
	if rb.closed {
		return nil
	}
	rb.closed = true
	rb.r.Reset(nil)
	requestBodyPool.Put(rb)
	return nil
}

// runDriverParallel 跑一个 driver 的 N 次串行请求。
func runRequests(b *testing.B, client *http.Client, url, method string, body io.Reader, wantStatus int) {
	b.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		b.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := client.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		if resp.StatusCode != wantStatus {
			b.Fatalf("status = %d, want %d", resp.StatusCode, wantStatus)
		}
		drain(resp)
	}
	b.StopTimer()
}

// perDriver 遍历所有 driver 依次执行 bench。
func perDriver(b *testing.B, fn func(b *testing.B, baseURL string, client *http.Client)) {
	for _, name := range driverNames {
		name := name
		b.Run(name, func(b *testing.B) {
			baseURL, client := benchEnv(b, name)
			b.ResetTimer()
			fn(b, baseURL, client)
		})
	}
}

// --- Benchmark 场景 ---

// BenchmarkDriver_RootGET 基础路由 GET / —— 测路由匹配 + 短响应基线开销。
func BenchmarkDriver_RootGET(b *testing.B) {
	perDriver(b, func(b *testing.B, baseURL string, client *http.Client) {
		runRequests(b, client, baseURL+"/", http.MethodGet, nil, http.StatusOK)
	})
}

// BenchmarkDriver_JSONResponse GET /user —— 测 JSON 响应写入开销。
func BenchmarkDriver_JSONResponse(b *testing.B) {
	perDriver(b, func(b *testing.B, baseURL string, client *http.Client) {
		runRequests(b, client, baseURL+"/user", http.MethodGet, nil, http.StatusOK)
	})
}

// BenchmarkDriver_JSONEcho POST /echo —— 读取请求体并回写，测请求解析 + 响应写入。
func BenchmarkDriver_JSONEcho(b *testing.B) {
	perDriver(b, func(b *testing.B, baseURL string, client *http.Client) {
		// 每次迭代用新的 body reader（Do 会消耗它）。
		req, _ := http.NewRequest(http.MethodPost, baseURL+"/echo", nil)
		req.Header.Set("Content-Type", "application/json")

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// 用带 WriteTo 的请求体，避免客户端 io.Copy 分配 32KB 中间 buffer。
			req.Body = newRequestBody(echoBody)
			req.GetBody = nil
			req.ContentLength = int64(len(echoBody))
			resp, err := client.Do(req)
			if err != nil {
				b.Fatal(err)
			}
			if resp.StatusCode != http.StatusOK {
				b.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
			}
			drain(resp)
		}
		b.StopTimer()
	})
}

// BenchmarkDriver_LongPath 长路径匹配开销。
func BenchmarkDriver_LongPath(b *testing.B) {
	perDriver(b, func(b *testing.B, baseURL string, client *http.Client) {
		runRequests(b, client, baseURL+"/api/v1/users/123/posts/456/comments",
			http.MethodGet, nil, http.StatusOK)
	})
}

// BenchmarkDriver_MethodNotAllowed 方法/路径不匹配的派发开销。
//
// 对已注册路径 /echo（仅 POST）用 DELETE 请求，各 driver 的精确状态码不一致：
//   - std driver 内部显式校验 method，返回 405 Method Not Allowed
//   - gin/chi/fiber 路由树按 path 命中但 method 不符，部分返回 404、部分返回 405
//
// 因此本场景接受任意 4xx（"非正常派发"路径），仅测框架处理未命中分支的开销。
func BenchmarkDriver_MethodNotAllowed(b *testing.B) {
	perDriver(b, func(b *testing.B, baseURL string, client *http.Client) {
		req, _ := http.NewRequest(http.MethodDelete, baseURL+"/echo", nil)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			resp, err := client.Do(req)
			if err != nil {
				b.Fatal(err)
			}
			if resp.StatusCode < 400 || resp.StatusCode >= 500 {
				b.Fatalf("status = %d, want 4xx", resp.StatusCode)
			}
			drain(resp)
		}
		b.StopTimer()
	})
}

// BenchmarkDriver_Parallel 并发 GET / —— 测高并发下 driver 的表现。
func BenchmarkDriver_Parallel(b *testing.B) {
	for _, name := range driverNames {
		name := name
		b.Run(name, func(b *testing.B) {
			baseURL, client := benchEnv(b, name)
			req, _ := http.NewRequest(http.MethodGet, baseURL+"/", nil)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					resp, err := client.Do(req)
					if err != nil {
						b.Fatal(err)
					}
					if resp.StatusCode != http.StatusOK {
						b.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
					}
					drain(resp)
				}
			})
			b.StopTimer()
		})
	}
}

package gin

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// startDriver 启动一个绑定随机端口的 gin driver，返回 baseURL 与 cleanup。
func startDriver(t *testing.T, d *Driver) (baseURL string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	errChan := make(chan error, 1)
	go func() { errChan <- d.Start(ctx, ln) }()

	baseURL = "http://" + ln.Addr().String()
	// 就绪探测
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := client.Get(baseURL + "/health")
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		// /health 未注册会 404，但只要连接成功即说明已就绪
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("driver not ready: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Cleanup(func() {
		cancel()
		_ = d.Stop(context.Background())
	})
	return baseURL
}

// TestHandleGin_ReceivesRealContext 验证 HandleGin 注册的 handler
// 拿到的是真正的 *gin.Context，能用 gin 全部能力（路径参数、JSON）。
func TestHandleGin_ReceivesRealContext(t *testing.T) {
	d := New()
	// 用 gin 原生能力：路径参数 :id + c.JSON
	d.HandleGin(http.MethodGet, "/users/:id", func(c *gin.Context) {
		id := c.Param("id") // gin 特有的路径参数提取
		c.JSON(http.StatusOK, gin.H{"id": id, "name": "alice"})
	})
	// 注册一个 health 路由用于就绪探测
	d.HandleGin(http.MethodGet, "/health", func(c *gin.Context) { c.Status(http.StatusOK) })

	baseURL := startDriver(t, d)

	resp, err := http.Get(baseURL + "/users/42")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	// 验证 gin 的路径参数提取生效
	if !strings.Contains(string(body), `"id":"42"`) {
		t.Errorf(`body should contain id:"42", got %s`, body)
	}
	if !strings.Contains(string(body), `"name":"alice"`) {
		t.Errorf(`body should contain name:"alice", got %s`, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("gin c.JSON should set json content-type, got %q", ct)
	}
}

// TestHandle_StandardInterface 验证 Handle（标准接口）仍正常工作，
// 且与 HandleGin 能在同一 driver 共存。
func TestHandle_StandardInterface(t *testing.T) {
	d := New()
	// 标准 http.HandlerFunc（走 Driver 接口）
	d.Handle(http.MethodGet, "/std", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("from-std"))
	})
	// gin 原生 handler（走 HandleGin）
	d.HandleGin(http.MethodGet, "/gin", func(c *gin.Context) {
		c.String(http.StatusOK, "from-gin")
	})
	d.HandleGin(http.MethodGet, "/health", func(c *gin.Context) { c.Status(http.StatusOK) })

	baseURL := startDriver(t, d)

	// 标准接口路由
	resp, err := http.Get(baseURL + "/std")
	if err != nil {
		t.Fatalf("Get /std: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != "from-std" {
		t.Errorf("/std: status=%d body=%q", resp.StatusCode, body)
	}

	// gin 原生路由
	resp2, err := http.Get(baseURL + "/gin")
	if err != nil {
		t.Fatalf("Get /gin: %v", err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 200 || string(body2) != "from-gin" {
		t.Errorf("/gin: status=%d body=%q", resp2.StatusCode, body2)
	}
}

// TestNewDriver_ReturnsInterface 验证 NewDriver 返回接口（向后兼容），
// New 返回具体类型（可访问 HandleGin）。
func TestNewDriver_ReturnsInterface(t *testing.T) {
	ifaceDriver := NewDriver()
	// NewDriver 返回的是接口类型，但底层是 *Driver
	gd, ok := ifaceDriver.(*Driver)
	if !ok {
		t.Fatalf("NewDriver() should return *Driver, got %T", ifaceDriver)
	}
	if gd.engine == nil {
		t.Error("underlying engine should be initialized")
	}

	concreteDriver := New()
	if concreteDriver.engine == nil {
		t.Error("New() should return initialized *Driver")
	}
	// 两者底层类型一致，都能注册路由
	concreteDriver.HandleGin(http.MethodGet, "/x", func(c *gin.Context) {})
}

// TestHandleGin_UsesGinMiddleware 验证 HandleGin 路由能用 gin 原生中间件
// （通过 Engine() 注册），证明高级场景可用。
func TestHandleGin_UsesGinMiddleware(t *testing.T) {
	d := New()
	// 通过 Engine 注册 gin 原生中间件（设置响应头）
	d.Engine().Use(func(c *gin.Context) {
		c.Header("X-Via-Gin-MW", "yes")
		c.Next()
	})
	d.HandleGin(http.MethodGet, "/health", func(c *gin.Context) { c.Status(http.StatusOK) })
	d.HandleGin(http.MethodGet, "/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	baseURL := startDriver(t, d)

	resp, err := http.Get(baseURL + "/x")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("X-Via-Gin-MW"); got != "yes" {
		t.Errorf("gin middleware should set header, got %q", got)
	}
}

// 编译期保证 *Driver 实现 http.Handler 不需要，但确认它满足 httptest 直接用
// （通过 engine）。这个测试用 httptest 验证 handler 逻辑本身，不发真实网络。
func TestHandleGin_UnitWithTestEngine(t *testing.T) {
	d := New()
	d.HandleGin(http.MethodGet, "/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	// 直接用 gin engine 经 httptest 测，避免端口/网络
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	d.Engine().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if w.Body.String() != "pong" {
		t.Errorf("body = %q, want pong", w.Body.String())
	}
}

// --- options 注入方案测试 ---

// TestNew_WithOptions 验证用 WithRoute + WithMiddleware 在构造时注入
// 框架原生路由与中间件（纯 options 风格复用 gin 代码）。
func TestNew_WithOptions(t *testing.T) {
	mwCalled := false
	d := New(
		// gin 原生中间件（作用于下面的原生路由）
		WithMiddleware(func(c *gin.Context) {
			mwCalled = true
			c.Header("X-Via-Gin-MW", "yes")
			c.Next()
		}),
		// gin 原生路由（复用存量 gin handler 风格）
		WithRoute(http.MethodGet, "/users/:id", func(c *gin.Context) {
			id := c.Param("id")
			c.JSON(http.StatusOK, gin.H{"id": id})
		}),
		WithRoute(http.MethodGet, "/health", func(c *gin.Context) {
			c.Status(http.StatusOK)
		}),
	)

	baseURL := startDriver(t, d)

	resp, err := http.Get(baseURL + "/users/42")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), `"id":"42"`) {
		t.Errorf("WithRoute should bind path param, got %s", body)
	}
	if got := resp.Header.Get("X-Via-Gin-MW"); got != "yes" {
		t.Errorf("WithMiddleware should apply, got header %q", got)
	}
	if !mwCalled {
		t.Error("gin middleware should be called")
	}
}

// TestOptions_MixedWithStandardRoute 验证 options 注入的原生路由
// 与 Handle 注册的标准路由能在同一 driver 共存。
func TestOptions_MixedWithStandardRoute(t *testing.T) {
	d := New(
		WithRoute(http.MethodGet, "/gin-route", func(c *gin.Context) {
			c.String(http.StatusOK, "from-gin-option")
		}),
	)
	// 标准 http.HandlerFunc（走 Driver 接口）
	d.Handle(http.MethodGet, "/std-route", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("from-std"))
	})
	d.HandleGin(http.MethodGet, "/health", func(c *gin.Context) { c.Status(http.StatusOK) })

	baseURL := startDriver(t, d)

	resp1, err := http.Get(baseURL + "/gin-route")
	if err != nil {
		t.Fatalf("Get /gin-route: %v", err)
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	if string(body1) != "from-gin-option" {
		t.Errorf("/gin-route: %q", body1)
	}

	resp2, err := http.Get(baseURL + "/std-route")
	if err != nil {
		t.Fatalf("Get /std-route: %v", err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if string(body2) != "from-std" {
		t.Errorf("/std-route: %q", body2)
	}
}

// TestWithRoute_ApplyOrder 验证 WithMiddleware 在 WithRoute 之前注册时，
// 中间件仍对路由生效（gin Use 注册的中间件对后续 Handle 注册的路由都生效）。
func TestWithRoute_ApplyOrder(t *testing.T) {
	d := New(
		WithMiddleware(func(c *gin.Context) {
			c.Header("X-Order-Check", "applied")
			c.Next()
		}),
		WithRoute(http.MethodGet, "/health", func(c *gin.Context) { c.Status(http.StatusOK) }),
		WithRoute(http.MethodGet, "/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") }),
	)

	baseURL := startDriver(t, d)

	resp, err := http.Get(baseURL + "/x")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("X-Order-Check"); got != "applied" {
		t.Errorf("middleware registered before route should still apply, got %q", got)
	}
}


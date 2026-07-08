package fiber

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

// startDriver 启动一个绑定随机端口的 fiber driver，返回 baseURL 与 cleanup。
// 关闭启动 banner 避免污染测试输出。
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
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := client.Get(baseURL + "/health")
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
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

// newTestDriver 创建一个关闭 banner 的测试 driver。
func newTestDriver() *Driver {
	return New(WithDisableStartupMessage())
}

// TestHandleFiber_ReceivesRealContext 验证 HandleFiber 注册的 handler
// 拿到原生 *fiber.Ctx，能用 fiber 全部能力（路径参数、JSON）。
func TestHandleFiber_ReceivesRealContext(t *testing.T) {
	d := newTestDriver()
	d.HandleFiber(http.MethodGet, "/users/:id", func(c *fiber.Ctx) error {
		id := c.Params("id") // fiber 原生路径参数
		return c.JSON(fiber.Map{"id": id, "name": "alice"})
	})
	d.HandleFiber(http.MethodGet, "/health", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

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
	if !strings.Contains(string(body), `"id":"42"`) {
		t.Errorf(`body should contain id:"42", got %s`, body)
	}
	if !strings.Contains(string(body), `"name":"alice"`) {
		t.Errorf(`body should contain name:"alice", got %s`, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("fiber c.JSON should set json content-type, got %q", ct)
	}
}

// TestHandle_StandardInterface 验证 Handle（标准接口）仍正常工作，
// 且与 HandleFiber 能在同一 driver 共存。
func TestHandle_StandardInterface(t *testing.T) {
	d := newTestDriver()
	// 标准 http.HandlerFunc（走 Driver 接口 + 适配层）
	d.Handle(http.MethodGet, "/std", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("from-std"))
	})
	// fiber 原生 handler（走 HandleFiber，不经适配层）
	d.HandleFiber(http.MethodGet, "/gin", func(c *fiber.Ctx) error {
		return c.SendString("from-fiber")
	})
	d.HandleFiber(http.MethodGet, "/health", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	baseURL := startDriver(t, d)

	resp, err := http.Get(baseURL + "/std")
	if err != nil {
		t.Fatalf("Get /std: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != "from-std" {
		t.Errorf("/std: status=%d body=%q", resp.StatusCode, body)
	}

	resp2, err := http.Get(baseURL + "/gin")
	if err != nil {
		t.Fatalf("Get /gin: %v", err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 200 || string(body2) != "from-fiber" {
		t.Errorf("/gin: status=%d body=%q", resp2.StatusCode, body2)
	}
}

// TestNewDriver_ReturnsInterface 验证 NewDriver 返回接口（向后兼容），
// New 返回具体类型（可访问 HandleFiber）。
func TestNewDriver_ReturnsInterface(t *testing.T) {
	ifaceDriver := NewDriver(WithDisableStartupMessage())
	fd, ok := ifaceDriver.(*Driver)
	if !ok {
		t.Fatalf("NewDriver() should return *Driver, got %T", ifaceDriver)
	}
	if fd.app == nil {
		t.Error("underlying app should be initialized")
	}

	concreteDriver := newTestDriver()
	if concreteDriver.app == nil {
		t.Error("New() should return initialized *Driver")
	}
	concreteDriver.HandleFiber(http.MethodGet, "/x", func(c *fiber.Ctx) error {
		return nil
	})
}

// TestHandleFiber_UsesFiberMiddleware 验证 HandleFiber 路由能用 fiber 原生中间件
// （通过 App() 注册），证明高级场景可用。
func TestHandleFiber_UsesFiberMiddleware(t *testing.T) {
	d := newTestDriver()
	// 通过 App 注册 fiber 原生中间件（设置响应头）
	d.App().Use(func(c *fiber.Ctx) error {
		c.Set("X-Via-Fiber-MW", "yes")
		return c.Next()
	})
	d.HandleFiber(http.MethodGet, "/health", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})
	d.HandleFiber(http.MethodGet, "/x", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	baseURL := startDriver(t, d)

	resp, err := http.Get(baseURL + "/x")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("X-Via-Fiber-MW"); got != "yes" {
		t.Errorf("fiber middleware should set header, got %q", got)
	}
}

// TestHandleFiber_QueryParser 验证 fiber 特有的结构体查询参数解析能力
// （这是适配层路径 Handle 无法直接提供的）。
func TestHandleFiber_QueryParser(t *testing.T) {
	d := newTestDriver()
	type filter struct {
		Name string `query:"name"`
		Age  int    `query:"age"`
	}
	d.HandleFiber(http.MethodGet, "/search", func(c *fiber.Ctx) error {
		var f filter
		if err := c.QueryParser(&f); err != nil {
			return err
		}
		return c.JSON(f)
	})
	d.HandleFiber(http.MethodGet, "/health", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	baseURL := startDriver(t, d)

	resp, err := http.Get(baseURL + "/search?name=alice&age=30")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// fiber QueryParser 默认按 Go 字段名序列化（Name/Age，首字母大写）。
	// 验证 name 和 age(int) 都被正确解析。
	if !strings.Contains(string(body), `"Name":"alice"`) {
		t.Errorf("QueryParser should parse name, got %s", body)
	}
	if !strings.Contains(string(body), `"Age":30`) {
		t.Errorf("QueryParser should parse age as int, got %s", body)
	}
}

// --- options 注入方案测试 ---

// TestNew_WithOptions 验证用 WithRoute + WithMiddleware + WithDisableStartupMessage
// 在构造时注入框架原生配置（纯 options 风格复用 fiber 代码）。
func TestNew_WithOptions(t *testing.T) {
	mwCalled := false
	d := New(
		WithDisableStartupMessage(),
		// fiber 原生中间件（作用于下面的原生路由）
		WithMiddleware(func(c *fiber.Ctx) error {
			mwCalled = true
			c.Set("X-Via-Fiber-MW", "yes")
			return c.Next()
		}),
		// fiber 原生路由（复用存量 fiber handler 风格）
		WithRoute(http.MethodGet, "/users/:id", func(c *fiber.Ctx) error {
			return c.JSON(fiber.Map{"id": c.Params("id")})
		}),
		WithRoute(http.MethodGet, "/health", func(c *fiber.Ctx) error {
			return c.SendStatus(http.StatusOK)
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
	if got := resp.Header.Get("X-Via-Fiber-MW"); got != "yes" {
		t.Errorf("WithMiddleware should apply, got header %q", got)
	}
	if !mwCalled {
		t.Error("fiber middleware should be called")
	}
}

// TestOptions_MixedWithStandardRoute 验证 options 注入的原生路由
// 与 Handle 注册的标准路由能在同一 driver 共存。
func TestOptions_MixedWithStandardRoute(t *testing.T) {
	d := New(WithDisableStartupMessage(),
		WithRoute(http.MethodGet, "/fiber-route", func(c *fiber.Ctx) error {
			return c.SendString("from-fiber-option")
		}),
	)
	// 标准 http.HandlerFunc（走 Driver 接口）
	d.Handle(http.MethodGet, "/std-route", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("from-std"))
	})
	d.HandleFiber(http.MethodGet, "/health", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	baseURL := startDriver(t, d)

	resp1, err := http.Get(baseURL + "/fiber-route")
	if err != nil {
		t.Fatalf("Get /fiber-route: %v", err)
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	if string(body1) != "from-fiber-option" {
		t.Errorf("/fiber-route: %q", body1)
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


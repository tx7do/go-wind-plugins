package fiber

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"

	"github.com/gofiber/fiber/v2"
	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
)

// fiberDriver 基于 fiber(fasthttp) 框架实现的 HTTP 服务器驱动。
type fiberDriver struct {
	app      *fiber.App
	stopOnce sync.Once
	stopErr  error
}

// NewDriver 创建一个基于 fiber 的驱动实例。
func NewDriver() httpPlugin.Driver {
	return &fiberDriver{app: fiber.New()}
}

// Handle 注册路由处理器，将标准 http.HandlerFunc 适配为 fiber handler。
func (d *fiberDriver) Handle(method, path string, handler http.HandlerFunc) {
	d.app.Add(method, path, adaptFiber(handler))
}

// Start 启动服务器并阻塞，直到 ctx 被取消时执行优雅关闭。
// listener 由 Server 创建并传入（已处理 TLS 包装）。
func (d *fiberDriver) Start(ctx context.Context, ln net.Listener) error {
	errChan := make(chan error, 1)
	go func() {
		if err := d.app.Listener(ln); err != nil {
			errChan <- err
			return
		}
		errChan <- nil
	}()
	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return d.Stop(context.Background())
	}
}

// Stop 优雅关闭服务器，保证只关闭一次。
func (d *fiberDriver) Stop(_ context.Context) error {
	d.stopOnce.Do(func() {
		d.stopErr = d.app.Shutdown()
	})
	return d.stopErr
}

// adaptFiber 将标准 http.HandlerFunc 适配为 fiber.Handler。
// 由于 fiber 基于 fasthttp，与 net/http 不兼容，这里做必要的请求/响应转换。
func adaptFiber(h http.HandlerFunc) fiber.Handler {
	return func(c *fiber.Ctx) error {
		req, err := newRequest(c)
		if err != nil {
			return err
		}
		rw := &fiberResponseWriter{header: make(http.Header), status: http.StatusOK}
		h(rw, req)
		return rw.flush(c)
	}
}

// newRequest 从 fiber.Ctx 构造标准 http.Request。
func newRequest(c *fiber.Ctx) (*http.Request, error) {
	var body io.Reader
	if b := c.Body(); len(b) > 0 {
		body = bytes.NewReader(b)
	} else {
		body = http.NoBody
	}

	scheme := string(c.Request().URI().Scheme())
	if scheme == "" {
		scheme = "http"
	}
	host := string(c.Request().Header.Host())

	req := &http.Request{
		Method: c.Method(),
		URL: &url.URL{
			Scheme:   scheme,
			Host:     host,
			Path:     string(c.Request().URI().Path()),
			RawQuery: string(c.Request().URI().QueryString()),
		},
		Header: make(http.Header),
		Body:   io.NopCloser(body),
		Host:   host,
	}
	c.Request().Header.VisitAll(func(k, v []byte) {
		req.Header.Add(string(k), string(v))
	})
	return req, nil
}

// fiberResponseWriter 实现 http.ResponseWriter，将写入缓存后统一 flush 到 fiber.Ctx。
type fiberResponseWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *fiberResponseWriter) Header() http.Header { return w.header }

func (w *fiberResponseWriter) Write(b []byte) (int, error) {
	return w.body.Write(b)
}

func (w *fiberResponseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
}

// flush 将缓存的状态码、响应头和响应体写入 fiber.Ctx。
func (w *fiberResponseWriter) flush(c *fiber.Ctx) error {
	c.Status(w.status)
	for k, vs := range w.header {
		for _, v := range vs {
			c.Set(k, v)
		}
	}
	_, err := c.Write(w.body.Bytes())
	return err
}

// Package chi provides an HTTP server driver based on the [chi] router.
//
// [chi] is a lightweight, idiomatic, and composable router for building Go HTTP
// services. It is designed to be 100% compatible with net/http, making it a
// natural drop-in replacement for the std driver with better routing
// capabilities (path parameters, nested routes, middleware groups).
//
// Usage:
//
//	import (
//	    httpServer "github.com/tx7do/go-wind-plugins/transport/http"
//	    "github.com/tx7do/go-wind-plugins/transport/http/driver/chi"
//	)
//
//	srv := httpServer.NewServer(":8080", httpServer.WithDriver(chi.NewDriver()))
package chi

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"
	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
)

// 编译期确保 *chiDriver 实现 httpPlugin.Driver 接口。
var _ httpPlugin.Driver = (*chiDriver)(nil)

// chiDriver 基于 chi 路由框架实现的 HTTP 服务器驱动。
type chiDriver struct {
	router *chi.Mux
	server *http.Server
}

// NewDriver 创建一个基于 chi 的驱动实例。
func NewDriver() httpPlugin.Driver {
	return &chiDriver{router: chi.NewMux()}
}

// Handle 注册路由处理器。
func (d *chiDriver) Handle(method, path string, handler http.HandlerFunc) {
	d.router.MethodFunc(method, path, handler)
}

// HandlePrefix 在 prefix 下挂载 handler，匹配 prefix 及其所有子路径。
// chi 的 Mount 只修改路由上下文（RouteCtxKey）用于嵌套路由匹配，
// 不会改写 r.URL.Path，因此挂载普通 http.Handler 时天然满足
// Driver 契约：handler 收到未经 strip 的原始请求路径。
func (d *chiDriver) HandlePrefix(prefix string, h http.Handler) {
	d.router.Mount(prefix, h)
}

// Start 启动服务器并阻塞，直到 ctx 被取消时执行优雅关闭。
// listener 由 Server 创建并传入（已处理 TLS 包装）。
func (d *chiDriver) Start(ctx context.Context, ln net.Listener) error {
	d.server = &http.Server{
		Handler: d.router,
	}
	errChan := make(chan error, 1)
	go func() {
		if err := d.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- err
			return
		}
		errChan <- nil
	}()
	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return d.server.Shutdown(context.Background())
	}
}

// Stop 主动关闭服务器。
func (d *chiDriver) Stop(ctx context.Context) error {
	if d.server == nil {
		return nil
	}
	return d.server.Shutdown(ctx)
}

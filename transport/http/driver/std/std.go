// Package std provides an HTTP server driver based on the Go standard library
// net/http package.
//
// This is the simplest driver available, using [http.ServeMux] for routing.
// For production use, consider the gin or fiber drivers which offer better
// performance and features.
//
// Usage:
//
//	import (
//	    httpServer "github.com/tx7do/go-wind-plugins/transport/http"
//	    "github.com/tx7do/go-wind-plugins/transport/http/driver/std"
//	)
//
//	srv := httpServer.NewServer(":8080", httpServer.WithDriver(std.NewDriver()))
package std

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
)

// stdDriver 基于 net/http 标准库实现的 HTTP 服务器驱动。
type stdDriver struct {
	mux    *http.ServeMux
	server *http.Server
}

// NewDriver 创建一个基于 net/http 标准库的驱动实例。
func NewDriver() httpPlugin.Driver {
	return &stdDriver{mux: http.NewServeMux()}
}

// Handle 注册路由处理器。
// 由于 net/http 的 ServeMux 仅按 path 匹配，这里在内部对请求方法进行校验。
func (d *stdDriver) Handle(method, path string, handler http.HandlerFunc) {
	d.mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		handler(w, r)
	})
}

// HandlePrefix 在 prefix 下挂载 handler，匹配 prefix 及其所有子路径。
// net/http 的 ServeMux 对以 "/" 结尾的 pattern 自动做子树匹配，
// 故直接转发即可。若 prefix 不以 "/" 结尾，按 net/http 约定补齐。
func (d *stdDriver) HandlePrefix(prefix string, h http.Handler) {
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	d.mux.Handle(prefix, h)
}

// Start 启动服务器并阻塞，直到 ctx 被取消时执行优雅关闭。
// listener 由 Server 创建并传入（已处理 TLS 包装）。
func (d *stdDriver) Start(ctx context.Context, ln net.Listener) error {
	d.server = &http.Server{
		Handler: d.mux,
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
func (d *stdDriver) Stop(ctx context.Context) error {
	if d.server == nil {
		return nil
	}
	return d.server.Shutdown(ctx)
}

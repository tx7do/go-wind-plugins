package http

import (
	"context"
	"errors"
	"net"
	"net/http"
)

// defaultDriver 基于标准库 net/http 实现的 HTTP 服务器驱动。
type defaultDriver struct {
	mux    *http.ServeMux
	server *http.Server
}

// NewDefaultDriver 创建一个基于 net/http 的默认驱动。
func NewDefaultDriver() Driver {
	return &defaultDriver{mux: http.NewServeMux()}
}

// Handle 注册路由处理器。
// 由于 net/http 的 ServeMux 仅按 path 匹配，这里在内部对请求方法进行校验。
func (d *defaultDriver) Handle(method, path string, handler http.HandlerFunc) {
	d.mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		handler(w, r)
	})
}

// Start 启动服务器并阻塞，直到 ctx 被取消时执行优雅关闭。
// listener 由 Server 创建并传入（已处理 TLS 包装）。
func (d *defaultDriver) Start(ctx context.Context, ln net.Listener) error {
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
func (d *defaultDriver) Stop(ctx context.Context) error {
	if d.server == nil {
		return nil
	}
	return d.server.Shutdown(ctx)
}

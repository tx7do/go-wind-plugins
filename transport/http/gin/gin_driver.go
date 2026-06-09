package gin

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
)

// ginDriver 基于 gin 框架实现的 HTTP 服务器驱动。
type ginDriver struct {
	engine *gin.Engine
	server *http.Server
}

// NewDriver 创建一个基于 gin 的驱动实例。
func NewDriver() httpPlugin.Driver {
	gin.SetMode(gin.ReleaseMode)
	return &ginDriver{engine: gin.New()}
}

// Handle 注册路由处理器，将标准 http.HandlerFunc 包装为 gin handler。
func (d *ginDriver) Handle(method, path string, handler http.HandlerFunc) {
	d.engine.Handle(method, path, gin.WrapF(handler))
}

// Start 启动服务器并阻塞，直到 ctx 被取消时执行优雅关闭。
// listener 由 Server 创建并传入（已处理 TLS 包装）。
func (d *ginDriver) Start(ctx context.Context, ln net.Listener) error {
	d.server = &http.Server{
		Handler: d.engine,
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
func (d *ginDriver) Stop(ctx context.Context) error {
	if d.server == nil {
		return nil
	}
	return d.server.Shutdown(ctx)
}

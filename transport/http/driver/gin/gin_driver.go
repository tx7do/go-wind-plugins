package gin

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
)

// Driver 基于 gin 框架实现的 HTTP 服务器驱动。
//
// 它同时实现 httpPlugin.Driver 接口（Handle/Start/Stop，接收标准
// http.HandlerFunc）和框架特有的 HandleGin（接收 gin.HandlerFunc）。
//
// 选型说明：
//   - 用 Handle（继承自 Driver 接口）：handler 走标准 net/http 接口，
//     能复用本插件的全部中间件与 binding，但拿不到 *gin.Context。
//   - 用 HandleGin：handler 拿到原生 *gin.Context，可用 gin 全部能力
//     （Param/Query/JSON/ShouldBind 等），适合复用存量 gin 业务代码；
//     但这些路由不经过 httpPlugin.Server 注册的中间件链。
type Driver struct {
	engine *gin.Engine
	server *http.Server
}

// 编译期确保 *Driver 实现 httpPlugin.Driver 接口。
var _ httpPlugin.Driver = (*Driver)(nil)

// Option 用于在构造 Driver 时注入框架原生的路由与中间件。
//
// 与 httpPlugin.Option（作用于 Server）不同，本选项作用于 driver 本身，
// 让用户能在纯 options 风格下复用存量 gin 业务代码，无需裸用 HandleGin。
//
// 示例：
//
//	d := gin.New(
//	    gin.WithRoute("GET", "/users/:id", GetUser),
//	    gin.WithMiddleware(gin.Logger(), gin.Recovery()),
//	)
//	srv := httpPlugin.NewServer(":8080", httpPlugin.WithDriver(d))
type Option func(*Driver)

// NewDriver 创建一个基于 gin 的驱动实例，返回 httpPlugin.Driver 接口。
//
// 适用于只需要标准 http.HandlerFunc 接口、不需要框架特有能力的场景。
// 若需注册 gin 原生 handler（复用存量 gin 代码），改用 New 获取具体类型。
func NewDriver() httpPlugin.Driver {
	return New()
}

// New 创建一个基于 gin 的驱动实例，返回具体类型 *Driver。
//
// opts 在 engine 创建后、返回前应用，可注入框架原生的路由与中间件
// （WithRoute / WithMiddleware），用于复用存量 gin 业务代码。
func New(opts ...Option) *Driver {
	gin.SetMode(gin.ReleaseMode)
	d := &Driver{engine: gin.New()}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// WithRoute 声明一个 gin 原生路由（method + path + gin.HandlerFunc）。
//
// 用于在 New(...) 时复用存量 gin handler，handler 拿到真正的 *gin.Context，
// 可用 gin 全部能力（路径参数、ShouldBind、JSON 等）。
//
// 注意：通过本选项注册的路由不经过 httpPlugin.Server.Use 注册的中间件链
// （那些中间件基于 http.HandlerFunc）。如需中间件，配合 WithMiddleware
// 注册 gin 原生中间件。
func WithRoute(method, path string, handler gin.HandlerFunc) Option {
	return func(d *Driver) {
		d.engine.Handle(method, path, handler)
	}
}

// WithMiddleware 注册 gin 原生中间件，作用于通过 WithRoute 注册的路由。
//
// 注意：这些中间件与 httpPlugin.Server.Use 的中间件是两套体系，互不作用。
// 框架原生路由用 gin 中间件，标准 http.HandlerFunc 路由用 Server 中间件。
func WithMiddleware(middleware ...gin.HandlerFunc) Option {
	return func(d *Driver) {
		d.engine.Use(middleware...)
	}
}

// Handle 注册路由处理器，将标准 http.HandlerFunc 包装为 gin handler。
//
// 这是 httpPlugin.Driver 接口的实现：handler 走标准 net/http 接口，
// 可复用本插件中间件与 binding，但无法使用 *gin.Context 的能力。
func (d *Driver) Handle(method, path string, handler http.HandlerFunc) {
	d.engine.Handle(method, path, gin.WrapF(handler))
}

// HandlePrefix 在 prefix 下挂载 handler，匹配 prefix 及其所有子路径。
// 用 gin 的通配符路由 "<prefix>/*filepath" 实现，handler 接收原始请求路径
// （不做 strip，与 httpPlugin.Driver 接口约定一致）。
// 接受所有 HTTP 方法（Any），适用于静态资源、Swagger UI 等场景。
func (d *Driver) HandlePrefix(prefix string, h http.Handler) {
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	d.engine.Any(prefix+"*filepath", gin.WrapH(h))
}

// HandleGin 注册 gin 原生 handler（func(*gin.Context)）。
//
// 与 Handle 的区别：
//   - handler 接收真正的 *gin.Context，可用 gin 全部能力
//     （c.Param / c.Query / c.JSON / c.ShouldBind / 路由参数 :id 等）
//   - 适用于复用存量 gin 业务代码，无需改写为 http.HandlerFunc
//
// 注意：通过 HandleGin 注册的路由不经过 httpPlugin.Server.Use 注册的
// 中间件链（因为那些中间件基于 http.HandlerFunc）。如需对这类路由
// 应用 gin 中间件，请用 Engine() 获取底层引擎直接注册。
func (d *Driver) HandleGin(method, path string, handler gin.HandlerFunc) {
	d.engine.Handle(method, path, handler)
}

// Engine 返回底层 *gin.Engine，用于需要直接操作引擎的高级场景
// （注册路由组、gin 原生中间件、websocket 等）。
//
// 警告：直接操作 Engine 会绕过 Driver 抽象，用户代码将与 gin 强耦合，
// 失去 driver 可替换性。仅在确有需要时使用。
func (d *Driver) Engine() *gin.Engine {
	return d.engine
}

// Start 启动服务器并阻塞，直到 ctx 被取消时执行优雅关闭。
// listener 由 Server 创建并传入（已处理 TLS 包装）。
func (d *Driver) Start(ctx context.Context, ln net.Listener) error {
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
func (d *Driver) Stop(ctx context.Context) error {
	if d.server == nil {
		return nil
	}
	return d.server.Shutdown(ctx)
}

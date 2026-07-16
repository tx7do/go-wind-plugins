package http

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"

	"github.com/tx7do/go-wind/transport"
)

// Middleware 定义标准 HTTP 中间件类型。
// 中间件接收下一个处理器并返回包装后的处理器，可在请求前后执行自定义逻辑。
type Middleware func(http.Handler) http.Handler

// Driver 定义 HTTP 服务器驱动接口，由具体的框架（如 gin、fiber 等）实现。
// 注意：Start 接收的是由 Server 创建好的 net.Listener，而非原始地址字符串。
// 这使得 Server 能够获取实际绑定地址（用于 :0 随机端口场景）并统一处理 TLS。
type Driver interface {
	Handle(method, path string, handler http.HandlerFunc)
	// HandlePrefix 在 prefix 下挂载一个 http.Handler，匹配 prefix 及其所有子路径。
	// 用于嵌入静态资源、Swagger UI 等「前缀路由」场景。
	// 与 Handle 不同：不经过 Server.Use 注册的中间件链，handler 接收原始路径（不做 strip）。
	HandlePrefix(prefix string, h http.Handler)
	Start(ctx context.Context, ln net.Listener) error
	Stop(ctx context.Context) error
}

// 确保 Server 实现了 wind transport.Server 接口。
var _ transport.Server = (*Server)(nil)

type Server struct {
	addr        string
	tlsConfig   *tls.Config
	listener    net.Listener
	driver      Driver
	middlewares []Middleware
}

// NewServer 创建一个 HTTP 服务器实例。
func NewServer(addr string, opts ...Option) *Server {
	srv := &Server{addr: addr}
	for _, opt := range opts {
		opt(srv)
	}
	return srv
}

// Use 注册全局中间件，对所有通过 Handle 注册的路由生效。
// 中间件按注册顺序执行：先注册的中间件最先被调用。
// 必须在 Handle 之前调用。
func (s *Server) Use(middlewares ...Middleware) {
	s.middlewares = append(s.middlewares, middlewares...)
}

// Handle 注册路由处理器，将请求转发到具体的驱动实现。
// 注册前会应用已添加的中间件链。
// 若未通过 WithDriver 设置驱动，将 panic。
func (s *Server) Handle(method, path string, handler http.HandlerFunc) {
	s.mustDriver()
	s.driver.Handle(method, path, s.wrapHandler(handler))
}

// GET 注册 GET 请求路由，等价于 Handle(http.MethodGet, ...).
func (s *Server) GET(path string, handler http.HandlerFunc) {
	s.Handle(http.MethodGet, path, handler)
}

// POST 注册 POST 请求路由。
func (s *Server) POST(path string, handler http.HandlerFunc) {
	s.Handle(http.MethodPost, path, handler)
}

// PUT 注册 PUT 请求路由。
func (s *Server) PUT(path string, handler http.HandlerFunc) {
	s.Handle(http.MethodPut, path, handler)
}

// DELETE 注册 DELETE 请求路由。
func (s *Server) DELETE(path string, handler http.HandlerFunc) {
	s.Handle(http.MethodDelete, path, handler)
}

// PATCH 注册 PATCH 请求路由。
func (s *Server) PATCH(path string, handler http.HandlerFunc) {
	s.Handle(http.MethodPatch, path, handler)
}

// HEAD 注册 HEAD 请求路由。
func (s *Server) HEAD(path string, handler http.HandlerFunc) {
	s.Handle(http.MethodHead, path, handler)
}

// OPTIONS 注册 OPTIONS 请求路由。
func (s *Server) OPTIONS(path string, handler http.HandlerFunc) {
	s.Handle(http.MethodOptions, path, handler)
}

// HandlePrefix 在 prefix 下挂载一个 http.Handler，匹配 prefix 及其所有子路径。
// 用于 Swagger UI、pprof、静态文件服务等「前缀路由」场景。
//
// 与 Handle 的区别：handler 不经过 Server.Use 注册的中间件链
// （这类 handler 通常自带完整逻辑，如静态资源服务，套业务中间件无意义且有开销），
// 且由 driver 负责前缀匹配，handler 收到的是原始请求路径。
//
// 若未通过 WithDriver 设置驱动，将 panic。
func (s *Server) HandlePrefix(prefix string, h http.Handler) {
	s.mustDriver()
	s.driver.HandlePrefix(prefix, h)
}

// Start 启动 HTTP 服务器。
// Server 自行创建 net.Listener 以获取实际绑定地址，并用 TLS 包装（若已配置）。
// 若未通过 WithDriver 设置驱动，将返回错误。
func (s *Server) Start(ctx context.Context) error {
	s.mustDriver()

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	if s.tlsConfig != nil {
		ln = tls.NewListener(ln, s.tlsConfig)
	}
	s.listener = ln
	return s.driver.Start(ctx, ln)
}

// Stop 停止 HTTP 服务器。
func (s *Server) Stop(ctx context.Context) error {
	if s.driver == nil {
		return nil
	}
	return s.driver.Stop(ctx)
}

// Endpoint 返回服务器的访问地址。
// 服务器已启动时返回实际监听地址（用于 :0 随机端口场景），
// 否则返回配置地址。自动根据 TLS 配置选择 http/https scheme，
// 并规范 IPv6 地址格式。
func (s *Server) Endpoint() string {
	scheme := "http"
	if s.tlsConfig != nil {
		scheme = "https"
	}
	addr := s.addr
	if s.listener != nil {
		addr = s.listener.Addr().String()
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return scheme + "://" + addr
	}
	if host == "" || host == "0.0.0.0" {
		host = "localhost"
	}
	return scheme + "://" + net.JoinHostPort(host, port)
}

// Addr 返回服务器的监听地址。
func (s *Server) Addr() string { return s.addr }

// wrapHandler 用中间件链包装路由处理器。
// 若无中间件则直接返回原处理器。
func (s *Server) wrapHandler(handler http.HandlerFunc) http.HandlerFunc {
	if len(s.middlewares) == 0 {
		return handler
	}
	h := http.Handler(handler)
	for i := len(s.middlewares) - 1; i >= 0; i-- {
		h = s.middlewares[i](h)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
	}
}

// Chain 将多个中间件组合为单个中间件，按入参顺序执行。
// 常用于构建可复用的中间件组。
func Chain(middlewares ...Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			next = middlewares[i](next)
		}
		return next
	}
}

// ErrNoDriver 表示未设置 HTTP 服务器驱动。
var ErrNoDriver = errors.New("http: no driver set, use WithDriver() to specify one (e.g. std.NewDriver(), gin.NewDriver())")

// mustDriver 确保驱动已设置，未设置时 panic。
func (s *Server) mustDriver() {
	if s.driver == nil {
		panic(ErrNoDriver)
	}
}

// Package graphql provides a GraphQL server based on [gqlgen] that implements
// the [transport.Server] interface.
//
// GraphQL runs over HTTP, so this server wraps a standard [http.Server] with
// a [http.ServeMux] for routing. It supports the same [Middleware] type as
// transport/http, allowing middleware (recovery, logging, request-id, etc.)
// to be shared between HTTP and GraphQL servers.
//
// Usage:
//
//	import (
//	    graphqlServer "github.com/tx7do/go-wind-plugins/transport/graphql"
//	    "github.com/99designs/gqlgen/graphql/handler"
//	)
//
//	srv := graphqlServer.NewServer(":8080")
//	srv.Handle("/query", api.NewExecutableSchema(api.Config{Resolvers: &resolver{}}))
//
//	if err := srv.Start(ctx); err != nil { ... }
package graphql

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	gqltransport "github.com/99designs/gqlgen/graphql/handler/transport"

	"github.com/tx7do/go-wind/transport"
)

const KindGraphQL = "graphql"

// 确保 Server 实现了 wind transport.Server 接口。
var _ transport.Server = (*Server)(nil)

// Middleware 是标准 HTTP 中间件类型。
// 使用类型别名（而非命名类型）使得 transport/http/middleware 下的中间件
// 可以直接传给 GraphQL server，无需类型转换：
//
//	srv.Use(recovery.Middleware())   // recovery 返回 httpPlugin.Middleware
//	srv.Use(logging.Middleware())
//	srv.Use(requestid.Middleware())
type Middleware = func(http.Handler) http.Handler

// Server 是基于 gqlgen 的 GraphQL 服务器，实现 transport.Server 接口。
type Server struct {
	addr        string
	tlsConfig   *tls.Config
	listener    net.Listener
	mux         *http.ServeMux
	server      *http.Server
	middlewares []Middleware
}

// NewServer 创建一个 GraphQL 服务器实例。
func NewServer(addr string, opts ...Option) *Server {
	srv := &Server{
		addr: addr,
		mux:  http.NewServeMux(),
	}
	for _, opt := range opts {
		opt(srv)
	}
	return srv
}

// Handle 注册一个 GraphQL schema 到指定路径。
func (s *Server) Handle(path string, es graphql.ExecutableSchema) {
	h := handler.New(es)
	// handler.New 本身不注册任何 HTTP 传输层，所有请求都会得到
	// 400 "transport not supported"。挂上标准的传输层，使
	// srv.Handle("/query", schema) 的用法开箱即用。
	h.AddTransport(gqltransport.Options{})
	h.AddTransport(gqltransport.GET{})
	h.AddTransport(gqltransport.POST{})
	h.AddTransport(gqltransport.MultipartForm{})
	s.mux.Handle(path, h)
}

// HandleFunc 注册普通 HTTP 处理器（如 /playground 等辅助页面）。
func (s *Server) HandleFunc(path string, h http.HandlerFunc) {
	s.mux.HandleFunc(path, h)
}

// Use 注册全局中间件，对所有路由生效。
// 中间件按注册顺序执行：先注册的中间件最先被调用。
// 必须在 Start 之前调用。
func (s *Server) Use(middlewares ...Middleware) {
	s.middlewares = append(s.middlewares, middlewares...)
}

// Start 启动 GraphQL 服务器，阻塞直到 ctx 被取消。
func (s *Server) Start(ctx context.Context) error {
	// Start 前先释放遗留的 listener，以获得干净的监听
	// （避免同一 listener 叠两个 accept 循环，或残留已关闭的 fd）
	if s.listener != nil {
		_ = s.listener.Close()
		s.listener = nil
	}

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	if s.tlsConfig != nil {
		ln = tls.NewListener(ln, s.tlsConfig)
	}
	s.listener = ln

	// 应用中间件链
	h := http.Handler(s.mux)
	for i := len(s.middlewares) - 1; i >= 0; i-- {
		h = s.middlewares[i](h)
	}

	s.server = &http.Server{Handler: h}
	s.server.BaseContext = func(net.Listener) context.Context { return ctx }

	LogInfof("server listening on: %s", ln.Addr().String())

	errChan := make(chan error, 1)
	go func() {
		if err := s.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- err
			return
		}
		errChan <- nil
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return s.server.Shutdown(context.Background())
	}
}

// Stop 优雅关闭服务器。
func (s *Server) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}

	// Shutdown 会关闭并使 http.Server 不可复用：释放 listener 引用，
	// 下次 Start 会重建 server 实例与 listener（支持 Stop→Start）
	s.listener = nil
	return s.server.Shutdown(ctx)
}

// Endpoint 返回服务器的访问地址。
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

// Addr 返回配置的监听地址。
func (s *Server) Addr() string { return s.addr }

package grpc

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"

	"github.com/tx7do/go-wind/transport"
	"google.golang.org/grpc"
)

// 确保 Server 实现了 wind transport.Server 接口。
var _ transport.Server = (*Server)(nil)

// Middleware 定义 gRPC 一元拦截器类型，用于在请求前后执行自定义逻辑。
type Middleware = grpc.UnaryServerInterceptor

// StreamMiddleware 定义 gRPC 流拦截器类型。
type StreamMiddleware = grpc.StreamServerInterceptor

// Server 封装 gRPC 服务器，实现 transport.Server 接口。
// 它管理与框架一致的 Start/Stop 生命周期，同时暴露底层 *grpc.Server
// 供调用方通过 RegisterService 注册 protobuf 服务。
type Server struct {
	addr             string
	server           *grpc.Server
	middlewares      []Middleware
	streamMiddleware []StreamMiddleware
	listener         net.Listener
	timeout          time.Duration
}

// NewServer 创建一个 gRPC 服务器实例。
//
// 若未通过选项提供 *grpc.Server，将在 Start 时按默认配置创建一个。
func NewServer(addr string, opts ...Option) *Server {
	srv := &Server{addr: addr}
	for _, opt := range opts {
		opt(srv)
	}
	return srv
}

// Use 注册一元拦截器中间件，按注册顺序执行。
// 必须在 Start 之前调用。
func (s *Server) Use(middlewares ...Middleware) {
	s.middlewares = append(s.middlewares, middlewares...)
}

// UseStream 注册流拦截器中间件，按注册顺序执行。
// 必须在 Start 之前调用。
func (s *Server) UseStream(middlewares ...StreamMiddleware) {
	s.streamMiddleware = append(s.streamMiddleware, middlewares...)
}

// Server 返回底层的 *grpc.Server，调用方可通过它注册 protobuf 服务。
// 若尚未初始化，返回 nil。
func (s *Server) Server() *grpc.Server { return s.server }

// RegisterService 注册一个 gRPC 服务及其实现，等价于直接操作底层 *grpc.Server。
func (s *Server) RegisterService(sd *grpc.ServiceDesc, ss any) {
	s.ensureServer()
	s.server.RegisterService(sd, ss)
}

// Start 启动 gRPC 服务器并阻塞，直到 ctx 被取消时执行优雅关闭。
func (s *Server) Start(ctx context.Context) error {
	s.ensureServer()

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.listener = ln

	errChan := make(chan error, 1)
	go func() {
		if err := s.server.Serve(ln); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			errChan <- err
			return
		}
		errChan <- nil
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return s.Stop(context.Background())
	}
}

// Stop 优雅关闭 gRPC 服务器。
// 若设置了 timeout（通过 WithTimeout），超时后强制关闭。
func (s *Server) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	stopped := make(chan struct{})
	go func() {
		s.server.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
		return nil
	case <-ctx.Done():
		s.server.Stop()
		return ctx.Err()
	}
}

// Endpoint 返回服务器的访问地址。
// 若服务器已启动，返回实际监听地址；否则返回配置地址。
func (s *Server) Endpoint() string {
	addr := s.addr
	if s.listener != nil {
		addr = s.listener.Addr().String()
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return addr
	}
	if host == "" || host == "0.0.0.0" {
		host = "localhost"
	}
	return "grpc://" + net.JoinHostPort(host, port)
}

// Addr 返回服务器的监听地址。
func (s *Server) Addr() string { return s.addr }

// ensureServer 确保底层 grpc.Server 已初始化，未设置时创建一个。
func (s *Server) ensureServer() {
	if s.server != nil {
		return
	}

	var opts []grpc.ServerOption

	// 一元拦截器链
	if len(s.middlewares) > 0 {
		opts = append(opts, grpc.ChainUnaryInterceptor(s.middlewares...))
	}

	// 流拦截器链
	if len(s.streamMiddleware) > 0 {
		opts = append(opts, grpc.ChainStreamInterceptor(s.streamMiddleware...))
	}

	s.server = grpc.NewServer(opts...)
}

// Chain 将多个一元拦截器组合为单个拦截器，按入参顺序执行。
// 常用于构建可复用的拦截器组，便于传入单个 Use 调用。
func Chain(middlewares ...Middleware) Middleware {
	switch len(middlewares) {
	case 0:
		return nil
	case 1:
		return middlewares[0]
	}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// 从最后一个拦截器向前组装调用链
		h := handler
		for i := len(middlewares) - 1; i >= 0; i-- {
			interceptor, next := middlewares[i], h
			h = func(ctx context.Context, req any) (any, error) {
				return interceptor(ctx, req, info, next)
			}
		}
		return h(ctx, req)
	}
}

// FormatEndpoint 将 host 和 port 格式化为 gRPC endpoint 字符串。
func FormatEndpoint(host string, port int) string {
	return "grpc://" + net.JoinHostPort(host, strconv.Itoa(port))
}

package grpc

import (
	"time"

	"google.golang.org/grpc"
)

// Option 定义 gRPC 服务器的配置选项。
type Option func(*Server)

// WithServer 通过选项直接设置底层 *grpc.Server。
// 若调用方已有自定义配置的 grpc.Server，可由此注入。
func WithServer(srv *grpc.Server) Option {
	return func(s *Server) { s.server = srv }
}

// WithMiddleware 通过选项设置一元拦截器中间件。
func WithMiddleware(middlewares ...Middleware) Option {
	return func(s *Server) { s.middlewares = append(s.middlewares, middlewares...) }
}

// WithStreamMiddleware 通过选项设置流拦截器中间件。
func WithStreamMiddleware(middlewares ...StreamMiddleware) Option {
	return func(s *Server) { s.streamMiddleware = append(s.streamMiddleware, middlewares...) }
}

// WithTimeout 设置服务器的优雅关闭超时时间。
// 若设置为 0，则使用 GracefulStop 无限等待。
func WithTimeout(timeout time.Duration) Option {
	return func(s *Server) { s.timeout = timeout }
}

package trpc

import (
	"crypto/tls"
	"net"
	"time"

	"trpc.group/trpc-go/trpc-go/filter"
	trpcServer "trpc.group/trpc-go/trpc-go/server"
)

type ServerOption func(o *Server)

// WithNamespace 设置命名空间。
func WithNamespace(namespace string) ServerOption {
	return func(s *Server) {
		s.trpcOptions = append(s.trpcOptions, trpcServer.WithNamespace(namespace))
	}
}

// WithAddress 设置服务监听地址。
func WithAddress(addr string) ServerOption {
	return func(s *Server) {
		s.trpcOptions = append(s.trpcOptions, trpcServer.WithAddress(addr))
		s.address = addr
	}
}

// WithEnvName 设置环境名称。
func WithEnvName(envName string) ServerOption {
	return func(s *Server) {
		s.trpcOptions = append(s.trpcOptions, trpcServer.WithEnvName(envName))
	}
}

// WithContainer 设置容器名称。
func WithContainer(container string) ServerOption {
	return func(s *Server) {
		s.trpcOptions = append(s.trpcOptions, trpcServer.WithContainer(container))
	}
}

// WithSetName 设置服务分组名称。
func WithSetName(setName string) ServerOption {
	return func(s *Server) {
		s.trpcOptions = append(s.trpcOptions, trpcServer.WithSetName(setName))
	}
}

// WithServiceName 设置服务名称。
func WithServiceName(name string) ServerOption {
	return func(s *Server) {
		s.trpcOptions = append(s.trpcOptions, trpcServer.WithServiceName(name))
	}
}

// WithNetwork 设置网络协议（tcp/udp 等）。
func WithNetwork(network string) ServerOption {
	return func(s *Server) {
		s.trpcOptions = append(s.trpcOptions, trpcServer.WithNetwork(network))
	}
}

// WithProtocol 设置应用层协议（trpc/http 等）。
func WithProtocol(protocol string) ServerOption {
	return func(s *Server) {
		s.trpcOptions = append(s.trpcOptions, trpcServer.WithProtocol(protocol))
	}
}

// WithTimeout 设置请求处理超时时间。
func WithTimeout(timeout time.Duration) ServerOption {
	return func(s *Server) {
		s.trpcOptions = append(s.trpcOptions, trpcServer.WithTimeout(timeout))
	}
}

// WithTLS 设置 TLS 证书。
func WithTLS(certFile, keyFile, caFile string) ServerOption {
	return func(s *Server) {
		s.trpcOptions = append(s.trpcOptions, trpcServer.WithTLS(certFile, keyFile, caFile))
	}
}

// WithIdleTimeout 设置空闲连接超时时间。
func WithIdleTimeout(timeout time.Duration) ServerOption {
	return func(s *Server) {
		s.trpcOptions = append(s.trpcOptions, trpcServer.WithIdleTimeout(timeout))
	}
}

// WithMaxRoutines 设置最大并发协程数。
func WithMaxRoutines(routines int) ServerOption {
	return func(s *Server) {
		s.trpcOptions = append(s.trpcOptions, trpcServer.WithMaxRoutines(routines))
	}
}

// WithCloseWaitTime 设置服务关闭等待时间。
func WithCloseWaitTime(timeout time.Duration) ServerOption {
	return func(s *Server) {
		s.trpcOptions = append(s.trpcOptions, trpcServer.WithCloseWaitTime(timeout))
	}
}

// WithDisableRequestTimeout 禁用请求超时。
func WithDisableRequestTimeout(disable bool) ServerOption {
	return func(s *Server) {
		s.trpcOptions = append(s.trpcOptions, trpcServer.WithDisableRequestTimeout(disable))
	}
}

// WithFilter 注入服务端过滤器（拦截器）。
// 多次调用会追加，过滤器按注册顺序依次执行。
func WithFilter(f filter.ServerFilter) ServerOption {
	return func(s *Server) {
		s.filters = append(s.filters, f)
	}
}

// WithFilters 批量注入服务端过滤器。
func WithFilters(fs []filter.ServerFilter) ServerOption {
	return func(s *Server) {
		s.filters = append(s.filters, fs...)
	}
}

// WithNamedFilter 注入具名过滤器（通过名称引用已注册的全局过滤器）。
func WithNamedFilter(name string, f filter.ServerFilter) ServerOption {
	return func(s *Server) {
		s.trpcOptions = append(s.trpcOptions, trpcServer.WithNamedFilter(name, f))
	}
}

// WithListener 注入自定义 net.Listener。
func WithListener(lis net.Listener) ServerOption {
	return func(s *Server) {
		s.trpcOptions = append(s.trpcOptions, trpcServer.WithListener(lis))
	}
}

// WithTrpcOptions 直接注入 tRPC 原生 Server 选项。
// 用于覆盖或使用尚未被封装的底层能力。
func WithTrpcOptions(opts ...trpcServer.Option) ServerOption {
	return func(s *Server) {
		s.trpcOptions = append(s.trpcOptions, opts...)
	}
}

// WithServerTLSConfig 使用 *tls.Config 设置 TLS。
// 注意：tRPC 框架本身通过 WithTLS 支持证书文件路径方式。
// 如需使用 *tls.Config，可通过 WithTrpcOptions 注入自定义 transport。
func WithServerTLSConfig(_ *tls.Config) ServerOption {
	return func(s *Server) {
		// tRPC 不直接支持 *tls.Config，保留接口以兼容
	}
}

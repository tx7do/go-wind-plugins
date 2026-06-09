package http

import (
	"crypto/tls"
)

type Option func(*Server)

func WithDriver(d Driver) Option {
	return func(s *Server) { s.driver = d }
}

// WithMiddleware 通过选项设置中间件，允许在创建 Server 时直接传入中间件。
func WithMiddleware(middlewares ...Middleware) Option {
	return func(s *Server) { s.middlewares = append(s.middlewares, middlewares...) }
}

// WithTLSConfig 设置 TLS 配置，启用 HTTPS。
func WithTLSConfig(c *tls.Config) Option {
	return func(s *Server) { s.tlsConfig = c }
}

// WithTLS 从证书和私钥文件加载 TLS 配置，启用 HTTPS。
func WithTLS(certFile, keyFile string) Option {
	return func(s *Server) {
		c, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			panic("http: LoadX509KeyPair: " + err.Error())
		}
		s.tlsConfig = &tls.Config{Certificates: []tls.Certificate{c}}
	}
}

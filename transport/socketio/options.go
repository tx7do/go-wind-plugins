package socketio

import (
	"crypto/tls"
	"net/http"

	socketIo "github.com/googollee/go-socket.io"
	"github.com/tx7do/go-wind-plugins/encoding"
)

type Option func(o *Server)

func WithNetwork(network string) Option {
	return func(s *Server) {
		s.network = network
	}
}

func WithAddress(addr string) Option {
	return func(s *Server) {
		s.address = addr
	}
}

func WithTLSConfig(c *tls.Config) Option {
	return func(o *Server) {
		o.tlsConf = c
	}
}

func WithCodec(c string) Option {
	return func(s *Server) {
		s.codec = encoding.GetCodec(c)
	}
}

func WithPath(path string) Option {
	return func(s *Server) {
		s.path = path
	}
}

// WithConnectHandler 注册连接回调。
// 注意：opts 在 server 实例创建前应用，此处仅做登记，由 createServer 重放。
func WithConnectHandler(namespace string, f func(socketIo.Conn) error) Option {
	return func(s *Server) {
		s.recordHandler("connect", namespace, "", f)
	}
}

// WithDisconnectHandler 注册断开回调。
// 注意：opts 在 server 实例创建前应用，此处仅做登记，由 createServer 重放。
func WithDisconnectHandler(namespace string, f func(socketIo.Conn, string)) Option {
	return func(s *Server) {
		s.recordHandler("disconnect", namespace, "", f)
	}
}

// WithErrorHandler 注册错误回调。
// 注意：opts 在 server 实例创建前应用，此处仅做登记，由 createServer 重放。
func WithErrorHandler(namespace string, f func(socketIo.Conn, error)) Option {
	return func(s *Server) {
		s.recordHandler("error", namespace, "", f)
	}
}

// WithEventHandler 注册事件回调。
// 注意：opts 在 server 实例创建前应用，此处仅做登记，由 createServer 重放。
func WithEventHandler(namespace, event string, f any) Option {
	return func(s *Server) {
		s.recordHandler("event", namespace, event, f)
	}
}

// WithCheckOrigin 自定义 Origin 校验（默认放行所有 Origin）。
func WithCheckOrigin(fn func(r *http.Request) bool) Option {
	return func(s *Server) {
		s.checkOrigin = fn
	}
}

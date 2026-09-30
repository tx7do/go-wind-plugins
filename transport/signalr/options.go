package signalr

import (
	"crypto/tls"
	"time"

	"github.com/philippseith/signalr"
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

func WithKeepAliveInterval(interval time.Duration) Option {
	return func(s *Server) {
		s.keepAliveInterval = interval
	}
}

func WithChanReceiveTimeout(timeout time.Duration) Option {
	return func(s *Server) {
		s.chanReceiveTimeout = timeout
	}
}

func WithStreamBufferCapacity(capacity uint) Option {
	return func(s *Server) {
		s.streamBufferCapacity = capacity
	}
}

func WithDebug(enable bool) Option {
	return func(s *Server) {
		s.debug = enable
	}
}

func WithHub(hub signalr.HubInterface) Option {
	return func(s *Server) {
		s.hub = hub
	}
}

// WithAllowedOrigins 配置 CORS 允许的 Origin 白名单；"*" 表示全部放行。
// 默认（未配置）不返回任何 CORS 头，仅同源可用。
func WithAllowedOrigins(origins []string, allowCredentials bool) Option {
	return func(s *Server) {
		s.allowedOrigins = origins
		s.allowCredentials = allowCredentials
	}
}

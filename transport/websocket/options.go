package websocket

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/tx7do/go-wind-plugins/encoding"
)

// Option 是 WebSocket 服务器的配置选项。
type Option func(*Server)

// WithTLSConfig 设置 TLS 配置，启用 WSS（WebSocket Secure）。
func WithTLSConfig(c *tls.Config) Option {
	return func(s *Server) { s.tlsConfig = c }
}

// WithTLS 从证书和私钥文件加载 TLS 配置，启用 WSS。
func WithTLS(certFile, keyFile string) Option {
	return func(s *Server) {
		c, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			panic("websocket: LoadX509KeyPair: " + err.Error())
		}
		s.tlsConfig = &tls.Config{Certificates: []tls.Certificate{c}}
	}
}

// WithMiddleware 通过选项设置中间件，允许在创建 Server 时直接传入中间件。
func WithMiddleware(middlewares ...Middleware) Option {
	return func(s *Server) { s.middlewares = append(s.middlewares, middlewares...) }
}

// WithPath 设置 WebSocket 的 HTTP 路由路径（默认 "/ws"）。
func WithPath(path string) Option {
	return func(s *Server) {
		if path != "" {
			s.path = path
		}
	}
}

// WithReadBufferSize 设置 WebSocket 读取缓冲区大小。
func WithReadBufferSize(size int) Option {
	return func(s *Server) {
		s.upgrader.ReadBufferSize = size
	}
}

// WithWriteBufferSize 设置 WebSocket 写入缓冲区大小。
func WithWriteBufferSize(size int) Option {
	return func(s *Server) {
		s.upgrader.WriteBufferSize = size
	}
}

// WithCheckOrigin 设置 Origin 检查函数，仅允许指定域名的连接。
func WithCheckOrigin(domain string) Option {
	return func(s *Server) {
		s.upgrader.CheckOrigin = func(r *http.Request) bool {
			return r.Header.Get("Origin") == domain
		}
	}
}

// WithEnableCompression 启用或禁用 WebSocket 压缩。
func WithEnableCompression(enable bool) Option {
	return func(s *Server) {
		s.upgrader.EnableCompression = enable
	}
}

// WithHandshakeTimeout 设置 WebSocket 握手超时时间。
func WithHandshakeTimeout(timeout time.Duration) Option {
	return func(s *Server) {
		s.upgrader.HandshakeTimeout = timeout
	}
}

// WithChannelBufferSize 设置每个会话发送通道的缓冲大小。
func WithChannelBufferSize(size int) Option {
	return func(_ *Server) {
		channelBufSize = size
	}
}

// WithPayloadType 设置消息载荷类型（二进制或文本）。
func WithPayloadType(payloadType PayloadType) Option {
	return func(s *Server) {
		s.payloadType = payloadType
	}
}

// WithCodec 设置编解码器名称（如 "json"、"proto" 等）。
// codec 会通过 encoding.GetCodec(name) 获取。
func WithCodec(name string) Option {
	return func(s *Server) {
		if name != "" {
			s.codec = encoding.GetCodec(name)
		}
	}
}

// WithInjectTokenToQuery 配置是否将 Sec-WebSocket-Protocol 中的 token
// 注入到查询参数中。
func WithInjectTokenToQuery(enable bool, tokenKey string) Option {
	return func(s *Server) {
		s.injectToken = enable
		s.tokenKey = tokenKey
	}
}

// WithMessageMarshaler 设置自定义的消息序列化函数。
func WithMessageMarshaler(m NetPacketMarshaler) Option {
	return func(s *Server) {
		s.netPacketMarshaler = m
	}
}

// WithMessageUnmarshaler 设置自定义的消息反序列化函数。
func WithMessageUnmarshaler(m NetPacketUnmarshaler) Option {
	return func(s *Server) {
		s.netPacketUnmarshaler = m
	}
}

// WithSocketConnectHandler 设置连接建立/断开时的回调函数。
func WithSocketConnectHandler(h SocketConnectHandler) Option {
	return func(s *Server) {
		s.socketConnectHandler = h
	}
}

// WithSocketRawDataHandler 设置自定义的原始数据处理函数。
// 设置后将绕过默认的类型分发逻辑。
func WithSocketRawDataHandler(h SocketRawDataHandler) Option {
	return func(s *Server) {
		if h != nil {
			s.socketRawDataHandler = h
		}
	}
}

// WithTimeout 设置会话空闲读超时；超时未收到任何消息即断开会话。0 表示不启用。
func WithTimeout(timeout time.Duration) Option {
	return func(s *Server) {
		s.timeout = timeout
	}
}

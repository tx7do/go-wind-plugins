// Package socketio provides a Socket.IO server that implements the
// [transport.Server] interface.
//
// It wraps the go-socket.io library with gorilla/mux routing and CORS support.
// The server lifecycle is managed via the standard Start/Stop pattern,
// making it compatible with [wind.App].
package socketio

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"sync"

	"github.com/tx7do/go-wind-plugins/encoding"
	"github.com/tx7do/go-wind/transport"

	socketIo "github.com/googollee/go-socket.io"
	"github.com/googollee/go-socket.io/engineio"
	socketIoTransport "github.com/googollee/go-socket.io/engineio/transport"
	"github.com/googollee/go-socket.io/engineio/transport/polling"
	"github.com/googollee/go-socket.io/engineio/transport/websocket"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

// KindSocketIo 是 Socket.IO 传输类型标识。
const KindSocketIo = "socket.io"

// 确保 Server 实现了 wind transport.Server 接口。
var _ transport.Server = (*Server)(nil)

type Server struct {
	*socketIo.Server

	lis     net.Listener
	tlsConf *tls.Config

	network string
	address string
	path    string

	codec encoding.Codec

	err error

	checkOrigin func(*http.Request) bool

	// handler 记录：socket.io Server Close 后不可复用，
	// 重启重建实例时按记录重放全部 handler 注册
	handlersMu    sync.Mutex
	Registrations []handlerRegistration
	closed        bool

	router      *mux.Router
	middlewares []Middleware
}

type handlerRegistration struct {
	kind      string // connect / disconnect / error / event
	namespace string
	event     string
	f         any
}

// createServer 用当前配置构造 socket.io 实例并重放已登记的 handler
func (s *Server) createServer() *socketIo.Server {
	server := socketIo.NewServer(&engineio.Options{
		Transports: []socketIoTransport.Transport{
			&polling.Transport{
				CheckOrigin: func(r *http.Request) bool { return s.checkOrigin(r) },
			},
			&websocket.Transport{
				CheckOrigin: func(r *http.Request) bool { return s.checkOrigin(r) },
			},
		},
	})

	s.handlersMu.Lock()
	for _, reg := range s.Registrations {
		switch reg.kind {
		case "connect":
			server.OnConnect(reg.namespace, reg.f.(func(socketIo.Conn) error))
		case "disconnect":
			server.OnDisconnect(reg.namespace, reg.f.(func(socketIo.Conn, string)))
		case "error":
			server.OnError(reg.namespace, reg.f.(func(socketIo.Conn, error)))
		case "event":
			server.OnEvent(reg.namespace, reg.event, reg.f)
		}
	}
	s.handlersMu.Unlock()

	return server
}

// Middleware 是标准 HTTP 中间件类型。
// 使用类型别名使得 transport/http/middleware 下的中间件可以直接复用。
type Middleware = func(http.Handler) http.Handler

func NewServer(opts ...Option) *Server {
	srv := &Server{
		network: "tcp",
		address: ":0",
		router:  mux.NewRouter(),
		path:    "/socket.io/",
	}

	srv.init(opts...)

	return srv
}

// Start 启动 Socket.IO 服务器，阻塞直到 ctx 被取消。
func (s *Server) Start(ctx context.Context) error {
	if s.err != nil {
		return s.err
	}

	lis, err := net.Listen(s.network, s.address)
	if err != nil {
		return err
	}
	s.lis = lis

	LogInfof("server listening on: %s", lis.Addr().String())

	// Close 后的 socket.io Server 不可复用（connChan 已关闭，新握手会
	// send-on-closed-channel panic）：重启时重建实例并重放 handler 注册
	// （路由上的委托 handler 会自动转发到新实例）
	if s.closed {
		s.Server = s.createServer()
		s.closed = false
	}

	go func() {
		if err := s.Server.Serve(); err != nil {
			LogErrorf("socketio serve error: %s", err.Error())
		}
	}()

	handler := handlers.CORS()(s.router)

	// 应用中间件链
	for i := len(s.middlewares) - 1; i >= 0; i-- {
		handler = s.middlewares[i](handler)
	}

	go func() {
		if s.tlsConf != nil {
			_ = http.ServeTLS(s.lis, handler, "", "")
		} else {
			_ = http.Serve(s.lis, handler)
		}
	}()

	// 阻塞等待 ctx 取消
	<-ctx.Done()

	_ = s.Server.Close()
	s.closed = true
	if s.lis != nil {
		_ = s.lis.Close()
		s.lis = nil
	}

	LogInfof("server stopped")
	return nil
}

// Stop 优雅关闭 Socket.IO 服务器。
func (s *Server) Stop(_ context.Context) error {
	err := s.Server.Close()
	s.closed = true
	if s.lis != nil {
		_ = s.lis.Close()
		s.lis = nil
	}
	LogInfof("server stopped")
	return err
}

// Endpoint 返回服务器的访问地址。
func (s *Server) Endpoint() string {
	var addr string
	if s.lis != nil {
		addr = s.lis.Addr().String()
	} else {
		addr = s.address
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return KindSocketIo + "://" + addr
	}
	if host == "" || host == "0.0.0.0" {
		host = "localhost"
	}
	return KindSocketIo + "://" + net.JoinHostPort(host, port)
}

// Use 注册全局标准 HTTP 中间件，对所有路由生效。
// 支持直接使用 transport/http/middleware 下的中间件。
// 必须在 Start 之前调用。
func (s *Server) Use(middlewares ...Middleware) {
	s.middlewares = append(s.middlewares, middlewares...)
}

func (s *Server) RegisterConnectHandler(namespace string, f func(socketIo.Conn) error) {
	s.recordHandler("connect", namespace, "", f)
	s.Server.OnConnect(namespace, f)
}

func (s *Server) RegisterDisconnectHandler(namespace string, f func(socketIo.Conn, string)) {
	s.recordHandler("disconnect", namespace, "", f)
	s.Server.OnDisconnect(namespace, f)
}

func (s *Server) RegisterErrorHandler(namespace string, f func(socketIo.Conn, error)) {
	s.recordHandler("error", namespace, "", f)
	s.Server.OnError(namespace, f)
}

func (s *Server) RegisterEventHandler(namespace string, event string, f any) {
	s.recordHandler("event", namespace, event, f)
	s.Server.OnEvent(namespace, event, f)
}

func (s *Server) recordHandler(kind, namespace, event string, f any) {
	s.handlersMu.Lock()
	defer s.handlersMu.Unlock()
	s.Registrations = append(s.Registrations, handlerRegistration{kind: kind, namespace: namespace, event: event, f: f})
}

func (s *Server) init(opts ...Option) {
	// 默认沿用旧行为（放行所有 Origin）；生产环境应通过 WithCheckOrigin 收紧
	if s.checkOrigin == nil {
		s.checkOrigin = func(r *http.Request) bool { return true }
	}

	// 必须先应用 opts 再创建 server：
	// Transport 的 CheckOrigin 在构造时捕获闭包，顺序反了会吞掉 WithCheckOrigin
	for _, o := range opts {
		o(s)
	}

	s.Server = s.createServer()
	if s.Server == nil {
		s.err = errors.New("create socket.io server failed")
		return
	}

	s.router.Use(mux.CORSMethodMiddleware(s.router))

	// 委托 handler：始终转发到当前 s.Server。
	// gorilla mux 的首条匹配路由生效且重复 Handle 不会覆盖旧路由，
	// 直接注册实例会导致重启后请求仍路由到已关闭的旧 server
	s.router.Handle(s.path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Server.ServeHTTP(w, r)
	}))
}

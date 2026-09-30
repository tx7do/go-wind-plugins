// Package websocket provides a WebSocket server that implements the
// [transport.Server] interface.
//
// The server wraps a standard [http.Server] with a [http.ServeMux] and uses
// [gorilla/websocket] for the WebSocket upgrade. It supports:
//   - Binary and text payload modes
//   - Message handler registration with type-based dispatch
//   - Session management with connect/disconnect callbacks
//   - Broadcast and targeted message sending
//   - Optional token injection via Sec-WebSocket-Protocol
//   - HTTP middleware (compatible with transport/http middleware)
//
// Usage:
//
//	import wsServer "github.com/tx7do/go-wind-plugins/transport/websocket"
//
//	srv := wsServer.NewServer(":8080",
//	    wsServer.WithPath("/ws"),
//	    wsServer.WithPayloadType(wsServer.PayloadTypeText),
//	)
//
//	// Register a message handler.
//	const MsgChat wsServer.NetMessageType = 1
//	srv.RegisterMessageHandler(MsgChat, func(sid wsServer.SessionID, msg wsServer.MessagePayload) error {
//	    fmt.Println("received:", msg)
//	    return nil
//	}, nil)
//
//	if err := srv.Start(ctx); err != nil { ... }
package websocket

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	ws "github.com/gorilla/websocket"

	"github.com/tx7do/go-wind-plugins/encoding"
	"github.com/tx7do/go-wind/transport"
)

// KindWebsocket 是 WebSocket 传输类型标识。
const KindWebsocket = "websocket"

// PayloadType 定义 WebSocket 消息的载荷格式。
type PayloadType uint8

const (
	// PayloadTypeBinary 二进制模式，使用 BinaryNetPacket 格式。
	PayloadTypeBinary PayloadType = 0
	// PayloadTypeText 文本模式，使用 JSON TextNetPacket 格式。
	PayloadTypeText PayloadType = 1
)

// 确保 Server 实现了 wind transport.Server 接口。
var _ transport.Server = (*Server)(nil)

// Middleware 是标准 HTTP 中间件类型。
// 使用类型别名使得 transport/http/middleware 下的中间件可以直接复用。
type Middleware = func(http.Handler) http.Handler

// Server 是 WebSocket 服务器，实现 transport.Server 接口。
type Server struct {
	addr      string
	tlsConfig *tls.Config
	listener  net.Listener
	mux       *http.ServeMux
	server    *http.Server

	middlewares []Middleware

	upgrader *ws.Upgrader

	path        string
	injectToken bool
	tokenKey    string

	sessionManager *SessionManager

	payloadType PayloadType

	messageHandlers NetMessageHandlerMap

	codec encoding.Codec

	netPacketMarshaler   NetPacketMarshaler
	netPacketUnmarshaler NetPacketUnmarshaler

	socketConnectHandler SocketConnectHandler
	socketRawDataHandler SocketRawDataHandler

	handlerMu sync.RWMutex

	timeout time.Duration
}

// NewServer 创建一个 WebSocket 服务器实例。
// addr 是监听地址（如 ":8080"）。
func NewServer(addr string, opts ...Option) *Server {
	srv := &Server{
		addr:        addr,
		path:        "/ws",
		injectToken: true,
		tokenKey:    "token",

		messageHandlers: make(NetMessageHandlerMap),

		sessionManager: NewSessionManager(nil),

		upgrader: &ws.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},

		payloadType: PayloadTypeBinary,

		mux: http.NewServeMux(),
	}

	srv.sessionManager.RegisterObserver(srv)

	for _, opt := range opts {
		opt(srv)
	}

	if srv.netPacketMarshaler == nil {
		srv.netPacketMarshaler = srv.defaultMarshalNetPacket
	}
	if srv.netPacketUnmarshaler == nil {
		srv.netPacketUnmarshaler = srv.defaultUnmarshalNetPacket
	}
	if srv.socketRawDataHandler == nil {
		srv.socketRawDataHandler = srv.defaultHandleSocketRawData
	}
	if srv.codec == nil {
		srv.codec = encoding.GetCodec("json")
	}

	// 注册 WebSocket 处理器到指定路径
	srv.mux.HandleFunc(srv.path, srv.wsHandler)

	return srv
}

// Start 启动 WebSocket 服务器，阻塞直到 ctx 被取消。
func (s *Server) Start(ctx context.Context) error {
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
		s.sessionManager.Clean()
		return s.server.Shutdown(context.Background())
	}
}

// Stop 优雅关闭服务器并清理所有会话。
func (s *Server) Stop(ctx context.Context) error {
	s.sessionManager.Clean()
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

// Endpoint 返回服务器的访问地址。
func (s *Server) Endpoint() string {
	scheme := "ws"
	if s.tlsConfig != nil {
		scheme = "wss"
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

// HandleFunc 注册普通 HTTP 处理器到指定路径。
func (s *Server) HandleFunc(path string, h http.HandlerFunc) {
	s.mux.HandleFunc(path, h)
}

// Use 注册全局中间件，对所有路由生效。必须在 Start 之前调用。
func (s *Server) Use(middlewares ...Middleware) {
	s.middlewares = append(s.middlewares, middlewares...)
}

// SessionCount 返回当前活跃会话数。
func (s *Server) SessionCount() int {
	return s.sessionManager.Count()
}

// ---------------------------------------------------------------------------
// 消息处理器注册
// ---------------------------------------------------------------------------

// RegisterMessageHandler 注册指定消息类型的处理器。
// binder 用于创建载荷实例以进行反序列化；如果为 nil 则直接传递原始字节。
func (s *Server) RegisterMessageHandler(messageType NetMessageType, handler NetMessageHandler, binder Creator) {
	s.handlerMu.Lock()
	defer s.handlerMu.Unlock()

	if _, ok := s.messageHandlers[messageType]; ok {
		return
	}

	s.messageHandlers[messageType] = &MessageHandlerData{
		Handler: handler,
		Creator: binder,
	}
}

// RegisterServerMessageHandler 是 RegisterMessageHandler 的泛型便捷方法。
// 它自动创建 T 类型的载荷实例并调用 handler。
func RegisterServerMessageHandler[T any](srv *Server, messageType NetMessageType, handler func(SessionID, *T) error) {
	srv.RegisterMessageHandler(messageType,
		func(sessionId SessionID, payload MessagePayload) error {
			switch t := payload.(type) {
			case *T:
				return handler(sessionId, t)
			default:
				return fmt.Errorf("invalid payload struct type: %T", t)
			}
		},
		func() any {
			var t T
			return &t
		},
	)
}

// DeregisterMessageHandler 注销指定消息类型的处理器。
func (s *Server) DeregisterMessageHandler(messageType NetMessageType) {
	s.handlerMu.Lock()
	defer s.handlerMu.Unlock()
	delete(s.messageHandlers, messageType)
}

// GetMessageHandler 返回指定消息类型的处理器数据。
func (s *Server) GetMessageHandler(messageType NetMessageType) *MessageHandlerData {
	s.handlerMu.RLock()
	defer s.handlerMu.RUnlock()
	return s.messageHandlers[messageType]
}

// ---------------------------------------------------------------------------
// 消息发送
// ---------------------------------------------------------------------------

// SendRawMessage 向指定会话发送原始字节消息。
func (s *Server) SendRawMessage(sessionId SessionID, message []byte) error {
	session := s.sessionManager.GetSession(sessionId)
	if session == nil {
		return errors.New("session not found: " + string(sessionId))
	}
	session.SendMessage(message)
	return nil
}

// SendMessage 向指定会话发送带类型的消息。
func (s *Server) SendMessage(sessionId SessionID, messageType NetMessageType, message MessagePayload) error {
	buf, err := s.marshalMessage(messageType, message)
	if err != nil {
		return err
	}
	return s.SendRawMessage(sessionId, buf)
}

// Broadcast 向所有活跃会话广播消息。
func (s *Server) Broadcast(messageType NetMessageType, message MessagePayload) {
	buf, err := s.marshalMessage(messageType, message)
	if err != nil {
		LogErrorf("broadcast marshal error: %v", err)
		return
	}

	s.sessionManager.RangeSessions(func(_ SessionID, session *Session) bool {
		session.SendMessage(buf)
		return true
	})
}

// ---------------------------------------------------------------------------
// SessionHooks 实现
// ---------------------------------------------------------------------------

func (s *Server) removeSession(session *Session) {
	s.sessionManager.RemoveSession(session)
}

func (s *Server) handleSocketRawData(sessionId SessionID, buf []byte) error {
	return s.socketRawDataHandler(sessionId, buf)
}

func (s *Server) getPayloadType() PayloadType {
	return s.payloadType
}

// ---------------------------------------------------------------------------
// SessionObserver 实现
// ---------------------------------------------------------------------------

// OnSessionAdded 在新会话建立时调用。
func (s *Server) OnSessionAdded(session *Session) {
	if s.socketConnectHandler != nil {
		s.socketConnectHandler(session.SessionID(), session.Queries(), true)
	}
}

// OnSessionRemoved 在会话断开时调用。
func (s *Server) OnSessionRemoved(session *Session) {
	if s.socketConnectHandler != nil {
		s.socketConnectHandler(session.SessionID(), session.Queries(), false)
	}
}

// ---------------------------------------------------------------------------
// WebSocket 处理器
// ---------------------------------------------------------------------------

func (s *Server) wsHandler(res http.ResponseWriter, req *http.Request) {
	var token string
	if req.Header != nil {
		token = req.Header.Get("Sec-Websocket-Protocol")
	}

	// 克隆 upgrader 以避免并发写入 Subprotocols
	upgrader := *s.upgrader
	if token != "" {
		upgrader.Subprotocols = []string{token}
	}

	conn, err := upgrader.Upgrade(res, req, nil)
	if err != nil {
		LogError("upgrade exception:", err)
		return
	}

	vars := req.URL.Query()

	if token != "" && s.injectToken {
		vars.Set(s.tokenKey, token)
	}

	session := NewSession(s, conn, vars)
	session.readTimeout = s.timeout
	s.sessionManager.AddSession(session)
	session.Listen()
}

// ---------------------------------------------------------------------------
// 内部方法：序列化/反序列化
// ---------------------------------------------------------------------------

func (s *Server) marshalMessage(messageType NetMessageType, message MessagePayload) ([]byte, error) {
	return s.netPacketMarshaler(messageType, message)
}

func (s *Server) defaultMarshalNetPacket(messageType NetMessageType, message MessagePayload) ([]byte, error) {
	payload, err := s.codec.Marshal(message)
	if err != nil {
		return nil, err
	}

	switch s.payloadType {
	case PayloadTypeBinary:
		var msg BinaryNetPacket
		msg.Type = messageType
		msg.Payload = payload
		return msg.Marshal()

	case PayloadTypeText:
		var msg TextNetPacket
		msg.Type = messageType
		msg.Payload = string(payload)
		return s.codec.Marshal(msg)
	}

	return nil, errors.New("unknown payload type")
}

func (s *Server) defaultUnmarshalNetPacket(buf []byte) (*MessageHandlerData, MessagePayload, error) {
	var messageType NetMessageType
	var rawPayload []byte

	switch s.payloadType {
	case PayloadTypeBinary:
		var msg BinaryNetPacket
		if err := msg.Unmarshal(buf); err != nil {
			return nil, nil, err
		}
		messageType = msg.Type
		rawPayload = msg.Payload

	case PayloadTypeText:
		var msg TextNetPacket
		if err := msg.Unmarshal(buf); err != nil {
			return nil, nil, err
		}
		messageType = msg.Type
		rawPayload = []byte(msg.Payload)
	}

	handler := s.GetMessageHandler(messageType)
	if handler == nil {
		return nil, nil, errors.New("message handler not found: " + fmt.Sprint(messageType))
	}

	var payload MessagePayload
	if payload = handler.Create(); payload == nil {
		payload = rawPayload
	} else {
		if err := s.codec.Unmarshal(rawPayload, payload); err != nil {
			return nil, nil, err
		}
	}

	return handler, payload, nil
}

func (s *Server) defaultHandleSocketRawData(sessionId SessionID, buf []byte) error {
	_, payload, err := s.defaultUnmarshalNetPacket(buf)
	if err != nil {
		return err
	}

	handler := s.GetMessageHandler(extractMessageType(buf, s.payloadType))
	if handler == nil {
		return errors.New("message handler not found")
	}

	return handler.Handler(sessionId, payload)
}

// extractMessageType 从原始数据中提取消息类型（用于 defaultHandleSocketRawData）。
func extractMessageType(buf []byte, payloadType PayloadType) NetMessageType {
	switch payloadType {
	case PayloadTypeBinary:
		var msg BinaryNetPacket
		_ = msg.Unmarshal(buf)
		return msg.Type
	case PayloadTypeText:
		var msg TextNetPacket
		_ = msg.Unmarshal(buf)
		return msg.Type
	}
	return 0
}

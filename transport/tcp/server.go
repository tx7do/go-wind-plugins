// Package tcp provides a TCP socket server that implements the
// [transport.Server] interface.
//
// It manages TCP connections with session-based lifecycle, message handler
// registration with type-based dispatch, and broadcast/targeted messaging.
// The server lifecycle is managed via the standard Start/Stop pattern,
// making it compatible with [wind.App].
//
// Usage:
//
//	import tcpServer "github.com/tx7do/go-wind-plugins/transport/tcp"
//
//	srv := tcpServer.NewServer(
//	    tcpServer.WithAddress(":8100"),
//	    tcpServer.WithCodec("json"),
//	)
//
//	tcpServer.RegisterServerMessageHandler(srv, 1, func(sid tcpServer.SessionID, msg *ChatMessage) error {
//	    srv.Broadcast(1, *msg)
//	    return nil
//	})
//
//	if err := srv.Start(ctx); err != nil { ... }
package tcp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/tx7do/go-utils/id"

	"github.com/tx7do/go-wind-plugins/encoding"
	"github.com/tx7do/go-wind-plugins/metrics"
	"github.com/tx7do/go-wind/transport"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// KindTcp 是 TCP 传输类型标识。
const KindTcp = "tcp"

// 确保 Server 实现了 wind transport.Server 接口。
var _ transport.Server = (*Server)(nil)

type Server struct {
	lis     net.Listener
	tlsConf *tls.Config

	network string
	address string

	timeout time.Duration

	codec encoding.Codec

	messageHandlers NetMessageHandlerMap

	socketConnectHandler SocketConnectHandler
	socketRawDataHandler SocketRawDataHandler

	netPacketMarshaler   NetPacketMarshaler
	netPacketUnmarshaler NetPacketUnmarshaler

	sessionManager *SessionManager

	tracer trace.Tracer
	m      metrics.Metrics

	running   bool
	stateMu   sync.RWMutex
	handlerMu sync.RWMutex
}

func NewServer(opts ...Option) *Server {
	srv := &Server{
		network: "tcp",
		address: ":0",
		// timeout 同时作为会话空闲读写超时；默认关闭（0），避免误杀长连接，
		// 需要回收半开连接时通过 WithTimeout 显式开启
		timeout: 0,

		messageHandlers: make(NetMessageHandlerMap),

		sessionManager: NewSessionManager(nil),
	}

	srv.sessionManager.RegisterObserver(srv)

	srv.init(opts...)

	return srv
}

func (s *Server) init(opts ...Option) {
	for _, o := range opts {
		o(s)
	}

	if s.netPacketMarshaler == nil {
		s.netPacketMarshaler = s.defaultMarshalNetPacket
	}
	if s.netPacketUnmarshaler == nil {
		s.netPacketUnmarshaler = s.defaultUnmarshalNetPacket
	}

	if s.socketRawDataHandler == nil {
		s.socketRawDataHandler = s.defaultHandleSocketRawData
	}

	if s.codec == nil {
		s.codec = encoding.GetCodec("json")
	}
}

// Start 启动 TCP 服务器，阻塞直到 ctx 被取消。
func (s *Server) Start(ctx context.Context) error {
	s.stateMu.Lock()
	if s.running {
		s.stateMu.Unlock()
		return nil
	}

	lis, err := net.Listen(s.network, s.address)
	if err != nil {
		s.stateMu.Unlock()
		return err
	}
	if s.tlsConf != nil {
		lis = tls.NewListener(lis, s.tlsConf)
	}
	s.lis = lis
	s.running = true
	s.stateMu.Unlock()

	LogInfof("server listening on: %s", lis.Addr().String())

	go s.doAccept()

	// 阻塞等待 ctx 取消
	<-ctx.Done()

	s.stateMu.Lock()
	s.running = false
	lis2 := s.lis
	s.lis = nil
	s.stateMu.Unlock()

	if lis2 != nil {
		_ = lis2.Close()
	}

	// 关闭全部会话：不关的话每连接读写 goroutine 在停机后继续存活
	if s.sessionManager != nil {
		s.sessionManager.rangeSessions(func(_ SessionID, session *Session) bool {
			if session != nil {
				session.Close()
			}
			return true
		})
	}

	LogInfof("server stopped")
	return nil
}

// Stop 优雅关闭 TCP 服务器。
func (s *Server) Stop(_ context.Context) error {
	s.stateMu.Lock()
	if !s.running {
		s.stateMu.Unlock()
		return nil
	}
	s.running = false
	s.stateMu.Unlock()

	var err error
	if s.lis != nil {
		err = s.lis.Close()
		s.lis = nil
	}

	// 关闭全部会话：不关的话每连接读写 goroutine 在 Stop 后继续存活
	if s.sessionManager != nil {
		s.sessionManager.rangeSessions(func(_ SessionID, session *Session) bool {
			if session != nil {
				session.Close()
			}
			return true
		})
	}

	LogInfof("server stopped")

	return err
}

// Endpoint 返回服务器的访问地址。
func (s *Server) Endpoint() string {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()

	var addr string
	if s.lis != nil {
		addr = s.lis.Addr().String()
	} else {
		addr = s.address
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return KindTcp + "://" + addr
	}
	if host == "" || host == "0.0.0.0" {
		host = "localhost"
	}
	return KindTcp + "://" + net.JoinHostPort(host, port)
}

// ---------------------------------------------------------------------------
// 消息处理器注册
// ---------------------------------------------------------------------------

func (s *Server) RegisterMessageHandler(messageType NetMessageType, handler NetMessageHandler, binder Creator) {
	s.handlerMu.Lock()
	defer s.handlerMu.Unlock()

	if _, ok := s.messageHandlers[messageType]; ok {
		return
	}

	s.messageHandlers[messageType] = MessageHandlerData{
		handler, binder,
	}
}

func RegisterServerMessageHandler[T any](srv *Server, messageType NetMessageType, handler func(SessionID, *T) error) {
	srv.RegisterMessageHandler(messageType,
		func(sessionId SessionID, payload NetMessagePayload) error {
			switch t := payload.(type) {
			case *T:
				return handler(sessionId, t)
			default:
				LogError("invalid payload struct type:", t)
				return errors.New("invalid payload struct type")
			}
		},
		func() any {
			var t T
			return &t
		},
	)
}

func (s *Server) DeregisterMessageHandler(messageType NetMessageType) {
	s.handlerMu.Lock()
	defer s.handlerMu.Unlock()

	delete(s.messageHandlers, messageType)
}

// GetMessageHandler find message handler
func (s *Server) GetMessageHandler(msgType NetMessageType) (*MessageHandlerData, error) {
	s.handlerMu.RLock()
	defer s.handlerMu.RUnlock()

	handlerData, ok := s.messageHandlers[msgType]
	if !ok {
		return nil, fmt.Errorf("[%d] message handler not found", msgType)
	}

	return &handlerData, nil
}

// ---------------------------------------------------------------------------
// 消息发送
// ---------------------------------------------------------------------------

// SendRawData send raw data to client
func (s *Server) SendRawData(sessionId SessionID, message []byte) error {
	session := s.sessionManager.getSession(sessionId)
	if session == nil {
		return fmt.Errorf("session not found: %s", sessionId)
	}

	session.SendMessage(message)
	return nil
}

func (s *Server) BroadcastRawData(message []byte) {
	s.sessionManager.rangeSessions(
		func(id SessionID, session *Session) bool {
			session.SendMessage(message)
			return false
		},
	)
}

func (s *Server) SendMessage(sessionId SessionID, messageType NetMessageType, message NetMessagePayload) error {
	buf, err := s.marshalNetPacket(messageType, message)
	if err != nil {
		return fmt.Errorf("marshal message exception: %w", err)
	}

	return s.SendRawData(sessionId, buf)
}

func (s *Server) Broadcast(messageType NetMessageType, message NetMessagePayload) {
	buf, err := s.marshalNetPacket(messageType, message)
	if err != nil {
		LogError("marshal message exception:", err)
		return
	}

	s.BroadcastRawData(buf)
}

// ---------------------------------------------------------------------------
// 内部方法
// ---------------------------------------------------------------------------

func (s *Server) marshalNetPacket(messageType NetMessageType, message NetMessagePayload) ([]byte, error) {
	return s.netPacketMarshaler(messageType, message)
}

func (s *Server) defaultMarshalNetPacket(messageType NetMessageType, message NetMessagePayload) ([]byte, error) {
	var msg NetPacket
	msg.Type = messageType
	var err error
	msg.Payload, err = s.codec.Marshal(message)
	if err != nil {
		return nil, err
	}
	return msg.Marshal()
}

func (s *Server) unmarshalNetPacket(buf []byte) (*MessageHandlerData, NetMessagePayload, error) {
	return s.netPacketUnmarshaler(buf)
}

func (s *Server) defaultUnmarshalNetPacket(buf []byte) (handler *MessageHandlerData, payload NetMessagePayload, err error) {
	var msg NetPacket
	if err = msg.Unmarshal(buf); err != nil {
		LogErrorf("decode message exception: %s", err)
		return
	}

	if handler, err = s.GetMessageHandler(msg.Type); err != nil {
		return
	}

	if payload = handler.Create(); payload == nil {
		payload = msg.Payload
	} else {
		if err = s.codec.Unmarshal(msg.Payload, payload); err != nil {
			LogErrorf("unmarshal message exception: %s", err)
			return
		}
	}

	return
}

func (s *Server) defaultHandleSocketRawData(sessionId SessionID, buf []byte) error {
	var span trace.Span
	var ctx context.Context

	if s.tracer != nil {
		ctx, span = s.tracer.Start(context.Background(), "tcp.message.receive",
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("rpc.system", "tcp"),
				attribute.String("tcp.session_id", string(sessionId)),
				attribute.Int("tcp.message_size", len(buf)),
			),
		)
		defer span.End()
	}

	start := time.Now()

	handler, payload, err := s.unmarshalNetPacket(buf)
	if err != nil {
		LogErrorf("unmarshal message failed: %s", err)
		if span != nil {
			span.SetStatus(codes.Error, err.Error())
		}
		if s.m != nil {
			s.m.Counter(ctx, "tcp_messages_total", 1, map[string]string{"status": "error"})
		}
		return err
	}

	if err = handler.Handler(sessionId, payload); err != nil {
		LogErrorf("message handler failed: %s", err)
		if span != nil {
			span.SetStatus(codes.Error, err.Error())
		}
		if s.m != nil {
			s.m.Counter(ctx, "tcp_messages_total", 1, map[string]string{"status": "error"})
		}
		return err
	}

	if s.m != nil {
		latency := time.Since(start).Seconds()
		s.m.Counter(ctx, "tcp_messages_total", 1, map[string]string{"status": "ok"})
		s.m.Histogram(ctx, "tcp_message_duration_seconds", latency, map[string]string{})
	}

	return nil
}

// doAccept accept connection handler
func (s *Server) doAccept() {
	for {
		s.stateMu.RLock()
		listener := s.lis
		running := s.running
		s.stateMu.RUnlock()

		if !running || listener == nil {
			return
		}

		conn, err := listener.Accept()
		if err != nil {
			s.stateMu.RLock()
			running = s.running
			s.stateMu.RUnlock()

			if !running || errors.Is(err, net.ErrClosed) {
				LogInfof("accept loop stopped: %s", err.Error())
				return
			}

			LogErrorf("accept connection failed: %s", err.Error())
			continue
		}

		if s.m != nil {
			s.m.Gauge(context.Background(), "tcp_connections_in_flight", 1, map[string]string{})
		}

		session := NewSession(conn, s)
		session.idleTimeout = s.timeout
		s.sessionManager.addSession(session)
		session.Listen()
	}
}

// removeSession removes a session from the manager, used by SessionHooks.
func (s *Server) removeSession(session *Session) {
	if s.sessionManager == nil {
		return
	}
	s.sessionManager.removeSession(session)
}

// handleSocketRawData dispatches raw message bytes, used by SessionHooks.
func (s *Server) handleSocketRawData(sessionId SessionID, buf []byte) error {
	if s.socketRawDataHandler != nil {
		return s.socketRawDataHandler(sessionId, buf)
	}
	return s.defaultHandleSocketRawData(sessionId, buf)
}

func (s *Server) OnSessionAdded(session *Session) {
	if s.socketConnectHandler != nil && session != nil {
		s.socketConnectHandler(session.SessionID(), true)
	}
}

func (s *Server) OnSessionRemoved(session *Session) {
	if s.socketConnectHandler != nil && session != nil {
		s.socketConnectHandler(session.SessionID(), false)
	}
	if s.m != nil {
		s.m.Gauge(context.Background(), "tcp_connections_in_flight", -1, map[string]string{})
	}
}

// SessionCount 返回当前活跃的会话数量。
func (s *Server) SessionCount() int {
	return s.sessionManager.count()
}

// 新增：供 session.go 使用
var _ = id.NewGUIDv4

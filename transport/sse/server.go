// Package sse provides a Server-Sent Events (SSE) server that implements the
// [transport.Server] interface.
//
// SSE is a standard HTTP-based push mechanism: the client opens a long-lived
// HTTP connection and the server pushes text events in a simple
// "field: value\n" wire format. The server supports multiple named streams,
// subscriber management, event replay (via Last-Event-ID), and optional
// authentication.
//
// Since SSE runs over HTTP, this server wraps a standard [http.Server] with a
// [http.ServeMux] and supports the same [Middleware] type as transport/http,
// allowing middleware (recovery, logging, request-id, etc.) to be shared.
//
// Usage:
//
//	import sseServer "github.com/tx7do/go-wind-plugins/transport/sse"
//
//	srv := sseServer.NewServer(":8080",
//	    sseServer.WithPath("/events"),
//	)
//
//	// Create a named stream.
//	srv.CreateStream("notifications")
//
//	// Publish events.
//	srv.Publish(context.Background(), "notifications", &sseServer.Event{
//	    Data: []byte("hello"),
//	})
//
//	if err := srv.Start(ctx); err != nil { ... }
package sse

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/tx7do/go-wind-plugins/encoding"
	"github.com/tx7do/go-wind/transport"
)

// KindSSE 是 SSE 传输类型标识。
const KindSSE = "sse"

// DefaultBufferSize 是每个流事件通道的默认缓冲大小。
const DefaultBufferSize = 1024

// 确保 Server 实现了 wind transport.Server 接口。
var _ transport.Server = (*Server)(nil)

// Middleware 是标准 HTTP 中间件类型。
// 使用类型别名（而非命名类型）使得 transport/http/middleware 下的中间件
// 可以直接传给 SSE server，无需类型转换。
type Middleware = func(http.Handler) http.Handler

// MessagePayload 表示可以被编码为 Event 的任意数据。
type MessagePayload any

// Server 是 SSE 服务器，实现 transport.Server 接口。
type Server struct {
	addr      string
	tlsConfig *tls.Config
	listener  net.Listener
	mux       *http.ServeMux
	server    *http.Server

	middlewares []Middleware

	// SSE 配置
	path        string
	streamIdKey string

	headers    map[string]string
	eventTTL   time.Duration
	bufferSize int

	encodeBase64 bool
	splitData    bool
	autoStream   bool
	autoReplay   bool

	corsAllowOrigin string

	codec encoding.Codec

	subscribeFunc   SubscriberFunction
	unsubscribeFunc SubscriberFunction
	authorizeFunc   AuthorizeFunc
	tokenExtractor  TokenExtractor

	streamMgr *StreamManager
}

// NewServer 创建一个 SSE 服务器实例。
// addr 是监听地址（如 ":8080"）。
func NewServer(addr string, opts ...Option) *Server {
	srv := &Server{
		addr:            addr,
		mux:             http.NewServeMux(),
		path:            "/events",
		streamIdKey:     "stream",
		bufferSize:      DefaultBufferSize,
		corsAllowOrigin: "*",
		autoReplay:      true,
		headers:         map[string]string{},
		tokenExtractor:  DefaultTokenExtractor,
		streamMgr:       NewStreamManager(),
	}
	for _, opt := range opts {
		opt(srv)
	}
	if srv.codec == nil {
		srv.codec = encoding.GetCodec("json")
	}
	// 注册 SSE 处理器到指定路径
	srv.mux.HandleFunc(srv.path, srv.ServeHTTP)
	return srv
}

// Start 启动 SSE 服务器，阻塞直到 ctx 被取消。
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

	fmt.Printf("[%s] server listening on: %s\n", KindSSE, ln.Addr().String())

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
		s.streamMgr.Clean()
		return s.server.Shutdown(context.Background())
	}
}

// Stop 优雅关闭服务器并清理所有流。
func (s *Server) Stop(ctx context.Context) error {
	s.streamMgr.Clean()
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

// Endpoint 返回服务器的访问地址。
func (s *Server) Endpoint() string {
	scheme := "http"
	if s.tlsConfig != nil {
		scheme = "https"
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

// Use 注册全局中间件，对所有路由生效。
// 必须在 Start 之前调用。
func (s *Server) Use(middlewares ...Middleware) {
	s.middlewares = append(s.middlewares, middlewares...)
}

// ---------------------------------------------------------------------------
// Stream 管理
// ---------------------------------------------------------------------------

// CreateStream 创建一个新流，如果已存在则返回已有流。
func (s *Server) CreateStream(streamId StreamID) *Stream {
	stream := s.streamMgr.Get(streamId)
	if stream != nil {
		return stream
	}

	stream = s.createStream(streamId)
	s.streamMgr.Add(stream)
	return stream
}

// RemoveStream 关闭并移除指定流。
func (s *Server) RemoveStream(streamId StreamID) {
	s.streamMgr.RemoveWithID(streamId)
}

// GetStream 返回指定 ID 的流，不存在返回 nil。
func (s *Server) GetStream(streamId StreamID) *Stream {
	return s.streamMgr.Get(streamId)
}

// StreamCount 返回当前活跃流的数量。
func (s *Server) StreamCount() int {
	return s.streamMgr.Count()
}

func (s *Server) createStream(streamId StreamID) *Stream {
	stream := newStream(streamId, s.bufferSize, s.autoReplay, s.autoStream, s.subscribeFunc, s.unsubscribeFunc)
	stream.run()
	return stream
}

// ---------------------------------------------------------------------------
// 事件发布
// ---------------------------------------------------------------------------

// Publish 向指定流推送一个事件。如果流不存在则忽略。
func (s *Server) Publish(_ context.Context, streamId StreamID, event *Event) {
	stream := s.streamMgr.Get(streamId)
	if stream == nil {
		return
	}

	select {
	case <-stream.quit:
	case stream.event <- s.process(event):
	}
}

// TryPublish 尝试向指定流推送事件，不阻塞。成功返回 true。
func (s *Server) TryPublish(_ context.Context, streamId StreamID, event *Event) bool {
	stream := s.streamMgr.Get(streamId)
	if stream == nil {
		return false
	}

	select {
	case stream.event <- s.process(event):
		return true
	default:
		return false
	}
}

// PublishData 将任意数据编码为 Event 并发布到指定流。
func (s *Server) PublishData(ctx context.Context, streamId StreamID, data MessagePayload) error {
	event, err := s.marshalEvent(data)
	if err != nil {
		return err
	}
	s.Publish(ctx, streamId, event)
	return nil
}

// PublishDataWithEventName 编码数据，设置事件名，并发布到指定流。
func (s *Server) PublishDataWithEventName(ctx context.Context, streamId StreamID, eventName string, data MessagePayload) error {
	return s.PublishDataWithMeta(ctx, streamId, data, WithEventName(eventName))
}

// PublishDataWithMeta 编码数据，应用元数据选项，并发布到指定流。
func (s *Server) PublishDataWithMeta(ctx context.Context, streamId StreamID, data MessagePayload, opts ...EventMetaOption) error {
	event, err := s.marshalEvent(data)
	if err != nil {
		return err
	}
	for _, o := range opts {
		o(event)
	}
	s.Publish(ctx, streamId, event)
	return nil
}

// Notify 向所有流广播一个事件。
func (s *Server) Notify(_ context.Context, event *Event) {
	s.streamMgr.Range(func(stream *Stream) {
		if stream == nil {
			return
		}
		select {
		case <-stream.quit:
		case stream.event <- s.process(event):
		}
	})
}

// NotifyData 编码数据并向所有流广播。
func (s *Server) NotifyData(ctx context.Context, data MessagePayload) error {
	event, err := s.marshalEvent(data)
	if err != nil {
		return err
	}
	s.Notify(ctx, event)
	return nil
}

// NotifyDataWithEventName 编码数据，设置事件名，并向所有流广播。
func (s *Server) NotifyDataWithEventName(ctx context.Context, eventName string, data MessagePayload) error {
	return s.NotifyDataWithMeta(ctx, data, WithEventName(eventName))
}

// NotifyDataWithMeta 编码数据，应用元数据选项，并向所有流广播。
func (s *Server) NotifyDataWithMeta(ctx context.Context, data MessagePayload, opts ...EventMetaOption) error {
	event, err := s.marshalEvent(data)
	if err != nil {
		return err
	}
	for _, o := range opts {
		o(event)
	}
	s.Notify(ctx, event)
	return nil
}

// ---------------------------------------------------------------------------
// 内部方法
// ---------------------------------------------------------------------------

// process 在事件投递前应用服务器端的转换。
func (s *Server) process(event *Event) *Event {
	// 同一 Event 指针可能分发给多个 stream，原地编码会导致
	// 第 2 个及以后的流收到双重 base64 数据，必须返回副本
	clone := *event
	// 时间戳在分发阶段统一赋值：
	// 此前只在 EventLog.Add（autoReplay 路径）里赋，eventTTL + autoReplay=false
	// 或纯 Comment 事件会因零值时间戳被 TTL 过滤全部丢弃
	if clone.timestamp.IsZero() {
		clone.timestamp = time.Now()
	}
	if s.encodeBase64 {
		clone.encodeBase64()
	}
	return &clone
}

// marshalEvent 将任意 payload 转换为 Event。
func (s *Server) marshalEvent(data MessagePayload) (*Event, error) {
	event := &Event{}
	if data != nil {
		var err error
		event.Data, err = s.codec.Marshal(data)
		if err != nil {
			return nil, err
		}
	}
	return event, nil
}

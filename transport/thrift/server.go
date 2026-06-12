// Package thrift provides a Thrift RPC server that implements the
// [transport.Server] interface.
//
// It wraps Apache Thrift's [thrift.TSimpleServer] with configurable protocol
// (binary, compact, json) and transport (buffered, framed) options. The server
// lifecycle is managed via the standard Start/Stop pattern.
//
// Cross-cutting concerns (tracing, logging, recovery, metrics) are added via
// [ProcessorWrapper] functions applied in the order they are given:
//
//	srv := thrift.NewServer(":7700",
//	    thrift.WithProcessor(processor),
//	    thrift.WithRecovery(nil),    // outermost: catches panics
//	    thrift.WithLogging(nil),     // logs every RPC
//	    thrift.WithTracerProvider(), // creates OTel spans
//	)
//
//	if err := srv.Start(ctx); err != nil { ... }
package thrift

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"

	"github.com/apache/thrift/lib/go/thrift"

	"github.com/tx7do/go-wind/transport"
)

const KindThrift = "thrift"

var (
	ErrInvalidProtocol  = errors.New("thrift: invalid protocol")
	ErrInvalidTransport = errors.New("thrift: invalid transport")
	ErrNoProcessor      = errors.New("thrift: no processor set, use WithProcessor() to specify one")
)

// 确保 Server 实现了 wind transport.Server 接口。
var _ transport.Server = (*Server)(nil)

// Server 是基于 Apache Thrift 的 RPC 服务器。
type Server struct {
	addr       string
	tlsConfig  *tls.Config
	processor  thrift.TProcessor
	protocol   string
	buffered   bool
	framed     bool
	bufferSize int

	wrappers []ProcessorWrapper

	server   *thrift.TSimpleServer
	listener net.Listener
}

// NewServer 创建一个 Thrift 服务器实例。
func NewServer(addr string, opts ...Option) *Server {
	srv := &Server{
		addr:       addr,
		protocol:   "binary",
		bufferSize: 8192,
	}
	for _, opt := range opts {
		opt(srv)
	}

	// Apply processor wrappers (if any) in order.
	// The first wrapper is outermost: it runs first on the way in.
	if srv.processor != nil {
		for i := len(srv.wrappers) - 1; i >= 0; i-- {
			srv.processor = srv.wrappers[i](srv.processor)
		}
	}

	return srv
}

// Start 启动 Thrift 服务器，阻塞直到 ctx 被取消。
func (s *Server) Start(ctx context.Context) error {
	if s.processor == nil {
		return ErrNoProcessor
	}

	protocolFactory := createProtocolFactory(s.protocol)
	if protocolFactory == nil {
		return ErrInvalidProtocol
	}

	cfg := &thrift.TConfiguration{
		TLSConfig: &tls.Config{InsecureSkipVerify: true},
	}

	transportFactory := createTransportFactory(cfg, s.buffered, s.framed, s.bufferSize)
	if transportFactory == nil {
		return ErrInvalidTransport
	}

	serverTransport, err := createServerTransport(s.addr, s.tlsConfig)
	if err != nil {
		return err
	}

	s.server = thrift.NewTSimpleServer4(s.processor, serverTransport, transportFactory, protocolFactory)

	fmt.Printf("[%s] server listening on: %s\n", KindThrift, s.addr)

	// 在 goroutine 中 Serve，通过 ctx 实现优雅关闭
	errChan := make(chan error, 1)
	go func() {
		errChan <- s.server.Serve()
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return s.server.Stop()
	}
}

// Stop 停止 Thrift 服务器。
func (s *Server) Stop(_ context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Stop()
}

// Endpoint 返回服务器的访问地址。
func (s *Server) Endpoint() string {
	addr := s.addr
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return KindThrift + "://" + addr
	}
	if host == "" || host == "0.0.0.0" {
		host = "localhost"
	}
	return KindThrift + "://" + net.JoinHostPort(host, port)
}

// Addr 返回配置的监听地址。
func (s *Server) Addr() string { return s.addr }

// ---------------------------------------------------------------------------
// Factory functions
// ---------------------------------------------------------------------------

// createProtocolFactory 根据协议名称创建 TProtocolFactory。
func createProtocolFactory(protocol string) thrift.TProtocolFactory {
	switch protocol {
	case "compact":
		return thrift.NewTCompactProtocolFactoryConf(nil)
	case "simplejson":
		return thrift.NewTSimpleJSONProtocolFactoryConf(nil)
	case "json":
		return thrift.NewTJSONProtocolFactory()
	case "binary", "":
		return thrift.NewTBinaryProtocolFactoryConf(nil)
	default:
		return nil
	}
}

// createTransportFactory 根据配置创建 TTransportFactory。
func createTransportFactory(cfg *thrift.TConfiguration, buffered, framed bool, bufferSize int) thrift.TTransportFactory {
	var transportFactory thrift.TTransportFactory
	if buffered {
		transportFactory = thrift.NewTBufferedTransportFactory(bufferSize)
	} else {
		transportFactory = thrift.NewTTransportFactory()
	}
	if framed {
		transportFactory = thrift.NewTFramedTransportFactoryConf(transportFactory, cfg)
	}
	return transportFactory
}

// createServerTransport 创建服务器传输层。
func createServerTransport(addr string, tlsConf *tls.Config) (thrift.TServerTransport, error) {
	if tlsConf != nil {
		return thrift.NewTSSLServerSocket(addr, tlsConf)
	}
	return thrift.NewTServerSocket(addr)
}

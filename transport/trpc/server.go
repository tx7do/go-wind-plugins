package trpc

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	trpcGo "trpc.group/trpc-go/trpc-go"
	"trpc.group/trpc-go/trpc-go/filter"
	trpcServer "trpc.group/trpc-go/trpc-go/server"
)

// ServerFilter 是 tRPC 服务端过滤器（拦截器）的类型别名。
// 用户可通过 WithFilter 或 Use 注入自定义拦截逻辑（鉴权、日志、指标等）。
type ServerFilter = filter.ServerFilter

// Server 是 tRPC 服务端的封装，提供统一的生命周期管理、
// 服务注册和过滤器链等能力。
type Server struct {
	Server *trpcServer.Server

	trpcOptions []trpcServer.Option
	address     string

	filters []ServerFilter

	sync.RWMutex
	started atomic.Bool

	baseCtx context.Context
	err     error
}

func NewServer(opts ...ServerOption) *Server {
	srv := &Server{
		baseCtx: context.Background(),
	}

	srv.init(opts...)

	return srv
}

func (s *Server) init(opts ...ServerOption) {
	for _, o := range opts {
		o(s)
	}
}

func (s *Server) Name() string {
	return KindTRPC
}

func (s *Server) Endpoint() string {
	return s.address
}

// Use 注册服务端过滤器，按注册顺序执行。必须在 Start 之前调用。
func (s *Server) Use(filters ...ServerFilter) {
	s.filters = append(s.filters, filters...)
}

// Start 启动 tRPC 服务端。
// 首次调用时会创建底层 tRPC Server 并开始监听；重复调用返回 nil。
func (s *Server) Start(ctx context.Context) error {
	if s.err != nil {
		return s.err
	}

	if s.started.Load() {
		return nil
	}

	// 将通过 Use 注册的过滤器注入到 tRPC 选项
	if len(s.filters) > 0 {
		s.trpcOptions = append(s.trpcOptions, trpcServer.WithFilters(s.filters))
	}

	s.Server = trpcGo.NewServer(s.trpcOptions...)
	if s.Server == nil {
		return fmt.Errorf("failed to create trpc server")
	}

	if s.err = s.Server.Serve(); s.err != nil {
		LogErrorf("server serve error: %v", s.err)
		return s.err
	}

	s.baseCtx = ctx
	s.started.Store(true)

	LogInfof("server listening on: %s", s.address)

	return nil
}

// Stop 优雅停止 tRPC 服务端。
func (s *Server) Stop(ctx context.Context) error {
	if !s.started.Load() {
		return nil
	}

	LogInfo("server stopping...")

	s.started.Store(false)
	err := s.Server.Close(nil)
	s.err = nil

	LogInfo("server stopped.")

	return err
}

// RegisterService 注册一个 tRPC 服务。
// serviceName 为服务名称，serviceDesc 为 protobuf 生成的服务描述，serviceImpl 为服务实现。
// 必须在 Start 之前调用。
func (s *Server) RegisterService(serviceName string, serviceDesc interface{}, serviceImpl interface{}) error {
	s.Lock()
	defer s.Unlock()

	if s.started.Load() {
		return fmt.Errorf("cannot register service %q after server started", serviceName)
	}

	if s.Server == nil {
		// 尚未 Start，先创建 Server 以便注册服务
		s.Server = trpcGo.NewServer(s.trpcOptions...)
		if s.Server == nil {
			return fmt.Errorf("failed to create trpc server for service registration")
		}
	}

	svc := s.Server.Service(serviceName)
	if err := svc.Register(serviceDesc, serviceImpl); err != nil {
		return fmt.Errorf("register service %q failed: %w", serviceName, err)
	}

	return nil
}

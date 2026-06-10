package trpc

import (
	"context"
	"fmt"

	trpcGo "trpc.group/trpc-go/trpc-go"
	trpcServer "trpc.group/trpc-go/trpc-go/server"
)

type Server struct {
	Server *trpcServer.Server

	trpcOptions []trpcServer.Option
	address     string

	err error
}

func NewServer(opts ...ServerOption) *Server {
	srv := &Server{}

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

func (s *Server) Start(_ context.Context) error {
	s.Server = trpcGo.NewServer(s.trpcOptions...)
	if s.Server == nil {
		return fmt.Errorf("failed to create trpc server")
	}

	if s.err = s.Server.Serve(); s.err != nil {
		LogErrorf("server serve error: %v", s.err)
		return s.err
	}

	return nil
}

func (s *Server) Stop(_ context.Context) error {
	LogInfo("server stopping...")

	err := s.Server.Close(nil)

	LogInfo("server stopped.")

	return err
}

func (s *Server) AddService(serviceName string, service trpcServer.Service) {
	if s.Server == nil {
		LogErrorf("server is not initialized, cannot add service %s", serviceName)
		return
	}

	s.Server.AddService(serviceName, service)
}

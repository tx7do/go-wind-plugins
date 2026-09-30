package mcp

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/mark3labs/mcp-go/server"
)

const (
	DefaultMCPServerName    = "MCP Server"
	DefaultMCPServerVersion = "1.0.0"
	DefaultMCPServerAddress = ":8080"
)

type Server struct {
	mu      sync.RWMutex
	started atomic.Bool

	baseCtx context.Context
	err     error

	serverName    string
	serverVersion string

	mcpServer  *server.MCPServer
	sseServer   *server.SSEServer
	httpServer  *server.StreamableHTTPServer
	endpoint    *url.URL

	mcpOpts []server.ServerOption

	serverType ServerType
	serverAddr string
}

func NewServer(opts ...ServerOption) *Server {
	srv := &Server{
		baseCtx:       context.Background(),
		started:       atomic.Bool{},
		serverType:    ServerTypeStdio,
		serverVersion: DefaultMCPServerVersion,
		serverName:    DefaultMCPServerName,
		serverAddr:    DefaultMCPServerAddress,
	}

	srv.init(opts...)

	return srv
}

func (s *Server) init(opts ...ServerOption) {
	for _, o := range opts {
		o(s)
	}

	switch s.serverType {
	case ServerTypeSSE, ServerTypeHTTP:
		LogInfof("MCP server type set to %s, address: %s", s.serverType, s.serverAddr)
	case ServerTypeInProcess:
		LogInfo("MCP server type set to IN_PROCESS")
	case ServerTypeStdio:
		LogInfo("MCP server type set to STDIO")
	default:
		LogWarnf("Unsupported MCP server type: %s, defaulting to STDIO", s.serverType)
		s.serverType = ServerTypeStdio
	}

	if s.endpoint == nil {
		host := s.serverAddr
		if host == "" {
			host = DefaultMCPServerAddress
		}
		if strings.HasPrefix(host, ":") {
			host = "localhost" + host
		}
		s.endpoint = &url.URL{
			Scheme: "http",
			Host:   host,
		}
	}

	// Create a new MCP server
	s.mcpServer = server.NewMCPServer(s.serverName, s.serverVersion, s.mcpOpts...)
}

func (s *Server) Name() string {
	return KindMCP
}

func (s *Server) Start(ctx context.Context) error {
	s.mu.RLock()
	if s.err != nil {
		e := s.err
		s.mu.RUnlock()
		return e
	}
	s.mu.RUnlock()

	if s.started.Load() {
		LogWarn("MCP server already started")
		return nil
	}

	// Start the MCP server
	go func() {
		if err := s.startMCPServer(); err != nil {
			s.setErr(err)
		}
	}()

	s.baseCtx = ctx
	s.started.Store(true)

	LogInfof("MCP server started, [%s][%s]", s.serverName, s.serverVersion)

	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	if !s.started.Load() {
		LogWarn("MCP server already stopped")
		return nil
	}

	LogInfof("MCP server stopping, name: %s", s.serverName)

	s.started.Store(false)

	// 停掉 SSE/HTTP 服务本体：否则 Stop 后端口仍被占用，
	// 再次 Start 会在同地址起第二个实例
	s.mu.Lock()
	sseServer := s.sseServer
	httpServer := s.httpServer
	s.sseServer = nil
	s.httpServer = nil
	s.setErrLocked(nil) // 清 sticky err，避免瞬时错误导致后续 Start 永久失败
	s.mu.Unlock()

	if sseServer != nil {
		if err := sseServer.Shutdown(ctx); err != nil {
			LogErrorf("sse server shutdown failed: %s", err.Error())
		}
	}
	if httpServer != nil {
		if err := httpServer.Shutdown(ctx); err != nil {
			LogErrorf("http server shutdown failed: %s", err.Error())
		}
	}

	s.mu.RLock()
	err := s.err
	s.mu.RUnlock()

	if err != nil {
		LogError("server stopped with error", err)
	} else {
		LogInfo("server stopped.")
	}

	return err
}

func (s *Server) Endpoint() string {
	if s.endpoint == nil {
		return ""
	}
	return s.endpoint.String()
}

func (s *Server) RegisterHandler(tool mcp.Tool, handler server.ToolHandlerFunc) error {
	if s.mcpServer == nil {
		return errors.New("mcp server is nil")
	}

	s.mcpServer.AddTool(tool, handler)

	return nil
}

func (s *Server) RegisterHandlerWithJsonString(jsonString string, handler server.ToolHandlerFunc) error {
	if s.mcpServer == nil {
		return errors.New("mcp server is nil")
	}

	tool, err := LoadToolFromJsonString(jsonString)
	if err != nil {
		return err
	}

	return s.RegisterHandler(tool, handler)
}

func (s *Server) RegisterHandlerWithJsonSchema(name, description string, jsonSchemaString string, handler server.ToolHandlerFunc) error {
	if s.mcpServer == nil {
		return errors.New("mcp server is nil")
	}

	raw := toRawMessage(jsonSchemaString)

	tool := mcp.NewToolWithRawSchema(name, description, raw)

	return s.RegisterHandler(tool, handler)
}

func (s *Server) startMCPServer() error {
	if s.mcpServer == nil {
		return errors.New("MCP server instance is nil")
	}

	switch s.serverType {
	case ServerTypeStdio:
		if err := server.ServeStdio(s.mcpServer); err != nil {
			LogErrorf("MCP server start failed: %s", err.Error())
			return errors.New("start MCP server: " + err.Error())
		}

	case ServerTypeSSE:
		sseServer := server.NewSSEServer(s.mcpServer)
		// 保存句柄供 Stop 关闭（此前是局部变量，Stop 后端口仍在服务）
		s.sseServer = sseServer
		if err := sseServer.Start(s.serverAddr); err != nil {
			s.sseServer = nil
			// 不能用 Fatalf：会 os.Exit 杀死整个应用进程
			LogErrorf("MCP server start failed: %s", err.Error())
			return errors.New("start MCP server: " + err.Error())
		}

	case ServerTypeHTTP:
		httpServer := server.NewStreamableHTTPServer(s.mcpServer)
		s.httpServer = httpServer
		if err := httpServer.Start(s.serverAddr); err != nil {
			s.httpServer = nil
			LogErrorf("MCP server start failed: %s", err.Error())
			return errors.New("start MCP server: " + err.Error())
		}

	case ServerTypeInProcess:

	default:
		return errors.New("unsupported MCP server type: " + string(s.serverType))
	}

	return nil
}

func (s *Server) waitGroup(wg *sync.WaitGroup, ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) setErr(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	s.err = errors.Join(s.err, err)
	s.mu.Unlock()
}

// setErrLocked 在已持锁情况下设置/清空错误（nil 清空 sticky err，避免 Start 永久失败）
func (s *Server) setErrLocked(err error) {
	s.err = err
}

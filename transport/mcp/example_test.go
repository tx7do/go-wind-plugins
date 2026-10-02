package mcp_test

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"

	mcptransport "github.com/tx7do/go-wind-plugins/transport/mcp"
)

func handleEcho(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	value, err := request.RequireString("text")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(value), nil
}

// ExampleNewServer constructs an MCP server in streamable HTTP mode and
// registers a tool together with its handler. Register the server with the
// application's transport lifecycle; MCP clients can then discover and invoke
// the registered tool at the advertised endpoint.
func ExampleNewServer() {
	srv := mcptransport.NewServer(
		mcptransport.WithServerName("Echo Demo"),
		mcptransport.WithServerVersion("1.0.0"),
		mcptransport.WithMCPServeType(mcptransport.ServerTypeHTTP),
		mcptransport.WithMCPServeAddress("localhost:8080"),
	)

	tool := mcp.NewTool("echo",
		mcp.WithDescription("Echo the provided text back to the caller"),
		mcp.WithString("text",
			mcp.Required(),
			mcp.Description("The text to echo back"),
		),
	)

	if err := srv.RegisterHandler(tool, handleEcho); err != nil {
		return
	}
}

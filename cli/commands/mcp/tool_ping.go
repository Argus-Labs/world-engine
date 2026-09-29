package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerPingTool registers the ping health check tool.
// Agents use this to verify the MCP server is reachable.
func registerPingTool(srv *server.MCPServer) {
	pingTool := mcp.NewTool(
		"ping",
		mcp.WithDescription("Simple health check for the Cardinal MCP server"),
	)
	srv.AddTool(pingTool, pingHandler)
}

// pingHandler implements the "ping" tool. It is intentionally trivial and
// just proves the MCP server is wired up correctly.
func pingHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText("cardinal-mcp is running"), nil
}

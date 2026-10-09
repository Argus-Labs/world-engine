package mcp

import (
	"github.com/mark3labs/mcp-go/server"
)

// ShardInstance describes one container (instance) of a shard pool.
type ShardInstance struct {
	Name         string `json:"name"                    jsonschema_description:"Instance name ('gameplay', or 'gameplay-2' … for pool_size>1)"`
	Container    string `json:"container,omitempty"     jsonschema_description:"Docker container name"`
	Phase        string `json:"phase,omitempty"         jsonschema_description:"Container state: running, exited, created, etc."`
	Ready        bool   `json:"ready"                   jsonschema_description:"Whether the shard accepts connections"`
	RestartCount int32  `json:"restart_count,omitempty" jsonschema_description:"Container restart count"`
	Age          string `json:"age,omitempty"           jsonschema_description:"How long the container has been running"`
}

// ShardPool describes a shard's pool of containers.
type ShardPool struct {
	ShardID   string          `json:"shard_id"            jsonschema_description:"Shard identifier from world.toml (e.g. 'gameplay')"`
	PoolSize  int32           `json:"pool_size,omitempty" jsonschema_description:"Number of instances in the pool"`
	Image     string          `json:"image,omitempty"     jsonschema_description:"Image the instances run"`
	Phase     string          `json:"phase,omitempty"     jsonschema_description:"Pool phase: Running, Partial, Stopped or NotDeployed"`
	Instances []ShardInstance `json:"instances"           jsonschema_description:"Per-container status"`
}

// NewServer creates and configures the Cardinal MCP server with all tools
// registered. The returned *server.MCPServer is transport-agnostic: callers
// choose how to serve it (stdio for the world-cli command, or an HTTP transport
// when hosted from a long-running app such as the Cardinal Editor).
func NewServer() *server.MCPServer {
	// Create the MCP server that will be launched by an MCP-capable client
	// (Claude, Cursor, etc.) as the "cardinal" tool server.
	srv := server.NewMCPServer(
		"cardinal-mcp",
		"0.1.0",
		// Enable tool capabilities.
		server.WithToolCapabilities(true),
	)

	// Register all tools
	registerPingTool(srv)
	registerDescribeWorldTool(srv)
	registerWorldStatusTool(srv)
	registerInspectShardTool(srv)
	registerGetShardLogsTool(srv)
	registerGetNatsLogsTool(srv)

	// Register RPC tools for interacting with Cardinal shards
	registerSendCommandTool(srv)
	registerIntrospectTool(srv)
	registerGetStateTool(srv)
	registerDebugControlTool(srv)
	registerReloadTool(srv)
	registerWorldLifecycleTool(srv)
	registerSdkGenerateTool(srv)

	return srv
}

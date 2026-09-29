package mcp

import (
	"github.com/mark3labs/mcp-go/server"
)

// ShardInstance describes one running pod (instance) of a shard pool.
type ShardInstance struct {
	Name         string `json:"name"                    jsonschema_description:"Operator instance name ('gameplay', or 'gameplay-2' … for pool_size>1)"`
	PodName      string `json:"pod_name,omitempty"      jsonschema_description:"Kubernetes pod name (ephemeral; changes on rolling deploy)"`
	Phase        string `json:"phase,omitempty"         jsonschema_description:"Pod phase: Running, Pending, Failed, etc."`
	Ready        bool   `json:"ready"                   jsonschema_description:"Whether the pod passes its readiness checks"`
	RestartCount int32  `json:"restart_count,omitempty" jsonschema_description:"Container restart count"`
	Age          string `json:"age,omitempty"           jsonschema_description:"How long the pod has been running"`
}

// ShardPool describes a shard's operator-managed deployment pool in the cluster.
type ShardPool struct {
	ShardID   string          `json:"shard_id"            jsonschema_description:"Shard identifier from world.toml (e.g. 'gameplay')"`
	Namespace string          `json:"namespace,omitempty" jsonschema_description:"Kubernetes namespace the pool runs in"`
	PoolSize  int32           `json:"pool_size,omitempty" jsonschema_description:"Number of instances in the pool"`
	ImageTag  string          `json:"image_tag,omitempty" jsonschema_description:"Deployed image tag"`
	Phase     string          `json:"phase,omitempty"     jsonschema_description:"Pool phase reported by the operator"`
	Instances []ShardInstance `json:"instances"           jsonschema_description:"Per-pod status"`
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
	registerClusterTool(srv)
	registerSdkGenerateTool(srv)

	return srv
}

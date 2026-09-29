package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// GetWorldStartStatusInput is the structured input for get_world_start_status.
type GetWorldStartStatusInput struct {
	OperatorURL string `json:"operator_url,omitempty" jsonschema_description:"cardinal-operator URL (defaults to http://localhost:8090 for local dev)"`
}

// GetWorldStartStatusOutput is the structured output for get_world_start_status.
type GetWorldStartStatusOutput struct {
	Pools []ShardPool `json:"pools" jsonschema_description:"Shard pools and their pod instances reported by the cardinal-operator"`
}

// registerWorldStatusTool registers the get_world_start_status tool.
// It lists the Cardinal shard pools the local cardinal-operator manages.
func registerWorldStatusTool(srv *server.MCPServer) {
	worldStatusTool := mcp.NewTool(
		"get_world_start_status",
		mcp.WithDescription(
			"List Cardinal shard pools and their pod instances from the cardinal-operator in the local k8s cluster. "+
				"Shows each shard's pool size, image tag, phase, and per-pod readiness/restarts. "+
				"Requires a running cluster (cardinal-editor, or 'world start').",
		),
		mcp.WithInputSchema[GetWorldStartStatusInput](),
		mcp.WithOutputSchema[GetWorldStartStatusOutput](),
	)
	srv.AddTool(worldStatusTool, strictToolHandler(getWorldStartStatusHandler))
}

// getWorldStartStatusHandler returns the operator's shard pools/instances.
func getWorldStartStatusHandler(
	ctx context.Context,
	_ mcp.CallToolRequest,
	args GetWorldStartStatusInput,
) (GetWorldStartStatusOutput, error) {
	ctx, cancel := ensureDeadline(ctx, defaultCommandTimeout)
	defer cancel()

	status, err := operatorStatus(ctx, args.OperatorURL)
	if err != nil {
		return GetWorldStartStatusOutput{}, err
	}
	return GetWorldStartStatusOutput{Pools: toShardPools(status)}, nil
}

package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// GetWorldStartStatusInput is the structured input for get_world_start_status.
type GetWorldStartStatusInput struct {
	Project string `json:"project,omitempty" jsonschema_description:"World project to read (defaults to the world.toml in the working directory)"`
}

// GetWorldStartStatusOutput is the structured output for get_world_start_status.
type GetWorldStartStatusOutput struct {
	Pools []ShardPool `json:"pools" jsonschema_description:"Shard pools and their container instances"`
}

// registerWorldStatusTool registers the get_world_start_status tool.
// It lists the Cardinal shard pools deployed for the project.
func registerWorldStatusTool(srv *server.MCPServer) {
	worldStatusTool := mcp.NewTool(
		"get_world_start_status",
		mcp.WithDescription(
			"List Cardinal shard pools and their container instances from local Docker. "+
				"Shows each shard's pool size, image, phase, and per-container readiness/restarts. "+
				"Requires a running world ('world start').",
		),
		mcp.WithInputSchema[GetWorldStartStatusInput](),
		mcp.WithOutputSchema[GetWorldStartStatusOutput](),
	)
	srv.AddTool(worldStatusTool, strictToolHandler(getWorldStartStatusHandler))
}

// getWorldStartStatusHandler returns the project's shard pools/instances.
func getWorldStartStatusHandler(
	ctx context.Context,
	_ mcp.CallToolRequest,
	args GetWorldStartStatusInput,
) (GetWorldStartStatusOutput, error) {
	ctx, cancel := ensureDeadline(ctx, defaultCommandTimeout)
	defer cancel()

	status, err := worldStatus(ctx, args.Project)
	if err != nil {
		return GetWorldStartStatusOutput{}, err
	}
	return GetWorldStartStatusOutput{Pools: toShardPools(status)}, nil
}

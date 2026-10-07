package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rotisserie/eris"
)

// InspectShardInput is the structured input for the inspect_shard tool.
type InspectShardInput struct {
	ShardID string `json:"shard_id"          jsonschema_description:"ID of the shard to inspect (matches a pool deployed for the project)"`
	Project string `json:"project,omitempty" jsonschema_description:"World project whose namespace to read (defaults to the world.toml in the working directory)"`
}

// InspectShardOutput is the structured output describing a shard pool's state.
type InspectShardOutput struct {
	Pool ShardPool `json:"pool" jsonschema_description:"The shard's pool and per-container instance status"`
}

// registerInspectShardTool registers the inspect_shard tool.
func registerInspectShardTool(srv *server.MCPServer) {
	inspectShardTool := mcp.NewTool(
		"inspect_shard",
		mcp.WithDescription(
			"Inspect the runtime state of a specific Cardinal shard: its pool and per-container "+
				"phase/readiness/restarts. Requires a running world.",
		),
		mcp.WithInputSchema[InspectShardInput](),
		mcp.WithOutputSchema[InspectShardOutput](),
	)
	srv.AddTool(inspectShardTool, strictToolHandler(inspectShardHandler))
}

// inspectShardHandler returns one shard pool's state.
func inspectShardHandler(
	ctx context.Context,
	_ mcp.CallToolRequest,
	args InspectShardInput,
) (InspectShardOutput, error) {
	shardID, err := requireShardID(args.ShardID)
	if err != nil {
		return InspectShardOutput{}, err
	}

	ctx, cancel := ensureDeadline(ctx, defaultCommandTimeout)
	defer cancel()

	status, err := worldStatus(ctx, args.Project)
	if err != nil {
		return InspectShardOutput{}, err
	}

	pools := toShardPools(status)
	for _, pool := range pools {
		if pool.ShardID == shardID {
			return InspectShardOutput{Pool: pool}, nil
		}
	}

	available := make([]string, 0, len(pools))
	for _, pool := range pools {
		available = append(available, pool.ShardID)
	}
	return InspectShardOutput{}, eris.Errorf("shard %q not found; available shards: %v", shardID, available)
}

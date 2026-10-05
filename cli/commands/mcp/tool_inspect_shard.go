package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rotisserie/eris"
)

// InspectShardInput is the structured input for the inspect_shard tool.
type InspectShardInput struct {
	ShardID     string `json:"shard_id"               jsonschema_description:"ID of the shard to inspect (matches a ShardPool managed by the operator)"`
	OperatorURL string `json:"operator_url,omitempty" jsonschema_description:"cardinal-operator URL (defaults to http://localhost:8090 for local dev)"`
}

// InspectShardOutput is the structured output describing a shard pool's state.
type InspectShardOutput struct {
	Pool ShardPool `json:"pool" jsonschema_description:"The shard's pool and per-pod instance status"`
}

// registerInspectShardTool registers the inspect_shard tool.
func registerInspectShardTool(srv *server.MCPServer) {
	inspectShardTool := mcp.NewTool(
		"inspect_shard",
		mcp.WithDescription(
			"Inspect the runtime state of a specific Cardinal shard: its operator pool and per-pod "+
				"phase/readiness/restarts. Requires a running cluster.",
		),
		mcp.WithInputSchema[InspectShardInput](),
		mcp.WithOutputSchema[InspectShardOutput](),
	)
	srv.AddTool(inspectShardTool, strictToolHandler(inspectShardHandler))
}

// inspectShardHandler returns one shard pool's state from the operator.
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

	status, err := operatorStatus(ctx, args.OperatorURL)
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
	return InspectShardOutput{}, eris.Errorf("shard %q not found in cluster; available shards: %v", shardID, available)
}

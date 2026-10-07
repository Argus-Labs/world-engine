package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rotisserie/eris"
)

const (
	// defaultLogTailLines is the per-container history returned when tail_lines is unset.
	defaultLogTailLines int32 = 2000
	// maxLogTailLines caps the per-container history.
	maxLogTailLines int32 = 2000
)

// clampTail normalizes a requested tail-line count to the package defaults/cap.
func clampTail(n int) int32 {
	tail := int32(n)
	if tail <= 0 {
		return defaultLogTailLines
	}
	if tail > maxLogTailLines {
		return maxLogTailLines
	}
	return tail
}

// GetShardLogsInput is the structured input for the get_shard_logs tool.
type GetShardLogsInput struct {
	ShardID      string `json:"shard_id"                jsonschema_description:"ID of the shard whose container logs should be fetched"`
	InstanceName string `json:"instance_name,omitempty" jsonschema_description:"Specific pool instance; accepts variants like 'game-5', 'game 5', 'game5', or '5'. Omit to fetch every instance of the shard."`
	TailLines    int    `json:"tail_lines,omitempty"    jsonschema_description:"Maximum log lines per container (0 or unset returns the full history; 2000 is the cap)"`
	Project      string `json:"project,omitempty"       jsonschema_description:"World project to read (defaults to the world.toml in the working directory)"`
}

// GetShardLogsOutput is the structured output for get_shard_logs.
type GetShardLogsOutput struct {
	ShardID    string   `json:"shard_id"`
	Containers []string `json:"containers" jsonschema_description:"Container names whose logs are included"`
	Logs       string   `json:"logs"`
}

// registerGetShardLogsTool registers the get_shard_logs tool.
func registerGetShardLogsTool(srv *server.MCPServer) {
	getShardLogsTool := mcp.NewTool(
		"get_shard_logs",
		mcp.WithDescription(
			"Fetch recent logs for a Cardinal shard's container(s). Requires a running world.",
		),
		mcp.WithInputSchema[GetShardLogsInput](),
		mcp.WithOutputSchema[GetShardLogsOutput](),
	)
	srv.AddTool(getShardLogsTool, strictToolHandler(getShardLogsHandler))
}

// getShardLogsHandler resolves a shard's containers and returns their recent logs.
func getShardLogsHandler(
	ctx context.Context,
	_ mcp.CallToolRequest,
	args GetShardLogsInput,
) (GetShardLogsOutput, error) {
	shardID, err := requireShardID(args.ShardID)
	if err != nil {
		return GetShardLogsOutput{}, err
	}
	tail := clampTail(args.TailLines)

	ctx, cancel := ensureDeadline(ctx, defaultCommandTimeout)
	defer cancel()

	status, err := worldStatus(ctx, args.Project)
	if err != nil {
		return GetShardLogsOutput{}, err
	}

	instanceName := strings.TrimSpace(args.InstanceName)
	containers := shardContainers(status, shardID, instanceName)
	if len(containers) == 0 {
		if instanceName != "" {
			return GetShardLogsOutput{}, errInstanceNotFound(status, shardID, instanceName)
		}
		return GetShardLogsOutput{}, eris.Errorf("no container found for shard %q", shardID)
	}

	var b strings.Builder
	for _, name := range containers {
		logs, err := collectContainerLogs(ctx, args.Project, name, tail)
		if err != nil {
			return GetShardLogsOutput{}, err
		}
		if len(containers) > 1 {
			fmt.Fprintf(&b, "=== %s ===\n", name)
		}
		b.WriteString(logs)
	}

	return GetShardLogsOutput{
		ShardID:    shardID,
		Containers: containers,
		Logs:       b.String(),
	}, nil
}

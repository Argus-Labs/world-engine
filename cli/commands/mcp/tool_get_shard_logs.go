package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/pkg/local"
)

const (
	// defaultLogTailLines is the per-container history returned when tail_lines is unset.
	defaultLogTailLines int32 = 2000
	// maxLogTailLines caps the per-container history.
	maxLogTailLines int32 = 2000
)

// collectShardLogs fetches each container in order. The project lock is not held
// across the status call and these fetches, so an instance a concurrent reload removed
// is noted and the rest still returned; if none was fetched the call fails. Once ctx is
// over the remaining instances are listed as skipped rather than fetched as empty.
func collectShardLogs(
	ctx context.Context,
	containers []string,
	fetch func(name string) (string, error),
) (string, error) {
	var b strings.Builder
	fetched := 0
	var lastErr error
	for i, name := range containers {
		if ctx.Err() != nil {
			fmt.Fprintf(&b, "(skipped, time budget spent: %s)\n", strings.Join(containers[i:], ", "))
			break
		}
		logs, err := fetch(name)
		switch {
		case err == nil:
			fetched++
			if len(containers) > 1 {
				fmt.Fprintf(&b, "=== %s ===\n", name)
			}
			b.WriteString(logs)
		case eris.Is(err, local.ErrContainerNotFound):
			lastErr = err
			fmt.Fprintf(&b, "=== %s ===\n(container not found; removed since the status check)\n", name)
		default:
			return "", err
		}
	}
	if fetched == 0 {
		if lastErr != nil {
			return "", lastErr
		}
		return "", eris.Wrap(ctx.Err(), "fetching shard logs")
	}
	return b.String(), nil
}

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
	TailLines    int    `json:"tail_lines,omitempty"    jsonschema_description:"Maximum log lines per container (unset or 0 returns the most recent 2000 lines, which is also the cap)"`
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

	logs, err := collectShardLogs(ctx, containers, func(name string) (string, error) {
		return collectContainerLogs(ctx, args.Project, name, tail)
	})
	if err != nil {
		return GetShardLogsOutput{}, err
	}

	return GetShardLogsOutput{
		ShardID:    shardID,
		Containers: containers,
		Logs:       logs,
	}, nil
}

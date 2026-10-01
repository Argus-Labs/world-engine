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
	// defaultLogTailLines is the per-pod history returned when tail_lines is unset.
	defaultLogTailLines int32 = 2000
	// maxLogTailLines caps the per-pod history (matches the operator's cap).
	maxLogTailLines int32 = 10000
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
	ShardID      string `json:"shard_id"                jsonschema_description:"ID of the shard whose pod logs should be fetched"`
	InstanceName string `json:"instance_name,omitempty" jsonschema_description:"Specific pool instance; accepts variants like 'game-5', 'game 5', 'game5', or '5'. Omit to fetch every instance of the shard."`
	TailLines    int    `json:"tail_lines,omitempty"    jsonschema_description:"Maximum log lines per pod (0 or unset defaults to 2000, capped at 10000)"`
	OperatorURL  string `json:"operator_url,omitempty"  jsonschema_description:"cardinal-operator URL (defaults to http://localhost:8090 for local dev)"`
}

// GetShardLogsOutput is the structured output for get_shard_logs.
type GetShardLogsOutput struct {
	ShardID string   `json:"shard_id"`
	Pods    []string `json:"pods"     jsonschema_description:"Pod names whose logs are included"`
	Logs    string   `json:"logs"`
}

// registerGetShardLogsTool registers the get_shard_logs tool.
func registerGetShardLogsTool(srv *server.MCPServer) {
	getShardLogsTool := mcp.NewTool(
		"get_shard_logs",
		mcp.WithDescription(
			"Fetch recent logs for a Cardinal shard's pod(s) via the cardinal-operator. Requires a running cluster.",
		),
		mcp.WithInputSchema[GetShardLogsInput](),
		mcp.WithOutputSchema[GetShardLogsOutput](),
	)
	srv.AddTool(getShardLogsTool, strictToolHandler(getShardLogsHandler))
}

// getShardLogsHandler resolves a shard's pods via the operator and returns their
// recent logs.
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

	status, err := operatorStatus(ctx, args.OperatorURL)
	if err != nil {
		return GetShardLogsOutput{}, err
	}

	instanceName := strings.TrimSpace(args.InstanceName)
	pods := shardPods(status, shardID, instanceName)
	if len(pods) == 0 {
		if instanceName != "" {
			// Empty pods means either "no instance matched" or "instance matched
			// but its pod isn't scheduled yet". shardPods collapses both, so re-check
			// the pool: a matched-but-podless instance exists and just isn't running
			// yet (the correct action is wait-and-retry, not pick another instance),
			// while a true miss falls through to errInstanceNotFound.
			if inst := findInstanceByName(status, shardID, instanceName); inst != nil {
				return GetShardLogsOutput{}, eris.Errorf(
					"instance %q for shard %q has no running pod yet (phase: %s); wait for the pod to be scheduled and retry",
					inst.GetName(),
					shardID,
					inst.GetPhase(),
				)
			}
			return GetShardLogsOutput{}, errInstanceNotFound(status, shardID, instanceName)
		}
		return GetShardLogsOutput{}, eris.Errorf("no running pod found for shard %q", shardID)
	}

	var b strings.Builder
	for _, pod := range pods {
		logs, err := collectPodLogs(ctx, args.OperatorURL, pod, tail)
		if err != nil {
			return GetShardLogsOutput{}, err
		}
		if len(pods) > 1 {
			fmt.Fprintf(&b, "=== pod %s ===\n", pod)
		}
		b.WriteString(logs)
	}

	return GetShardLogsOutput{
		ShardID: shardID,
		Pods:    pods,
		Logs:    b.String(),
	}, nil
}

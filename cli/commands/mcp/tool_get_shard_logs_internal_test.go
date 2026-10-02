package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	operatorv1 "github.com/argus-labs/world-engine/cli/proto/gen/go/cardinal/operator/v1"
	"github.com/argus-labs/world-engine/cli/proto/gen/go/cardinal/operator/v1/operatorv1connect"
)

// mcpFakeOperator implements just enough of the operator service for the
// get_shard_logs handler tests: Status serves a scripted pool set, and
// StreamPodLogs serves one canned line per pod so the happy path (pods with
// logs) is exercisable end-to-end through the connect httptest server, the
// same pattern cli/pkg/cluster/logs_internal_test.go uses.
type mcpFakeOperator struct {
	operatorv1connect.UnimplementedOperatorServiceHandler

	pools []*operatorv1.ShardPoolStatus
	// logLines maps pod name to the lines StreamPodLogs emits for it. A pod
	// with no entry emits a single "log for <pod>" line.
	logLines map[string][]string
}

func (f *mcpFakeOperator) Status(_ context.Context, _ *connect.Request[operatorv1.StatusRequest],
) (*connect.Response[operatorv1.StatusResponse], error) {
	return connect.NewResponse(&operatorv1.StatusResponse{Pools: f.pools}), nil
}

func (f *mcpFakeOperator) StreamPodLogs(
	_ context.Context,
	req *connect.Request[operatorv1.StreamPodLogsRequest],
	stream *connect.ServerStream[operatorv1.StreamPodLogsResponse],
) error {
	pod := req.Msg.GetPodName()
	lines := f.logLines[pod]
	if lines == nil {
		lines = []string{"log for " + pod}
	}
	for _, line := range lines {
		if err := stream.Send(&operatorv1.StreamPodLogsResponse{
			Lines: []*operatorv1.PodLogLine{{Line: line}},
		}); err != nil {
			return err
		}
	}
	return nil
}

// newFakeOperatorServer stands up a connect operator service backed by op behind
// an httptest server, returning its URL for use as operator_url.
func newFakeOperatorServer(t *testing.T, op *mcpFakeOperator) string {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := operatorv1connect.NewOperatorServiceHandler(op)
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// callGetShardLogs invokes the wrapped get_shard_logs handler with the given
// arguments and returns the agent-facing CallToolResult. The handler converts
// a Go error into an MCP tool-error result (IsError=true) rather than returning
// a transport error, so a non-nil result is the norm.
func callGetShardLogs(t *testing.T, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	h := strictToolHandler(getShardLogsHandler)
	result, err := h(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "get_shard_logs", Arguments: args},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	return result
}

func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok, "expected TextContent, got %T", result.Content[0])
	return text.Text
}

// TestGetShardLogsHandler_PodslessMatchedInstance_ReportsNoPodNotNotFound is the
// regression test for the bug: an instance that exists in the pool but whose pod
// is not yet scheduled (empty PodName) used to be reported as "not found" while
// being simultaneously listed in "available instances" — a self-contradictory
// message. The fix surfaces the instance as existing but having no running pod
// (with its phase), and never says "not found".
func TestGetShardLogsHandler_PodslessMatchedInstance_ReportsNoPodNotNotFound(t *testing.T) {
	t.Parallel()
	op := &mcpFakeOperator{pools: []*operatorv1.ShardPoolStatus{{
		ShardId: "game",
		Instances: []*operatorv1.ShardInstanceStatus{
			{Name: "game", PodName: "pod-1"},
			{Name: "game-5", PodName: "", Phase: "Creating"}, // exists, pod not scheduled
		},
	}}}
	url := newFakeOperatorServer(t, op)

	result := callGetShardLogs(t, map[string]any{
		"shard_id":      "game",
		"instance_name": "game-5",
		"operator_url":  url,
	})

	require.True(t, result.IsError)
	text := resultText(t, result)
	t.Logf("agent-facing message: %q", text)

	// The instance is named and its unscheduled state (phase) is surfaced so an
	// agent knows to wait and retry rather than pick a different instance.
	assert.Contains(t, text, `"game-5"`)
	assert.Contains(t, text, "no running pod")
	assert.Contains(t, text, "Creating")
	// The contradictory "not found" framing and the all-instances "available"
	// listing must both be gone — game-5 IS in the pool.
	assert.NotContains(t, text, "not found")
	assert.NotContains(t, text, "available instances")
}

// A genuinely unknown instance still gets errInstanceNotFound with the shard's
// available instances — the fix must not turn a real miss into a misleading
// "no running pod" message.
func TestGetShardLogsHandler_UnknownInstance_StillNotFoundWithAvailable(t *testing.T) {
	t.Parallel()
	op := &mcpFakeOperator{pools: []*operatorv1.ShardPoolStatus{{
		ShardId: "game",
		Instances: []*operatorv1.ShardInstanceStatus{
			{Name: "game", PodName: "pod-1"},
			{Name: "game-5", PodName: "", Phase: "Creating"},
		},
	}}}
	url := newFakeOperatorServer(t, op)

	result := callGetShardLogs(t, map[string]any{
		"shard_id":      "game",
		"instance_name": "game-99",
		"operator_url":  url,
	})

	require.True(t, result.IsError)
	text := resultText(t, result)
	assert.Contains(t, text, `instance "game-99" not found`)
	assert.Contains(t, text, "available instances")
}

// A running instance still returns its logs end-to-end (Status → pod resolution
// → StreamPodLogs → JSON output), so the fix's re-check didn't regress the happy
// path. A second running instance is included when no instance_name is given.
func TestGetShardLogsHandler_RunningInstance_ReturnsLogs(t *testing.T) {
	t.Parallel()
	op := &mcpFakeOperator{
		pools: []*operatorv1.ShardPoolStatus{{
			ShardId: "game",
			Instances: []*operatorv1.ShardInstanceStatus{
				{Name: "game", PodName: "pod-1"},
				{Name: "game-5", PodName: "pod-5"},
			},
		}},
		logLines: map[string][]string{
			"pod-1": {"hello from game"},
			"pod-5": {"hello from game-5"},
		},
	}
	url := newFakeOperatorServer(t, op)

	t.Run("named instance", func(t *testing.T) {
		result := callGetShardLogs(t, map[string]any{
			"shard_id":      "game",
			"instance_name": "game-5",
			"operator_url":  url,
		})

		require.False(t, result.IsError)
		var out GetShardLogsOutput
		require.NoError(t, json.Unmarshal([]byte(resultText(t, result)), &out))
		assert.Equal(t, "game", out.ShardID)
		assert.Equal(t, []string{"pod-5"}, out.Pods)
		assert.Equal(t, "hello from game-5\n", out.Logs)
	})

	t.Run("all instances", func(t *testing.T) {
		result := callGetShardLogs(t, map[string]any{
			"shard_id":     "game",
			"operator_url": url,
		})

		require.False(t, result.IsError)
		var out GetShardLogsOutput
		require.NoError(t, json.Unmarshal([]byte(resultText(t, result)), &out))
		assert.Equal(t, "game", out.ShardID)
		assert.ElementsMatch(t, []string{"pod-1", "pod-5"}, out.Pods)
		// Both pods' logs are present, each under a multi-pod header.
		assert.Contains(t, out.Logs, "=== pod pod-1 ===")
		assert.Contains(t, out.Logs, "hello from game")
		assert.Contains(t, out.Logs, "=== pod pod-5 ===")
		assert.Contains(t, out.Logs, "hello from game-5")
	})
}

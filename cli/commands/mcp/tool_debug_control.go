package mcp

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rotisserie/eris"

	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
)

// Debug operations supported by the debug_control tool.
const (
	debugOpPause  = "pause"
	debugOpResume = "resume"
	debugOpStep   = "step"
	debugOpReset  = "reset"
)

// DebugControlInput is the structured input for the debug_control tool.
type DebugControlInput struct {
	ShardID      string `json:"shard_id"                jsonschema_description:"ID of the shard to control"`
	Operation    string `json:"operation"               jsonschema_description:"Debug op: 'pause' halts ticking; 'resume' continues after a pause; 'step' advances exactly one tick (requires paused); 'reset' restores the world to its pre-tick-0 state (requires paused)." jsonschema:"enum=pause,enum=resume,enum=step,enum=reset"`
	InstanceName string `json:"instance_name,omitempty" jsonschema_description:"Specific pool instance; accepts variants like 'game-5', 'game 5', 'game5', or '5'. Defaults to the shard's first instance."`
	Organization string `json:"organization,omitempty"  jsonschema_description:"Organization (auto-derived from the project's releases when omitted)"`
	Project      string `json:"project,omitempty"       jsonschema_description:"Project (defaults to the world.toml in the working directory)"`
}

// DebugControlOutput is the structured output for the debug_control tool.
type DebugControlOutput struct {
	Operation  string `json:"operation"   jsonschema_description:"The operation that was applied"`
	TickHeight uint64 `json:"tick_height" jsonschema_description:"Current tick height; meaningful for pause and step, 0 for resume and reset"`
	Status     string `json:"status"      jsonschema_description:"Human-readable result, e.g. 'paused at tick 42'"`
}

// registerDebugControlTool registers the debug_control tool. It drives a
// Cardinal shard's debug lifecycle (pause/resume/step/reset) by dialing the
// shard's DebugService directly through the local edge.
func registerDebugControlTool(srv *server.MCPServer) {
	debugControlTool := mcp.NewTool(
		"debug_control",
		mcp.WithDescription(
			"Drive a Cardinal shard's debug lifecycle: "+
				"pause (halt ticking), resume (continue after a pause), step (advance one tick, requires "+
				"paused), or reset (restore the pre-tick-0 state, requires paused). Pause and step return "+
				"the current tick height. Requires a running world. Pair with the get_state tool to "+
				"inspect the world's entities while paused.",
		),
		mcp.WithInputSchema[DebugControlInput](),
		mcp.WithOutputSchema[DebugControlOutput](),
	)
	srv.AddTool(debugControlTool, strictToolHandler(debugControlHandler))
}

// debugControlHandler applies a debug operation to a shard's DebugService.
func debugControlHandler(
	ctx context.Context,
	_ mcp.CallToolRequest,
	args DebugControlInput,
) (DebugControlOutput, error) {
	shardID, err := requireShardID(args.ShardID)
	if err != nil {
		return DebugControlOutput{}, err
	}
	op := strings.ToLower(strings.TrimSpace(args.Operation))
	switch op {
	case debugOpPause, debugOpResume, debugOpStep, debugOpReset:
	default:
		return DebugControlOutput{}, eris.Errorf(
			"operation must be one of pause, resume, step, reset; got %q", args.Operation)
	}

	ctx, cancel := ensureDeadline(ctx, defaultCommandTimeout)
	defer cancel()

	client, _, err := dialShardDebugService(ctx, debugTargetArgs{
		shardID:      shardID,
		instanceName: args.InstanceName,
		organization: args.Organization,
		project:      args.Project,
	})
	if err != nil {
		return DebugControlOutput{}, err
	}

	var tickHeight uint64
	switch op {
	case debugOpPause:
		res, err := client.Pause(ctx, connect.NewRequest(&cardinalv1.PauseRequest{}))
		if err != nil {
			return DebugControlOutput{}, eris.Wrapf(err, "failed to pause shard %q", shardID)
		}
		tickHeight = res.Msg.GetTickHeight()
	case debugOpResume:
		if _, err := client.Resume(ctx, connect.NewRequest(&cardinalv1.ResumeRequest{})); err != nil {
			return DebugControlOutput{}, eris.Wrapf(err, "failed to resume shard %q", shardID)
		}
	case debugOpStep:
		res, err := client.Step(ctx, connect.NewRequest(&cardinalv1.StepRequest{}))
		if err != nil {
			return DebugControlOutput{}, eris.Wrapf(err, "failed to step shard %q", shardID)
		}
		tickHeight = res.Msg.GetTickHeight()
	// debugOpReset. Default to satisfy Qodana error
	default:
		if _, err := client.Reset(ctx, connect.NewRequest(&cardinalv1.ResetRequest{})); err != nil {
			return DebugControlOutput{}, eris.Wrapf(err, "failed to reset shard %q", shardID)
		}
	}

	return DebugControlOutput{
		Operation:  op,
		TickHeight: tickHeight,
		Status:     debugStatusMessage(op, tickHeight),
	}, nil
}

// debugStatusMessage builds a human-readable summary of the applied operation.
func debugStatusMessage(op string, tickHeight uint64) string {
	switch op {
	case debugOpPause:
		return fmt.Sprintf("paused at tick %d", tickHeight)
	case debugOpStep:
		return fmt.Sprintf("stepped to tick %d", tickHeight)
	case debugOpResume:
		return "resumed"
	case debugOpReset:
		return "reset to pre-tick-0 state"
	default:
		return op
	}
}

package mcp

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/pkg/local"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// Operations supported by the world_lifecycle tool.
const (
	lifecycleOpStart = "start"
	lifecycleOpStop  = "stop"
	lifecycleOpPurge = "purge"
)

// Timeouts are hang backstops, not budgets: start gets headroom for a cold
// Docker cache while stop and purge — which never build — fail much sooner.
const (
	lifecycleStartTimeout    = 5 * time.Minute
	lifecycleTeardownTimeout = 2 * time.Minute
)

// startFailureAdvice is appended to runtime-side start failures. Stale containers
// or volumes are the usual cause and a purge clears them — but a purge destroys
// the world's state, so this only ever recommends. The caller must put it to the
// user and get an explicit yes before running a purge; it must never chain one
// automatically off a failure. Build failures deliberately don't carry this: a
// purge cannot fix code that doesn't compile.
const startFailureAdvice = " (the world may be in a bad state — a purge usually clears it, but that " +
	"deletes the world's containers, volumes and all its state, so ask the user first and only re-run start with " +
	"purge=true if they agree; never purge automatically)"

// WorldLifecycleInput is the structured input for the world_lifecycle tool.
type WorldLifecycleInput struct {
	Operation string `json:"operation"            jsonschema:"enum=start,enum=stop,enum=purge" jsonschema_description:"Lifecycle op. 'start' starts NATS, Postgres and the shards as containers — run/boot/launch it. 'stop' stops the containers and keeps database and NATS state — shut down/turn off/pause. 'purge' removes the world's containers, volumes and network — wipe/clean/nuke/reset/start over; this stops it too, so never pair a stop with it."`
	WorldPath string `json:"world_path,omitempty"                                              jsonschema_description:"Path to the world project directory containing world.toml. Required for every operation: 'start' compiles and deploys its shards; 'stop' and 'purge' act on its containers."`
	// Organization and Project are filled from world.toml for stop/purge.
	Organization string `json:"-"`
	Project      string `json:"-"`
	Purge        bool   `json:"purge,omitempty" jsonschema_description:"Only valid with 'start': purge the world first so it comes up completely fresh — the 'clean everything and start it again' request. The 'purge' operation already wipes the world, and 'stop' deliberately preserves state."`
}

// WorldLifecycleOutput is the structured output for the world_lifecycle tool.
type WorldLifecycleOutput struct {
	Operation    string   `json:"operation"              jsonschema_description:"The operation that was applied"`
	Purged       bool     `json:"purged"                 jsonschema_description:"Whether the world's containers, volumes and state were deleted as part of this call"`
	Organization string   `json:"organization,omitempty" jsonschema_description:"World organization (start only)"`
	Project      string   `json:"project,omitempty"      jsonschema_description:"World project (start only)"`
	Shards       []string `json:"shards,omitempty"       jsonschema_description:"Shard IDs deployed (start only)"`
	Status       string   `json:"status"                 jsonschema_description:"Human-readable result summary"`
}

// registerWorldLifecycleTool registers the world_lifecycle tool — the MCP counterpart of
// `world start` / `world stop` / `world purge`.
func registerWorldLifecycleTool(srv *server.MCPServer) {
	tool := mcp.NewTool(
		"world_lifecycle",
		mcp.WithDescription(
			"Drive the local world's lifecycle on Docker: bring the world up, shut it down, or wipe it. "+
				"Callers rarely use these words — 'run the world', 'turn it off', 'clean the "+
				"docker and start it again', 'nuke everything', 'start from scratch' all land here. "+
				"'start' runs NATS, Postgres and the shards of the world at world_path as containers, building "+
				"the shards first so code that won't compile leaves Docker untouched; the edge proxy on "+
				"localhost:8080 runs inside this MCP process. 'stop' stops the containers and preserves state, "+
				"so a later start resumes quickly. 'purge' removes the containers, volumes and network, losing all "+
				"state — that stops it too, so never stop first. Pass purge with start for 'clean slate' "+
				"requests that should end with the world running. "+
				"If a start fails on the Docker side, the fix is usually a purge — recommend it to the user "+
				"and wait for them to agree before running it, since it destroys all state; never purge "+
				"automatically off a failure. A start that fails to BUILD is a code problem: report the "+
				"compiler error, do not suggest a purge. "+
				"Prefer other tools when they fit: for fresh GAME state (tick back to 0), reload with purge "+
				"takes seconds; to run new shard code in an already-running world, plain reload beats a "+
				"stop/start cycle.",
		),
		mcp.WithInputSchema[WorldLifecycleInput](),
		mcp.WithOutputSchema[WorldLifecycleOutput](),
	)
	srv.AddTool(tool, strictToolHandler(worldLifecycleHandler))
}

// worldLifecycleHandler applies a lifecycle operation to the local world.
func worldLifecycleHandler(
	ctx context.Context,
	_ mcp.CallToolRequest,
	args WorldLifecycleInput,
) (WorldLifecycleOutput, error) {
	op := strings.ToLower(strings.TrimSpace(args.Operation))
	switch op {
	case lifecycleOpStart, lifecycleOpStop, lifecycleOpPurge:
	default:
		return WorldLifecycleOutput{}, eris.Errorf(
			"operation must be one of start, stop, purge; got %q", args.Operation)
	}
	// Rejected rather than ignored: silently dropping it would let a caller think
	// it had asked for something it hadn't.
	if args.Purge && op != lifecycleOpStart {
		return WorldLifecycleOutput{}, eris.Errorf(
			"purge is only valid with 'start'; the 'purge' operation already deletes the world, "+
				"and 'stop' preserves state by design (got operation %q)", op)
	}

	timeout := lifecycleTeardownTimeout
	if op == lifecycleOpStart {
		timeout = lifecycleStartTimeout
	}
	ctx, cancel := ensureDeadline(ctx, timeout)
	defer cancel()

	worldPath := strings.TrimSpace(args.WorldPath)
	if worldPath == "" {
		return WorldLifecycleOutput{}, eris.New(
			"world_path is required (path to the world project directory containing world.toml)",
		)
	}
	worldPath = filepath.Clean(worldPath)
	rt, err := started.open(worldPath)
	if err != nil {
		return WorldLifecycleOutput{}, eris.Wrap(err, "open world")
	}
	args.Organization, args.Project = rt.Config().WorldToml.Organization, rt.Config().Project

	switch op {
	case lifecycleOpStop:
		started.stopEdge(args.Project)
		if err := rt.Stop(ctx, nil); err != nil {
			return WorldLifecycleOutput{}, eris.Wrap(err, "failed to stop world")
		}
		return WorldLifecycleOutput{
			Operation: op,
			Status:    "containers stopped (database and NATS state preserved; start again to resume)",
		}, nil

	case lifecycleOpPurge:
		started.stopEdge(args.Project)
		if err := rt.Purge(ctx, false, nil); err != nil {
			return WorldLifecycleOutput{}, eris.Wrap(err, "failed to purge world")
		}
		return WorldLifecycleOutput{
			Operation: op,
			Purged:    true,
			Status:    "containers, volumes and network removed (all state wiped)",
		}, nil
	}

	return startWorld(ctx, rt, worldPath, args)
}

// startWorld builds the world's shards, optionally purges the existing
// containers, then starts the platform and deploys onto it. The build runs before
// anything is torn down or started, so a compile error leaves Docker exactly as
// it was — the same guarantee the reload tool gives.
func startWorld(
	ctx context.Context,
	rt *local.Runtime,
	worldPath string,
	args WorldLifecycleInput,
) (WorldLifecycleOutput, error) {
	cfg, deployOpts, err := buildWorldShards(ctx, worldPath, "", true)
	if err != nil {
		// No purge advice here: nothing was touched and a purge cannot fix
		// code that doesn't compile.
		return WorldLifecycleOutput{}, eris.Wrap(err, "build failed (did the shard code compile?)")
	}

	if args.Purge {
		started.stopEdge(cfg.Project)
		if err := rt.Purge(ctx, false, nil); err != nil {
			return WorldLifecycleOutput{}, eris.Wrap(err, "failed to purge world before start")
		}
	}

	if err := rt.StartPlatform(ctx, nil); err != nil {
		return WorldLifecycleOutput{}, eris.Wrap(err, "failed to start platform"+startFailureAdvice)
	}
	if err := rt.Deploy(ctx, deployOpts); err != nil {
		return WorldLifecycleOutput{}, eris.Wrap(err, "failed to deploy shards"+startFailureAdvice)
	}
	if err := rt.DeployServices(ctx); err != nil {
		return WorldLifecycleOutput{}, eris.Wrap(err, "failed to deploy services"+startFailureAdvice)
	}
	if err := started.serveEdge(rt); err != nil {
		return WorldLifecycleOutput{}, eris.Wrap(err, "failed to start the edge proxy")
	}
	ready, expected := rt.WaitForShardsReady(ctx, nil, nil) // a start brings up every instance

	shardIDs := deployedShardIDs(deployOpts)
	status := fmt.Sprintf("started %d shard(s) for %s/%s on Docker; API at %s/%s/%s/<instance>",
		len(shardIDs), cfg.Organization, cfg.Project, local.APIEndpoint, sanitizedOrg(cfg), sanitizedProject(cfg))
	if args.Purge {
		status += " (all previous state wiped)"
	}
	// Reporting a clean start for a world that never came up would send the caller
	// looking in the wrong place; name the shortfall and where to look instead.
	if ready < expected {
		status += fmt.Sprintf("; WARNING only %d/%d instances are accepting connections — check `get_shard_logs`",
			ready, expected)
	}
	return WorldLifecycleOutput{
		Operation:    lifecycleOpStart,
		Purged:       args.Purge,
		Organization: cfg.Organization,
		Project:      cfg.Project,
		Shards:       shardIDs,
		Status:       status,
	}, nil
}

func sanitizedOrg(cfg worldtoml.Config) string {
	org, _ := local.Sanitized(cfg.Organization, cfg.Project)
	return org
}

func sanitizedProject(cfg worldtoml.Config) string {
	_, project := local.Sanitized(cfg.Organization, cfg.Project)
	return project
}

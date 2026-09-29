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

	"github.com/argus-labs/world-engine/cli/pkg/cluster"
)

// Cluster lifecycle operations supported by the cluster tool.
const (
	clusterOpStart = "start"
	clusterOpStop  = "stop"
	clusterOpPurge = "purge"
)

// Timeouts are hang backstops, not budgets: a fresh start runs about a minute
// and a half, so start gets headroom for a cold Docker cache while stop and
// purge — which only talk to k3d and never build — fail much sooner.
const (
	clusterStartTimeout     = 5 * time.Minute
	clusterLifecycleTimeout = 2 * time.Minute
)

// startFailureAdvice is appended to cluster-side start failures. A stale or
// half-created cluster is the usual cause and a purge clears it — but a purge
// destroys every world's state, so this only ever recommends. The caller must
// put it to the user and get an explicit yes before running a purge; it must
// never chain one automatically off a failure. Build failures deliberately
// don't carry this: a purge cannot fix code that doesn't compile.
const startFailureAdvice = " (the cluster may be in a bad state — a purge usually clears it, but that " +
	"deletes the cluster and all its state, so ask the user first and only re-run start with purge=true " +
	"if they agree; never purge automatically)"

// ClusterInput is the structured input for the cluster tool.
type ClusterInput struct {
	Operation string `json:"operation"            jsonschema:"enum=start,enum=stop,enum=purge" jsonschema_description:"Lifecycle op. 'start' brings the cluster up and deploys the world — run/boot/launch it. 'stop' pauses the cluster and keeps state — shut down/turn off/pause. 'purge' deletes the cluster and its containers, losing all state — wipe/clean/nuke/reset/start over/'clean the docker'; this stops it too, so never pair a stop with it."`
	WorldPath string `json:"world_path,omitempty"                                              jsonschema_description:"Path to the world project directory containing world.toml. Required for 'start' (its shards are compiled and deployed); unused by 'stop' and 'purge', which act on the cluster alone."`
	Purge     bool   `json:"purge,omitempty"                                                   jsonschema_description:"Only valid with 'start': delete the existing cluster first so the world comes up on a completely fresh one — the 'clean everything and start it again' request. The 'purge' operation already deletes the cluster, and 'stop' deliberately preserves state."`
}

// ClusterOutput is the structured output for the cluster tool.
type ClusterOutput struct {
	Operation    string   `json:"operation"              jsonschema_description:"The operation that was applied"`
	Purged       bool     `json:"purged"                 jsonschema_description:"Whether the cluster was deleted as part of this call"`
	Organization string   `json:"organization,omitempty" jsonschema_description:"World organization (start only)"`
	Project      string   `json:"project,omitempty"      jsonschema_description:"World project (start only)"`
	Shards       []string `json:"shards,omitempty"       jsonschema_description:"Shard IDs deployed (start only)"`
	Status       string   `json:"status"                 jsonschema_description:"Human-readable result summary"`
}

// registerClusterTool registers the cluster tool — the MCP counterpart of
// `world start` / `world stop` / `world purge`.
func registerClusterTool(srv *server.MCPServer) {
	clusterTool := mcp.NewTool(
		"cluster",
		mcp.WithDescription(
			"Drive the local cluster's lifecycle: bring the world up, shut it down, or wipe the "+
				"environment. Callers rarely use these words — 'run the world', 'turn it off', 'clean the "+
				"docker and start it again', 'nuke everything', 'start from scratch' all land here. "+
				"'start' brings the cluster up and deploys the world at world_path, building its shards "+
				"first so code that won't compile leaves the cluster untouched. 'stop' pauses the cluster "+
				"and preserves state, so a later start resumes quickly. 'purge' deletes the cluster and its "+
				"containers, losing all state — that stops it too, so never stop first. Pass purge with "+
				"start for 'clean slate' requests that should end with the world running. "+
				"If a start fails on the cluster side, the fix is usually a purge — recommend it to the user "+
				"and wait for them to agree before running it, since it destroys all state; never purge "+
				"automatically off a failure. A start that fails to BUILD is a code problem: report the "+
				"compiler error, do not suggest a purge. "+
				"Prefer other tools when they fit: for fresh GAME state (tick back to 0) without rebuilding "+
				"the environment, reload with purge takes seconds instead of ~90s; to run new shard code in "+
				"an already-running world, plain reload beats a stop/start cycle.",
		),
		mcp.WithInputSchema[ClusterInput](),
		mcp.WithOutputSchema[ClusterOutput](),
	)
	srv.AddTool(clusterTool, strictToolHandler(clusterHandler))
}

// clusterHandler applies a lifecycle operation to the local cluster.
func clusterHandler(
	ctx context.Context,
	_ mcp.CallToolRequest,
	args ClusterInput,
) (ClusterOutput, error) {
	op := strings.ToLower(strings.TrimSpace(args.Operation))
	switch op {
	case clusterOpStart, clusterOpStop, clusterOpPurge:
	default:
		return ClusterOutput{}, eris.Errorf(
			"operation must be one of start, stop, purge; got %q", args.Operation)
	}
	// Rejected rather than ignored: silently dropping it would let a caller think
	// it had asked for something it hadn't.
	if args.Purge && op != clusterOpStart {
		return ClusterOutput{}, eris.Errorf(
			"purge is only valid with 'start'; the 'purge' operation already deletes the cluster, "+
				"and 'stop' preserves state by design (got operation %q)", op)
	}

	timeout := clusterLifecycleTimeout
	if op == clusterOpStart {
		timeout = clusterStartTimeout
	}
	ctx, cancel := ensureDeadline(ctx, timeout)
	defer cancel()

	cli := clusterClient()

	switch op {
	case clusterOpStop:
		if err := cli.Stop(ctx, cluster.StopOpts{}); err != nil {
			return ClusterOutput{}, eris.Wrap(err, "failed to stop cluster")
		}
		return ClusterOutput{
			Operation: op,
			Status:    "cluster stopped (state preserved; start again to resume)",
		}, nil

	case clusterOpPurge:
		if err := cli.Purge(ctx, cluster.PurgeOpts{}); err != nil {
			return ClusterOutput{}, eris.Wrap(err, "failed to purge cluster")
		}
		return ClusterOutput{
			Operation: op,
			Purged:    true,
			Status:    "cluster deleted (all state wiped)",
		}, nil
	}

	return startWorld(ctx, cli, args)
}

// startWorld builds the world's shards, optionally deletes the existing cluster,
// then brings the cluster up and deploys onto it. The build runs before anything
// is torn down or started, so a compile error leaves the cluster exactly as it
// was — the same guarantee the reload tool gives.
func startWorld(ctx context.Context, cli *cluster.Client, args ClusterInput) (ClusterOutput, error) {
	worldPath := strings.TrimSpace(args.WorldPath)
	if worldPath == "" {
		return ClusterOutput{}, eris.New(
			"world_path is required for 'start' (path to the world project directory containing world.toml)")
	}
	worldPath = filepath.Clean(worldPath)

	cfg, deployOpts, err := buildWorldShards(ctx, worldPath, "")
	if err != nil {
		// No purge advice here: the cluster is untouched and a purge cannot fix
		// code that doesn't compile.
		return ClusterOutput{}, eris.Wrap(err, "build failed (did the shard code compile?)")
	}

	if args.Purge {
		if err := cli.Purge(ctx, cluster.PurgeOpts{}); err != nil {
			return ClusterOutput{}, eris.Wrap(err, "failed to purge cluster before start")
		}
	}

	// Start composes the platform bring-up and DeployWorld, so the ShardPools are
	// applied by the time it returns; Deploy then hands over the built images.
	if err := cli.Start(ctx, cluster.StartOpts{Project: cfg.Project, Config: cfg}); err != nil {
		return ClusterOutput{}, eris.Wrap(err, "failed to start cluster"+startFailureAdvice)
	}
	if err := cli.Deploy(ctx, deployOpts); err != nil {
		return ClusterOutput{}, eris.Wrap(err, "failed to deploy shards"+startFailureAdvice)
	}
	cli.WaitForShardsReady(ctx, cfg, nil)

	shardIDs := deployedShardIDs(deployOpts)
	status := fmt.Sprintf("started cluster and deployed %d shard(s) for %s/%s",
		len(shardIDs), cfg.Organization, cfg.Project)
	if args.Purge {
		status += " on a fresh cluster (all previous state wiped)"
	}
	return ClusterOutput{
		Operation:    clusterOpStart,
		Purged:       args.Purge,
		Organization: cfg.Organization,
		Project:      cfg.Project,
		Shards:       shardIDs,
		Status:       status,
	}, nil
}

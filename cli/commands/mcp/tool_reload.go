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
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// reloadTimeout is a hang backstop, not a budget: a warm reload runs in seconds,
// so this only needs headroom for a cold Docker cache.
const reloadTimeout = 5 * time.Minute

// ReloadInput is the structured input for the reload tool.
type ReloadInput struct {
	WorldPath   string `json:"world_path"             jsonschema_description:"Path to the world project directory containing world.toml (the source that gets rebuilt). Required — unlike the cluster-only tools, this one compiles local code."`
	ShardID     string `json:"shard_id,omitempty"     jsonschema_description:"Shard to reload, e.g. 'game'. Defaults to every shard in world.toml. A pool reloads as a whole — its replicas cannot roll independently."`
	Purge       bool   `json:"purge,omitempty"        jsonschema_description:"Wipe the reloaded shards' state (NATS JetStream) so they restart from tick 0. Defaults to false, which keeps existing state and just rolls the new images in."`
	OperatorURL string `json:"operator_url,omitempty" jsonschema_description:"cardinal-operator URL (defaults to http://localhost:8090 for local dev)"`
}

// ReloadOutput is the structured output for the reload tool.
type ReloadOutput struct {
	Organization string   `json:"organization" jsonschema_description:"World organization from world.toml"`
	Project      string   `json:"project"      jsonschema_description:"World project from world.toml"`
	Shards       []string `json:"shards"       jsonschema_description:"Shard IDs that were rebuilt and reloaded"`
	Purged       bool     `json:"purged"       jsonschema_description:"Whether the reloaded shards' state was wiped"`
	Status       string   `json:"status"       jsonschema_description:"Human-readable result summary"`
}

// registerReloadTool registers the reload tool — the MCP counterpart of
// `world reload`, for rolling freshly compiled shard code into a running world.
func registerReloadTool(srv *server.MCPServer) {
	reloadTool := mcp.NewTool(
		"reload",
		mcp.WithDescription(
			"Rebuild a running world's Cardinal shards from local source and roll them into the cluster. "+
				"Use this after editing shard code to (1) confirm the new code compiles — a build failure "+
				"aborts with the compiler error and leaves the world untouched — and (2) run against it. "+
				"By default every shard is reloaded and existing state is kept; set shard_id to reload one "+
				"shard, and purge to wipe the reloaded shards' state so they restart from tick 0. The build "+
				"runs first; only once it succeeds is anything in the cluster touched. The cardinal-operator "+
				"stays up throughout, so the reload skips its cold start. A reload that changed any command, "+
				"component, or event makes earlier introspect results stale — run introspect again for the "+
				"updated schemas. A reload of pure system-logic changes leaves them valid. Requires a running "+
				"cluster and a local world.toml at world_path.",
		),
		mcp.WithInputSchema[ReloadInput](),
		mcp.WithOutputSchema[ReloadOutput](),
	)
	srv.AddTool(reloadTool, strictToolHandler(reloadHandler))
}

// reloadHandler builds the world's shards, then rolls them into the cluster,
// optionally onto wiped state. Building BEFORE any cluster mutation means the
// common failure — code that won't compile — leaves the running world untouched.
func reloadHandler(
	ctx context.Context,
	_ mcp.CallToolRequest,
	args ReloadInput,
) (ReloadOutput, error) {
	worldPath := strings.TrimSpace(args.WorldPath)
	if worldPath == "" {
		return ReloadOutput{}, eris.New(
			"world_path is required (path to the world project directory containing world.toml)")
	}
	worldPath = filepath.Clean(worldPath)
	shardID := strings.TrimSpace(args.ShardID)

	ctx, cancel := ensureDeadline(ctx, reloadTimeout)
	defer cancel()

	// Build first. A compile error returns here with nothing touched, so the
	// running world is left exactly as it was — the "does my new code compile?" check.
	cfg, deployOpts, err := buildWorldShards(ctx, worldPath, shardID)
	if err != nil {
		return ReloadOutput{}, eris.Wrap(err, "build failed (did the new shard code compile?)")
	}
	if err := reloadShards(ctx, cfg, deployOpts, args.Purge); err != nil {
		return ReloadOutput{}, err
	}

	shardIDs := deployedShardIDs(deployOpts)
	status := fmt.Sprintf("rebuilt and reloaded %d shard(s) for %s/%s",
		len(shardIDs), cfg.Organization, cfg.Project)
	if args.Purge {
		status += " on fresh state (tick reset to 0)"
	}
	return ReloadOutput{
		Organization: cfg.Organization,
		Project:      cfg.Project,
		Shards:       shardIDs,
		Purged:       args.Purge,
		Status:       status,
	}, nil
}

// reloadShards rolls already-built images into the cluster. Without purge that
// is a plain redeploy onto existing state. With purge each targeted pool is
// undeployed first — so no shard is alive to re-snapshot over the wipe — then
// its replicas' JetStream state is wiped and its ShardPool re-applied. The
// operator stays up throughout so the redeploy skips its cold start.
func reloadShards(ctx context.Context, cfg worldtoml.Config, deployOpts cluster.DeployOpts, purge bool) error {
	cli := clusterClient()

	if purge {
		for _, shard := range deployOpts.Shards {
			if err := cli.UndeployShard(ctx, shard.ID); err != nil {
				return eris.Wrapf(err, "failed to undeploy shard %q for purge", shard.ID)
			}
			// Replicas each keep their own state, so wipe per instance. A wipe
			// failure IS fatal: redeploying over stale snapshots would resume the
			// old tick — the opposite of the fresh start asked for.
			for _, instance := range shardInstances(cfg, shard.ID) {
				if err := cli.PurgeShardState(ctx, cfg.Organization, cfg.Project, instance); err != nil {
					return eris.Wrapf(err, "failed to wipe state for %q", instance)
				}
			}
			if err := cli.DeployShard(ctx, cfg, shard.ID); err != nil {
				return eris.Wrapf(err, "failed to re-apply shard %q", shard.ID)
			}
		}
	}

	// Reload only rolls its targets, so nothing else reconciles the pool set.
	if err := cli.PruneOrphanedShards(ctx, cfg); err != nil {
		return eris.Wrap(err, "failed to prune orphaned shards")
	}
	if err := cli.Deploy(ctx, deployOpts); err != nil {
		return eris.Wrap(err, "failed to deploy shards")
	}
	// Hold until the fresh pods are Ready: Deploy returns before they leave
	// ContainerCreating, and callers act the moment this returns.
	cli.WaitForShardsReady(ctx, cfg, nil)
	return nil
}

// shardInstances lists a pool's replica instance IDs — the granularity
// PurgeShardState wipes at, since each replica keeps its own state.
func shardInstances(cfg worldtoml.Config, poolID string) []string {
	instances := make([]string, 0, 1)
	for _, s := range cfg.Shards {
		if s.ID != poolID {
			continue
		}
		instance := s.InstanceID
		if instance == "" {
			instance = s.ID
		}
		instances = append(instances, instance)
	}
	return instances
}

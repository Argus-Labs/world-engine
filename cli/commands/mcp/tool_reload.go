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

// reloadTimeout is a hang backstop, not a budget: a warm reload runs in seconds,
// so this only needs headroom for a cold Docker cache.
const reloadTimeout = 5 * time.Minute

// ReloadInput is the structured input for the reload tool.
type ReloadInput struct {
	WorldPath string `json:"world_path"         jsonschema_description:"Path to the world project directory containing world.toml (the source that gets rebuilt). Required — unlike the read-only tools, this one compiles local code."`
	ShardID   string `json:"shard_id,omitempty" jsonschema_description:"Shard to reload, e.g. 'game'. Defaults to every shard in world.toml. A pool reloads as a whole — its replicas cannot roll independently."`
	Purge     bool   `json:"purge,omitempty"    jsonschema_description:"Wipe the reloaded shards' state (NATS JetStream) so they restart from tick 0. Defaults to false, which keeps existing state and just rolls the new images in."`
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
			"Rebuild a running world's Cardinal shards from local source and recreate their containers. "+
				"Use this after editing shard code to (1) confirm the new code compiles — a build failure "+
				"aborts with the compiler error and leaves the world untouched — and (2) run against it. "+
				"By default every shard is reloaded and existing state is kept; set shard_id to reload one "+
				"shard, and purge to wipe the reloaded shards' state so they restart from tick 0. The build "+
				"runs first; only once it succeeds is any container touched. NATS and Postgres "+
				"stay up throughout, so the reload skips the cold start. A reload that changed any command, "+
				"component, or event makes earlier introspect results stale — run introspect again for the "+
				"updated schemas. A reload of pure system-logic changes leaves them valid. Requires a running "+
				"world and a local world.toml at world_path.",
		),
		mcp.WithInputSchema[ReloadInput](),
		mcp.WithOutputSchema[ReloadOutput](),
	)
	srv.AddTool(reloadTool, strictToolHandler(reloadHandler))
}

// reloadHandler builds the world's shards, then recreates their containers,
// optionally onto wiped state. Building BEFORE any container mutation means the
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
	rt, err := started.open(worldPath)
	if err != nil {
		return ReloadOutput{}, eris.Wrap(err, "open world")
	}
	if err := reloadShards(ctx, rt, cfg, deployOpts, args.Purge); err != nil {
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

// reloadShards recreates containers on already-built images. Without purge that
// is a plain redeploy onto existing state. With purge each targeted pool is
// removed first — so no shard is alive to re-snapshot over the wipe — then its
// replicas' JetStream state is wiped; the deploy below brings it back on the new image.
func reloadShards(
	ctx context.Context,
	rt *local.Runtime,
	cfg worldtoml.Config,
	deployOpts local.DeployOpts,
	purge bool,
) error {
	if purge {
		for _, shard := range deployOpts.Shards {
			if err := rt.UndeployShard(ctx, shard.ID); err != nil {
				return eris.Wrapf(err, "failed to undeploy shard %q for purge", shard.ID)
			}
			// Replicas each keep their own state, so wipe per instance. A wipe
			// failure IS fatal: redeploying over stale snapshots would resume the
			// old tick — the opposite of the fresh start asked for.
			for _, instance := range shardInstances(cfg, shard.ID) {
				if err := rt.PurgeShardState(ctx, instance); err != nil {
					return eris.Wrapf(err, "failed to wipe state for %q", instance)
				}
			}
		}
	}

	// Reload only rolls its targets, so nothing else reconciles the instance set.
	if err := rt.PruneOrphanedShards(ctx); err != nil {
		return eris.Wrap(err, "failed to prune orphaned shards")
	}
	if err := rt.Deploy(ctx, deployOpts); err != nil {
		return eris.Wrap(err, "failed to deploy shards")
	}
	// Hold until the fresh containers accept connections; callers act the moment this
	// returns, so a shard that never comes back is an error rather than a silent pass.
	// Only the reloaded pools were touched; waiting on an instance the caller left down
	// would burn the whole timeout and then report a shortfall that is not this reload's.
	var reloaded []string
	for _, shard := range deployOpts.Shards {
		reloaded = append(reloaded, shardInstances(cfg, shard.ID)...)
	}
	if ready, expected := rt.WaitForShardsReady(ctx, reloaded, nil); ready < expected {
		return eris.Errorf("only %d/%d shard instances came back up; check the shard logs", ready, expected)
	}
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

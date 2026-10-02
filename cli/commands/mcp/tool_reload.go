package mcp

import (
	"context"
	"errors"
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
	if err := reloadShards(ctx, clusterClient(), cfg, deployOpts, args.Purge); err != nil {
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

// reloadClient is the cluster-facing seam reloadShards drives. Defined as an
// interface so the purge loop's resilience contract (always re-apply the CR,
// always roll images, surface wipe errors afterward — mirroring `world
// reload`'s purgeAndRedeploy + deployShardImages) can be exercised without a
// live k3d cluster; *cluster.Client satisfies it in production.
type reloadClient interface {
	UndeployShard(ctx context.Context, shardID string) error
	PurgeShardState(ctx context.Context, org, project, instanceID string) error
	DeployShard(ctx context.Context, cfg worldtoml.Config, shardID string) error
	PruneOrphanedShards(ctx context.Context, cfg worldtoml.Config) error
	Deploy(ctx context.Context, opts cluster.DeployOpts) error
	WaitForShardsReady(ctx context.Context, cfg worldtoml.Config, onProgress func(ready, expected int))
}

// reloadShards rolls already-built images into the cluster. Without purge that
// is a plain redeploy onto existing state. With purge each targeted pool is
// undeployed first — so no shard is alive to re-snapshot over the wipe — then
// its replicas' JetStream state is wiped and its ShardPool re-applied. The
// operator stays up throughout so the redeploy skips its cold start.
//
// Purge errors are accumulated, never returned early: a wipe failure is fatal
// to the purge outcome (don't claim fresh state), but must NOT leave the shard
// undeployed or skip the image roll. The CR is always re-applied via
// DeployShard and the trailing Deploy always runs so freshly built images roll
// to every targeted shard; any purge error is surfaced only after the world
// is made runnable. This mirrors `world reload`'s contract, where the same
// purge is "fatal" to the wipe but never to the cluster's runnability.
func reloadShards(
	ctx context.Context,
	cli reloadClient,
	cfg worldtoml.Config,
	deployOpts cluster.DeployOpts,
	purge bool,
) error {
	var purgeErr error
	if purge {
		for _, shard := range deployOpts.Shards {
			// Undeploy deletes the ShardPool CR first (k8s GCs its pods). If it
			// fails the CR is still there, so there is nothing to wipe or
			// re-apply for this shard — record the error and continue so the
			// rest of the world is still purged and its images still roll.
			if err := cli.UndeployShard(ctx, shard.ID); err != nil {
				purgeErr = errors.Join(purgeErr, eris.Wrapf(err, "failed to undeploy shard %q for purge", shard.ID))
				continue
			}
			// Replicas each keep their own state, so wipe per instance. A wipe
			// failure IS fatal to the *purge*: redeploying over stale snapshots
			// would resume the old tick — the opposite of the fresh start asked
			// for. But it is NOT fatal to the world: the CR is re-applied below
			// so the shard is not left undeployed, and the trailing Deploy
			// still rolls the new image.
			for _, instance := range shardInstances(cfg, shard.ID) {
				if err := cli.PurgeShardState(ctx, cfg.Organization, cfg.Project, instance); err != nil {
					purgeErr = errors.Join(purgeErr, eris.Wrapf(err, "failed to wipe state for %q", instance))
				}
			}
			// Always re-apply the CR so a wipe failure can't leave the shard
			// undeployed — UndeployShard already deleted it up above.
			if err := cli.DeployShard(ctx, cfg, shard.ID); err != nil {
				purgeErr = errors.Join(purgeErr, eris.Wrapf(err, "failed to re-apply shard %q", shard.ID))
			}
		}
	}

	// Always run, even after a purge error, so the re-applied ShardPool CRs get
	// their images and the freshly built code rolls to every targeted shard —
	// avoiding ImagePullBackOff. (Mirrors `world reload`'s deployShardImages,
	// whose "Always runs, even after a purge error" comment names this exact
	// guarantee.)
	var stepErr error
	if err := cli.PruneOrphanedShards(ctx, cfg); err != nil {
		stepErr = eris.Wrap(err, "failed to prune orphaned shards")
	} else if err := cli.Deploy(ctx, deployOpts); err != nil {
		stepErr = eris.Wrap(err, "failed to deploy shards")
	} else {
		// Hold until the fresh pods are Ready: Deploy returns before they leave
		// ContainerCreating, and callers act the moment this returns. Best-effort
		// (returns no error), so it only runs on a clean deploy.
		cli.WaitForShardsReady(ctx, cfg, nil)
	}

	// A trailing step failure takes precedence but is reported together with a
	// purge error so neither masks the other; a clean deploy surfaces any
	// deferred purge error only after the world is runnable.
	switch {
	case stepErr != nil && purgeErr != nil:
		return errors.Join(eris.Wrap(purgeErr, "purge shards"), stepErr)
	case stepErr != nil:
		return stepErr
	case purgeErr != nil:
		return eris.Wrap(purgeErr, "purge shards")
	default:
		return nil
	}
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

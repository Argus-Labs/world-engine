package cluster

import (
	"context"
	"slices"
	"sync"

	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/pkg/toml"
)

// Client orchestrates the local Cardinal cluster lifecycle: k3d bring-up,
// platform install, per-project operator + ShardPools, and Deploy RPC calls.
type Client struct {
	cfg Config

	// mu guards the memoized kube client. Building it fetches the k3d kubeconfig
	// (Docker round-trips) and constructs k8s clients, so a long-lived Client
	// reuses it across calls; transient per-operation Clients just build it once.
	// Only the client/connection is reused — data is always read live.
	mu         sync.Mutex
	cachedKube *kubeClient
}

// NewClient constructs a Client, defaulting unset fields. Also installs the
// log level into k3d's global logger (package-level state in k3d's lib).
func NewClient(cfg Config) *Client {
	c := &Client{cfg: cfg.withDefaults()}
	setK3dLogLevel(c.cfg.LogLevel, c.cfg.OnK3DLog)
	return c
}

// Config returns the resolved configuration used by this Client.
func (c *Client) Config() Config { return c.cfg }

// ResetLogRouting stops routing k3d's global logger through OnK3DLog. k3d's
// logger is package-level state — it stays wired to the most recent
// NewClient's callback even after that call returns, so callers whose
// OnK3DLog updates per-operation UI (e.g. a phasebox row) must call this
// once the operation finishes, or a lingering background goroutine could
// resurrect already-torn-down UI. Falls back to a silent no-op, not
// [os.Stderr], since a stray late line has nowhere useful to go.
func (c *Client) ResetLogRouting() {
	setK3dLogLevel(c.cfg.LogLevel, func(string) {})
}

// StartOpts controls Start behavior.
type StartOpts struct {
	Project string
	Config  toml.Config
	// OnStep, if non-nil, gets a short label before each phase of
	// StartPlatform and DeployWorld (see StartPlatform's doc) — lets `world
	// start` show a live checklist instead of a static spinner during the
	// ~15-60s cold start.
	OnStep func(step string)
}

// Start brings up the local cluster end-to-end for one project — the bundled
// convenience path (world-cli `world start`) composing StartPlatform then
// DeployWorld. The editor instead drives those two separately so the cluster
// lifecycle is independent of any one world.
func (c *Client) Start(ctx context.Context, opts StartOpts) error {
	if opts.Project == "" {
		return eris.New("Start: project is required")
	}
	if err := c.StartPlatform(ctx, opts.OnStep); err != nil {
		return err
	}
	return c.DeployWorld(ctx, opts.Config, opts.OnStep)
}

// StartPlatform brings up the shared, world-agnostic cluster environment:
//
//	k3d up → apply CRD → ensure platform (NATS, Traefik).
//
// No operator, no world resources — those are DeployWorld's job (mirroring prod,
// the operator is namespace-scoped and co-located with its shards, so it comes up
// per-world at deploy time). StartPlatform blocks until shared NATS and Traefik
// are Ready so shards can connect and clients can route as soon as DeployWorld
// creates them.
//
// onStep, if non-nil, gets a short label before each phase so callers (the
// editor's cluster toggle) can show progress through the ~15-25s cold start.
func (c *Client) StartPlatform(ctx context.Context, onStep func(step string)) error {
	step := func(label string) {
		if onStep != nil {
			onStep(label)
		}
	}

	step("Starting k3d cluster")
	if err := c.ensureCluster(ctx); err != nil {
		return eris.Wrap(err, "ensure cluster")
	}

	k, err := c.kube(ctx)
	if err != nil {
		return err
	}

	step("Installing CRDs")
	if err := c.ensureCRD(ctx, k); err != nil {
		return err
	}
	step("Installing platform (NATS, Traefik)")
	if err := c.ensurePlatform(ctx, k); err != nil {
		return err
	}

	// Gate on NATS readiness before deploying shard pods that connect to it.
	// Operator is deliberately absent — DeployWorld brings it up.
	step("Waiting for NATS")
	if err := k.waitForServiceReady(ctx, natsNamespace, natsService); err != nil {
		return eris.Wrap(err, "wait for NATS to be ready")
	}
	step("Waiting for Traefik")
	if err := k.waitForServiceReady(ctx, traefikNamespace, traefikService); err != nil {
		return eris.Wrap(err, "wait for Traefik to be ready")
	}
	return nil
}

// DeployWorld brings a world up on an already-running cluster (via StartPlatform):
// installs the operator, waits for Ready, then applies the world's ShardPool CRs.
// Operator and shards co-locate in operatorNamespace (prod-style, namespace-
// scoped); one world at a time keeps that namespace unambiguous. Idempotent.
//
// onStep, if non-nil, gets a short label before each phase — same contract
// as StartPlatform's onStep.
func (c *Client) DeployWorld(ctx context.Context, cfg toml.Config, onStep func(step string)) error {
	step := func(label string) {
		if onStep != nil {
			onStep(label)
		}
	}

	pools, err := c.ShardPoolsFromConfig(cfg)
	if err != nil {
		return eris.Wrap(err, "translate ShardPools")
	}

	k, err := c.kube(ctx)
	if err != nil {
		return err
	}
	// Enforce one-world-at-a-time at the cluster (the source of truth), so it holds
	// for every caller and survives races the editor's in-memory run-state can't see.
	if err := c.ensureSingleWorld(ctx, k, cfg); err != nil {
		return err
	}
	// Operator first so it's reconciling before the ShardPools land.
	step("Installing operator")
	if err := c.ensureOperator(ctx, k, shardDBDSN(cfg)); err != nil {
		return err
	}
	step("Waiting for operator")
	if err := k.waitForServiceReady(ctx, operatorNamespace, operatorService); err != nil {
		return eris.Wrap(err, "wait for operator to be ready")
	}

	// Provision the project DB first; skipped only when a config_db service brings its own.
	if !hasConfigDBService(cfg) {
		step("Provisioning project DB")
		if err := ensureProjectDB(ctx, k, cfg.Project); err != nil {
			return eris.Wrap(err, "ensure project DB")
		}
	}
	step("Applying services")
	if err := c.ensureServices(ctx, k, cfg); err != nil {
		return eris.Wrap(err, "ensure services")
	}
	// Reap services dropped from world.toml since the last Start (best-effort).
	if err := gcOrphanedServices(ctx, k, cfg); err != nil {
		return eris.Wrap(err, "gc orphaned services")
	}

	step("Applying ShardPools")
	if err := c.applyShardPools(ctx, k, pools); err != nil {
		return err
	}
	// Reap ShardPools dropped from world.toml, as gcOrphanedServices does above.
	if err := gcOrphanedShardPools(ctx, k, cfg); err != nil {
		return eris.Wrap(err, "gc orphaned ShardPools")
	}
	return nil
}

// UndeployWorld tears a world down: deletes its ShardPool CRs (k8s GCs the owned
// shard Deployments via owner references) then removes the operator, reversing
// DeployWorld. The shared cluster stays up, and it needs no world config so an
// unparseable world can still be torn down.
//
// It leaves the operator's Service/RBAC for the next DeployWorld to re-apply;
// operator RPCs during the no-world window get connection-refused, so callers
// should read ShardPool CRs (DeployedWorldKeys) to tell what's deployed.
func (c *Client) UndeployWorld(ctx context.Context) error {
	k, err := c.kube(ctx)
	if err != nil {
		return err
	}
	if err := k.deleteShardPoolsInNamespace(ctx, operatorNamespace); err != nil {
		return err
	}
	return k.deleteOperatorDeployment(ctx, operatorNamespace)
}

// UndeployShards is the shards-only half of UndeployWorld: it deletes this world's
// ShardPool CRs (k8s GCs the owned pods) but leaves the operator running. Keeping
// the operator up makes the next DeployWorld cheap — its ensureOperator +
// readiness wait become no-ops instead of a ~10-25s operator cold start.
func (c *Client) UndeployShards(ctx context.Context) error {
	k, err := c.kube(ctx)
	if err != nil {
		return err
	}
	return k.deleteShardPoolsInNamespace(ctx, operatorNamespace)
}

// UndeployShard is the single-shard analog of UndeployShards: it deletes just
// shardID's ShardPool CR (k8s GCs its owned pods), leaving every other shard and
// the operator untouched. Pairs with DeployShard + PurgeShardState so reloading
// one shard with --purge can't take down the rest of the world.
func (c *Client) UndeployShard(ctx context.Context, shardID string) error {
	k, err := c.kube(ctx)
	if err != nil {
		return err
	}
	return k.deleteShardPool(ctx, operatorNamespace, shardID)
}

// DeployShard re-applies a single shard's ShardPool CR translated from cfg,
// without touching the operator, project DB, [[services]], or any other shard's
// pool. The operator must already be running (DeployWorld's job) — this only
// brings shardID's pool back, e.g. after UndeployShard + PurgeShardState.
func (c *Client) DeployShard(ctx context.Context, cfg toml.Config, shardID string) error {
	pools, err := c.ShardPoolsFromConfig(cfg)
	if err != nil {
		return eris.Wrap(err, "translate ShardPools")
	}
	idx := slices.IndexFunc(pools, func(p ShardPool) bool { return p.ShardID == shardID })
	if idx < 0 {
		return eris.Errorf("shard %q not found in world.toml", shardID)
	}

	k, err := c.kube(ctx)
	if err != nil {
		return err
	}
	return c.applyShardPools(ctx, k, pools[idx:idx+1])
}

// PruneOrphanedShards deletes the ShardPool CRs of shards no longer in cfg.
func (c *Client) PruneOrphanedShards(ctx context.Context, cfg toml.Config) error {
	k, err := c.kube(ctx)
	if err != nil {
		return err
	}
	return gcOrphanedShardPools(ctx, k, cfg)
}

// ensureSingleWorld refuses to deploy cfg's world when a different world's
// ShardPools are already on the cluster. One world at a time: operator and all
// shards share a namespace, so a second world would collide on ShardPool names
// and the one operator would reconcile both. Re-deploying the same world
// (org/project) is an allowed idempotent update.
//
// Check-then-act, not an atomic lock: two DeployWorld calls racing could both
// see no world and proceed. We don't take a cluster lease — single-user dev, and
// the editor already serializes its own deploys (worldruntime.beginStart), so
// hitting it needs two hand-driven clients. Bounded: overlapping shard IDs
// converge via SSA (last write wins), and two-world state self-heals next deploy.
func (c *Client) ensureSingleWorld(ctx context.Context, k *kubeClient, cfg toml.Config) error {
	existing, err := k.listShardPoolWorldKeys(ctx, operatorNamespace)
	if err != nil {
		return eris.Wrap(err, "check for an already-deployed world")
	}
	want := WorldKey(cfg.Organization, cfg.Project)
	for key := range existing {
		if key != want {
			return eris.Errorf(
				"another world (%s) is already running — stop it before starting %s", key, want)
		}
	}
	return nil
}

// kube returns a kubeClient for the shared cluster, building it from the k3d
// kubeconfig on first use and memoizing it for reuse across calls on this Client.
func (c *Client) kube(ctx context.Context) (*kubeClient, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cachedKube != nil {
		return c.cachedKube, nil
	}
	kubeconfig, err := k3dKubeconfig(ctx, c.cfg.ClusterName)
	if err != nil {
		return nil, err
	}
	k, err := newKubeClient(kubeconfig)
	if err != nil {
		return nil, err
	}
	c.cachedKube = k
	return k, nil
}

// invalidateKube drops the memoized kubeClient so the next kube() rebuilds it —
// used to recover a long-lived Client whose connection died (e.g. the cluster
// restarted).
func (c *Client) invalidateKube() {
	c.mu.Lock()
	c.cachedKube = nil
	c.mu.Unlock()
}

// withKube runs fn with the memoized kube client, rebuilding it and retrying once
// if fn fails — so a long-lived Client self-heals after a cluster restart while a
// healthy call never re-fetches the kubeconfig.
func (c *Client) withKube(ctx context.Context, fn func(*kubeClient) error) error {
	k, err := c.kube(ctx)
	if err != nil {
		return err
	}
	if err = fn(k); err == nil {
		return nil
	}
	c.invalidateKube()
	k, retryErr := c.kube(ctx)
	if retryErr != nil {
		return retryErr
	}
	return fn(k)
}

// StopOpts controls Stop behavior. Reserved for future multi-project use.
type StopOpts struct{}

// Stop pauses the k3d cluster (`k3d cluster stop`). Pod state is preserved on
// disk; the next Start brings everything back up quickly. For single-project
// dev (the v1 surface), this is what `world stop` does.
//
// Multi-project label-selector deletion (per ADR-055's stop semantics) is
// deferred until per-project namespacing lands.
func (c *Client) Stop(ctx context.Context, _ StopOpts) error {
	exists, err := k3dExists(ctx, c.cfg.ClusterName)
	if err != nil {
		return err
	}
	if !exists {
		return nil // nothing to stop
	}
	return k3dStop(ctx, c.cfg.ClusterName)
}

// IsRunning reports whether the shared local cluster exists and is currently
// running. A cluster paused via Stop exists but is not running. Reconciling
// editor run-state after a restart uses this to confirm a persisted k8s world
// is still live before adopting it — without re-establishing port-forwards.
func (c *Client) IsRunning(ctx context.Context) (bool, error) {
	return k3dRunning(ctx, c.cfg.ClusterName)
}

// PurgeOpts controls Purge behavior. Reserved for future multi-project use.
type PurgeOpts struct{}

// Purge deletes the k3d cluster entirely (`k3d cluster delete`). State lost.
func (c *Client) Purge(ctx context.Context, _ PurgeOpts) error {
	exists, err := k3dExists(ctx, c.cfg.ClusterName)
	if err != nil {
		return err
	}
	if !exists {
		return nil // nothing to delete
	}
	if err := k3dDelete(ctx, c.cfg.ClusterName); err != nil {
		return err
	}
	c.invalidateKube()
	return nil
}

// DeployOpts controls Deploy behavior.
type DeployOpts struct {
	Project string
	Shards  []DeployShard

	// OnResult, if non-nil, gets each shard's result once (nil on success).
	OnResult func(shardID string, err error)
}

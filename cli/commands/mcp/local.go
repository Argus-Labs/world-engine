package mcp

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/pkg/docker"
	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	"github.com/argus-labs/world-engine/cli/pkg/local"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
	"github.com/argus-labs/world-engine/cli/pkg/worldstatus"
)

// logWindow bounds how long a log fetch follows a live container: the tail is
// replayed immediately, then the context expires and the stream returns.
const logWindow = 3 * time.Second

// resolveProject returns the explicit project, else the one in the working directory's world.toml.
func resolveProject(project string) (string, error) {
	if p := strings.TrimSpace(project); p != "" {
		return p, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", eris.Wrap(err, "current directory")
	}
	cfg, err := worldtoml.LoadFile(filepath.Join(cwd, worldtoml.FileName))
	if err != nil {
		return "", eris.Wrap(err, "project not given and no world.toml in the working directory")
	}
	return cfg.Project, nil
}

// worlds holds one Runtime per project this MCP process has started or reloaded.
// The edge proxy for a started world runs here, in the MCP process, so routes
// refreshed by a reload reach the proxy clients are talking to.
type worlds struct {
	mu       sync.Mutex
	runtimes map[string]*local.Runtime
	edges    map[string]context.CancelFunc
}

//nolint:gochecknoglobals // process-wide registry of started worlds
var started = &worlds{runtimes: map[string]*local.Runtime{}, edges: map[string]context.CancelFunc{}}

// open returns the Runtime for the world at worldPath, creating and caching it.
func (w *worlds) open(worldPath string) (*local.Runtime, error) {
	cfg, err := docker.NewClientConfig(worldPath, false)
	if err != nil {
		return nil, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if rt, ok := w.runtimes[cfg.Project]; ok {
		return rt, nil
	}
	dc, err := docker.NewClient(cfg, &docker.ClientOptions{Logger: slog.Default()})
	if err != nil {
		return nil, eris.Wrap(err, "docker client")
	}
	rt := local.New(dc, cfg)
	w.runtimes[cfg.Project] = rt
	return rt, nil
}

// serveEdge starts the project's edge proxy if it is not already running.
func (w *worlds) serveEdge(rt *local.Runtime) error {
	project := rt.Config().Project
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.edges[project]; ok {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- rt.ServeEdge(ctx) }()
	select {
	case err := <-errCh:
		cancel()
		return err
	case <-time.After(200 * time.Millisecond):
	}
	w.edges[project] = cancel
	return nil
}

// stopEdge shuts the project's edge down, if this process runs one.
func (w *worlds) stopEdge(project string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if cancel, ok := w.edges[project]; ok {
		cancel()
		delete(w.edges, project)
	}
}

// reader returns a Runtime for reads on a project: the cached one when this
// process started it, else one built from the project label alone (no world.toml
// needed; status and logs come from the containers).
func (w *worlds) reader(project string) (*local.Runtime, func(), error) {
	project, err := resolveProject(project)
	if err != nil {
		return nil, nil, err
	}
	w.mu.Lock()
	rt, ok := w.runtimes[project]
	w.mu.Unlock()
	if ok {
		return rt, func() {}, nil
	}
	cfg := &service.Config{
		Project:   project,
		NATSURL:   service.NatsURL(project),
		WorldToml: worldtoml.Config{Project: project},
	}
	dc, err := docker.NewClient(cfg, &docker.ClientOptions{Logger: slog.Default()})
	if err != nil {
		return nil, nil, eris.Wrap(err, "docker client")
	}
	return local.New(dc, cfg), func() { _ = dc.Close() }, nil
}

// worldStatus reads the project's pools from its containers.
func worldStatus(ctx context.Context, project string) ([]worldstatus.PoolStatus, error) {
	rt, done, err := started.reader(project)
	if err != nil {
		return nil, err
	}
	defer done()
	status, err := rt.Status(ctx)
	if err != nil {
		return nil, eris.Wrap(err, "world status request failed")
	}
	return status, nil
}

// toShardPools converts the runtime status into the MCP status shape.
func toShardPools(status []worldstatus.PoolStatus) []ShardPool {
	pools := make([]ShardPool, 0, len(status))
	for _, p := range status {
		instances := make([]ShardInstance, 0, len(p.Instances))
		for _, inst := range p.Instances {
			instances = append(instances, ShardInstance{
				Name:         inst.Name,
				Container:    inst.Runtime,
				Phase:        inst.Phase,
				Ready:        inst.Ready,
				RestartCount: inst.RestartCount,
				Age:          inst.Age,
			})
		}
		pools = append(pools, ShardPool{
			ShardID:   p.ShardID,
			PoolSize:  p.PoolSize,
			Image:     p.Image,
			Phase:     p.Phase,
			Instances: instances,
		})
	}
	return pools
}

// poolFor returns a shard's pool, or an empty one when the shard has none. Scoping instance
// lookups to the shard's own pool is what keeps a reference like "2" from resolving into a
// different shard's pool.
func poolFor(status []worldstatus.PoolStatus, shardID string) worldstatus.PoolStatus {
	for _, pool := range status {
		if pool.ShardID == shardID {
			return pool
		}
	}
	return worldstatus.PoolStatus{ShardID: shardID}
}

// shardContainers returns the container names backing a shard. With instanceName
// empty, every instance of the pool; otherwise only the matching instance, using
// lenient matching (see instanceMatches) so "game 5"/"game5"/"game-5"/"5" all
// resolve to instance "game-5".
func shardContainers(status []worldstatus.PoolStatus, shardID, instanceName string) []string {
	var names []string
	for _, inst := range poolFor(status, shardID).Instances {
		if instanceName != "" && !instanceMatches(inst.Name, shardID, instanceName) {
			continue
		}
		if inst.Runtime != "" {
			names = append(names, inst.Runtime)
		}
	}
	return names
}

// normalizeInstanceKey reduces an instance identifier to a comparison key,
// dropping case and any separators/spaces so "game 5", "game5" and "GAME-5"
// reduce to the same key as "game-5".
func normalizeInstanceKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// instanceMatches reports whether the instance named instName satisfies
// a user-supplied reference for the given shard, tolerating separator/spacing/
// case variants. A pool's first instance is the bare shard ID (there is no
// "<shard>-1"), so it also matches an index-1 reference ("game 1"/"1"); a
// "<shard>-N" instance additionally matches the bare index "N".
func instanceMatches(instName, shardID, input string) bool {
	if input == "" {
		return false
	}
	if instName == input {
		return true
	}
	in := normalizeInstanceKey(input)
	normInst := normalizeInstanceKey(instName)
	normShard := normalizeInstanceKey(shardID)
	if in == normInst {
		return true
	}
	if instName == shardID {
		return in == "1" || in == normShard+"1"
	}
	if idx, ok := strings.CutPrefix(normInst, normShard); ok && idx != "" {
		return in == idx
	}
	return false
}

// shardInstanceNames lists a shard's instance names, for "instance not found" errors.
func shardInstanceNames(status []worldstatus.PoolStatus, shardID string) []string {
	var names []string
	for _, inst := range poolFor(status, shardID).Instances {
		names = append(names, inst.Name)
	}
	return names
}

// errInstanceNotFound is the uniform "instance not found" error, listing the
// shard's live instances so the caller can correct the reference.
func errInstanceNotFound(status []worldstatus.PoolStatus, shardID, instanceName string) error {
	return eris.Errorf("instance %q not found for shard %q; available instances: %v",
		instanceName, shardID, shardInstanceNames(status, shardID))
}

// resolveInstanceName canonicalizes a user-supplied instance reference for a
// shard against the live instances (lenient matching, scoped to the shard's own
// pool). Empty input defaults to the pool's first instance (the bare shard ID).
func resolveInstanceName(ctx context.Context, project, shardID, instanceName string) (string, error) {
	instanceName = strings.TrimSpace(instanceName)
	if instanceName == "" {
		return shardID, nil
	}
	status, err := worldStatus(ctx, project)
	if err != nil {
		return "", err
	}
	for _, inst := range poolFor(status, shardID).Instances {
		if instanceMatches(inst.Name, shardID, instanceName) {
			return inst.Name, nil
		}
	}
	return "", errInstanceNotFound(status, shardID, instanceName)
}

// collectContainerLogs returns up to tail lines of one container, following it for logWindow.
func collectContainerLogs(ctx context.Context, project, container string, tail int32) (string, error) {
	rt, done, err := started.reader(project)
	if err != nil {
		return "", err
	}
	defer done()
	out := make(chan worldstatus.LogLine, 256)
	errCh := make(chan error, 1)
	go func() {
		errCh <- rt.StreamContainerLogs(ctx, container, worldstatus.LogsOpts{TailLines: tail, FollowWindow: logWindow}, out)
	}()
	var b strings.Builder
	for line := range out {
		b.WriteString(line.Line)
		b.WriteByte('\n')
	}
	if err := <-errCh; err != nil && !eris.Is(err, context.Canceled) && !eris.Is(err, context.DeadlineExceeded) {
		return "", eris.Wrapf(err, "logs for %s", container)
	}
	return b.String(), nil
}

// deployedShard is one shard container's world identity, read from its labels.
type deployedShard struct {
	ShardID      string
	Organization string
	Project      string
}

// deployedShards lists the project's shard pools with their organization/project from container labels.
func deployedShards(ctx context.Context, project string) ([]deployedShard, error) {
	project, err := resolveProject(project)
	if err != nil {
		return nil, err
	}
	rt, done, err := started.reader(project)
	if err != nil {
		return nil, err
	}
	defer done()
	cs, err := rt.Containers(ctx)
	if err != nil {
		return nil, eris.Wrap(err, "failed to list shard containers")
	}
	seen := map[string]bool{}
	var out []deployedShard
	for _, c := range cs {
		id := c.Labels[service.ShardIDLabel]
		if c.Labels[service.RoleLabel] != service.RoleShard || seen[id] {
			continue
		}
		seen[id] = true
		out = append(
			out,
			deployedShard{
				ShardID:      id,
				Organization: c.Labels[service.OrgLabel],
				Project:      c.Labels[service.ProjectLabel],
			},
		)
	}
	return out, nil
}

// resolveShardWorld returns the organization/project of a deployed shard.
// Explicit org+project (both set) short-circuit the lookup.
func resolveShardWorld(ctx context.Context, shardID, org, project string) (string, string, error) {
	if org != "" && project != "" {
		return org, project, nil
	}
	shards, err := deployedShards(ctx, project)
	if err != nil {
		return "", "", err
	}
	for _, s := range shards {
		if s.ShardID == shardID {
			return s.Organization, s.Project, nil
		}
	}
	available := make([]string, 0, len(shards))
	for _, s := range shards {
		available = append(available, s.ShardID)
	}
	return "", "", eris.Errorf("shard %q is not running; running shards: %v", shardID, available)
}

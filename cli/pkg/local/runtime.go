// Package local runs a world as Docker containers: one network, NATS and Postgres
// per project, one container per shard instance, plus the in-process edge proxy
// that gives them the same /<org>/<project>/<instance> URLs a cluster serves.
package local

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/pkg/dnslabel"
	"github.com/argus-labs/world-engine/cli/pkg/docker"
	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	"github.com/argus-labs/world-engine/cli/pkg/edge"
	"github.com/argus-labs/world-engine/cli/pkg/natsstate"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
	"github.com/argus-labs/world-engine/cli/pkg/worldstatus"
)

// Region is the CARDINAL_REGION every local shard runs with; the shard env is
// built from the same constant, so addressing and the container agree.
const Region = service.CardinalRegion

// EdgeAddr is where the edge listens; APIEndpoint is its URL for humans and clients.
const (
	EdgeAddr    = edge.DefaultAddr
	APIEndpoint = "http://localhost:8080"
)

// NatsHostURL is how the host (world reload --purge, clients) reaches the project's NATS.
const NatsHostURL = "nats://127.0.0.1:4222"

// DBEndpoint is the project Postgres as seen from the host.
const DBEndpoint = "localhost:5432"

// stateRunning is Docker's running container state.
const stateRunning = "running"

// Runtime drives one project's containers and routes.
type Runtime struct {
	docker *docker.Client
	cfg    *service.Config
	edge   *edge.Edge
}

// New wraps a Docker client opened on the project (docker.WithClient).
func New(dc *docker.Client, cfg *service.Config) *Runtime {
	return &Runtime{docker: dc, cfg: cfg, edge: edge.New()}
}

// Config returns the project config the runtime was opened on.
func (r *Runtime) Config() *service.Config { return r.cfg }

func (r *Runtime) project() string { return r.cfg.WorldToml.Project }

// ShardAPIURL is the edge URL of one shard instance, the same path the chart routes on a cluster.
func ShardAPIURL(org, project, instance string) string {
	return APIEndpoint + "/" + edge.RouteKey(org, project, instance)
}

// StartPlatform creates the network and starts NATS, the project DB and image-kind
// [[services]], waiting for each to be ready. step reports progress labels.
func (r *Runtime) StartPlatform(ctx context.Context, step func(string)) error {
	if step == nil {
		step = func(string) {}
	}
	step("Network")
	if err := r.docker.EnsureNetwork(ctx, service.NetworkName(r.project())); err != nil {
		return err
	}
	var svcs []service.Service
	svcs = append(svcs, service.NATS(r.cfg))
	if service.NeedsAutoProjectDB(r.cfg.WorldToml) {
		svcs = append(svcs, service.ProjectDBService(r.cfg))
	}
	for _, gs := range r.cfg.WorldToml.Services {
		if !gs.IsBuiltFromSource() {
			svcs = append(svcs, service.GameServiceFromConfig(r.cfg, gs))
		}
	}
	// Recreate so a changed world.toml (image, env, ports) takes effect; volumes stay.
	names := make([]string, 0, len(svcs))
	for _, s := range svcs {
		names = append(names, s.Name)
	}
	if err := r.docker.RemoveContainers(ctx, names, true, nil); err != nil {
		return err
	}
	return r.docker.StartContainers(ctx, svcs, func(p docker.Progress) {
		if p.State == docker.StateStarting && p.Err == nil {
			step(p.Name)
		}
	})
}

// DeployShard names one pool to (re)create from its freshly built image.
type DeployShard struct {
	ID string
}

// DeployOpts selects pools to deploy; OnResult reports each pool as it finishes.
type DeployOpts struct {
	Shards   []DeployShard
	OnResult func(shardID string, err error)
}

// Deploy recreates every instance container of the given pools on the current
// image and env, starts them, and refreshes the edge routes. Instances that
// stopped keep their volumes; the containers themselves are replaced.
func (r *Runtime) Deploy(ctx context.Context, opts DeployOpts) error {
	report := func(id string, err error) {
		if opts.OnResult != nil {
			opts.OnResult(id, err)
		}
	}
	if len(opts.Shards) == 0 {
		return eris.New("Deploy: at least one shard is required")
	}
	var firstErr error
	for _, s := range opts.Shards {
		svcs := r.shardServices(s.ID)
		var err error
		if len(svcs) == 0 {
			err = eris.Errorf("shard %q not found in world.toml", s.ID)
		} else {
			err = r.recreate(ctx, svcs)
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
		report(s.ID, err)
	}
	if err := r.RefreshRoutes(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

// DeployServices recreates the path-kind [[services]] (built from source) on their new images.
func (r *Runtime) DeployServices(ctx context.Context) error {
	var svcs []service.Service
	for _, gs := range r.cfg.WorldToml.Services {
		if gs.IsBuiltFromSource() {
			svcs = append(svcs, service.GameServiceFromConfig(r.cfg, gs))
		}
	}
	if len(svcs) == 0 {
		return nil
	}
	return r.recreate(ctx, svcs)
}

func (r *Runtime) recreate(ctx context.Context, svcs []service.Service) error {
	names := make([]string, 0, len(svcs))
	for _, s := range svcs {
		names = append(names, s.Name)
	}
	if err := r.docker.RemoveContainers(ctx, names, true, nil); err != nil {
		return err
	}
	return r.docker.StartContainers(ctx, svcs, nil)
}

// shardServices returns the instance containers of one pool, in world.toml order.
func (r *Runtime) shardServices(shardID string) []service.Service {
	var out []service.Service
	for i, sh := range r.cfg.WorldToml.Shards {
		if sh.ID == shardID {
			out = append(out, service.CardinalFromShard(r.cfg, sh, service.ShardHostPort(i)))
		}
	}
	return out
}

// UndeployShard removes one pool's containers, keeping volumes, and drops its routes.
func (r *Runtime) UndeployShard(ctx context.Context, shardID string) error {
	names, err := r.containerNames(ctx, func(c docker.ContainerSummary) bool {
		return c.Labels[service.RoleLabel] == service.RoleShard && c.Labels[service.ShardIDLabel] == shardID
	})
	if err != nil {
		return err
	}
	if err := r.docker.RemoveContainers(ctx, names, true, nil); err != nil {
		return err
	}
	return r.RefreshRoutes(ctx)
}

// PruneOrphanedShards removes shard containers whose instance no longer exists in world.toml.
func (r *Runtime) PruneOrphanedShards(ctx context.Context) error {
	known := map[string]bool{}
	for _, sh := range r.cfg.WorldToml.Shards {
		known[sh.InstanceID] = true
	}
	names, err := r.containerNames(ctx, func(c docker.ContainerSummary) bool {
		return c.Labels[service.RoleLabel] == service.RoleShard && !known[c.Labels[service.InstanceLabel]]
	})
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return nil
	}
	return r.docker.RemoveContainers(ctx, names, true, nil)
}

// PurgeShardState wipes one instance's JetStream snapshot bucket. The instance must be undeployed.
func (r *Runtime) PurgeShardState(ctx context.Context, instanceID string) error {
	return natsstate.PurgeShard(ctx, NatsHostURL, r.cfg.WorldToml.Organization, r.project(), instanceID)
}

// Stop stops every container of the project; volumes and containers stay for the next start.
func (r *Runtime) Stop(ctx context.Context, progress func(docker.Progress)) error {
	names, err := r.containerNames(ctx, nil)
	if err != nil {
		return err
	}
	return r.docker.StopContainers(ctx, names, progress)
}

// Purge removes the project's containers, volumes and network; images too when asked.
func (r *Runtime) Purge(ctx context.Context, images bool, progress func(docker.Progress)) error {
	names, err := r.containerNames(ctx, nil)
	if err != nil {
		return err
	}
	if err := r.docker.RemoveContainers(ctx, names, false, progress); err != nil {
		return err
	}
	if err := r.docker.RemoveNetwork(ctx, service.NetworkName(r.project())); err != nil {
		return err
	}
	if images {
		return r.docker.PruneCardinalImages(ctx)
	}
	return nil
}

// IsRunning reports whether the project's NATS is running: the platform is what
// a world start left behind, so reload and logs still work after every shard crashed.
func (r *Runtime) IsRunning(ctx context.Context) (bool, error) {
	cs, err := r.docker.ListProjectContainers(ctx, r.project())
	if err != nil {
		return false, err
	}
	for _, c := range cs {
		if c.Labels[service.RoleLabel] == service.RoleNATS && c.State == stateRunning {
			return true, nil
		}
	}
	return false, nil
}

func (r *Runtime) containerNames(ctx context.Context, keep func(docker.ContainerSummary) bool) ([]string, error) {
	cs, err := r.docker.ListProjectContainers(ctx, r.project())
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(cs))
	for _, c := range cs {
		if keep == nil || keep(c) {
			names = append(names, c.Name)
		}
	}
	return names, nil
}

// Status reports every pool: those in world.toml (so undeployed ones show as
// NotDeployed) and any other shard container carrying the project label, so a
// runtime opened without world.toml (MCP given only a project name) still sees
// what is running.
func (r *Runtime) Status(ctx context.Context) ([]worldstatus.PoolStatus, error) {
	cs, err := r.docker.ListProjectContainers(ctx, r.project())
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var out []worldstatus.PoolStatus
	index := map[string]int{}
	pool := func(shardID string) *worldstatus.PoolStatus {
		if p, ok := index[shardID]; ok {
			return &out[p]
		}
		index[shardID] = len(out)
		out = append(
			out,
			worldstatus.PoolStatus{ShardID: shardID, Image: service.CardinalShardImageName(r.project(), shardID)},
		)
		return &out[len(out)-1]
	}
	declared := map[string]int32{}
	for _, sh := range r.cfg.WorldToml.Shards {
		declared[sh.ID]++
		pool(sh.ID)
	}
	for _, c := range cs {
		if c.Labels[service.RoleLabel] != service.RoleShard {
			continue
		}
		p := pool(c.Labels[service.ShardIDLabel])
		inst := worldstatus.InstanceStatus{Name: c.Labels[service.InstanceLabel], Runtime: c.Name, Phase: c.State}
		if st, err := r.docker.ContainerState(ctx, c.Name); err == nil {
			inst.RestartCount = int32(st.RestartCount)
			if st.Running && !st.StartedAt.IsZero() {
				inst.Age = humanDuration(now.Sub(st.StartedAt))
			}
			if port, err := r.docker.CardinalHostPort(ctx, c.Name); err == nil {
				inst.Ready = st.Running && shardReady(ctx, port)
			}
		}
		p.Instances = append(p.Instances, inst)
	}
	for i := range out {
		sort.Slice(out[i].Instances, func(a, b int) bool { return out[i].Instances[a].Name < out[i].Instances[b].Name })
		out[i].PoolSize = max(declared[out[i].ShardID], int32(len(out[i].Instances)))
		out[i].Phase = poolPhase(out[i])
	}
	return out, nil
}

func poolPhase(p worldstatus.PoolStatus) string {
	if len(p.Instances) == 0 {
		return "NotDeployed"
	}
	running := 0
	for _, in := range p.Instances {
		if in.Phase == stateRunning {
			running++
		}
	}
	switch {
	case running == 0:
		return "Stopped"
	case running == int(p.PoolSize):
		return "Running"
	default:
		return "Partial"
	}
}

func humanDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}

// shardReady reports whether the shard on a host port is serving. Dialling the port is
// not enough: Docker's proxy accepts connections the moment the container starts, so a
// shard that is still booting — or has already crashed — would look ready. Only a reply
// to an HTTP request comes from the process itself.
func shardReady(ctx context.Context, port int) bool {
	ctx, cancel := context.WithTimeout(ctx, shardProbeTimeout)
	defer cancel()
	url := "http://127.0.0.1:" + strconv.Itoa(port) + "/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := probeClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return true
}

const (
	// shardReadyTimeout bounds WaitForShardsReady so a crash-looping shard cannot hang
	// a reload (or make an MCP call sit silently until its own deadline).
	shardReadyTimeout = 2 * time.Minute
	// shardProbeTimeout bounds one readiness request.
	shardProbeTimeout = 2 * time.Second
)

// probeClient has no keep-alives: a pooled connection to a container that has since been
// replaced would answer for its successor.
//
//nolint:gochecknoglobals // a stateless client shared by every readiness probe
var probeClient = &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}

// WaitForShardsReady polls until the named instances serve, ctx ends, or
// shardReadyTimeout elapses. Empty instanceIDs waits for every instance in world.toml.
// onProgress gets (ready, expected) on each change. It returns how many were ready when
// it stopped, so callers can report a partial start instead of claiming success.
func (r *Runtime) WaitForShardsReady(
	ctx context.Context, instanceIDs []string, onProgress func(ready, expected int),
) (int, int) {
	want := map[string]bool{}
	for _, id := range instanceIDs {
		want[id] = true
	}
	ports := make([]int, 0, len(r.cfg.WorldToml.Shards))
	for i, sh := range r.cfg.WorldToml.Shards {
		if len(want) == 0 || want[sh.InstanceID] {
			ports = append(ports, service.ShardHostPort(i))
		}
	}
	expected := len(ports)

	// The probes use the caller's context, not the deadline's: a probe cancelled by the
	// deadline would report every instance down and overwrite a true count with 0.
	deadline, cancel := context.WithTimeout(ctx, shardReadyTimeout)
	defer cancel()
	last := -1
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		ready := 0
		for _, port := range ports {
			if shardReady(ctx, port) {
				ready++
			}
		}
		if ready != last && onProgress != nil {
			onProgress(ready, expected)
			last = ready
		}
		if ready == expected {
			return ready, expected
		}
		select {
		case <-deadline.Done():
			return ready, expected
		case <-ticker.C:
		}
	}
}

// RefreshRoutes rebuilds the edge table: every instance in world.toml maps to
// its fixed host port, whether or not its container is up (a down shard answers
// 502, not 404), so reloads from another process keep routing. Without world.toml
// the running shard containers' port bindings are used instead.
func (r *Runtime) RefreshRoutes(ctx context.Context) error {
	org := r.cfg.WorldToml.Organization
	routes := map[string]*url.URL{}
	for i, sh := range r.cfg.WorldToml.Shards {
		routes[edge.RouteKey(org, r.project(), sh.InstanceID)] = hostURL(service.ShardHostPort(i))
	}
	if len(routes) == 0 {
		cs, err := r.docker.ListProjectContainers(ctx, r.project())
		if err != nil {
			return err
		}
		for _, c := range cs {
			if c.Labels[service.RoleLabel] != service.RoleShard || c.State != stateRunning {
				continue
			}
			port, err := r.docker.CardinalHostPort(ctx, c.Name)
			if err != nil {
				continue
			}
			routes[edge.RouteKey(c.Labels[service.OrgLabel], r.project(), c.Labels[service.InstanceLabel])] = hostURL(
				port,
			)
		}
	}
	r.edge.SetRoutes(routes)
	return nil
}

func hostURL(port int) *url.URL {
	return &url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(port)}
}

// ServeEdge runs the proxy on EdgeAddr until ctx ends.
func (r *Runtime) ServeEdge(ctx context.Context) error {
	return r.edge.Serve(ctx, EdgeAddr)
}

// Routes returns the current edge table, for status output and tests.
func (r *Runtime) Routes() map[string]*url.URL { return r.edge.Routes() }

// StreamShardLogs tails shards (opts.InstanceNames, else every instance in
// world.toml) into out until ctx ends. Containers that do not exist yet are skipped.
func (r *Runtime) StreamShardLogs(
	ctx context.Context,
	opts worldstatus.LogsOpts,
	out chan<- worldstatus.LogLine,
) error {
	defer close(out)
	instances := r.cfg.WorldToml.Shards
	if len(opts.InstanceNames) > 0 {
		var err error
		instances, err = r.cfg.WorldToml.ResolveInstanceIDs(opts.InstanceNames)
		if err != nil {
			return err
		}
	}
	if len(opts.ShardIDs) > 0 {
		keep := map[string]bool{}
		for _, id := range opts.ShardIDs {
			keep[id] = true
		}
		var filtered []worldtoml.Shard
		for _, sh := range instances {
			if keep[sh.ID] {
				filtered = append(filtered, sh)
			}
		}
		instances = filtered
	}
	// Cancelling on the way out stops the sibling streams too, so an early return
	// cannot leave goroutines blocked writing into entries.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if opts.FollowWindow > 0 {
		var stop context.CancelFunc
		ctx, stop = context.WithTimeout(ctx, opts.FollowWindow)
		defer stop()
	}
	entries := make(chan docker.LogEntry, 256)
	errCh := make(chan error, len(instances))
	active := 0
	for _, sh := range instances {
		name := service.CardinalShardContainerName(r.project(), sh.InstanceID)
		active++
		go func(name string) {
			errCh <- r.docker.StreamContainerLogs(ctx, name, int(opts.TailLines), true, entries)
		}(name)
	}
	byContainer := map[string]worldtoml.Shard{}
	for _, sh := range instances {
		byContainer[service.CardinalShardContainerName(r.project(), sh.InstanceID)] = sh
	}
	for active > 0 {
		select {
		case e := <-entries:
			sh := byContainer[e.Container]
			line := worldstatus.LogLine{ShardID: sh.ID, InstanceName: sh.InstanceID, Source: e.Container, Line: e.Line}
			if !e.Timestamp.IsZero() {
				line.Timestamp = e.Timestamp.Format(time.RFC3339Nano)
			}
			select {
			case out <- line:
			case <-ctx.Done():
			}
		case err := <-errCh:
			active--
			if err != nil && ctx.Err() == nil && !isNotFound(err) {
				return err
			}
		}
	}
	return nil
}

// StreamContainerLogs tails one named container (nats, db, a [[service]]) into out.
func (r *Runtime) StreamContainerLogs(
	ctx context.Context,
	name string,
	opts worldstatus.LogsOpts,
	out chan<- worldstatus.LogLine,
) error {
	defer close(out)
	if opts.FollowWindow > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.FollowWindow)
		defer cancel()
	}
	entries := make(chan docker.LogEntry, 256)
	errCh := make(chan error, 1)
	go func() { errCh <- r.docker.StreamContainerLogs(ctx, name, int(opts.TailLines), true, entries) }()
	for {
		select {
		case e := <-entries:
			line := worldstatus.LogLine{InstanceName: name, Source: name, Line: e.Line}
			if !e.Timestamp.IsZero() {
				line.Timestamp = e.Timestamp.Format(time.RFC3339Nano)
			}
			select {
			case out <- line:
			case <-ctx.Done():
			}
		case err := <-errCh:
			if err != nil && ctx.Err() == nil {
				return err
			}
			return nil
		}
	}
}

func isNotFound(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "no such container")
}

// PlatformContainers lists the non-shard containers of the project (nats, db, services), for log pickers.
func (r *Runtime) PlatformContainers() []string {
	out := []string{service.NatsContainerName(r.project())}
	if service.NeedsAutoProjectDB(r.cfg.WorldToml) {
		out = append(out, service.ProjectDBContainerName(r.project()))
	}
	for _, gs := range r.cfg.WorldToml.Services {
		out = append(out, service.GameServiceContainerName(r.project(), gs.ID))
	}
	return out
}

// Sanitized returns the org/project path segments the edge and the chart agree on.
func Sanitized(org, project string) (string, string) {
	return dnslabel.Sanitize(org), dnslabel.Sanitize(project)
}

// Containers lists every container of the project with its labels.
func (r *Runtime) Containers(ctx context.Context) ([]docker.ContainerSummary, error) {
	return r.docker.ListProjectContainers(ctx, r.project())
}

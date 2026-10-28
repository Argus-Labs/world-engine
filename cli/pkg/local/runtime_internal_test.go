package local

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/network"

	"github.com/argus-labs/world-engine/cli/pkg/docker"
	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
	"github.com/argus-labs/world-engine/cli/pkg/worldstatus"
)

func TestShardAPIURLMatchesChartPath(t *testing.T) {
	got := ShardAPIURL("my-game", "gameplay-2")
	if got != "http://localhost:8080/my-game/gameplay-2" {
		t.Fatalf("url = %q", got)
	}
}

func TestPoolPhase(t *testing.T) {
	run := worldstatus.InstanceStatus{Phase: "running"}
	exited := worldstatus.InstanceStatus{Phase: "exited"}
	cases := map[string]worldstatus.PoolStatus{
		"NotDeployed": {PoolSize: 2},
		"Stopped":     {PoolSize: 1, Instances: []worldstatus.InstanceStatus{exited}},
		"Running":     {PoolSize: 2, Instances: []worldstatus.InstanceStatus{run, run}},
		"Partial":     {PoolSize: 2, Instances: []worldstatus.InstanceStatus{run, exited}},
	}
	for want, p := range cases {
		if got := poolPhase(p); got != want {
			t.Errorf("%v: phase = %q, want %q", p, got, want)
		}
	}
}

func TestPlatformContainersAndShardServices(t *testing.T) {
	cfg := &service.Config{Project: "g", WorldToml: worldtoml.Config{
		Organization: "o",
		Project:      "g",
		Shards: []worldtoml.Shard{
			{ID: "a", InstanceID: "a"},
			{ID: "a", InstanceID: "a-2"},
			{ID: "b", InstanceID: "b"},
		},
		Services: []worldtoml.GameService{{ID: "meta", Path: "x"}},
	}}
	r := New(nil, cfg)
	want := []string{"g-nats", "g-db", "g-meta-service"}
	got := r.PlatformContainers()
	if len(got) != len(want) {
		t.Fatalf("platform = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("platform = %v, want %v", got, want)
		}
	}
	a := r.shardServices("a")
	if len(a) != 2 || a[1].Name != "g-a-2-shard" ||
		a[1].PortBindings[network.MustParsePort("8080/tcp")][0].HostPort != "8082" {
		t.Fatalf("shard a services = %+v", a)
	}
	if b := r.shardServices(
		"b",
	); len(b) != 1 ||
		b[0].PortBindings[network.MustParsePort("8080/tcp")][0].HostPort != "8083" {
		t.Fatalf("shard b services = %+v", b)
	}
	if len(r.shardServices("nope")) != 0 {
		t.Fatal("unknown shard must have no services")
	}
}

func TestRefreshRoutesIsStaticFromWorldToml(t *testing.T) {
	cfg := &service.Config{Project: "g", WorldToml: worldtoml.Config{
		Organization: "argus", Project: "g",
		Shards: []worldtoml.Shard{{ID: "a", InstanceID: "a"}, {ID: "a", InstanceID: "a-2"}},
	}}
	r := New(nil, cfg)
	if err := r.RefreshRoutes(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := r.Routes()
	// Both prefixes the chart routes: the one clients dial, and the org-prefixed one kept
	// while the world has an organization.
	want := map[string]string{
		"g/a": "127.0.0.1:8081", "g/a-2": "127.0.0.1:8082",
		"argus/g/a": "127.0.0.1:8081", "argus/g/a-2": "127.0.0.1:8082",
	}
	if len(got) != len(want) {
		t.Fatalf("routes = %v, want %d entries", got, len(want))
	}
	for key, host := range want {
		if got[key] == nil || got[key].Host != host {
			t.Fatalf("route %q = %v, want host %s", key, got[key], host)
		}
	}
}

// A producer pushes its tail into entries and returns at once, so errCh is ready while
// lines are still buffered; none of them may be lost — the end of the log is where a
// crash on boot shows.
func TestPumpLogsDrainsWhatProducersLeftBuffered(t *testing.T) {
	for _, producers := range []int{1, 3} {
		entries := make(chan docker.LogEntry, 64)
		errCh := make(chan error, producers)
		for i := range 10 {
			entries <- docker.LogEntry{Container: "c", Line: strconv.Itoa(i)}
		}
		for range producers {
			errCh <- nil
		}
		var got []string
		err := pumpLogs(context.Background(), entries, errCh, producers, func(e docker.LogEntry) {
			got = append(got, e.Line)
		})
		if err != nil || len(got) != 10 {
			t.Fatalf("producers=%d: err=%v, delivered %d/10 lines", producers, err, len(got))
		}
	}
}

func TestPumpLogsSurfacesAProducerError(t *testing.T) {
	entries := make(chan docker.LogEntry, 1)
	errCh := make(chan error, 2)
	errCh <- errors.New("boom")
	err := pumpLogs(context.Background(), entries, errCh, 2, func(docker.LogEntry) {})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("err = %v, want the producer's error", err)
	}
	errCh <- errors.New("Error: No such container: x")
	errCh <- nil
	if err := pumpLogs(context.Background(), entries, errCh, 2, func(docker.LogEntry) {}); err != nil {
		t.Fatalf("a missing container is an empty stream, got %v", err)
	}
}

// A reload from a process that does not serve the edge must not move or add an instance:
// the `world start` process built its routes from the original world.toml.
func TestLayoutMismatch(t *testing.T) {
	shards := []worldtoml.Shard{
		{ID: "game", InstanceID: "game"}, {ID: "game", InstanceID: "game-2"}, {ID: "chat", InstanceID: "chat"},
	}
	if err := layoutMismatch(shards, nil); err != nil {
		t.Fatalf("nothing deployed yet: %v", err)
	}
	same := map[string]int{"game": 8081, "game-2": 8082, "chat": 8083}
	if err := layoutMismatch(shards, same); err != nil {
		t.Fatalf("unchanged layout: %v", err)
	}
	// Before pool_size was raised, chat held 8082; a reload would move it under game-2's route.
	moved := map[string]int{"game": 8081, "chat": 8082}
	err := layoutMismatch(shards, moved)
	if err == nil || !strings.Contains(err.Error(), "game-2") {
		t.Fatalf("err = %v, want the added instance named", err)
	}
	shifted := map[string]int{"game": 8081, "game-2": 8083, "chat": 8082}
	err = layoutMismatch(shards, shifted)
	if err == nil || !strings.Contains(err.Error(), "would move") {
		t.Fatalf("err = %v, want a moved instance refused", err)
	}
}

// fakeLister is a stand-in containerLister for IsRunning, mirroring the
// dockerImageClient fake in pkg/docker: it serves a fixed container population
// so the running-world detection can be exercised without a live daemon.
type fakeLister struct {
	containers []docker.ContainerSummary
	err        error
}

func (f fakeLister) ListProjectContainers(_ context.Context, _ string) ([]docker.ContainerSummary, error) {
	return f.containers, f.err
}

func cs(state, role string) docker.ContainerSummary {
	return docker.ContainerSummary{
		Name:   role + "-" + state,
		State:  state,
		Labels: map[string]string{service.RoleLabel: role},
	}
}

// natsRunning is what IsRunning uses to tell a reload target from a stopped
// world: the platform (NATS), not the shards, is what a `world start` left
// behind. Pin every case the reload guard depends on.
func TestNatsRunning(t *testing.T) {
	cases := map[string]struct {
		containers []docker.ContainerSummary
		want       bool
	}{
		"empty (purged or never started)": {nil, false},
		"only shards, no NATS":            {[]docker.ContainerSummary{cs("running", service.RoleShard)}, false},
		"stopped NATS after world stop":   {[]docker.ContainerSummary{cs("exited", service.RoleNATS)}, false},
		"running NATS":                    {[]docker.ContainerSummary{cs("running", service.RoleNATS)}, true},
		"running shards but stopped NATS (platform down, the bug scenario)": {
			[]docker.ContainerSummary{cs("running", service.RoleShard), cs("exited", service.RoleNATS)}, false,
		},
		"stopped shards but running NATS (platform up, shards crashed — reload should proceed)": {
			[]docker.ContainerSummary{cs("exited", service.RoleShard), cs("running", service.RoleNATS)}, true,
		},
		"DB and service present, NATS down": {
			[]docker.ContainerSummary{
				cs("running", service.RoleDB),
				cs("running", service.RoleService),
				cs("exited", service.RoleNATS),
			},
			false,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := natsRunning(tc.containers); got != tc.want {
				t.Fatalf("natsRunning = %v, want %v", got, tc.want)
			}
		})
	}
}

// isPlatformRunning ties the lister call to natsRunning: a stopped world's
// container list yields not-running, a running NATS yields running, and a
// daemon failure surfaces (the error the MCP guard wraps).
func TestIsPlatformRunning(t *testing.T) {
	ctx := context.Background()
	// A stopped world: containers present but NATS exited -> not running, no error.
	running, err := isPlatformRunning(
		ctx,
		fakeLister{containers: []docker.ContainerSummary{cs("exited", service.RoleNATS)}},
		"demo",
	)
	if err != nil || running {
		t.Fatalf("stopped world: running=%v err=%v, want false/nil", running, err)
	}
	// A running world: NATS up -> running.
	running, err = isPlatformRunning(
		ctx,
		fakeLister{containers: []docker.ContainerSummary{cs("running", service.RoleNATS)}},
		"demo",
	)
	if err != nil || !running {
		t.Fatalf("running world: running=%v err=%v, want true/nil", running, err)
	}
	// A purged world: no containers at all -> not running, no error.
	running, err = isPlatformRunning(ctx, fakeLister{containers: nil}, "demo")
	if err != nil || running {
		t.Fatalf("purged world: running=%v err=%v, want false/nil", running, err)
	}
	// A daemon failure surfaces as an error so the guard can wrap it rather
	// than silently treating the world as stopped.
	listErr := errors.New("cannot connect to docker endpoint")
	running, err = isPlatformRunning(ctx, fakeLister{err: listErr}, "demo")
	if err == nil || running {
		t.Fatalf("daemon error: running=%v err=%v, want false/non-nil", running, err)
	}
}

// deadSocketDockerClient builds a Docker client aimed at a nonexistent socket, so any
// daemon call fails with an error instead of needing a running Docker. Mirrors the
// pattern in mcp_internal_test.go's TestResolveInstanceName_UnknownReturnsError.
func deadSocketDockerClient(t *testing.T) *docker.Client {
	t.Helper()
	t.Setenv("DOCKER_HOST", "unix:///nonexistent/docker.sock")
	cfg := &service.Config{Project: "demo", WorldToml: worldtoml.Config{Project: "demo"}}
	dc, err := docker.NewClient(cfg, nil)
	if err != nil {
		t.Fatalf("docker client: %v", err)
	}
	t.Cleanup(func() { _ = dc.Close() })
	return dc
}

// ReleaseEdge clears the ownership ListenEdge recorded.
func TestReleaseEdge_ClearsServesEdge(t *testing.T) {
	r := New(nil, &service.Config{Project: "g", WorldToml: worldtoml.Config{Project: "g"}})
	r.servesEdge = true
	r.ReleaseEdge()
	if r.servesEdge {
		t.Fatal("ReleaseEdge must set servesEdge back to false")
	}
}

// A second ReleaseEdge is a harmless no-op: ownership is already released, and the
// reload guard must not be re-armed by a redundant stop.
func TestReleaseEdge_Idempotent(t *testing.T) {
	r := New(nil, &service.Config{Project: "g", WorldToml: worldtoml.Config{Project: "g"}})
	r.ReleaseEdge()
	r.ReleaseEdge()
	if r.servesEdge {
		t.Fatal("ReleaseEdge must keep servesEdge false")
	}
}

// While this runtime owns the edge, CheckLayout short-circuits and never touches
// Docker — the edge owner refreshes its own routes, so there is nothing to guard.
func TestCheckLayout_ShortCircuitsWhileServingEdge(t *testing.T) {
	// A nil docker client is safe here precisely because the short-circuit returns
	// before it is dereferenced; if the guard ever stopped short-circuiting this
	// test would panic on nil, which is the point.
	r := New(nil, &service.Config{Project: "g", WorldToml: worldtoml.Config{Project: "g"}})
	r.servesEdge = true
	if err := r.CheckLayout(context.Background()); err != nil {
		t.Fatalf("serving edge: CheckLayout = %v, want nil (short-circuit)", err)
	}
}

// After ReleaseEdge a runtime that once owned the edge no longer claims to, so
// CheckLayout resumes its Docker-level layout check: it lists project containers,
// which fails against the dead socket — proving the guard is no longer skipped.
// Without the fix servesEdge stays true and CheckLayout keeps returning nil here.
func TestCheckLayout_ResumesGuardAfterReleaseEdge(t *testing.T) {
	r := New(deadSocketDockerClient(t), &service.Config{Project: "demo", WorldToml: worldtoml.Config{Project: "demo"}})
	r.servesEdge = true
	if err := r.CheckLayout(context.Background()); err != nil {
		t.Fatalf("serving edge: CheckLayout = %v, want nil (short-circuit)", err)
	}
	r.ReleaseEdge()
	if err := r.CheckLayout(context.Background()); err == nil {
		t.Fatal("after ReleaseEdge: CheckLayout = nil, want a Docker error (guard resumed)")
	}
}

func TestOrphanedShard(t *testing.T) {
	t.Parallel()

	shards := []worldtoml.Shard{
		{ID: "game", InstanceID: "game"},
		{ID: "game", InstanceID: "game-2"},
	}
	shard := func(instance string) docker.ContainerSummary {
		return docker.ContainerSummary{
			Name:   "p-" + instance,
			Labels: service.Labels("p", service.RoleShard, "game", instance),
		}
	}
	cases := []struct {
		name string
		c    docker.ContainerSummary
		want bool
	}{
		{"known instance", shard("game"), false},
		{"known pool instance", shard("game-2"), false},
		{"removed instance", shard("old"), true},
		{"pool shrunk", shard("game-3"), true},
		{"stopped orphan", docker.ContainerSummary{
			Name: "p-old", State: "exited", Labels: service.Labels("p", service.RoleShard, "game", "old"),
		}, true},
		{"service is never a shard", docker.ContainerSummary{
			Name: "p-meta-service", Labels: service.Labels("p", service.RoleService, "", "meta"),
		}, false},
		{"nats is never a shard", docker.ContainerSummary{
			Name: "p-nats", Labels: service.Labels("p", service.RoleNATS, "", ""),
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := orphanedShard(shards)(tc.c); got != tc.want {
				t.Fatalf("orphanedShard = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestOrphanedShard_EmptyWorldTomlMatchesEveryShard(t *testing.T) {
	t.Parallel()
	c := docker.ContainerSummary{Labels: service.Labels("p", service.RoleShard, "game", "game")}
	if !orphanedShard(nil)(c) {
		t.Fatal("with no shards declared every shard container is an orphan")
	}
}

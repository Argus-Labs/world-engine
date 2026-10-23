package local

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// -------------------------------------------------------------------------------------------------
// pruneOrphanedShards tests
// -------------------------------------------------------------------------------------------------

// fakeShardPruneClient is a stand-in shardPruneClient for unit-testing the prune path
// without a running Docker daemon. It serves a fixed population of containers and records
// every RemoveContainers call (names, keepVolumes and the returned error).
type fakeShardPruneClient struct {
	containers  []docker.ContainerSummary
	removed     []string
	keepVolumes *bool
	listErr     error
	removeErr   error
}

func (f *fakeShardPruneClient) ListProjectContainers(_ context.Context, _ string) ([]docker.ContainerSummary, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.containers, nil
}

func (f *fakeShardPruneClient) RemoveContainers(
	_ context.Context, names []string, keepVolumes bool, _ func(docker.Progress),
) error {
	f.removed = append(f.removed, names...)
	f.keepVolumes = &keepVolumes
	return f.removeErr
}

// pruneShardContainer builds a container summary for a shard-role container labelled with
// the full set the real Runtime stamps, matching the layout PruneOrphanedShards keys off.
func pruneShardContainer(project, shardID, instance string) docker.ContainerSummary {
	return docker.ContainerSummary{
		Name:   service.CardinalShardContainerName(project, instance),
		State:  docker.ContainerRunning,
		Labels: service.Labels(project, service.RoleShard, shardID, instance),
	}
}

// prunePlatformContainer builds a container summary for a non-shard (nats/db/service)
// container, which the prune must never touch.
func prunePlatformContainer(project, role, name string) docker.ContainerSummary {
	return docker.ContainerSummary{
		Name:   name,
		State:  docker.ContainerRunning,
		Labels: service.Labels(project, role, "", ""),
	}
}

// TestPruneOrphanedShards_RemovesOrphanedInstances is the primary regression guard:
// a shard container whose instance was removed from world.toml must be removed by the
// prune, while those still declared survive.
func TestPruneOrphanedShards_RemovesOrphanedInstances(t *testing.T) {
	t.Parallel()
	fake := &fakeShardPruneClient{
		containers: []docker.ContainerSummary{
			pruneShardContainer("rampage", "game", "game"),     // kept
			pruneShardContainer("rampage", "old", "old-2"),     // orphan: instance not in world.toml
			pruneShardContainer("rampage", "stale", "stale-3"), // orphan: shard removed from world.toml
		},
	}
	shards := []worldtoml.Shard{{ID: "game", InstanceID: "game"}}

	err := pruneOrphanedShards(context.Background(), fake, "rampage", shards)

	require.NoError(t, err)
	assert.ElementsMatch(t,
		[]string{"rampage-old-2-shard", "rampage-stale-3-shard"}, fake.removed)
}

// TestPruneOrphanedShards_KeepsKnownInstances confirms no shard declared in world.toml
// is ever removed, including pooled replicas sharing one shard ID.
func TestPruneOrphanedShards_KeepsKnownInstances(t *testing.T) {
	t.Parallel()
	fake := &fakeShardPruneClient{
		containers: []docker.ContainerSummary{
			pruneShardContainer("rampage", "game", "game"),
			pruneShardContainer("rampage", "game", "game-2"),
		},
	}
	shards := []worldtoml.Shard{
		{ID: "game", InstanceID: "game"},
		{ID: "game", InstanceID: "game-2"},
	}

	err := pruneOrphanedShards(context.Background(), fake, "rampage", shards)

	require.NoError(t, err)
	assert.Empty(t, fake.removed, "no known instance should be removed")
}

// TestPruneOrphanedShards_IgnoresNonShardContainers confirms the prune filter is scoped
// to the shard role: NATS, the project DB and [[services]] are never pruned, even when
// world.toml has no shards at all.
func TestPruneOrphanedShards_IgnoresNonShardContainers(t *testing.T) {
	t.Parallel()
	fake := &fakeShardPruneClient{
		containers: []docker.ContainerSummary{
			prunePlatformContainer("rampage", service.RoleNATS, "rampage-nats"),
			prunePlatformContainer("rampage", service.RoleDB, "rampage-db"),
			prunePlatformContainer("rampage", service.RoleService, "rampage-meta-service"),
		},
	}

	err := pruneOrphanedShards(context.Background(), fake, "rampage", nil)

	require.NoError(t, err)
	assert.Empty(t, fake.removed, "platform containers (nats/db/service) must never be pruned")
}

// TestPruneOrphanedShards_NoOrphansIsNoOp confirms RemoveContainers is not called at all
// when every shard container's instance is still declared — the common case on a start
// that follows an unchanged world.toml.
func TestPruneOrphanedShards_NoOrphansIsNoOp(t *testing.T) {
	t.Parallel()
	fake := &fakeShardPruneClient{
		containers: []docker.ContainerSummary{
			pruneShardContainer("rampage", "game", "game"),
		},
	}
	shards := []worldtoml.Shard{{ID: "game", InstanceID: "game"}}

	err := pruneOrphanedShards(context.Background(), fake, "rampage", shards)

	require.NoError(t, err)
	assert.Empty(t, fake.removed, "RemoveContainers must not be called when there are no orphans")
	assert.Nil(t, fake.keepVolumes, "RemoveContainers must not be called at all")
}

// TestPruneOrphanedShards_EmptyWorldTomlRemovesAllShards covers the stop/start cycle that
// leaves stopped shard containers behind: with no shards in world.toml, every shard
// container is an orphan.
func TestPruneOrphanedShards_EmptyWorldTomlRemovesAllShards(t *testing.T) {
	t.Parallel()
	fake := &fakeShardPruneClient{
		containers: []docker.ContainerSummary{
			pruneShardContainer("rampage", "game", "game"),
			pruneShardContainer("rampage", "lobby", "lobby"),
			pruneShardContainer("rampage", "chat", "chat-2"),
		},
	}

	err := pruneOrphanedShards(context.Background(), fake, "rampage", nil)

	require.NoError(t, err)
	assert.Len(t, fake.removed, 3, "every shard container is an orphan when world.toml has no shards")
}

// TestPruneOrphanedShards_MixedOrphansAndKnown exercises a realistic world.toml edit: one
// replica of a pool was removed, an entire pool was removed, and a platform container is
// present — only the orphans should be removed.
func TestPruneOrphanedShards_MixedOrphansAndKnown(t *testing.T) {
	t.Parallel()
	fake := &fakeShardPruneClient{
		containers: []docker.ContainerSummary{
			pruneShardContainer("rampage", "game", "game"),                      // kept
			pruneShardContainer("rampage", "game", "game-2"),                    // kept
			pruneShardContainer("rampage", "game", "game-3"),                    // orphan (replica removed)
			pruneShardContainer("rampage", "lobby", "lobby"),                    // orphan (entire pool removed)
			prunePlatformContainer("rampage", service.RoleNATS, "rampage-nats"), // untouched
		},
	}
	shards := []worldtoml.Shard{
		{ID: "game", InstanceID: "game"},
		{ID: "game", InstanceID: "game-2"},
	}

	err := pruneOrphanedShards(context.Background(), fake, "rampage", shards)

	require.NoError(t, err)
	assert.ElementsMatch(t,
		[]string{"rampage-game-3-shard", "rampage-lobby-shard"}, fake.removed)
}

// TestPruneOrphanedShards_ListErrorPropagates confirms a Docker list failure is returned
// to the caller rather than swallowed — the start path surfaces it as a deploy error.
func TestPruneOrphanedShards_ListErrorPropagates(t *testing.T) {
	t.Parallel()
	fake := &fakeShardPruneClient{listErr: errors.New("daemon unavailable")}

	err := pruneOrphanedShards(context.Background(), fake, "rampage", nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "daemon unavailable")
	assert.Empty(t, fake.removed)
}

// TestPruneOrphanedShards_RemoveErrorPropagates confirms a removal failure is returned,
// so a start that cannot remove an orphan reports the error instead of silently leaving
// it behind.
func TestPruneOrphanedShards_RemoveErrorPropagates(t *testing.T) {
	t.Parallel()
	fake := &fakeShardPruneClient{
		containers: []docker.ContainerSummary{
			pruneShardContainer("rampage", "old", "old"),
		},
		removeErr: errors.New("container busy"),
	}

	err := pruneOrphanedShards(context.Background(), fake, "rampage", nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "container busy")
}

// TestPruneOrphanedShards_KeepVolumesTrue pins the keepVolumes contract: orphan removal
// keeps volumes so a later start of a re-added instance can resume rather than wipe state.
func TestPruneOrphanedShards_KeepVolumesTrue(t *testing.T) {
	t.Parallel()
	fake := &fakeShardPruneClient{
		containers: []docker.ContainerSummary{
			pruneShardContainer("rampage", "old", "old"),
		},
	}

	err := pruneOrphanedShards(context.Background(), fake, "rampage", nil)

	require.NoError(t, err)
	require.NotNil(t, fake.keepVolumes, "keepVolumes must be passed to RemoveContainers")
	assert.True(t, *fake.keepVolumes, "orphan removal keeps volumes so a later start can resume")
}

// TestPruneOrphanedShards_RemovesStoppedOrphans confirms the prune does not skip
// containers that are not running: a stopped orphan from a stop/start cycle must also be
// removed, since the state label is not part of the prune filter.
func TestPruneOrphanedShards_RemovesStoppedOrphans(t *testing.T) {
	t.Parallel()
	stoppedOrphan := pruneShardContainer("rampage", "old", "old")
	stoppedOrphan.State = "exited"
	fake := &fakeShardPruneClient{
		containers: []docker.ContainerSummary{stoppedOrphan},
	}

	err := pruneOrphanedShards(context.Background(), fake, "rampage", nil)

	require.NoError(t, err)
	assert.Len(t, fake.removed, 1, "state (running vs stopped) does not affect the prune decision")
}

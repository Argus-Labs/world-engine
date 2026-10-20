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

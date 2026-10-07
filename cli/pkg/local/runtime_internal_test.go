package local

import (
	"context"
	"testing"

	"github.com/moby/moby/api/types/network"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
	"github.com/argus-labs/world-engine/cli/pkg/worldstatus"
)

func TestShardAPIURLMatchesChartPath(t *testing.T) {
	got := ShardAPIURL("Argus Labs", "My_Game", "gameplay-2")
	if got != "http://localhost:8080/argus-labs/my-game/gameplay-2" {
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
		Organization: "Argus Labs", Project: "g",
		Shards: []worldtoml.Shard{{ID: "a", InstanceID: "a"}, {ID: "a", InstanceID: "a-2"}},
	}}
	r := New(nil, cfg)
	if err := r.RefreshRoutes(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := r.Routes()
	if got["argus-labs/g/a"].Host != "127.0.0.1:8081" || got["argus-labs/g/a-2"].Host != "127.0.0.1:8082" ||
		len(got) != 2 {
		t.Fatalf("routes = %v", got)
	}
}

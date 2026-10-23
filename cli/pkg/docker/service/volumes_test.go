package service_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// TestVolumeNamesListsAllPlatformAndServiceVolumes pins the volume sweep purge
// relies on: it must return the deterministic name of every named volume
// `world start` creates for the project — NATS' JetStream store, the
// auto-provisioned project DB, and one per [[services]] entry that declares a
// volume — so purge can remove them by name even when their container is gone.
func TestVolumeNamesListsAllPlatformAndServiceVolumes(t *testing.T) {
	t.Parallel()

	cfg := &service.Config{WorldToml: worldtoml.Config{
		Project: "rampage",
		Services: []worldtoml.GameService{
			{ID: "meta", Image: "postgres:16", Volume: "/var/lib/postgresql/data"},
			{ID: "chat", Path: "services/chat", Volume: "/data"},
		},
	}}
	got := service.VolumeNames(cfg)
	// NATS, the auto-provisioned project DB, then the [[services]] in declaration
	// order; both image-kind (meta) and path-kind (chat) volumes are included.
	want := []string{
		"rampage-nats-data",
		"rampage-db-data",
		"rampage-meta-service",
		"rampage-chat-service",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("VolumeNames = %v, want %v", got, want)
	}
}

// TestVolumeNamesOmitsAutoProjectDBWhenConfigDBDeclared locks the db coupling:
// a hand-declared config_db [[service]] replaces the auto-provisioned
// "<project>-db", so VolumeNames must not ask purge to remove a volume that was
// never created — and the config_db service's own volume is still listed via
// the [[services]] walk.
func TestVolumeNamesOmitsAutoProjectDBWhenConfigDBDeclared(t *testing.T) {
	t.Parallel()

	cfg := &service.Config{WorldToml: worldtoml.Config{
		Project: "rampage",
		Services: []worldtoml.GameService{
			{
				ID:       "postgres",
				Image:    "postgres:16",
				ConfigDB: true,
				Ports:    []int{5432},
				Volume:   "/var/lib/postgresql/data",
			},
		},
	}}
	got := service.VolumeNames(cfg)
	want := []string{"rampage-nats-data", "rampage-postgres-service"}
	if !slices.Equal(got, want) {
		t.Fatalf("VolumeNames = %v, want %v", got, want)
	}
}

// TestVolumeNamesSkipsServicesWithoutAVolume: a service that declares no volume
// creates no named volume, so it must not contribute a phantom (container-name)
// entry purge would try to remove. Shards have no volumes either, and they are
// absent by construction (VolumeNames never builds shard services).
func TestVolumeNamesSkipsServicesWithoutAVolume(t *testing.T) {
	t.Parallel()

	cfg := &service.Config{WorldToml: worldtoml.Config{
		Project: "rampage",
		Shards:  []worldtoml.Shard{{ID: "game", InstanceID: "game", Path: "shards/game"}},
		Services: []worldtoml.GameService{
			{ID: "stateless", Image: "redis:7"},                 // no volume
			{ID: "meta", Image: "postgres:16", Volume: "/data"}, // volume
		},
	}}
	got := service.VolumeNames(cfg)
	want := []string{"rampage-nats-data", "rampage-db-data", "rampage-meta-service"}
	if !slices.Equal(got, want) {
		t.Fatalf("VolumeNames = %v, want %v", got, want)
	}
}

// TestVolumeNamesAlwaysIncludesNatsAndDB: even a shards-only world (no
// [[services]]) still runs NATS and the auto-provisioned project DB, whose
// JetStream store and Postgres data are the bulk of resurrected state, so both
// volumes must always be present for purge to wipe.
func TestVolumeNamesAlwaysIncludesNatsAndDB(t *testing.T) {
	t.Parallel()

	cfg := &service.Config{WorldToml: worldtoml.Config{
		Project: "lonely",
		Shards:  []worldtoml.Shard{{ID: "game", InstanceID: "game", Path: "shards/game"}},
	}}
	got := service.VolumeNames(cfg)
	want := []string{"lonely-nats-data", "lonely-db-data"}
	if !slices.Equal(got, want) {
		t.Fatalf("VolumeNames = %v, want %v", got, want)
	}
}

// TestVolumeNamesScopesToProject: names carry the project prefix, so one
// project's purge does not reach another project's volumes (the names are
// exact, never prefix-matched, which would be unsafe once projects can contain
// hyphens — "a" prefixing "a-b-...").
func TestVolumeNamesScopesToProject(t *testing.T) {
	t.Parallel()

	got := service.VolumeNames(&service.Config{WorldToml: worldtoml.Config{Project: "demo"}})
	for _, name := range got {
		if !strings.HasPrefix(name, "demo-") {
			t.Fatalf("volume %q is not scoped to project demo", name)
		}
	}
}

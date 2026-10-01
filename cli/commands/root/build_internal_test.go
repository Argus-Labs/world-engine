package root

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/docker"
	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// resolveServices mirrors docker.Client.ResolveServices without requiring a
// running Docker daemon: it calls each builder with cfg in order. This must
// stay byte-for-byte identical to cli/pkg/docker/client.go's ResolveServices so
// tests exercise the same resolved-service slice the build command sees.
func resolveServices(cfg *service.Config, builders []service.Builder) []service.Service {
	services := make([]service.Service, 0, len(builders))
	for _, b := range builders {
		services = append(services, b(cfg))
	}
	return services
}

// TestBuildSummaryReportsDistinctImageCount locks the Build phase success
// summary to the distinct Cardinal image count (len(imageNames)) — the
// regression guard for the over-counting bug. Reverting buildSummary to use the
// raw service-slice length makes this test fail.
func TestBuildSummaryReportsDistinctImageCount(t *testing.T) {
	t.Parallel()

	imageNames := []string{"ns-game-shard", "ns-meta-service"}
	summary := buildSummary(imageNames)

	msg := summary(3 * time.Second)
	require.Equal(t, "2 image(s) built (3s)", msg)
}

// TestBuildSummaryEmptyImageList reports zero when nothing is built, the
// boundary the one-per-image formula must handle without panicking.
func TestBuildSummaryEmptyImageList(t *testing.T) {
	t.Parallel()

	msg := buildSummary(nil)(0)
	require.Equal(t, "0 image(s) built (0s)", msg)
}

// TestBuildCmdSummaryCountsDistinctImagesNotServices reproduces the
// over-counting bug report's realistic project (one pool_size=3 shard, a
// path-built db=true service, and a pulled redis service) and asserts the
// summary — via buildSummary(imageNames) — reports the distinct built-image
// count (2), not the raw service count (7). The 7 service entries are: 3 pool
// replicas (sharing one image), the path-built meta service, the
// auto-provisioned project database, the pulled redis image, and NATS. Only
// the two Cardinal images (the shared pool image and the meta service image)
// are actually built.
func TestBuildCmdSummaryCountsDistinctImagesNotServices(t *testing.T) {
	t.Parallel()

	cfg := &service.Config{
		Namespace: "rampage",
		NATSURL:   "nats://world-engine-nats:4222",
		WorldToml: worldtoml.Config{
			Project: "rampage",
			// Pool-expanded shards: validation's expandPools (toml.go) runs at
			// load time, so callers with raw configs see one entry per replica —
			// all sharing image "rampage-game-shard".
			Shards: []worldtoml.Shard{
				{ID: "game", InstanceID: "game", Path: "shards/game/"},
				{ID: "game", InstanceID: "game-2", Path: "shards/game/"},
				{ID: "game", InstanceID: "game-3", Path: "shards/game/"},
			},
			Services: []worldtoml.GameService{
				{ID: "meta", Path: "services/meta/cmd", DB: true}, // built from source + db consumer
				{ID: "redis", Image: "redis:7"},                   // pulled, not built
			},
		},
	}

	builders := service.GetServices(cfg, service.CardinalShardsFirst)
	dockerServices := resolveServices(cfg, builders)
	imageNames := docker.CardinalBuildImageNames(dockerServices)

	// The buggy count: the raw service-slice length is 7 (3 pool replicas + meta
	// + auto-provisioned db + redis + NATS), only one of the three pool entries
	// maps to a real image build, and three entries aren't Cardinal at all.
	require.Equal(t, 7, len(dockerServices), "dockerServices entry count")
	// The correct count: exactly two distinct Cardinal images are built.
	require.Equal(t, 2, len(imageNames), "distinct built image count")
	require.Equal(t,
		[]string{"rampage-game-shard", "rampage-meta-service"}, imageNames)

	// The summary must report the distinct built count, never the service count.
	msg := buildSummary(imageNames)(90 * time.Second)
	require.Equal(t, "2 image(s) built (1m30s)", msg)
	require.NotContains(t, msg, "7 image(s)", "summary must not report the raw service count")
}

// TestBuildCmdSummaryNoShardFlagMinimalProject covers the default (no --shard)
// invocation's most trivial project: a single pool_size=1 shard with no
// [[services]]. GetServices still appends NATS unconditionally, so
// len(dockerServices)==2 (shard + NATS) while exactly one image is built. The
// summary must report 1, not 2.
func TestBuildCmdSummaryNoShardFlagMinimalProject(t *testing.T) {
	t.Parallel()

	cfg := &service.Config{
		Namespace: "rampage",
		NATSURL:   "nats://world-engine-nats:4222",
		WorldToml: worldtoml.Config{
			Project: "rampage",
			Shards:  []worldtoml.Shard{{ID: "game", InstanceID: "game", Path: "shards/game/"}},
		},
	}

	dockerServices := resolveServices(cfg, service.GetServices(cfg, service.CardinalShardsFirst))
	imageNames := docker.CardinalBuildImageNames(dockerServices)

	require.Equal(t, 2, len(dockerServices), "shard + NATS")
	require.Equal(t, 1, len(imageNames), "one built image")
	require.Equal(t, []string{"rampage-game-shard"}, imageNames)

	msg := buildSummary(imageNames)(5 * time.Second)
	require.Equal(t, "1 image(s) built (5s)", msg)
}

// TestBuildCmdSummaryShardFlagPooledReplica covers the --shard <id> path with
// pool_size>1: the inline shard filter keeps every pool replica (all sharing
// one image), so the raw slice length is the pool size while exactly one image
// is built. Before the fix this path reported pool_size as the image count.
func TestBuildCmdSummaryShardFlagPooledReplica(t *testing.T) {
	t.Parallel()

	cfg := &service.Config{
		Namespace: "rampage",
		NATSURL:   "nats://world-engine-nats:4222",
		WorldToml: worldtoml.Config{
			Project: "rampage",
			Shards: []worldtoml.Shard{
				{ID: "game", InstanceID: "game", Path: "shards/game/"},
				{ID: "game", InstanceID: "game-2", Path: "shards/game/"},
				{ID: "game", InstanceID: "game-3", Path: "shards/game/"},
			},
			Services: []worldtoml.GameService{
				{ID: "meta", Path: "services/meta/cmd", DB: true},
			},
		},
	}

	all := resolveServices(cfg, service.GetServices(cfg, service.CardinalShardsFirst))

	// Mirror the inline --shard filter from build.go: keep only services whose
	// image matches the targeted shard's image. All 3 pool replicas survive.
	wantImage := service.CardinalShardImageName(cfg.Namespace, "game")
	filtered := make([]service.Service, 0, 1)
	for _, s := range all {
		if s.Image == wantImage {
			filtered = append(filtered, s)
		}
	}

	imageNames := docker.CardinalBuildImageNames(filtered)

	require.Equal(t, 3, len(filtered), "filtered keeps every pool replica")
	require.Equal(t, 1, len(imageNames), "one shared image across the pool")
	require.Equal(t, []string{"rampage-game-shard"}, imageNames)

	msg := buildSummary(imageNames)(12 * time.Second)
	require.Equal(t, "1 image(s) built (12s)", msg)
	require.NotContains(t, msg, "3 image(s)", "summary must not report the pool size")
}

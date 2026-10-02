package toml_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/toml"
)

// src and img build the two service kinds without going through toml.Load, so
// the pure enumeration logic can be exercised free of validate's "at least one
// shard" rule. SourceServiceIDs only inspects c.Services.
func src(id string) toml.GameService { return toml.GameService{ID: id, Path: "services/" + id} }
func img(id string) toml.GameService { return toml.GameService{ID: id, Image: id + ":latest"} }

func TestSourceServiceIDs_PathKindOnly(t *testing.T) {
	t.Parallel()

	cfg := toml.Config{Services: []toml.GameService{src("meta"), src("ranks")}}

	got := cfg.SourceServiceIDs()
	require.Equal(t, []string{"meta", "ranks"}, got)
}

func TestSourceServiceIDs_ExcludesImageKind(t *testing.T) {
	t.Parallel()

	cfg := toml.Config{Services: []toml.GameService{
		src("meta"),     // built from source -> kept
		img("postgres"), // pulled -> dropped
		src("ranks"),    // built from source -> kept
		{ID: "config_db", Image: "postgres:16", ConfigDB: true}, // pulled config_db -> dropped
	}}

	got := cfg.SourceServiceIDs()
	require.Equal(t, []string{"meta", "ranks"}, got)
}

func TestSourceServiceIDs_PreservesDeclarationOrder(t *testing.T) {
	t.Parallel()

	cfg := toml.Config{Services: []toml.GameService{
		src("ranks"),
		src("meta"),
		src("lobby"),
	}}

	got := cfg.SourceServiceIDs()
	require.Equal(t, []string{"ranks", "meta", "lobby"}, got,
		"declaration order must be preserved; purge pass 2 lists images per ID")
}

func TestSourceServiceIDs_EmptyWhenNoServices(t *testing.T) {
	t.Parallel()

	// A config with shards but no [[services]] — the common shard-only case.
	cfg := toml.Config{Shards: []toml.Shard{{ID: "gameplay", InstanceID: "gameplay"}}}

	got := cfg.SourceServiceIDs()
	require.Empty(t, got)
}

func TestSourceServiceIDs_EmptyWhenAllImageKind(t *testing.T) {
	t.Parallel()

	cfg := toml.Config{Services: []toml.GameService{img("postgres"), img("redis")}}

	require.Empty(t, cfg.SourceServiceIDs())
}

func TestSourceServiceIDs_Dedupes(t *testing.T) {
	t.Parallel()

	// validate guarantees unique service IDs, but the helper dedupes defensively
	// to mirror ShardIDs (which dedupes pooled instances). A duplicate must not
	// produce a double listing — pass 2 would just no-op the second time, but the
	// count and order contract should still hold.
	cfg := toml.Config{Services: []toml.GameService{src("meta"), src("meta")}}

	got := cfg.SourceServiceIDs()
	require.Equal(t, []string{"meta"}, got)
}

func TestSourceServiceIDs_DisjointFromShardIDs(t *testing.T) {
	t.Parallel()

	// The two enumeration helpers must be independent: purge pass 2 unions them,
	// so a service ID must never surface through ShardIDs and vice versa. Guards
	// against a future refactor that points the two at the same slice.
	cfg := toml.Config{
		Shards:   []toml.Shard{{ID: "gameplay", InstanceID: "gameplay"}, {ID: "gameplay", InstanceID: "gameplay-2"}},
		Services: []toml.GameService{src("meta")},
	}

	assert.Equal(t, []string{"gameplay"}, cfg.ShardIDs(), "ShardIDs collapses pooled instances to the shard ID")
	assert.Equal(t, []string{"meta"}, cfg.SourceServiceIDs(), "SourceServiceIDs returns service IDs only")

	// The combined set purgeK8sImages builds must contain exactly the shard ID and
	// the source service ID — the union the bug report's fix wires together.
	combined := append(cfg.ShardIDs(), cfg.SourceServiceIDs()...)
	assert.Equal(t, []string{"gameplay", "meta"}, combined)
}

func TestSourceServiceIDs_IndependentOfShards(t *testing.T) {
	t.Parallel()

	// Adding shards must not change SourceServiceIDs output — purge pass 2 relies
	// on the two enumerations being additively combined (append), not interleaved.
	cfg := toml.Config{Services: []toml.GameService{src("meta")}}
	want := cfg.SourceServiceIDs()

	cfg.Shards = []toml.Shard{{ID: "gameplay", InstanceID: "gameplay"}}
	require.Equal(t, want, cfg.SourceServiceIDs())
}

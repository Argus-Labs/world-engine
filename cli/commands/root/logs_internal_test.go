package root

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	"github.com/argus-labs/world-engine/cli/pkg/local"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// TestBuildLogTargets is the contract the unified picker depends on: each
// pool-expanded shard instance shows as its own row, then the platform
// containers (NATS, project DB, [[services]]) in PlatformContainers() order.
func TestBuildLogTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		shards    []worldtoml.Shard
		wantShard []logTarget
	}{
		{
			name:      "no shards still surfaces platform containers",
			shards:    nil,
			wantShard: nil,
		},
		{
			name: "single shard",
			shards: []worldtoml.Shard{
				{ID: "gameplay", InstanceID: "gameplay"},
			},
			wantShard: []logTarget{
				{Instance: "gameplay"},
			},
		},
		{
			name: "pool expansion: each instance shows as its own row",
			shards: []worldtoml.Shard{
				{ID: "gameplay", InstanceID: "gameplay"},
				{ID: "gameplay", InstanceID: "gameplay-2"},
				{ID: "gameplay", InstanceID: "gameplay-3"},
				{ID: "lobby", InstanceID: "lobby"},
				{ID: "meta", InstanceID: "meta"},
			},
			wantShard: []logTarget{
				{Instance: "gameplay"},
				{Instance: "gameplay-2"},
				{Instance: "gameplay-3"},
				{Instance: "lobby"},
				{Instance: "meta"},
			},
		},
		{
			name: "declaration order preserved",
			shards: []worldtoml.Shard{
				{ID: "meta", InstanceID: "meta"},
				{ID: "gameplay", InstanceID: "gameplay"},
				{ID: "gameplay", InstanceID: "gameplay-2"},
				{ID: "lobby", InstanceID: "lobby"},
			},
			wantShard: []logTarget{
				{Instance: "meta"},
				{Instance: "gameplay"},
				{Instance: "gameplay-2"},
				{Instance: "lobby"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &service.Config{
				Project:   "rampage",
				WorldToml: worldtoml.Config{Project: "rampage", Shards: tt.shards},
			}
			rt := local.New(nil, cfg)
			got := buildLogTargets(rt, cfg)

			platforms := rt.PlatformContainers()
			require.Equal(t, []string{"rampage-nats", "rampage-db"}, platforms)
			require.Len(t, got, len(tt.wantShard)+len(platforms), "row count")

			for i, want := range tt.wantShard {
				require.Equal(t, want.Instance, got[i].Instance, "shard row %d Instance", i)
				require.False(t, got[i].IsPlatform(), "shard row %d should not be a platform container", i)
			}
			for i, name := range platforms {
				row := got[len(tt.wantShard)+i]
				require.Equal(t, name, row.Instance, "platform row %d Instance", i)
				require.Equal(t, name, row.Container)
			}
		})
	}
}

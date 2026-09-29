package root

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/cluster"
	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// TestBuildLogTargets is the contract the unified picker depends on: each
// pool-expanded shard instance shows as its own row, every platform component
// (gateway, nats) is appended in PlatformPods() order, and the per-project
// [[services]] + auto project DB are appended last via ProjectServicePods().
func TestBuildLogTargets(t *testing.T) {
	t.Parallel()

	platforms := cluster.PlatformPods()

	tests := []struct {
		name      string
		shards    []worldtoml.Shard
		wantShard []logTarget
	}{
		{
			name:      "no shards still surfaces platform components",
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
			cfg := &service.Config{WorldToml: worldtoml.Config{Project: "rampage", Shards: tt.shards}}
			got := buildLogTargets(cfg)

			// Expected = shard rows, then platform rows, then the per-project
			// service/DB rows from ProjectServicePods. These shard-only configs
			// have no [[services]] and no config_db service, so ProjectServicePods
			// returns the single auto "{project}-db" row.
			services := cluster.ProjectServicePods(cfg.WorldToml.Project, cfg.WorldToml)
			require.Len(t, got, len(tt.wantShard)+len(platforms)+len(services), "row count")

			for i, want := range tt.wantShard {
				require.Equal(t, want.Instance, got[i].Instance, "shard row %d Instance", i)
				require.Nil(t, got[i].Platform, "shard row %d should not be a platform pod", i)
			}
			for i, ref := range platforms {
				row := got[len(tt.wantShard)+i]
				require.Equal(t, ref.Name, row.Instance, "platform row %d Instance", i)
				require.NotNil(t, row.Platform, "platform row %d should carry a PlatformPodRef", i)
				require.Equal(t, ref.Name, row.Platform.Name)
				require.Equal(t, ref.Namespace, row.Platform.Namespace)
			}
			for i, ref := range services {
				row := got[len(tt.wantShard)+len(platforms)+i]
				require.Equal(t, ref.Name, row.Instance, "service row %d Instance", i)
				require.NotNil(t, row.Platform, "service row %d should carry a PlatformPodRef", i)
				require.Equal(t, ref.Name, row.Platform.Name)
				require.Equal(t, ref.Namespace, row.Platform.Namespace)
				require.Equal(t, ref.Selector, row.Platform.Selector)
			}
		})
	}
}

package cluster_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/cluster"
	"github.com/argus-labs/world-engine/cli/pkg/toml"
)

func TestWorldKey_JoinsOrgAndProject(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		org  string
		proj string
		want string
	}{
		{"simple", "argus", "rampage", "argus/rampage"},
		{"preserves case and underscores", "Argus_Labs", "My_Game", "Argus_Labs/My_Game"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, cluster.WorldKey(tc.org, tc.proj))
		})
	}
}

// loadCfg parses an inline world.toml fragment and returns the resulting
// Config. Fails the test if parsing errors.
func loadCfg(t *testing.T, body string) toml.Config {
	t.Helper()
	cfg, err := toml.Load(strings.NewReader(body))
	require.NoError(t, err)
	return cfg
}

func TestShardPoolsFromConfig_MinimalShardUsesAllDefaults(t *testing.T) {
	t.Parallel()

	cfg := loadCfg(t, `
		organization = "argus"
		project = "rampage"

		[[shards]]
		id = "gameplay"
	`)

	c := cluster.NewClient(cluster.Config{})
	pools, err := c.ShardPoolsFromConfig(cfg)
	require.NoError(t, err)
	require.Len(t, pools, 1)

	p := pools[0]
	require.Equal(t, "gameplay", p.ShardID)
	require.Equal(t, "argus", p.Organization)
	require.Equal(t, "rampage", p.Project)
	require.Equal(t, "us-west1", p.Region)
	require.Equal(t, "k3d-world-engine-registry.localhost:5000/rampage/gameplay", p.Image)
	require.Empty(t, p.ImageTag)
	require.Equal(t, int32(0), p.TickRate) // pass-through (0 = "use operator default")
	require.Empty(t, p.Mode)               // pass-through
	require.Equal(t, int32(1), p.PoolSize)
	require.Equal(t, cluster.DefaultResources(), p.Resources)
}

func TestShardPoolsFromConfig_ExplicitFieldsPassThrough(t *testing.T) {
	t.Parallel()

	cfg := loadCfg(t, `
		organization = "argus"
		project = "rampage"

		[[shards]]
		id = "gameplay"
		tick_rate = 30
		mode = "LEADER"
		log_level = "warn"
		resources = { requests = { cpu = 500, memory = 512 }, limits = { cpu = 2000, memory = 2048 } }
	`)

	c := cluster.NewClient(cluster.Config{})
	pools, err := c.ShardPoolsFromConfig(cfg)
	require.NoError(t, err)
	require.Len(t, pools, 1)

	p := pools[0]
	require.Equal(t, int32(30), p.TickRate)
	require.Equal(t, "LEADER", p.Mode)
	require.Equal(t, "warn", p.LogLevel)
	require.Equal(t, cluster.Resources{
		Requests: cluster.ResourceValues{CPU: 500, Memory: 512},
		Limits:   cluster.ResourceValues{CPU: 2000, Memory: 2048},
	}, p.Resources)
}

func TestShardPoolsFromConfig_PoolExpansionCollapsesToSinglePoolWithPoolSize(t *testing.T) {
	t.Parallel()

	cfg := loadCfg(t, `
		organization = "argus"
		project = "rampage"

		[[shards]]
		id = "gameplay"
		pool_size = 3

		[[shards]]
		id = "meta"
	`)

	c := cluster.NewClient(cluster.Config{})
	pools, err := c.ShardPoolsFromConfig(cfg)
	require.NoError(t, err)
	require.Len(t, pools, 2)

	// gameplay collapses 3 expanded entries back into one CR with PoolSize=3.
	require.Equal(t, "gameplay", pools[0].ShardID)
	require.Equal(t, int32(3), pools[0].PoolSize)

	require.Equal(t, "meta", pools[1].ShardID)
	require.Equal(t, int32(1), pools[1].PoolSize)
}

func TestShardPoolsFromConfig_PreservesDeclarationOrder(t *testing.T) {
	t.Parallel()

	cfg := loadCfg(t, `
		organization = "argus"
		project = "rampage"

		[[shards]]
		id = "lobby"

		[[shards]]
		id = "gameplay"

		[[shards]]
		id = "meta"
	`)

	c := cluster.NewClient(cluster.Config{})
	pools, err := c.ShardPoolsFromConfig(cfg)
	require.NoError(t, err)
	require.Len(t, pools, 3)
	require.Equal(t, "lobby", pools[0].ShardID)
	require.Equal(t, "gameplay", pools[1].ShardID)
	require.Equal(t, "meta", pools[2].ShardID)
}

func TestShardPoolsFromConfig_CustomRegistryNameAppearsInImage(t *testing.T) {
	t.Parallel()

	cfg := loadCfg(t, `
		organization = "argus"
		project = "rampage"

		[[shards]]
		id = "gameplay"
	`)

	c := cluster.NewClient(cluster.Config{RegistryName: "my-registry"})
	pools, err := c.ShardPoolsFromConfig(cfg)
	require.NoError(t, err)
	require.Equal(t, "k3d-my-registry.localhost:5000/rampage/gameplay", pools[0].Image)
}

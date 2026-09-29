package cluster_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/cluster"
	"github.com/argus-labs/world-engine/cli/pkg/toml"
)

func TestNewClient_ZeroConfig_FillsDefaults(t *testing.T) {
	t.Parallel()

	c := cluster.NewClient(cluster.Config{})
	cfg := c.Config()

	require.Equal(t, "world-engine", cfg.ClusterName)
	require.Equal(t, "world-engine-registry", cfg.RegistryName)
	require.Equal(t, "http://localhost:8080", cfg.APIEndpoint)
	require.Equal(t, "http://localhost:8090", cfg.OperatorEndpoint)
	require.Equal(t, "localhost:5432", cfg.DBEndpoint)
}

func TestNewClient_PartialConfig_KeepsExplicitFields(t *testing.T) {
	t.Parallel()

	c := cluster.NewClient(cluster.Config{ClusterName: "custom"})
	cfg := c.Config()

	require.Equal(t, "custom", cfg.ClusterName)
	require.Equal(t, "world-engine-registry", cfg.RegistryName) // defaulted
}

// DeployWorld translates the world config to ShardPools BEFORE touching the
// cluster, so an invalid config (missing org/project) fails fast without any k3d
// interaction. The config here is built directly rather than via toml.Load,
// which would reject it up front. (UndeployWorld needs no such guard: shards
// live in a single fixed namespace, so it doesn't derive anything from the
// config.)
func TestDeployWorld_MissingOrgProject_FailsBeforeCluster(t *testing.T) {
	t.Parallel()

	c := cluster.NewClient(cluster.Config{})
	cfg := toml.Config{Shards: []toml.Shard{{ID: "game"}}} // no organization/project

	err := c.DeployWorld(context.Background(), cfg, nil)
	require.Error(t, err)
}

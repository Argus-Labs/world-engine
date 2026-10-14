package toml_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	toml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// pooled loads a two-pool config so the shards come back the way the CLI sees
// them: pools expanded, InstanceID set on every replica.
func pooled(t *testing.T) toml.Config {
	t.Helper()
	cfg, err := toml.Load(strings.NewReader(`
organization = "org"
project = "proj"

[[shards]]
id = "game"
pool_size = 2

[[shards]]
id = "chat"
`))
	require.NoError(t, err)
	return cfg
}

func instances(shards []toml.Shard) []string {
	out := make([]string, 0, len(shards))
	for _, s := range shards {
		out = append(out, s.InstanceID)
	}
	return out
}

func TestConfigShardIDsDedupesInstances(t *testing.T) {
	assert.Equal(t, []string{"game", "chat"}, pooled(t).ShardIDs())
}

func TestResolveInstanceIDsNamesOneInstanceEach(t *testing.T) {
	cfg := pooled(t)

	// The first instance has the unsuffixed ID "game".
	got, err := cfg.ResolveInstanceIDs([]string{"game"})
	require.NoError(t, err)
	assert.Equal(t, []string{"game"}, instances(got))

	got, err = cfg.ResolveInstanceIDs([]string{"game-2"})
	require.NoError(t, err)
	assert.Equal(t, []string{"game-2"}, instances(got))
	assert.Equal(t, "game", got[0].ID, "replica keeps its shard ID")

	// Reaching both means naming both.
	got, err = cfg.ResolveInstanceIDs([]string{"game", "game-2"})
	require.NoError(t, err)
	assert.Equal(t, []string{"game", "game-2"}, instances(got))
}

func TestResolveInstanceIDsPreservesOrderAndDeduplicates(t *testing.T) {
	got, err := pooled(t).ResolveInstanceIDs([]string{"chat", "game-2", "chat"})
	require.NoError(t, err)
	assert.Equal(t, []string{"chat", "game-2"}, instances(got))
}

func TestResolveInstanceIDsEmptyMeansEveryInstance(t *testing.T) {
	cfg := pooled(t)
	for name, ids := range map[string][]string{
		"nil":    nil,
		"empty":  {},
		"blanks": {"", "   "},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := cfg.ResolveInstanceIDs(ids)
			require.NoError(t, err)
			assert.Equal(t, []string{"game", "game-2", "chat"}, instances(got))
		})
	}
}

func TestResolveInstanceIDsReportsEveryUnknownID(t *testing.T) {
	cfg := pooled(t)

	_, err := cfg.ResolveInstanceIDs([]string{"nope"})
	require.Error(t, err)
	assert.Equal(t, `instance IDs not found in world.toml: "nope"`, err.Error())
	assert.ErrorContains(t, err, "not found")

	// One typo must not hide the next, and a valid id alongside them still fails
	// the whole command rather than quietly acting on the subset that matched.
	_, err = cfg.ResolveInstanceIDs([]string{"nope", "game", "game-3"})
	require.Error(t, err)
	assert.Equal(t, `instance IDs not found in world.toml: "nope", "game-3"`, err.Error())
	assert.ErrorContains(t, err, "not found")
}

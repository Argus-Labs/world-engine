package cardinal

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A shard that sets no auth mode must fail to start rather than fall back to dev auth, which would
// let any caller act as any player.
func TestWorldOptionsRequireAuthMode(t *testing.T) {
	t.Setenv("CARDINAL_AUTH_MODE", "")

	cfg, err := loadWorldOptionsEnv()
	require.NoError(t, err)

	opts := newDefaultWorldOptions()
	opts.apply(cfg.toOptions())
	opts.apply(WorldOptions{
		Region:       "us-west1",
		Organization: "argus",
		Project:      "rampage",
		ShardID:      "game",
		TickRate:     1,
		SnapshotRate: 1,
	})
	require.ErrorContains(t, opts.validate(), "auth mode must be specified")

	// The mode can come from code instead of the environment.
	opts.apply(WorldOptions{AuthMode: AuthModeDev})
	require.NoError(t, opts.validate())
}

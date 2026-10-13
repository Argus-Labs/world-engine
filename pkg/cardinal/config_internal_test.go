package cardinal

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewWorld_ArgusAuthGameID checks that ARGUS mode needs a game ID, and that it may come from
// WorldOptions while the mode and URL come from the environment.
func TestNewWorld_ArgusAuthGameID(t *testing.T) {
	t.Setenv("CARDINAL_PROJECT", "rampage")
	t.Setenv("CARDINAL_SHARD_ID", "gameplay-usw1-3")
	t.Setenv("CARDINAL_AUTH_MODE", "ARGUS")
	t.Setenv("CARDINAL_ARGUS_AUTH_URL", "https://auth.example")
	opts := WorldOptions{TickRate: 1, SnapshotRate: 1}

	_, err := NewWorld(opts)
	require.ErrorContains(t, err, "argus auth game ID cannot be empty when auth mode is ARGUS")

	opts.ArgusAuthGameID = "argus/rampage"
	w, err := NewWorld(opts)
	require.NoError(t, err)
	require.Equal(t, "argus/rampage", w.options.ArgusAuthGameID)
	require.Equal(t, "rampage.gameplay-usw1-3", w.address.GetProject()+"."+w.address.GetServiceId())
}

// TestNewWorld_RejectsInvalidAddress checks that a project or shard ID that cannot be a NATS subject
// token, or a project with an underscore, fails at startup.
func TestNewWorld_RejectsInvalidAddress(t *testing.T) {
	t.Parallel()

	for _, opts := range []WorldOptions{
		{Project: "my_game", ShardID: "game"},
		{Project: "my.game", ShardID: "game"},
		{Project: "rampage", ShardID: "Game"},
		{Project: "rampage", ShardID: "game.1"},
	} {
		opts.TickRate, opts.SnapshotRate = 1, 1
		_, err := NewWorld(opts)
		require.ErrorContains(t, err, "invalid project or shard ID", "%s.%s", opts.Project, opts.ShardID)
	}
}

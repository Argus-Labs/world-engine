package cardinal

import (
	"math"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/testutils"
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

// TestNewWorld_TickRateEnvOverridesOptions checks that CARDINAL_TICK_RATE, which the World CLI sets
// from world.toml's tick_rate, paces the world even when the code compiles in another rate, and
// that the compiled rate applies when the variable is unset.
func TestNewWorld_TickRateEnvOverridesOptions(t *testing.T) {
	for _, tc := range []struct {
		name         string
		env          string
		compiledRate float64
		want         time.Duration
	}{
		{name: "unset keeps compiled rate", env: "", compiledRate: 1, want: time.Second},
		{name: "zero counts as unset", env: "0", compiledRate: 1, want: time.Second},
		{name: "env overrides compiled rate", env: "20", compiledRate: 1, want: 50 * time.Millisecond},
		{name: "env alone sets rate", env: "20", compiledRate: 0, want: 50 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CARDINAL_PROJECT", "rampage")
			t.Setenv("CARDINAL_SHARD_ID", "game")
			t.Setenv("CARDINAL_TICK_RATE", tc.env)

			w, err := NewWorld(WorldOptions{TickRate: tc.compiledRate, SnapshotRate: 1})
			require.NoError(t, err)
			require.Equal(t, tc.want, w.options.tickInterval())
			require.InDelta(t, 1/tc.want.Seconds(), w.TickRate(), 1e-9, "TickRate reports the running rate")
		})
	}

	t.Run("malformed env fails startup", func(t *testing.T) {
		t.Setenv("CARDINAL_PROJECT", "rampage")
		t.Setenv("CARDINAL_SHARD_ID", "game")

		for _, env := range []string{"-1", "0.5"} {
			t.Setenv("CARDINAL_TICK_RATE", env)

			_, err := NewWorld(WorldOptions{TickRate: 1, SnapshotRate: 1})
			require.ErrorContains(t, err, `parse error on field "TickRate"`, env)
		}
	})
}

// TestRunDST_ReportsNoTickRate checks that a DST world reports no tick rate, as a TestWorld does, even
// with CARDINAL_TICK_RATE set, so plugins paced by World.TickRate keep their defaults on its synthetic
// one-second clock.
func TestRunDST_ReportsNoTickRate(t *testing.T) {
	t.Setenv("CARDINAL_TICK_RATE", "30")

	rate := -1.0

	newDSTFixture(t, newDSTConfig(testutils.NewRand(t)), func(w *World) { rate = w.TickRate() })
	require.Zero(t, rate)
}

// TestNewWorld_RejectsTickRateWithZeroInterval checks that a rate whose tick interval truncates to
// 0 ns fails at startup instead of panicking the ticker, whether it comes from the environment or
// WorldOptions.
func TestNewWorld_RejectsTickRateWithZeroInterval(t *testing.T) {
	t.Setenv("CARDINAL_PROJECT", "rampage")
	t.Setenv("CARDINAL_SHARD_ID", "game")

	t.Setenv("CARDINAL_TICK_RATE", "1000000000")

	w, err := NewWorld(WorldOptions{TickRate: 1, SnapshotRate: 1})
	require.NoError(t, err)
	require.Equal(t, time.Nanosecond, w.options.tickInterval())

	t.Setenv("CARDINAL_TICK_RATE", "1000000001")

	_, err = NewWorld(WorldOptions{TickRate: 1, SnapshotRate: 1})
	require.ErrorContains(t, err, "tick rate 1.000000001e+09 is too high")

	t.Setenv("CARDINAL_TICK_RATE", "")

	_, err = NewWorld(WorldOptions{TickRate: math.Inf(1), SnapshotRate: 1})
	require.ErrorContains(t, err, "is too high")
}

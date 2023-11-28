package testutils

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/alicebob/miniredis/v2"
	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal"
)

// NewTestWorld creates a World object suitable for unit tests.
// Relevant resources are automatically cleaned up at the completion of each test.
func NewTestWorld(t testing.TB, opts ...cardinal.WorldOption) *cardinal.World {
	// Init testing environment
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	s := miniredis.RunT(t)
	return NewTestWorldWithCustomRedis(t, s, opts...)
}

func NewTestWorldWithCustomRedis(
	t testing.TB,
	miniRedis *miniredis.Miniredis,
	opts ...cardinal.WorldOption) *cardinal.World {
	t.Setenv("CARDINAL_DEPLOY_MODE", "development")
	t.Setenv("REDIS_ADDRESS", miniRedis.Addr())
	opts = append([]cardinal.WorldOption{cardinal.WithCustomMockRedis(miniRedis)}, opts...)
	world, err := cardinal.NewWorld(opts...)
	if err != nil {
		t.Fatalf("Unable to initialize test world: %v", err)
	}
	assert.NilError(t, err)
	return world
}

package testutils

import (
	"github.com/alicebob/miniredis/v2"
	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/cardinal"
	"testing"
)

// NewTestWorld creates a World object suitable for unit tests.
// Relevant resources are automatically cleaned up at the completion of each test.
func NewTestWorld(t testing.TB, opts ...cardinal.WorldOption) *cardinal.World {
	// Init testing environment
	s := miniredis.RunT(t)
	t.Setenv("CARDINAL_DEPLOY_MODE", "development")
	t.Setenv("REDIS_ADDRESS", s.Addr())

	world, err := cardinal.NewWorld(opts...)
	if err != nil {
		t.Fatalf("Unable to initialize test world: %v", err)
	}
	assert.NilError(t, err)
	return world
}

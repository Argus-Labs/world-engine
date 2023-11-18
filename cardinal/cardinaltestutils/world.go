package cardinaltestutils

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

// NewTestWorld creates a World object suitable for unit tests.
// Relevant resources are automatically cleaned up at the completion of each test.
func NewTestWorld(t testing.TB, opts ...cardinal.WorldOption) *cardinal.World {
	// Init testing environment
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	s := miniredis.RunT(t)
	t.Setenv("CARDINAL_DEPLOY_MODE", "development")
	t.Setenv("REDIS_ADDRESS", s.Addr())

	world, err := cardinal.NewWorld(opts...)
	if err != nil {
		t.Fatalf("Unable to initialize test world: %v", err)
	}
	testutils.AssertNilErrorWithTrace(t, err)
	return world
}

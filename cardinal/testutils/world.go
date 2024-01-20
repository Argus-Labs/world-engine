package testutils

import (
	"fmt"
	"net"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/rs/zerolog"
	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/cardinal"
)

// NewTestWorld creates a World object suitable for unit tests.
// Relevant resources are automatically cleaned up at the completion of each test.
func NewTestWorld(t testing.TB, opts ...cardinal.WorldOption) *cardinal.World {
	world, _ := NewTestWorldAndServerAddress(t, opts...)
	return world
}

func NewTestWorldAndServerAddress(t testing.TB, opts ...cardinal.WorldOption) (world *cardinal.World, addr string) {
	// Init testing environment
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	s := miniredis.RunT(t)
	return NewTestWorldWithCustomRedis(t, s, opts...)
}

func NewTestWorldWithCustomRedis(
	t testing.TB,
	miniRedis *miniredis.Miniredis,
	opts ...cardinal.WorldOption,
) (world *cardinal.World, url string) {
	port := getOpenPort(t)
	t.Setenv("CARDINAL_DEPLOY_MODE", "development")
	t.Setenv("REDIS_ADDRESS", miniRedis.Addr())
	opts = append([]cardinal.WorldOption{cardinal.WithCustomMockRedis(miniRedis)}, opts...)
	opts = append(opts, cardinal.WithPort(port))
	world, err := cardinal.NewWorld(opts...)
	if err != nil {
		t.Fatalf("Unable to initialize test world: %v", err)
	}
	assert.NilError(t, err)
	t.Cleanup(func() {
		err = world.ShutDown()
		assert.NilError(t, err)
	})
	return world, fmt.Sprintf("localhost:%s", port)
}

func getOpenPort(t testing.TB) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	defer func() {
		assert.NilError(t, l.Close())
	}()

	assert.NilError(t, err)
	tcpAddr, err := net.ResolveTCPAddr(l.Addr().Network(), l.Addr().String())
	assert.NilError(t, err)
	return fmt.Sprintf("%d", tcpAddr.Port)
}

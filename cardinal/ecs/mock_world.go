package ecs

import (
	"fmt"
	"log"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/alicebob/miniredis/v2"
	"pkg.world.dev/world-engine/cardinal/ecs/ecb"

	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

// NewMockWorld creates an ecs.World that uses a mock redis DB as the storage
// layer. This is only suitable for local development. If you are creating an ecs.World for
// unit tests, use NewTestWorld.
func NewMockWorld(opts ...Option) (world *World, cleanup func()) {
	// We manually set the start address to make the port deterministic
	s := miniredis.NewMiniRedis()
	err := s.StartAddr(":12345")
	if err != nil {
		panic("Unable to initialize in-memory redis")
	}
	log.Printf("Miniredis started at %s", s.Addr())

	w, err := newMockWorld(s, opts...)
	if err != nil {
		panic(fmt.Errorf("unable to initialize world: %w", err))
	}

	return w, func() {
		s.Close()
	}
}

// NewTestWorld creates an ecs.World suitable for running in tests. Relevant resources
// are automatically cleaned up at the completion of each test.
func NewTestWorld(t testing.TB, opts ...Option) *World {
	s := miniredis.RunT(t)
	w, err := newMockWorld(s, opts...)
	if err != nil {
		t.Fatalf("Unable to initialize world: %v", err)
	}
	assert.NilError(t, err)
	return w
}

func newMockWorld(s *miniredis.Miniredis, opts ...Option) (*World, error) {
	redisStore := storage.NewRedisStorage(storage.Options{
		Addr:     s.Addr(),
		Password: "", // no password set
		DB:       0,  // use default DB
	}, "in-memory-world")
	entityStore, err := ecb.NewManager(redisStore.Client)
	if err != nil {
		return nil, err
	}

	return NewWorld(&redisStore, entityStore, opts...)
}

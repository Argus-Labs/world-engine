package ecb

// Most tests in this package are under ecb_test.go. This makes the tests act like external clients
// that can import both the ecs package and the ecb package. Tests in this file verify that the
// internal state of redis is correct, so they need access to the package private methods in keys.go.

import (
	"context"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"pkg.world.dev/world-engine/cardinal/ecs/component/metadata"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

func TestComponentValuesAreDeletedFromRedis(t *testing.T) {
	s := miniredis.RunT(t)
	options := redis.Options{
		Addr:     s.Addr(),
		Password: "", // no password set
		DB:       0,  // use default DB
	}
	client := redis.NewClient(&options)

	type Alpha struct{ Value int }
	type Beta struct{ Value int }
	alphaComp := storage.NewMockComponentType[Alpha](Alpha{}, Alpha{})
	betaComp := storage.NewMockComponentType[Beta](Beta{}, Beta{})
	testutils.AssertNilErrorWithTrace(t, alphaComp.SetID(77))
	testutils.AssertNilErrorWithTrace(t, betaComp.SetID(88))

	manager, err := NewManager(client)
	testutils.AssertNilErrorWithTrace(t, err)
	err = manager.RegisterComponents([]metadata.ComponentMetadata{alphaComp, betaComp})
	testutils.AssertNilErrorWithTrace(t, err)

	id, err := manager.CreateEntity(alphaComp, betaComp)
	testutils.AssertNilErrorWithTrace(t, err)

	startValue := Alpha{99}
	testutils.AssertNilErrorWithTrace(t, manager.SetComponentForEntity(alphaComp, id, startValue))
	testutils.AssertNilErrorWithTrace(t, manager.CommitPending())

	key := redisComponentKey(alphaComp.ID(), id)
	// Make sure the value actually made it to the redis DB.
	ctx := context.Background()
	bz, err := client.Get(ctx, key).Bytes()
	testutils.AssertNilErrorWithTrace(t, err)

	gotValue, err := alphaComp.Decode(bz)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, startValue, gotValue.(Alpha))

	// Now remove the alpha component from the entity.
	testutils.AssertNilErrorWithTrace(t, manager.RemoveComponentFromEntity(alphaComp, id))
	testutils.AssertNilErrorWithTrace(t, manager.CommitPending())

	// Verify the component in question no longer exists in the DB
	err = client.Get(ctx, key).Err()
	assert.ErrorIs(t, err, redis.Nil)
}

package ecs_test

import (
	"context"
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/query"
	"pkg.world.dev/world-engine/cardinal/world_namespace"
)

func TestCreatePersonaTransactionAutomaticallyCreated(t *testing.T) {
	// Verify that the CreatePersonaTransaction is automatically created and registered with a world.
	world := ecs.NewTestWorld(t)
	err := world.RegisterComponents()
	assert.NilError(t, err)
	assert.NilError(t, world.LoadGameState())

	wantTag := "CoolMage"
	wantAddress := "123-456"
	ecs.CreatePersonaTx.AddToQueue(world.GetTxQueue(), ecs.CreatePersonaTransaction{
		PersonaTag:    wantTag,
		SignerAddress: wantAddress,
	})
	// This CreatePersonaTx has the same persona tag, but it shouldn't be registered because
	// it comes second.
	ecs.CreatePersonaTx.AddToQueue(world.GetTxQueue(), ecs.CreatePersonaTransaction{
		PersonaTag:    wantTag,
		SignerAddress: "some-other-address",
	})

	// PersonaTag registration doesn't take place until the relevant system is run during a game tick.
	assert.NilError(t, world.Tick(context.Background()))

	count := 0
	query.NewQuery(filter.Exact(ecs.SignerComp)).Each(world_namespace.Namespace(world.Namespace()), world.Store(), func(id entity.ID) bool {
		count++
		sc, err := ecs.SignerComp.Get(world.StoreManager(), id)
		assert.NilError(t, err)
		assert.Equal(t, sc.PersonaTag, wantTag)
		assert.Equal(t, sc.SignerAddress, wantAddress)
		return true
	})
	assert.Equal(t, 1, count)
}

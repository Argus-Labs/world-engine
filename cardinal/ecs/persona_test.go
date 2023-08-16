package ecs_test

import (
	"context"
	"fmt"
	"testing"

	"gotest.tools/v3/assert"
	. "pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/inmem"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

func TestCreatePersonaTransactionAutomaticallyCreated(t *testing.T) {
	// Verify that the CreatePersonaTransaction is automatically created and registered with a world.
	world := inmem.NewECSWorldForTest(t)
	assert.NilError(t, world.LoadGameState())

	wantTag := "CoolMage"
	wantAddress := "123-456"
	CreatePersonaTx.AddToQueue(world, CreatePersonaTransaction{
		PersonaTag:    wantTag,
		SignerAddress: wantAddress,
	})
	// This CreatePersonaTx has the same persona tag, but it shouldn't be registered because
	// it comes second.
	CreatePersonaTx.AddToQueue(world, CreatePersonaTransaction{
		PersonaTag:    wantTag,
		SignerAddress: "some-other-address",
	})

	// PersonaTag registration doesn't take place until the relevant system is run during a game tick.
	assert.NilError(t, world.Tick(context.Background()))

	count := 0
	NewQuery(filter.Exact(SignerComp)).Each(world, func(id storage.EntityID) bool {
		count++
		sc, err := SignerComp.Get(world, id)
		assert.NilError(t, err)
		assert.Equal(t, sc.PersonaTag, wantTag)
		assert.Equal(t, sc.SignerAddress, wantAddress)
		return true
	})
	assert.Equal(t, 1, count)
}

func TestGetSignerForPersonaTagReturnsErrorWhenNotRegistered(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	assert.NilError(t, world.LoadGameState())
	ctx := context.Background()

	// Tick the game forward a bit to simulate a game that has been running for a bit of time.
	for i := 0; i < 10; i++ {
		assert.NilError(t, world.Tick(ctx))
	}

	_, err := world.GetSignerForPersonaTag("missing_persona", 1)
	assert.ErrorIs(t, err, ErrorPersonaTagHasNoSigner)

	// Queue up a CreatePersonaTx
	personaTag := "foobar"
	signerAddress := "xyzzy"
	CreatePersonaTx.AddToQueue(world, CreatePersonaTransaction{
		PersonaTag:    personaTag,
		SignerAddress: signerAddress,
	})
	// This CreatePersonaTx will not be processed until the world.CurrentTick() is greater than the tick that
	// originally got the CreatePersonaTx.
	tick := world.CurrentTick()
	_, err = world.GetSignerForPersonaTag(personaTag, tick)
	assert.ErrorIs(t, err, ErrorCreatePersonaTxsNotProcessed)

	assert.NilError(t, world.Tick(ctx))
	// The CreatePersonaTx has now been processed
	addr, err := world.GetSignerForPersonaTag(personaTag, tick)
	assert.NilError(t, err)
	assert.Equal(t, addr, signerAddress)
}

func TestDuplicatePersonaTagsInTickAreOnlyRegisteredOnce(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	assert.NilError(t, world.LoadGameState())

	personaTag := "jeff"

	for i := 0; i < 10; i++ {
		// Attempt to register many different signer addresses with the same persona tag.
		CreatePersonaTx.AddToQueue(world, CreatePersonaTransaction{
			PersonaTag:    personaTag,
			SignerAddress: fmt.Sprintf("address-%d", i),
		})
	}
	tick := world.CurrentTick()

	ctx := context.Background()
	assert.NilError(t, world.Tick(ctx))

	addr, err := world.GetSignerForPersonaTag(personaTag, tick)
	assert.NilError(t, err)
	// Only the first address should be associated with the user
	assert.Equal(t, addr, "address-0")

	// Attempt to register this persona tag again in a different tick. We should still maintain the original
	// signer address.
	CreatePersonaTx.AddToQueue(world, CreatePersonaTransaction{
		PersonaTag:    personaTag,
		SignerAddress: "some-other-address",
	})

	assert.NilError(t, world.Tick(ctx))
	addr, err = world.GetSignerForPersonaTag(personaTag, tick)
	assert.NilError(t, err)
	// The saved address should be unchanged
	assert.Equal(t, addr, "address-0")
}

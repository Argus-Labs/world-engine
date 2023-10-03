package persona_test

import (
	"context"
	"fmt"
	"testing"

	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/persona"
	"pkg.world.dev/world-engine/cardinal/public"
	"pkg.world.dev/world-engine/sign"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
)

func TestCreatePersonaTransactionAutomaticallyCreated(t *testing.T) {
	// Verify that the CreatePersonaTransaction is automatically created and registered with a world.
	world := ecs.NewTestWorld(t)
	err := world.RegisterComponents()
	assert.NilError(t, err)
	assert.NilError(t, world.LoadGameState())

	wantTag := "CoolMage"
	wantAddress := "123-456"
	persona.CreatePersonaTx.AddToQueue(world, persona.CreatePersonaTransaction{
		PersonaTag:    wantTag,
		SignerAddress: wantAddress,
	})
	// This CreatePersonaTx has the same persona tag, but it shouldn't be registered because
	// it comes second.
	persona.CreatePersonaTx.AddToQueue(world, persona.CreatePersonaTransaction{
		PersonaTag:    wantTag,
		SignerAddress: "some-other-address",
	})

	// PersonaTag registration doesn't take place until the relevant system is run during a game tick.
	assert.NilError(t, world.Tick(context.Background()))

	count := 0
	component.NewQuery(filter.Exact(persona.SignerComp)).Each(world, func(id public.EntityID) bool {
		count++
		sc, err := persona.SignerComp.Get(world, id)
		assert.NilError(t, err)
		assert.Equal(t, sc.PersonaTag, wantTag)
		assert.Equal(t, sc.SignerAddress, wantAddress)
		return true
	})
	assert.Equal(t, 1, count)
}

func TestGetSignerForPersonaTagReturnsErrorWhenNotRegistered(t *testing.T) {
	world := ecs.NewTestWorld(t)
	err := world.RegisterComponents()
	assert.NilError(t, err)
	assert.NilError(t, world.LoadGameState())
	ctx := context.Background()

	// Tick the game forward a bit to simulate a game that has been running for a bit of time.
	for i := 0; i < 10; i++ {
		assert.NilError(t, world.Tick(ctx))
	}

	_, err = persona.GetSignerForPersonaTag(world, "missing_persona", 1)
	assert.ErrorIs(t, err, persona.ErrorPersonaTagHasNoSigner)

	// Queue up a CreatePersonaTx
	personaTag := "foobar"
	signerAddress := "xyzzy"
	persona.CreatePersonaTx.AddToQueue(world, persona.CreatePersonaTransaction{
		PersonaTag:    personaTag,
		SignerAddress: signerAddress,
	})
	// This CreatePersonaTx will not be processed until the world.CurrentTick() is greater than the tick that
	// originally got the CreatePersonaTx.
	tick := world.CurrentTick()
	_, err = persona.GetSignerForPersonaTag(world, personaTag, tick)
	assert.ErrorIs(t, err, persona.ErrorCreatePersonaTxsNotProcessed)

	assert.NilError(t, world.Tick(ctx))
	// The CreatePersonaTx has now been processed
	addr, err := persona.GetSignerForPersonaTag(world, personaTag, tick)
	assert.NilError(t, err)
	assert.Equal(t, addr, signerAddress)
}

func TestDuplicatePersonaTagsInTickAreOnlyRegisteredOnce(t *testing.T) {
	world := ecs.NewTestWorld(t)
	assert.NilError(t, world.LoadGameState())

	personaTag := "jeff"

	for i := 0; i < 10; i++ {
		// Attempt to register many different signer addresses with the same persona tag.
		persona.CreatePersonaTx.AddToQueue(world, persona.CreatePersonaTransaction{
			PersonaTag:    personaTag,
			SignerAddress: fmt.Sprintf("address-%d", i),
		})
	}
	tick := world.CurrentTick()

	ctx := context.Background()
	assert.NilError(t, world.Tick(ctx))

	addr, err := persona.GetSignerForPersonaTag(world, personaTag, tick)
	assert.NilError(t, err)
	// Only the first address should be associated with the user
	assert.Equal(t, addr, "address-0")

	// Attempt to register this persona tag again in a different tick. We should still maintain the original
	// signer address.
	persona.CreatePersonaTx.AddToQueue(world, persona.CreatePersonaTransaction{
		PersonaTag:    personaTag,
		SignerAddress: "some-other-address",
	})

	assert.NilError(t, world.Tick(ctx))
	addr, err = persona.GetSignerForPersonaTag(world, personaTag, tick)
	assert.NilError(t, err)
	// The saved address should be unchanged
	assert.Equal(t, addr, "address-0")
}

func TestCanAuthorizeAddress(t *testing.T) {
	// Verify that the CreatePersonaTransaction is automatically created and registered with a world.
	world := ecs.NewTestWorld(t)
	assert.NilError(t, world.LoadGameState())

	wantTag := "CoolMage"
	wantSigner := "123-456"
	persona.CreatePersonaTx.AddToQueue(world, persona.CreatePersonaTransaction{
		PersonaTag:    wantTag,
		SignerAddress: wantSigner,
	})

	wantAddr := "0xfoobar"
	persona.AuthorizePersonaAddressTx.AddToQueue(world, persona.AuthorizePersonaAddress{
		PersonaTag: wantTag,
		Address:    wantAddr,
	}, &sign.SignedPayload{PersonaTag: wantTag})
	// PersonaTag registration doesn't take place until the relevant system is run during a game tick.
	assert.NilError(t, world.Tick(context.Background()))

	count := 0
	component.NewQuery(filter.Exact(persona.SignerComp)).Each(world, func(id public.EntityID) bool {
		count++
		sc, err := persona.SignerComp.Get(world, id)
		assert.NilError(t, err)
		assert.Equal(t, sc.PersonaTag, wantTag)
		assert.Equal(t, sc.SignerAddress, wantSigner)
		assert.Equal(t, len(sc.AuthorizedAddresses), 1)
		assert.Equal(t, sc.AuthorizedAddresses[0], wantAddr)
		return true
	})
	// verify that the query was even ran. if for some reason there were no SignerComponents in the state,
	// this test would still pass (false positive).
	assert.Equal(t, count, 1)
}

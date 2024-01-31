package ecs_test

import (
	"context"
	"fmt"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/persona/component"
	"pkg.world.dev/world-engine/cardinal/persona/msg"
	"testing"

	"pkg.world.dev/world-engine/cardinal/ecs/filter"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/cardinal/types/entity"
	"pkg.world.dev/world-engine/sign"

	"pkg.world.dev/world-engine/assert"

	"pkg.world.dev/world-engine/cardinal/ecs"
)

func TestCreatePersonaTransactionAutomaticallyCreated(t *testing.T) {
	// Verify that the CreatePersona is automatically created and registered with a engine.
	world := testutils.NewTestFixture(t, nil).World
	assert.NilError(t, world.LoadGameState())

	wantTag := "CoolMage"
	wantAddress := "123_456"
	cardinal.CreatePersonaMsg.AddToQueue(
		world, msg.CreatePersona{
			PersonaTag:    wantTag,
			SignerAddress: wantAddress,
		},
	)
	// This CreatePersona has the same persona tag, but it shouldn't be registered because
	// it comes second.
	cardinal.CreatePersonaMsg.AddToQueue(
		world, msg.CreatePersona{
			PersonaTag:    wantTag,
			SignerAddress: "some_other_address",
		},
	)

	// PersonaTag registration doesn't take place until the relevant system is run during a game tick.
	assert.NilError(t, world.Tick(context.Background()))

	signers := getSigners(t, world)
	ourSigner := signers[0]
	count := len(signers)
	assert.Equal(t, ourSigner.PersonaTag, wantTag)
	assert.Equal(t, ourSigner.SignerAddress, wantAddress)
	assert.Equal(t, 1, count)
}

func TestGetSignerForPersonaTagReturnsErrorWhenNotRegistered(t *testing.T) {
	world := testutils.NewTestFixture(t, nil).World
	assert.NilError(t, world.LoadGameState())
	ctx := context.Background()

	// Tick the game forward a bit to simulate a game that has been running for a bit of time.
	for i := 0; i < 10; i++ {
		assert.NilError(t, world.Tick(ctx))
	}

	_, err := world.GetSignerForPersonaTag("missing_persona", 1)
	assert.ErrorIs(t, err, cardinal.ErrPersonaTagHasNoSigner)

	// Queue up a CreatePersona
	personaTag := "foobar"
	signerAddress := "xyzzy"
	cardinal.CreatePersonaMsg.AddToQueue(
		world, msg.CreatePersona{
			PersonaTag:    personaTag,
			SignerAddress: signerAddress,
		},
	)
	// This CreatePersona will not be processed until the engine.CurrentTick() is greater than the tick that
	// originally got the CreatePersona.
	tick := world.CurrentTick()
	_, err = world.GetSignerForPersonaTag(personaTag, tick)
	assert.ErrorIs(t, err, cardinal.ErrCreatePersonaTxsNotProcessed)

	assert.NilError(t, world.Tick(ctx))
	// The CreatePersona has now been processed
	addr, err := world.GetSignerForPersonaTag(personaTag, tick)
	assert.NilError(t, err)
	assert.Equal(t, addr, signerAddress)
}

func TestDuplicatePersonaTagsInTickAreOnlyRegisteredOnce(t *testing.T) {
	world := testutils.NewTestFixture(t, nil).World
	assert.NilError(t, world.LoadGameState())

	personaTag := "jeff"

	for i := 0; i < 10; i++ {
		// Attempt to register many different signer addresses with the same persona tag.
		cardinal.CreatePersonaMsg.AddToQueue(
			world, msg.CreatePersona{
				PersonaTag:    personaTag,
				SignerAddress: fmt.Sprintf("address_%d", i),
			},
		)
	}
	tick := world.CurrentTick()

	ctx := context.Background()
	assert.NilError(t, world.Tick(ctx))

	addr, err := world.GetSignerForPersonaTag(personaTag, tick)
	assert.NilError(t, err)
	// Only the first address should be associated with the user
	assert.Equal(t, addr, "address_0")

	// Attempt to register this persona tag again in a different tick. We should still maintain the original
	// signer address.
	cardinal.CreatePersonaMsg.AddToQueue(
		world, msg.CreatePersona{
			PersonaTag:    personaTag,
			SignerAddress: "some_other_address",
		},
	)

	assert.NilError(t, world.Tick(ctx))
	addr, err = world.GetSignerForPersonaTag(personaTag, tick)
	assert.NilError(t, err)
	// The saved address should be unchanged
	assert.Equal(t, addr, "address_0")
}

func TestCreatePersonaFailsIfTagIsInvalid(t *testing.T) {
	// Verify that the CreatePersona is automatically created and registered with a engine.
	world := testutils.NewTestFixture(t, nil).World
	assert.NilError(t, world.LoadGameState())

	cardinal.CreatePersonaMsg.AddToQueue(
		world, msg.CreatePersona{
			PersonaTag:    "INVALID PERSONA TAG WITH SPACES",
			SignerAddress: "123_456",
		},
	)

	// PersonaTag registration doesn't take place until the relevant system is run during a game tick.
	assert.NilError(t, world.Tick(context.Background()))

	signers := getSigners(t, world)
	count := len(signers)
	assert.Equal(t, count, 0) // Assert that no signer components were found
}

func TestSamePersonaWithDifferentCaseCannotBeClaimed(t *testing.T) {
	// Verify that the CreatePersona is automatically created and registered with a engine.
	world := testutils.NewTestFixture(t, nil).World
	assert.NilError(t, world.LoadGameState())

	cardinal.CreatePersonaMsg.AddToQueue(
		world, msg.CreatePersona{
			PersonaTag:    "WowTag",
			SignerAddress: "123_456",
		},
	)

	// This one should fail because it is the same tag with different casing!
	cardinal.CreatePersonaMsg.AddToQueue(
		world, msg.CreatePersona{
			PersonaTag:    "wowtag",
			SignerAddress: "123_456",
		},
	)

	// PersonaTag registration doesn't take place until the relevant system is run during a game tick.
	assert.NilError(t, world.Tick(context.Background()))

	signers := getSigners(t, world)
	count := len(signers)
	assert.Equal(t, count, 1) // Assert that only one signer component was found and it was the first one
	assert.Equal(t, signers[0].PersonaTag, "WowTag")
}

func TestCanAuthorizeAddress(t *testing.T) {
	// Verify that the CreatePersona is automatically created and registered with a engine.
	world := testutils.NewTestFixture(t, nil).World
	assert.NilError(t, world.LoadGameState())

	wantTag := "CoolMage"
	wantSigner := "123_456"
	cardinal.CreatePersonaMsg.AddToQueue(
		world, msg.CreatePersona{
			PersonaTag:    wantTag,
			SignerAddress: wantSigner,
		},
	)

	wantAddr := "0xd5e099c71b797516c10ed0f0d895f429c2781142"
	cardinal.AuthorizePersonaAddressMsg.AddToQueue(
		world, msg.AuthorizePersonaAddress{
			Address: wantAddr,
		}, &sign.Transaction{PersonaTag: wantTag},
	)
	// PersonaTag registration doesn't take place until the relevant system is run during a game tick.
	assert.NilError(t, world.Tick(context.Background()))

	signers := getSigners(t, world)
	ourSigner := signers[0]
	count := len(signers)
	assert.Equal(t, ourSigner.PersonaTag, wantTag)
	assert.Equal(t, ourSigner.SignerAddress, wantSigner)
	assert.Equal(t, len(ourSigner.AuthorizedAddresses), 1)
	assert.Equal(t, ourSigner.AuthorizedAddresses[0], wantAddr)

	// verify that the query was even ran. if for some reason there were no SignerComponents in the state,
	// this test would still pass (false positive).
	assert.Equal(t, count, 1)
}

func TestAuthorizeAddressFailsOnInvalidAddress(t *testing.T) {
	// Verify that the CreatePersona is automatically created and registered with a engine.
	world := testutils.NewTestFixture(t, nil).World
	assert.NilError(t, world.LoadGameState())

	personaTag := "CoolMage"
	invalidAddr := "123-456"
	cardinal.CreatePersonaMsg.AddToQueue(
		world, msg.CreatePersona{
			PersonaTag:    personaTag,
			SignerAddress: invalidAddr,
		},
	)

	wantAddr := "INVALID ADDRESS"
	cardinal.AuthorizePersonaAddressMsg.AddToQueue(
		world, msg.AuthorizePersonaAddress{
			Address: wantAddr,
		}, &sign.Transaction{PersonaTag: personaTag},
	)
	// PersonaTag registration doesn't take place until the relevant system is run during a game tick.
	assert.NilError(t, world.Tick(context.Background()))

	signers := getSigners(t, world)
	ourSigner := signers[0]
	count := len(signers)
	assert.Equal(t, ourSigner.PersonaTag, personaTag)
	assert.Equal(t, ourSigner.SignerAddress, invalidAddr)
	assert.Len(t, ourSigner.AuthorizedAddresses, 0) // Assert that no authorized address was added

	// verify that the query was even ran. if for some reason there were no SignerComponents in the state,
	// this test would still pass (false positive).
	assert.Equal(t, count, 1)
}

func getSigners(t *testing.T, world *cardinal.World) []*component.SignerComponent {
	eCtx := cardinal.NewWorldContext(world)
	var signers = make([]*component.SignerComponent, 0)

	q := cardinal.NewSearch(eCtx, filter.Exact(component.SignerComponent{}))

	err := q.Each(
		func(id entity.ID) bool {
			sc, err := ecs.GetComponent[component.SignerComponent](eCtx, id)
			assert.NilError(t, err)
			signers = append(signers, sc)
			return true
		},
	)
	assert.NilError(t, err)
	return signers
}

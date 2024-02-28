package persona_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/persona"
	"pkg.world.dev/world-engine/cardinal/persona/component"
	"pkg.world.dev/world-engine/cardinal/persona/msg"
	personaQuery "pkg.world.dev/world-engine/cardinal/persona/query"
	"pkg.world.dev/world-engine/cardinal/types"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/sign"

	"pkg.world.dev/world-engine/assert"
)

func TestIsAlphanumericWithUnderscore(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"abc123", true},
		{"ABC_123", true},
		{"123", true},
		{"abc 123", false}, // contains a space
		{"abc123 ", false}, // contains a trailing space
		{"abc@123", false}, // contains a special character
		{"", false},        // empty string
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := persona.IsValidPersonaTag(test.input)
			assert.Equal(t, result, test.expected)
		})
	}
}

func TestCreatePersonaTransactionAutomaticallyCreated(t *testing.T) {
	// Verify that the cardinal.CreatePersona is automatically cardinal.Created and registered with a engine.
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	tf.StartWorld()

	wantTag := "CoolMage"
	wantAddress := "123_456"
	createPersonaMsg, err := cardinal.GetMessageFromWorld[msg.CreatePersona, msg.CreatePersonaResult](world)
	assert.NilError(t, err)
	tf.AddTransaction(
		createPersonaMsg.ID(), msg.CreatePersona{
			PersonaTag:    wantTag,
			SignerAddress: wantAddress,
		},
	)
	// This cardinal.CreatePersona has the same persona tag, but it shouldn't be registered because
	// it comes second.
	tf.AddTransaction(
		createPersonaMsg.ID(), msg.CreatePersona{
			PersonaTag:    wantTag,
			SignerAddress: "some_other_address",
		},
	)

	// PersonaTag registration doesn't take place until the relevant system is run during a game tick.
	assert.NilError(t, world.Tick(context.Background(), uint64(time.Now().Unix())))

	signers := getSigners(t, world)
	ourSigner := signers[0]
	count := len(signers)
	assert.Equal(t, ourSigner.PersonaTag, wantTag)
	assert.Equal(t, ourSigner.SignerAddress, wantAddress)
	assert.Equal(t, 1, count)
}

func TestGetSignerForPersonaTagReturnsErrorWhenNotRegistered(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	tf.StartWorld()
	ctx := context.Background()

	// Tick the game forward a bit to simulate a game that has been running for a bit of time.
	for i := 0; i < 10; i++ {
		assert.NilError(t, world.Tick(ctx, uint64(time.Now().Unix())))
	}

	_, err := world.GetSignerForPersonaTag("missing_persona", 1)
	assert.ErrorIs(t, err, persona.ErrPersonaTagHasNoSigner)

	// Queue up a cardinal.CreatePersona
	personaTag := "foobar"
	signerAddress := "xyzzy"
	createPersonaMsg, err := cardinal.GetMessageFromWorld[msg.CreatePersona, msg.CreatePersonaResult](world)
	assert.NilError(t, err)
	tf.AddTransaction(
		createPersonaMsg.ID(), msg.CreatePersona{
			PersonaTag:    personaTag,
			SignerAddress: signerAddress,
		},
	)
	// This cardinal.CreatePersona will not be processed until the engine.CurrentTick() is greater than the tick that
	// originally got the cardinal.CreatePersona.
	tick := world.CurrentTick()
	_, err = world.GetSignerForPersonaTag(personaTag, tick)
	assert.ErrorIs(t, err, persona.ErrCreatePersonaTxsNotProcessed)

	assert.NilError(t, world.Tick(ctx, uint64(time.Now().Unix())))
	// The cardinal.CreatePersona has now been processed
	addr, err := world.GetSignerForPersonaTag(personaTag, tick)
	assert.NilError(t, err)
	assert.Equal(t, addr, signerAddress)
}

func TestDuplicatePersonaTagsInTickAreOnlyRegisteredOnce(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	tf.StartWorld()

	personaTag := "jeff"
	createPersonaMsg, err := cardinal.GetMessageFromWorld[msg.CreatePersona, msg.CreatePersonaResult](world)
	assert.NilError(t, err)
	for i := 0; i < 10; i++ {
		// Attempt to register many different signer addresses with the same persona tag.
		tf.AddTransaction(
			createPersonaMsg.ID(), msg.CreatePersona{
				PersonaTag:    personaTag,
				SignerAddress: fmt.Sprintf("address_%d", i),
			},
		)
	}
	tick := world.CurrentTick()

	ctx := context.Background()
	assert.NilError(t, world.Tick(ctx, uint64(time.Now().Unix())))

	addr, err := world.GetSignerForPersonaTag(personaTag, tick)
	assert.NilError(t, err)
	// Only the first address should be associated with the user
	assert.Equal(t, addr, "address_0")

	// Attempt to register this persona tag again in a different tick. We should still maintain the original
	// signer address.
	tf.AddTransaction(
		createPersonaMsg.ID(), msg.CreatePersona{
			PersonaTag:    personaTag,
			SignerAddress: "some_other_address",
		},
	)

	assert.NilError(t, world.Tick(ctx, uint64(time.Now().Unix())))
	addr, err = world.GetSignerForPersonaTag(personaTag, tick)
	assert.NilError(t, err)
	// The saved address should be unchanged
	assert.Equal(t, addr, "address_0")
}

func TestCreatePersonaFailsIfTagIsInvalid(t *testing.T) {
	// Verify that the cardinal.CreatePersona is automatically cardinal.Created and registered with a engine.
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	tf.StartWorld()

	createPersonaMsg, err := cardinal.GetMessageFromWorld[msg.CreatePersona, msg.CreatePersonaResult](world)
	assert.NilError(t, err)
	tf.AddTransaction(
		createPersonaMsg.ID(), msg.CreatePersona{
			PersonaTag:    "INVALID PERSONA TAG WITH SPACES",
			SignerAddress: "123_456",
		},
	)

	// PersonaTag registration doesn't take place until the relevant system is run during a game tick.
	assert.NilError(t, world.Tick(context.Background(), uint64(time.Now().Unix())))

	signers := getSigners(t, world)
	count := len(signers)
	assert.Equal(t, count, 0) // Assert that no signer components were found
}

func TestSamePersonaWithDifferentCaseCannotBeClaimed(t *testing.T) {
	// Verify that the cardinal.CreatePersona is automatically cardinal.Created and registered with a engine.
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	tf.StartWorld()

	createPersonaMsg, err := cardinal.GetMessageFromWorld[msg.CreatePersona, msg.CreatePersonaResult](world)
	assert.NilError(t, err)
	tf.AddTransaction(
		createPersonaMsg.ID(), msg.CreatePersona{
			PersonaTag:    "WowTag",
			SignerAddress: "123_456",
		},
	)

	// This one should fail because it is the same tag with different casing!
	tf.AddTransaction(
		createPersonaMsg.ID(), msg.CreatePersona{
			PersonaTag:    "wowtag",
			SignerAddress: "123_456",
		},
	)

	// PersonaTag registration doesn't take place until the relevant system is run during a game tick.
	assert.NilError(t, world.Tick(context.Background(), uint64(time.Now().Unix())))

	signers := getSigners(t, world)
	count := len(signers)
	assert.Equal(t, count, 1) // Assert that only one signer component was found and it was the first one
	assert.Equal(t, signers[0].PersonaTag, "WowTag")
}

func TestCanAuthorizeAddress(t *testing.T) {
	// Verify that the cardinal.CreatePersona is automatically cardinal.Created and registered with a engine.
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	tf.StartWorld()

	wantTag := "CoolMage"
	wantSigner := "123_456"
	createPersonaMsg, err := cardinal.GetMessageFromWorld[msg.CreatePersona, msg.CreatePersonaResult](world)
	assert.NilError(t, err)
	tf.AddTransaction(
		createPersonaMsg.ID(), msg.CreatePersona{
			PersonaTag:    wantTag,
			SignerAddress: wantSigner,
		},
	)
	wantAddr := "0xd5e099c71b797516c10ed0f0d895f429c2781142"
	authorizePersonaAddressMsg, err :=
		cardinal.GetMessageFromWorld[msg.AuthorizePersonaAddress, msg.AuthorizePersonaAddressResult](world)
	assert.NilError(t, err)
	tf.AddTransaction(
		authorizePersonaAddressMsg.ID(),
		msg.AuthorizePersonaAddress{
			Address: wantAddr,
		},
		&sign.Transaction{
			PersonaTag: wantTag,
		},
	)
	// PersonaTag registration doesn't take place until the relevant system is run during a game tick.
	assert.NilError(t, world.Tick(context.Background(), uint64(time.Now().Unix())))

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
	// Verify that the cardinal.CreatePersona is automatically cardinal.Created and registered with a engine.
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	tf.StartWorld()

	personaTag := "CoolMage"
	invalidAddr := "123-456"
	createPersonaMsg, err := cardinal.GetMessageFromWorld[msg.CreatePersona, msg.CreatePersonaResult](world)
	assert.NilError(t, err)
	tf.AddTransaction(
		createPersonaMsg.ID(), msg.CreatePersona{
			PersonaTag:    personaTag,
			SignerAddress: invalidAddr,
		},
	)

	wantAddr := "INVALID ADDRESS"
	tf.AddTransaction(
		createPersonaMsg.ID(), msg.AuthorizePersonaAddress{
			Address: wantAddr,
		}, &sign.Transaction{PersonaTag: personaTag},
	)
	// PersonaTag registration doesn't take place until the relevant system is run during a game tick.
	assert.NilError(t, world.Tick(context.Background(), uint64(time.Now().Unix())))

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

func TestQuerySigner(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	personaTag := "CoolMage"
	signerAddr := "123_456"
	createPersonaMsg, err := cardinal.GetMessageFromWorld[msg.CreatePersona, msg.CreatePersonaResult](world)
	assert.NilError(t, err)
	world.AddTransaction(createPersonaMsg.ID(), msg.CreatePersona{
		PersonaTag:    personaTag,
		SignerAddress: signerAddr,
	}, &sign.Transaction{})
	tf.DoTick()

	query, err := world.GetQueryByName("signer")
	assert.NilError(t, err)

	res, err := query.HandleQuery(cardinal.NewReadOnlyWorldContext(world), &personaQuery.PersonaSignerQueryRequest{
		PersonaTag: personaTag,
	})
	assert.NilError(t, err)

	response, ok := res.(*personaQuery.PersonaSignerQueryResponse)
	assert.True(t, ok)
	assert.Equal(t, response.SignerAddress, signerAddr)
	assert.Equal(t, response.Status, personaQuery.PersonaStatusAssigned)
}

func TestQuerySignerAvailable(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	tf.DoTick()

	query, err := world.GetQueryByName("signer")
	assert.NilError(t, err)
	res, err := query.HandleQuery(cardinal.NewReadOnlyWorldContext(world), &personaQuery.PersonaSignerQueryRequest{
		PersonaTag: "some-random-nonexistent-persona-tag",
	})
	assert.NilError(t, err)
	response, ok := res.(*personaQuery.PersonaSignerQueryResponse)
	assert.True(t, ok)

	assert.Equal(t, response.Status, personaQuery.PersonaStatusAvailable)
}

func TestQuerySignerUnknown(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	engine := tf.World
	tf.DoTick()

	query, err := engine.GetQueryByName("signer")
	assert.NilError(t, err)
	res, err := query.HandleQuery(cardinal.NewReadOnlyWorldContext(engine), &personaQuery.PersonaSignerQueryRequest{
		PersonaTag: "doesnt_matter",
		Tick:       engine.CurrentTick(),
	})
	assert.NilError(t, err)

	response, ok := res.(*personaQuery.PersonaSignerQueryResponse)
	assert.True(t, ok)
	assert.Equal(t, response.Status, personaQuery.PersonaStatusUnknown)
}

func getSigners(t *testing.T, world *cardinal.World) []*component.SignerComponent {
	wCtx := cardinal.NewWorldContext(world)
	var signers = make([]*component.SignerComponent, 0)

	q := cardinal.NewSearch(wCtx, filter.Exact(component.SignerComponent{}))

	err := q.Each(
		func(id types.EntityID) bool {
			sc, err := cardinal.GetComponent[component.SignerComponent](wCtx, id)
			assert.NilError(t, err)
			signers = append(signers, sc)
			return true
		},
	)
	assert.NilError(t, err)
	return signers
}

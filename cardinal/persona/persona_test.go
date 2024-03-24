package persona_test

import (
	"fmt"
	"strings"
	"testing"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/persona"
	"pkg.world.dev/world-engine/cardinal/persona/component"
	"pkg.world.dev/world-engine/cardinal/persona/msg"
	personaQuery "pkg.world.dev/world-engine/cardinal/persona/query"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/sign"
)

func TestPersonaTagIsValid(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"abc123", true},
		{"ABC_123", true},
		{"123", true},
		{"abc 123", false},               // contains a space
		{"abc123 ", false},               // contains a trailing space
		{"abc@123", false},               // contains a special character
		{"snow☃man", false},              // contains a unicode character
		{"", false},                      // empty string
		{"a", false},                     // too short
		{"aa", false},                    // too short
		{strings.Repeat("a", 17), false}, // too long,
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
	createPersonaMsg, ok := world.GetMessageByFullName("persona.create-persona")
	assert.True(t, ok)
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
	_, err := tf.DoTick()
	assert.NilError(t, err)

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

	// Tick the game forward a bit to simulate a game that has been running for a bit of time.
	for i := 0; i < 10; i++ {
		_, err := tf.DoTick()
		assert.NilError(t, err)
	}

	_, err := world.GetSignerForPersonaTag("missing_persona", 1)
	assert.ErrorIs(t, err, persona.ErrPersonaTagHasNoSigner)

	// Queue up a cardinal.CreatePersona
	personaTag := "foobar"
	signerAddress := "xyzzy"
	_, err = world.GetSignerForPersonaTag(personaTag, world.CurrentTick())
	assert.ErrorIs(t, err, persona.ErrCreatePersonaTxsNotProcessed)

	tf.CreatePersona(personaTag, signerAddress)

	// The cardinal.CreatePersona has now been processed
	addr, err := world.GetSignerForPersonaTag(personaTag, world.CurrentTick()-1)
	assert.NilError(t, err)
	assert.Equal(t, addr, signerAddress)
}

func TestDuplicatePersonaTagsInTickAreOnlyRegisteredOnce(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	tf.StartWorld()
	personaTag := "jeff"
	for i := 0; i < 10; i++ {
		tf.CreatePersona(personaTag, fmt.Sprintf("address_%d", i))
	}

	addr, err := world.GetSignerForPersonaTag(personaTag, world.CurrentTick()-1)
	assert.NilError(t, err)
	// Only the first address should be associated with the user
	assert.Equal(t, addr, "address_0")

	tf.CreatePersona(personaTag, "some_other_address")
	addr, err = world.GetSignerForPersonaTag(personaTag, world.CurrentTick()-1)
	assert.NilError(t, err)
	// The saved address should be unchanged
	assert.Equal(t, addr, "address_0")
}

func TestCreatePersonaFailsIfTagIsInvalid(t *testing.T) {
	// Verify that the cardinal.CreatePersona is automatically cardinal.Created and registered with a engine.
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	tf.StartWorld()
	tf.CreatePersona("INVALID PERSONA TAG WITH SPACES", "123_456")

	// PersonaTag registration doesn't take place until the relevant system is run during a game tick.
	_, err := tf.DoTick()
	assert.NilError(t, err)

	signers := getSigners(t, world)
	count := len(signers)
	assert.Equal(t, count, 0) // Assert that no signer components were found
}

func TestSamePersonaWithDifferentCaseCannotBeClaimed(t *testing.T) {
	// Verify that the cardinal.CreatePersona is automatically cardinal.Created and registered with a engine.
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	tf.StartWorld()
	tf.CreatePersona("WowTag", "123_456")
	tf.CreatePersona("wowtag", "123_456")

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
	tf.CreatePersona(wantTag, wantSigner)

	wantAddr := "0xd5e099c71b797516c10ed0f0d895f429c2781142"
	authorizePersonaAddressMsg, ok := world.GetMessageByFullName("game.authorize-persona-address")
	assert.True(t, ok)
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
	_, err := tf.DoTick()
	assert.NilError(t, err)

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
	addr := "123-456"
	tf.CreatePersona(personaTag, addr)

	invalidAuthAddress := "INVALID ADDRESS"
	authMsg, exists := world.GetMessageByFullName("game.authorize-persona-address")
	assert.True(t, exists)
	tf.AddTransaction(
		authMsg.ID(), msg.AuthorizePersonaAddress{
			Address: invalidAuthAddress,
		}, &sign.Transaction{PersonaTag: personaTag},
	)
	// PersonaTag registration doesn't take place until the relevant system is run during a game tick.
	_, err := tf.DoTick()
	assert.NilError(t, err)

	signers := getSigners(t, world)
	ourSigner := signers[0]
	count := len(signers)
	assert.Equal(t, ourSigner.PersonaTag, personaTag)
	assert.Equal(t, ourSigner.SignerAddress, addr)
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
	tf.CreatePersona(personaTag, signerAddr)

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
	_, err := tf.DoTick()
	assert.NilError(t, err)

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
	_, err := tf.DoTick()
	assert.NilError(t, err)

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

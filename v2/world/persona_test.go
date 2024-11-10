package world_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/v2"
	"pkg.world.dev/world-engine/cardinal/v2/gamestate/search/filter"
	"pkg.world.dev/world-engine/cardinal/v2/types"
	"pkg.world.dev/world-engine/cardinal/v2/world"
)

func TestGetPersonaComponent(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)

	tf.CreatePersona("hello")

	tf.DoTick()

	err := tf.Cardinal.World().View(func(wCtx world.WorldContextReadOnly) error {
		err := wCtx.Search(filter.Exact(world.Persona{})).Each(
			func(id types.EntityID) bool {
				p, err := world.GetComponent[world.Persona](wCtx, id)
				assert.NilError(t, err)
				assert.Equal(t, p.PersonaTag, "hello")
				assert.Equal(t, p.SignerAddress, tf.SignerAddress())
				return true
			},
		)
		assert.NilError(t, err)
		return nil
	})
	assert.NilError(t, err)
}

func TestCreatePersonaSystem_WithNoPersonaTagCreateTxs_TickShouldBeFast(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)

	const trials = 100
	startTime := time.Now()
	// Collect a baseline average tick duration when there are no persona tags and nothing is going on
	for i := 0; i < trials; i++ {
		tf.DoTick()
	}
	baselineDuration := time.Since(startTime) / trials

	// Create 100 persona tags and make sure they exist
	for i := 0; i < 100; i++ {
		msg := world.CreatePersona{
			PersonaTag: fmt.Sprintf("personatag%d", i),
		}

		txHash := tf.AddTransaction(world.CreatePersona{}.Name(), msg)

		tf.DoTick()

		// Make sure the persona tag was actually added
		receipt, err := tf.Cardinal.World().GetReceipt(txHash)
		assert.NilError(t, err)
		assert.Empty(t, receipt.Error)
	}

	startTime = time.Now()
	// Collect another average for ticks that have no persona tag registrations. These ticks should be similar
	// in duration to the baseline.
	for i := 0; i < trials; i++ {
		tf.DoTick()
	}
	saturatedDuration := time.Since(startTime) / trials
	slowdownRatio := float64(saturatedDuration) / float64(baselineDuration)
	// Fail this test if the second batch of ticks is more than 5 times slower than the original
	// batch of ticks. The previously registered persona tags should have no performance impact on these systems.
	assert.True(t, slowdownRatio < 5, "ticks are much slower when many persona tags have been registered")
}

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
			result := world.IsValidPersonaTag(test.input)
			assert.Equal(t, result, test.expected)
		})
	}
}

func TestCreatePersonaTransactionAutomaticallyCreated(t *testing.T) {
	// Verify that the world.CreatePersona is automatically world.Created and registered with a engine.
	tf := cardinal.NewTestCardinal(t, nil)
	tf.StartWorld()

	wantTag := "CoolMage"
	tf.AddTransactionWithPersona(
		world.CreatePersona{}.Name(),
		wantTag,
		world.CreatePersona{
			PersonaTag: wantTag,
		},
	)
	// This world.CreatePersona has the same persona tag, but it shouldn't be registered because
	// it comes second.
	tf.AddTransactionWithPersona(
		world.CreatePersona{}.Name(),
		wantTag,
		world.CreatePersona{
			PersonaTag: wantTag,
		},
	)

	// PersonaTag registration doesn't take place until the relevant system is run during a game tick.
	tf.DoTick()

	signers := getPersona(t, tf.Cardinal.World())
	count := len(signers)
	assert.Equal(t, signers[0].PersonaTag, wantTag)
	assert.Equal(t, signers[0].SignerAddress, tf.SignerAddress())
	assert.Equal(t, 1, count)
}

func TestCreatePersonaFailsIfTagIsInvalid(t *testing.T) {
	// Verify that the world.CreatePersona is automatically world.Created and registered with a engine.
	tf := cardinal.NewTestCardinal(t, nil)
	tf.StartWorld()
	tf.CreatePersona("INVALID PERSONA TAG WITH SPACES")

	// PersonaTag registration doesn't take place until the relevant system is run during a game tick.
	tf.DoTick()

	signers := getPersona(t, tf.Cardinal.World())
	count := len(signers)
	assert.Equal(t, count, 0) // Assert that no signer components were found
}

func TestSamePersonaWithDifferentCaseCannotBeClaimed(t *testing.T) {
	// Verify that the world.CreatePersona is automatically world.Created and registered with a engine.
	tf := cardinal.NewTestCardinal(t, nil)
	tf.StartWorld()
	tf.CreatePersona("WowTag")
	tf.CreatePersona("wowtag")

	signers := getPersona(t, tf.Cardinal.World())
	count := len(signers)
	assert.Equal(t, count, 1) // Assert that only one signer component was found and it was the first one
	assert.Equal(t, signers[0].PersonaTag, "WowTag")
}

func TestCanAuthorizeAddress(t *testing.T) {
	// Verify that the world.CreatePersona is automatically world.Created and registered with a engine.
	tf := cardinal.NewTestCardinal(t, nil)
	tf.StartWorld()

	wantTag := "CoolMage"
	tf.CreatePersona(wantTag)

	authAddr := "0xd5e099c71b797516c10ed0f0d895f429c2781142"
	tf.AddTransactionWithPersona(
		world.AuthorizePersonaAddress{}.Name(),
		wantTag,
		world.AuthorizePersonaAddress{
			Address: authAddr,
		},
	)
	// PersonaTag registration doesn't take place until the relevant system is run during a game tick.
	tf.DoTick()

	signers := getPersona(t, tf.Cardinal.World())
	ourSigner := signers[0]
	count := len(signers)
	assert.Equal(t, ourSigner.PersonaTag, wantTag)
	assert.Equal(t, len(ourSigner.AuthorizedAddresses), 1)
	assert.Equal(t, ourSigner.AuthorizedAddresses[0], authAddr)

	// verify that the query was even ran. if for some reason there were no SignerComponents in the state,
	// this test would still pass (false positive).
	assert.Equal(t, count, 1)
}

func TestAuthorizeAddressFailsOnInvalidAddress(t *testing.T) {
	// Verify that the world.CreatePersona is automatically world.Created and registered with a engine.
	tf := cardinal.NewTestCardinal(t, nil)
	tf.StartWorld()

	personaTag := "CoolMage"
	tf.CreatePersona(personaTag)

	invalidAuthAddress := "INVALID ADDRESS"
	tf.AddTransactionWithPersona(
		world.AuthorizePersonaAddress{}.Name(),
		personaTag,
		world.AuthorizePersonaAddress{
			Address: invalidAuthAddress,
		},
	)
	// PersonaTag registration doesn't take place until the relevant system is run during a game tick.
	tf.DoTick()

	signers := getPersona(t, tf.Cardinal.World())
	ourSigner := signers[0]
	count := len(signers)
	assert.Equal(t, ourSigner.PersonaTag, personaTag)
	assert.Equal(t, ourSigner.SignerAddress, tf.SignerAddress())
	assert.Len(t, ourSigner.AuthorizedAddresses, 0) // Assert that no authorized address was added

	// verify that the query was even ran. if for some reason there were no SignerComponents in the state,
	// this test would still pass (false positive).
	assert.Equal(t, count, 1)
}

func TestQuerySigner(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)
	personaTag := "CoolMage"
	tf.CreatePersona(personaTag)

	reqBz, err := json.Marshal(&world.PersonaQueryReq{
		PersonaTag: personaTag,
	})

	resBz, err := tf.Cardinal.World().HandleQuery("persona", "info", reqBz)
	assert.NilError(t, err)

	var res world.PersonaQueryResp
	err = json.Unmarshal(resBz, &res)
	assert.NilError(t, err)

	assert.Equal(t, res.Persona.SignerAddress, tf.SignerAddress())
	assert.Equal(t, res.Status, world.PersonaStatusAssigned)
}

func TestQuerySignerAvailable(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)
	tf.DoTick()

	reqBz, err := json.Marshal(&world.PersonaQueryReq{
		PersonaTag: "some-random-nonexistent-persona-tag",
	})
	assert.NilError(t, err)

	resBz, err := tf.Cardinal.World().HandleQuery("persona", "info", reqBz)
	assert.NilError(t, err)

	var res world.PersonaQueryResp
	err = json.Unmarshal(resBz, &res)
	assert.NilError(t, err)

	assert.Equal(t, res.Status, world.PersonaStatusAvailable)
}

func getPersona(t *testing.T, w *world.World) []world.Persona {
	var signers = make([]world.Persona, 0)
	err := w.View(func(wCtx world.WorldContextReadOnly) error {
		err := wCtx.Search(filter.Exact(filter.Component[world.Persona]())).Each(
			func(id types.EntityID) bool {
				sc, err := world.GetComponent[world.Persona](wCtx, id)
				assert.NilError(t, err)
				signers = append(signers, *sc)
				return true
			},
		)
		assert.NilError(t, err)
		return nil
	})
	assert.NilError(t, err)
	return signers
}

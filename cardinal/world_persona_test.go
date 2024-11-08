package cardinal_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/persona/component"
	"pkg.world.dev/world-engine/cardinal/persona/msg"
	"pkg.world.dev/world-engine/cardinal/server/sign"
	"pkg.world.dev/world-engine/cardinal/types"
)

func TestGetSignerComponentForPersona(t *testing.T) {
	tf := cardinal.NewTestFixture(t, nil)
	world := tf.World
	msgType, exists := world.GetMessageByFullName("persona.create-persona")
	assert.True(t, exists)
	personaTag := "tyler"
	signer := "foobar"
	createPersonaMsg := msg.CreatePersona{
		PersonaTag:    personaTag,
		SignerAddress: signer,
	}
	world.AddTransaction(msgType.ID(), createPersonaMsg, &sign.Transaction{})
	tf.DoTick()

	sc, err := world.GetSignerComponentForPersona(personaTag)
	assert.NilError(t, err)
	assert.Equal(t, sc.PersonaTag, personaTag)
	assert.Equal(t, sc.SignerAddress, signer)

	notRealPersona := "nobody_important"
	sc, err = world.GetSignerComponentForPersona(notRealPersona)
	assert.ErrorContains(t, err, fmt.Sprintf("persona tag %q not found", notRealPersona))
	assert.Nil(t, sc)
}

func TestCreatePersonaSystem_WithNoPersonaTagCreateTxs_TickShouldBeFast(t *testing.T) {
	tf := cardinal.NewTestFixture(t, nil)

	const trials = 100
	startTime := time.Now()
	// Collect a baseline average tick duration when there are no persona tags and nothing is going on
	for i := 0; i < trials; i++ {
		tf.DoTick()
	}
	baselineDuration := time.Since(startTime) / trials

	msgType, exists := tf.World.GetMessageByFullName("persona.create-persona")
	assert.True(t, exists)

	// Create 100 persona tags and make sure they exist
	for i := 0; i < 100; i++ {
		createPersonaMsg := msg.CreatePersona{
			PersonaTag:    fmt.Sprintf("personatag%d", i),
			SignerAddress: fmt.Sprintf("some-signer-%d", i),
		}
		tf.World.AddTransaction(msgType.ID(), createPersonaMsg, &sign.Transaction{})
		tf.DoTick()
		// Make sure the persona tag was actually added
		receipts, err := tf.World.GetTransactionReceiptsForTick(tf.World.CurrentTick() - 1)
		assert.NilError(t, err)
		assert.Equal(t, 1, len(receipts))
		assert.Equal(t, 0, len(receipts[0].Errs))
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

func TestCreatePersonaSystem_WhenCardinalIsRestarted_PersonaTagsAreStillRegistered(t *testing.T) {
	tf := cardinal.NewTestFixture(t, nil)

	type countPersonaTagsResult struct {
		personaTags map[string]bool
		err         error
	}

	// This is a buffered channel so that the System that uses it doesn't block. It has to be drained
	// after the DoTick call.
	numOfPersonaTags := make(chan countPersonaTagsResult, 1)

	// emitNumberOfPersonaTagsSystem is a system that finds all the registered persona tags and sends them
	// across a channel.
	emitNumberOfPersonaTagsSystem := func(wCtx cardinal.WorldContext) error {
		result := countPersonaTagsResult{personaTags: map[string]bool{}}
		err := cardinal.NewSearch().
			Entity(filter.Exact(filter.Component[component.SignerComponent]())).
			Each(wCtx, func(id types.EntityID) bool {
				comp, err := cardinal.GetComponent[component.SignerComponent](wCtx, id)
				result.err = errors.Join(result.err, err)
				result.personaTags[comp.PersonaTag] = true
				return true
			})
		result.err = errors.Join(result.err, err)
		numOfPersonaTags <- result
		return nil
	}
	assert.NilError(t, cardinal.RegisterSystems(tf.World, emitNumberOfPersonaTagsSystem))

	msgType, exists := tf.World.GetMessageByFullName("persona.create-persona")
	assert.True(t, exists)

	// Register 10 persona tags. None of these should fail.
	for i := 0; i < 10; i++ {
		currPersonaTag := fmt.Sprintf("pt%d", i)
		tf.World.AddTransaction(msgType.ID(), msg.CreatePersona{
			PersonaTag:    currPersonaTag,
			SignerAddress: fmt.Sprintf("sa%d", i),
		}, &sign.Transaction{})
		tf.DoTick()
		result := <-numOfPersonaTags
		assert.NilError(t, result.err)
		assert.Equal(t, i+1, len(result.personaTags))
		assert.True(t, result.personaTags[currPersonaTag])
	}
	// There should now be 10 persona tags registered.

	// Simulate a cardinal restart by creating a new test fixture with the same redis DB.
	tf = cardinal.NewTestFixture(t, tf.Redis)
	assert.NilError(t, cardinal.RegisterSystems(tf.World, emitNumberOfPersonaTagsSystem))

	// Make sure there are still 10 persona tags registered.
	tf.DoTick()
	result := <-numOfPersonaTags
	assert.NilError(t, result.err)
	assert.Equal(t, 10, len(result.personaTags))
	for i := 0; i < 10; i++ {
		currPersonaTag := fmt.Sprintf("pt%d", i)
		assert.True(t, result.personaTags[currPersonaTag])
	}

	// This persona tag has already been registered, so it should fail to register this time.
	repeatPersonaTag := "pt5"
	tf.World.AddTransaction(msgType.ID(), msg.CreatePersona{
		PersonaTag:    repeatPersonaTag,
		SignerAddress: "some-sa",
	}, &sign.Transaction{})
	tf.DoTick()

	// Make sure the receipt from the previous tick talks about the failed persona tag registration.
	receipts, err := tf.World.GetTransactionReceiptsForTick(tf.World.CurrentTick() - 1)
	assert.NilError(t, err)
	assert.Len(t, receipts, 1)
	errs := receipts[0].Errs
	assert.Len(t, errs, 1)
	assert.ErrorContains(t, errs[0], "persona tag pt5 has already been registered")
}

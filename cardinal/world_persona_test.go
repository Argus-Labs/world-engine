package cardinal_test

import (
	"fmt"
	"testing"
	"time"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/persona/msg"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/sign"
)

func TestGetSignerComponentForPersona(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
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
	tf := testutils.NewTestFixture(t, nil)

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

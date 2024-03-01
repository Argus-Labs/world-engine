package cardinal_test

import (
	"fmt"
	"pkg.world.dev/world-engine/assert"
	msg2 "pkg.world.dev/world-engine/cardinal/persona/msg"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/sign"
	"testing"
)

func TestGetSignerComponentForPersona(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	msg, exists := world.GetMessageByName("create-persona")
	assert.True(t, exists)
	personaTag := "tyler"
	signer := "foobar"
	createPersonaMsg := msg2.CreatePersona{
		PersonaTag:    personaTag,
		SignerAddress: signer,
	}
	world.AddTransaction(msg.ID(), createPersonaMsg, &sign.Transaction{})
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

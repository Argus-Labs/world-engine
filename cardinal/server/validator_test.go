package server_test

import (
	"crypto/ecdsa"
	"encoding/json"
	"math/rand"
	"net/http"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/suite"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/persona/msg"
	"pkg.world.dev/world-engine/cardinal/server/utils"
	"pkg.world.dev/world-engine/sign"
)

type ValidatorTestSuite struct {
	suite.Suite

	privateKey *ecdsa.PrivateKey
	signerAddr string
	namespace  string
}

func TestServerValidator(t *testing.T) {
	suite.Run(t, new(ValidatorTestSuite))
}

// SetupSuite runs before each test in the suite.
func (s *ValidatorTestSuite) SetupTest() {
	var err error
	s.privateKey, err = crypto.GenerateKey()
	s.Require().NoError(err)
	s.signerAddr = crypto.PubkeyToAddress(s.privateKey.PublicKey).Hex()
	s.namespace = "boo"
}

// TearDownTest runs after each test in the suite.
func (s *ValidatorTestSuite) TearDownTest() {
}

// TestCanSendTxWithoutSigVerification tests that you can submit a tx with just a persona and body when sig verification
// is disabled.
func (s *ValidatorTestSuite) TestCanSendTxWithoutSigVerification() {
	s.setupWorld(cardinal.WithDisableSignatureVerification())
	s.fixture.DoTick()
	persona := s.CreateRandomPersona()
	s.createPersona(persona)
	msg := MoveMsgInput{Direction: "up"}
	msgBz, err := json.Marshal(msg)
	s.Require().NoError(err)
	tx := &sign.Transaction{
		PersonaTag: persona,
		Body:       msgBz,
	}
	moveMessage, ok := s.world.GetMessageByFullName("game." + moveMsgName)
	s.Require().True(ok)
	url := "/tx/game/" + moveMessage.Name()
	res := s.fixture.Post(url, tx)
	s.Require().Equal(fiber.StatusOK, res.StatusCode, s.readBody(res.Body))
	s.fixture.DoTick()

	// check the component was successfully updated, despite not using any signature data.
	res = s.fixture.Post("query/game/location", QueryLocationRequest{Persona: persona})
	var loc LocationComponent
	err = json.Unmarshal([]byte(s.readBody(res.Body)), &loc)
	s.Require().NoError(err)
	s.Require().Equal(LocationComponent{0, 1}, loc)
}

func (s *ValidatorTestSuite) TestMissingSignerAddressIsOKWhenSigVerificationIsDisabled() {
	t := s.T()
	s.setupWorld(cardinal.WithDisableSignatureVerification())
	s.fixture.DoTick()
	unclaimedPersona := "some-persona"
	moveMessage, ok := s.world.GetMessageByFullName("game." + moveMsgName)
	assert.True(t, ok)
	// This persona tag does not have a signer address, but since signature verification is disabled it should
	// encounter no errors
	payload := MoveMsgInput{Direction: "up"}

	tx, err := sign.NewTransaction(s.privateKey, unclaimedPersona, s.world.Namespace(), payload)
	assert.NilError(t, err)

	// This request should not fail because signature verification is disabled
	res := s.fixture.Post(utils.GetTxURL(moveMessage.Group(), moveMessage.Name()), tx)
	assert.Equal(t, http.StatusOK, res.StatusCode)
}

func (s *ValidatorTestSuite) TestSignerAddressIsRequiredWhenSigVerificationIsEnabled() {
	t := s.T()
	// Signature verification is enabled
	s.setupWorld()
	s.fixture.DoTick()
	unclaimedPersona := "some-persona"
	moveMessage, ok := s.world.GetMessageByFullName("game." + moveMsgName)
	assert.True(t, ok)
	payload := MoveMsgInput{Direction: "up"}
	tx, err := sign.NewTransaction(s.privateKey, unclaimedPersona, s.world.Namespace(), payload)
	assert.NilError(t, err)

	// This request should fail because signature verification is enabled, and we have not yet
	// registered the given personaTag
	res := s.fixture.Post(utils.GetTxURL(moveMessage.Group(), moveMessage.Name()), tx)
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
}

func (s *ValidatorTestSuite) TestRejectExpiredTransaction() {
	s.setupWorld(cardinal.WithMessageExpiration(1)) // very short expiration
	s.fixture.DoTick()

	personaTag := s.CreateRandomPersona()
	moveMessage, ok := s.world.GetMessageByFullName("game." + moveMsgName)
	s.Require().True(ok)

	// Create a transaction with an expired timestamp
	payload := MoveMsgInput{Direction: "up"}
	tx, err := sign.NewTransaction(
		s.privateKey, personaTag, s.world.Namespace(), payload)
	s.Require().NoError(err)

	// now wait until the transaction has expired before sending it
	time.Sleep(2 * time.Second)

	// Attempt to submit the transaction
	res := s.fixture.Post(utils.GetTxURL(moveMessage.Group(), moveMessage.Name()), tx)
	s.Require().Equal(fiber.StatusRequestTimeout, res.StatusCode, s.readBody(res.Body))
}

func (s *ValidatorTestSuite) TestReceivedTransactionHashIsIgnored() {
	s.setupWorld(cardinal.WithMessageExpiration(1)) // very short expiration
	s.fixture.DoTick()

	personaTag := s.CreateRandomPersona()
	moveMessage, ok := s.world.GetMessageByFullName("game." + moveMsgName)
	s.Require().True(ok)

	// Create a transaction with an expired timestamp
	payload := MoveMsgInput{Direction: "up"}
	tx, err := sign.NewTransaction(
		s.privateKey, personaTag, s.world.Namespace(), payload)
	tx.Hash = common.Hash{0x0b, 0xad}
	s.Require().NoError(err)

	// Attempt to submit the transaction
	res := s.fixture.Post(utils.GetTxURL(moveMessage.Group(), moveMessage.Name()), tx)
	s.Require().Equal(fiber.StatusOK, res.StatusCode, s.readBody(res.Body))
}

func (s *ValidatorTestSuite) TestRejectBadTransactionTimestamp() {
	s.setupWorld(cardinal.WithMessageExpiration(1)) // very short expiration
	s.fixture.DoTick()

	personaTag := s.CreateRandomPersona()
	moveMessage, ok := s.world.GetMessageByFullName("game." + moveMsgName)
	s.Require().True(ok)

	// Create a transaction with an expired timestamp
	payload := MoveMsgInput{Direction: "up"}
	tx, err := sign.NewTransaction(
		s.privateKey, personaTag, s.world.Namespace(), payload)
	time.Sleep(1 * time.Second)
	tx.Timestamp = sign.TimestampNow()
	s.Require().NoError(err)

	// Attempt to submit the transaction
	res := s.fixture.Post(utils.GetTxURL(moveMessage.Group(), moveMessage.Name()), tx)
	s.Require().Equal(fiber.StatusUnauthorized, res.StatusCode, s.readBody(res.Body))
}

func (s *ValidatorTestSuite) TestRejectDuplicateTransactionHash() {
	s.setupWorld()
	s.fixture.DoTick()

	personaTag := s.CreateRandomPersona()
	moveMessage, ok := s.world.GetMessageByFullName("game." + moveMsgName)
	s.Require().True(ok)

	// Create a transaction
	tx, err := sign.NewTransaction(s.privateKey, personaTag, s.world.Namespace(), MoveMsgInput{Direction: "up"})
	s.Require().NoError(err)

	// Submit the transaction for the first time
	res := s.fixture.Post(utils.GetTxURL(moveMessage.Group(), moveMessage.Name()), tx)
	s.Require().Equal(fiber.StatusOK, res.StatusCode, s.readBody(res.Body))

	s.fixture.DoTick()

	// Attempt to submit the same transaction again
	res = s.fixture.Post(utils.GetTxURL(moveMessage.Group(), moveMessage.Name()), tx)
	s.Require().Equal(fiber.StatusForbidden, res.StatusCode, s.readBody(res.Body))
}

// Creates a persona with the specified tag.
func (s *ValidatorTestSuite) createPersona(personaTag string) {
	createPersonaTx := msg.CreatePersona{
		PersonaTag:    personaTag,
		SignerAddress: s.signerAddr,
	}
	tx, err := sign.NewSystemTransaction(s.privateKey, s.world.Namespace(), createPersonaTx)
	s.Require().NoError(err)
	res := s.fixture.Post(utils.GetTxURL("persona", "create-persona"), tx)
	s.Require().Equal(fiber.StatusOK, res.StatusCode, s.readBody(res.Body))
	s.fixture.DoTick()
}

// CreateRandomPersona Creates a random persona and returns it as a string
func (s *ValidatorTestSuite) CreateRandomPersona() string {
	letterRunes := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	length := 5
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		result[i] = byte(letterRunes[r.Intn(len(letterRunes))])
	}
	persona := string(result)
	s.createPersona(persona)
	return persona
}

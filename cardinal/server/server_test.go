package server_test

import (
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/suite"
	"github.com/swaggo/swag"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/persona/msg"
	"pkg.world.dev/world-engine/cardinal/server/handler"
	"pkg.world.dev/world-engine/cardinal/server/utils"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/sign"
)

// Used for Registering message.
type MoveMsgInput struct {
	Direction string
}

// Used for Registering message.
type MoveMessageOutput struct {
	Location LocationComponent
}

type QueryLocationRequest struct {
	Persona string
}

type QueryLocationResponse struct {
	LocationComponent
}

type ServerTestSuite struct {
	suite.Suite

	fixture *cardinal.TestFixture
	world   *cardinal.World

	privateKey *ecdsa.PrivateKey
	signerAddr string
}

var moveMsgName = "move"

func TestServer(t *testing.T) {
	suite.Run(t, new(ServerTestSuite))
}

// SetupSuite runs before each test in the suite.
func (s *ServerTestSuite) SetupTest() {
	var err error
	s.privateKey, err = crypto.GenerateKey()
	s.Require().NoError(err)
	s.signerAddr = crypto.PubkeyToAddress(s.privateKey.PublicKey).Hex()
}

// TearDownTest runs after each test in the suite.
func (s *ServerTestSuite) TearDownTest() {
	s.fixture.World.Shutdown()
}

// TestCanClaimPersonaSendGameTxAndQueryGame tests that you can claim a persona, send a tx, and then query.
func (s *ServerTestSuite) TestCanClaimPersonaSendGameTxAndQueryGame() {
	s.setupWorld()
	s.fixture.DoTick()
	personaTag := s.CreateRandomPersona()
	moveMessage, ok := s.world.GetMessageByFullName("game." + moveMsgName)
	s.Require().True(ok)
	s.runTx(personaTag, moveMessage, MoveMsgInput{Direction: "up"})
	res := s.fixture.Post("query/game/location", QueryLocationRequest{Persona: personaTag})
	var loc LocationComponent
	err := json.Unmarshal([]byte(s.readBody(res.Body)), &loc)
	s.Require().NoError(err)
	s.Require().Equal(LocationComponent{0, 1}, loc)
}

// TestGetFieldInformation tests the fields endpoint.
func (s *ServerTestSuite) TestGetWorld() {
	s.setupWorld()
	s.fixture.DoTick()
	res := s.fixture.Get("/world")
	var result handler.GetWorldResponse
	err := json.Unmarshal([]byte(s.readBody(res.Body)), &result)
	s.Require().NoError(err)
	comps := s.world.GetRegisteredComponents()
	msgs := s.world.GetRegisteredMessages()
	queries := s.world.GetRegisteredQueries()

	s.Require().Len(comps, len(result.Components))
	s.Require().Len(msgs, len(result.Messages))
	s.Require().Len(queries, len(result.Queries))

	// check that the component, message, query name are in the list
	for _, comp := range comps {
		assert.True(s.T(), slices.ContainsFunc(result.Components, func(field types.FieldDetail) bool {
			return comp.Name() == field.Name
		}))
	}
	for _, msg := range msgs {
		assert.True(s.T(), slices.ContainsFunc(result.Messages, func(field types.FieldDetail) bool {
			return msg.Name() == field.Name
		}))
	}
	for _, query := range queries {
		assert.True(s.T(), slices.ContainsFunc(result.Queries, func(field types.FieldDetail) bool {
			return query.Name() == field.Name
		}))
	}
	assert.Equal(s.T(), s.world.Namespace(), result.Namespace)
}

// TestSwaggerEndpointsAreActuallyCreated verifies the non-variable endpoints that are declared in the swagger.yml file
// actually have endpoints when the cardinal server starts.
func (s *ServerTestSuite) TestSwaggerEndpointsAreActuallyCreated() {
	s.setupWorld()
	s.fixture.DoTick()
	m := map[string]any{}
	s.Require().NoError(json.Unmarshal([]byte(swag.GetSwagger("swagger").ReadDoc()), &m))
	paths, ok := m["paths"].(map[string]any)
	s.Require().True(ok)

	for path, iface := range paths {
		info, ok := iface.(map[string]any)
		s.Require().True(ok)
		if strings.ContainsAny(path, "{}") {
			// Don't bother verifying paths that contain variables.
			continue
		}
		if _, ok := info["get"]; ok {
			res := s.fixture.Get(path)
			// This test is only checking to make sure the endpoint can be found.
			s.NotEqualf(404, res.StatusCode,
				"swagger defines GET %q, but that endpoint was not found", path)
			s.NotEqualf(405, res.StatusCode,
				"swagger defines GET %q, but GET is not allowed on that endpoint", path)
		}
		if _, ok := info["post"]; ok {
			emptyPayload := struct{}{}
			res := s.fixture.Post(path, emptyPayload)
			// This test is only checking to make sure the endpoint can be found.
			s.NotEqualf(404, res.StatusCode,
				"swagger defines POST %q, but that endpoint was not found", path)
			s.NotEqualf(405, res.StatusCode,
				"swagger defines GET %q, but POST is not allowed on that endpoint", path)
		}
	}
}

// TestCanSendTxWithoutSigVerification tests that you can submit a tx with just a persona and body when sig verification
// is disabled.
func (s *ServerTestSuite) TestCanSendTxWithoutSigVerification() {
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

func (s *ServerTestSuite) TestQueryCustomGroup() {
	type SomeRequest struct{}
	type SomeResponse struct{}
	s.setupWorld()
	name := "foo"
	group := "bar"
	called := false
	err := cardinal.RegisterQuery[SomeRequest, SomeResponse](
		s.world,
		name,
		func(_ cardinal.WorldContext, _ *SomeRequest) (*SomeResponse, error) {
			called = true
			return &SomeResponse{}, nil
		},
		cardinal.WithCustomQueryGroup[SomeRequest, SomeResponse](group),
	)
	s.Require().NoError(err)
	s.fixture.DoTick()
	res := s.fixture.Post(utils.GetQueryURL(group, name), SomeRequest{})
	s.Require().Equal(fiber.StatusOK, res.StatusCode)
	s.Require().True(called)
}

func (s *ServerTestSuite) TestMissingSignerAddressIsOKWhenSigVerificationIsDisabled() {
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

func (s *ServerTestSuite) TestSignerAddressIsRequiredWhenSigVerificationIsEnabled() {
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

func (s *ServerTestSuite) TestRejectExpiredTransaction() {
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

func (s *ServerTestSuite) TestReceivedTransactionHashIsIgnored() {
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

func (s *ServerTestSuite) TestRejectBadTransactionTimestamp() {
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

func (s *ServerTestSuite) TestRejectDuplicateTransactionHash() {
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

// Creates a transaction with the given message, and runs it in a tick.
func (s *ServerTestSuite) runTx(personaTag string, msg types.Message, payload any) {
	tx, err := sign.NewTransaction(s.privateKey, personaTag, s.world.Namespace(), payload)
	s.Require().NoError(err)
	res := s.fixture.Post(utils.GetTxURL(msg.Group(), msg.Name()), tx)
	s.Require().Equal(fiber.StatusOK, res.StatusCode, s.readBody(res.Body))
	s.fixture.DoTick()
}

// Creates a persona with the specified tag.
func (s *ServerTestSuite) createPersona(personaTag string) {
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

// setupWorld sets up a world with a simple movement system, message, and query.
func (s *ServerTestSuite) setupWorld(opts ...cardinal.WorldOption) {
	s.fixture = cardinal.NewTestFixture(s.T(), nil, opts...)
	s.world = s.fixture.World
	err := cardinal.RegisterComponent[LocationComponent](s.world)
	s.Require().NoError(err)
	err = cardinal.RegisterMessage[MoveMsgInput, MoveMessageOutput](s.world, moveMsgName)
	s.Require().NoError(err)
	personaToPosition := make(map[string]types.EntityID)
	err = cardinal.RegisterSystems(s.world, func(context cardinal.WorldContext) error {
		return cardinal.EachMessage[MoveMsgInput, MoveMessageOutput](context,
			func(tx cardinal.TxData[MoveMsgInput]) (MoveMessageOutput, error) {
				posID, exists := personaToPosition[tx.Tx.PersonaTag]
				if !exists {
					id, err := cardinal.Create(context, LocationComponent{})
					s.Require().NoError(err)
					personaToPosition[tx.Tx.PersonaTag] = id
					posID = id
				}
				var resultLocation LocationComponent
				err = cardinal.UpdateComponent[LocationComponent](context, posID,
					func(loc *LocationComponent) *LocationComponent {
						switch tx.Msg.Direction {
						case "up":
							loc.Y++
						case "down":
							loc.Y--
						case "right":
							loc.X++
						case "left":
							loc.X--
						}
						resultLocation = *loc
						return loc
					})
				s.Require().NoError(err)
				return MoveMessageOutput{resultLocation}, nil
			})
	})
	assert.NilError(s.T(), err)
	err = cardinal.RegisterQuery[QueryLocationRequest, QueryLocationResponse](
		s.world,
		"location",
		func(wCtx cardinal.WorldContext, req *QueryLocationRequest) (*QueryLocationResponse, error) {
			locID, exists := personaToPosition[req.Persona]
			if !exists {
				return nil, fmt.Errorf("location for %q does not exists", req.Persona)
			}
			loc, err := cardinal.GetComponent[LocationComponent](wCtx, locID)
			s.Require().NoError(err)

			return &QueryLocationResponse{*loc}, nil
		},
	)
	s.Require().NoError(err)
}

// returns the body of an http response as string.
func (s *ServerTestSuite) readBody(body io.ReadCloser) string {
	buf, err := io.ReadAll(body)
	s.Require().NoError(err)
	return string(buf)
}

// CreateRandomPersona Creates a random persona and returns it as a string.
func (s *ServerTestSuite) CreateRandomPersona() string {
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

type LocationComponent struct {
	X, Y uint64
}

func (LocationComponent) Name() string {
	return "location"
}

func (s *ServerTestSuite) TestCQL() {
	s.setupWorld()
	s.fixture.DoTick()

	wCtx := cardinal.NewWorldContext(s.world)
	_, err := cardinal.CreateMany(wCtx, 10, LocationComponent{})
	assert.NilError(s.T(), err)

	s.fixture.DoTick()

	res := s.fixture.Post("/cql", handler.CQLQueryRequest{CQL: "CONTAINS(location)"})
	var result handler.CQLQueryResponse
	err = json.Unmarshal([]byte(s.readBody(res.Body)), &result)
	s.Require().NoError(err)
	s.Require().Len(result.Results, 10)
}

func (s *ServerTestSuite) TestCQL_InvalidFormat() {
	s.setupWorld()
	s.fixture.DoTick()

	wCtx := cardinal.NewWorldContext(s.world)
	_, err := cardinal.CreateMany(wCtx, 10, LocationComponent{})
	assert.NilError(s.T(), err)

	s.fixture.DoTick()

	res := s.fixture.Post("/cql", handler.CQLQueryRequest{CQL: "MEOW(location)"})
	var result handler.CQLQueryResponse
	err = json.Unmarshal([]byte(s.readBody(res.Body)), &result)
	s.Require().Error(err)
}

func (s *ServerTestSuite) TestCQL_NonExistentComponent() {
	s.setupWorld()
	s.fixture.DoTick()

	wCtx := cardinal.NewWorldContext(s.world)
	_, err := cardinal.CreateMany(wCtx, 10, LocationComponent{})
	assert.NilError(s.T(), err)

	s.fixture.DoTick()

	res := s.fixture.Post("/cql", handler.CQLQueryRequest{CQL: "CONTAINS(meow)"})
	var result handler.CQLQueryResponse
	err = json.Unmarshal([]byte(s.readBody(res.Body)), &result)
	s.Require().Error(err)
}

package server_test

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"pkg.world.dev/world-engine/cardinal/query"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/suite"
	"github.com/swaggo/swag"
	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/sign"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/persona/msg"
	"pkg.world.dev/world-engine/cardinal/server/handler"
	"pkg.world.dev/world-engine/cardinal/server/utils"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

// Used for Registering message
type MoveMsgInput struct {
	Direction string
}

// Used for Registering message
type MoveMessageOutput struct {
	Location LocationComponent
}

var moveMsgName = "move"

type QueryLocationRequest struct {
	Persona string
}

type QueryLocationResponse struct {
	LocationComponent
}

type ServerTestSuite struct {
	suite.Suite

	fixture *testutils.TestFixture
	world   *cardinal.World

	privateKey *ecdsa.PrivateKey
	signerAddr string
	nonce      uint64
}

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
	s.Require().NoError(s.fixture.World.Shutdown())
}

// TestCanClaimPersonaSendGameTxAndQueryGame tests that you can claim a persona, send a tx, and then query.
func (s *ServerTestSuite) TestCanClaimPersonaSendGameTxAndQueryGame() {
	s.setupWorld()
	s.fixture.DoTick()
	personaTag := s.CreateRandomPersona()
	moveMessage, ok := s.world.GetMessageByName(moveMsgName)
	s.Require().True(ok)
	s.runTx(personaTag, moveMessage, MoveMsgInput{Direction: "up"})
	res := s.fixture.Post("query/game/location", QueryLocationRequest{Persona: personaTag})
	var loc LocationComponent
	err := json.Unmarshal([]byte(s.readBody(res.Body)), &loc)
	s.Require().NoError(err)
	s.Require().Equal(loc, LocationComponent{0, 1})
}

// TestCanListEndpoints tests the endpoints endpoint.
func (s *ServerTestSuite) TestCanListEndpoints() {
	s.setupWorld()
	s.fixture.DoTick()
	res := s.fixture.Get("/query/http/endpoints")
	var result handler.GetEndpointsResponse
	err := json.Unmarshal([]byte(s.readBody(res.Body)), &result)
	s.Require().NoError(err)
	msgs := s.world.GetRegisteredMessages()
	queries := s.world.GetRegisteredQueries()

	s.Require().Len(msgs, len(result.TxEndpoints))
	s.Require().Len(queries, len(result.QueryEndpoints))

	// Map iters are not guaranteed to be in the same order, so we just check that the endpoints are in the list
	for _, msg := range msgs {
		s.Require().True(slices.Contains(result.TxEndpoints, utils.GetTxURL(msg.Group(), msg.Name())))
	}
	for _, query := range queries {
		s.Require().True(slices.Contains(result.QueryEndpoints, utils.GetQueryURL(query.Group(), query.Name())))
	}
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
		assert.True(s.T(), slices.ContainsFunc(result.Components, func(field handler.FieldDetail) bool {
			return comp.Name() == field.Name
		}))
	}
	for _, msg := range msgs {
		assert.True(s.T(), slices.ContainsFunc(result.Messages, func(field handler.FieldDetail) bool {
			return msg.Name() == field.Name
		}))
	}
	for _, query := range queries {
		assert.True(s.T(), slices.ContainsFunc(result.Queries, func(field handler.FieldDetail) bool {
			return query.Name() == field.Name
		}))
	}
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
			s.NotEqualf(res.StatusCode, 404,
				"swagger defines GET %q, but that endpoint was not found", path)
			s.NotEqualf(res.StatusCode, 405,
				"swagger defines GET %q, but GET is not allowed on that endpoint", path)
		}
		if _, ok := info["post"]; ok {
			emptyPayload := struct{}{}
			res := s.fixture.Post(path, emptyPayload)
			// This test is only checking to make sure the endpoint can be found.
			s.NotEqualf(res.StatusCode, 404,
				"swagger defines POST %q, but that endpoint was not found", path)
			s.NotEqualf(res.StatusCode, 405,
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
	moveMessage, ok := s.world.GetMessageByName(moveMsgName)
	s.Require().True(ok)
	url := "/tx/game/" + moveMessage.Name()
	res := s.fixture.Post(url, tx)
	s.Require().Equal(fiber.StatusOK, res.StatusCode, s.readBody(res.Body))
	err = s.world.Tick(context.Background(), uint64(time.Now().Unix()))
	s.Require().NoError(err)
	s.nonce++

	// check the component was successfully updated, despite not using any signature data.
	res = s.fixture.Post("query/game/location", QueryLocationRequest{Persona: persona})
	var loc LocationComponent
	err = json.Unmarshal([]byte(s.readBody(res.Body)), &loc)
	s.Require().NoError(err)
	s.Require().Equal(loc, LocationComponent{0, 1})
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
		func(_ engine.Context, _ *SomeRequest) (*SomeResponse, error) {
			called = true
			return &SomeResponse{}, nil
		},
		query.WithCustomQueryGroup[SomeRequest, SomeResponse](group),
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
	moveMessage, ok := s.world.GetMessageByName(moveMsgName)
	assert.True(t, ok)
	// This persona tag does not have a signer address, but since signature verification is disabled it should
	// encounter no errors
	s.runTx(unclaimedPersona, moveMessage, MoveMsgInput{Direction: "up"})
}

func (s *ServerTestSuite) TestSignerAddressIsRequiredWhenSigVerificationIsDisabled() {
	t := s.T()
	// Signature verification is enabled
	s.setupWorld()
	s.fixture.DoTick()
	unclaimedPersona := "some-persona"
	moveMessage, ok := s.world.GetMessageByName(moveMsgName)
	assert.True(t, ok)
	payload := MoveMsgInput{Direction: "up"}
	tx, err := sign.NewTransaction(s.privateKey, unclaimedPersona, s.world.Namespace().String(), s.nonce, payload)
	assert.NilError(t, err)

	// This request should fail because signature verification is enabled, and we have not yet
	// registered the given personaTag
	res := s.fixture.Post(utils.GetTxURL(moveMessage.Group(), moveMessage.Name()), tx)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
}

// Creates a transaction with the given message, and runs it in a tick.
func (s *ServerTestSuite) runTx(personaTag string, msg types.Message, payload any) {
	tx, err := sign.NewTransaction(s.privateKey, personaTag, s.world.Namespace().String(), s.nonce, payload)
	s.Require().NoError(err)
	res := s.fixture.Post(utils.GetTxURL(msg.Group(), msg.Name()), tx)
	s.Require().Equal(fiber.StatusOK, res.StatusCode, s.readBody(res.Body))
	err = s.world.Tick(context.Background(), uint64(time.Now().Unix()))
	s.Require().NoError(err)
	s.nonce++
}

// Creates a persona with the specified tag.
func (s *ServerTestSuite) createPersona(personaTag string) {
	createPersonaTx := msg.CreatePersona{
		PersonaTag:    personaTag,
		SignerAddress: s.signerAddr,
	}
	tx, err := sign.NewSystemTransaction(s.privateKey, s.world.Namespace().String(), s.nonce, createPersonaTx)
	s.Require().NoError(err)
	res := s.fixture.Post(utils.GetTxURL("persona", "create-persona"), tx)
	s.Require().Equal(fiber.StatusOK, res.StatusCode, s.readBody(res.Body))
	err = s.world.Tick(context.Background(), uint64(time.Now().Unix()))
	s.Require().NoError(err)
	s.nonce++
}

// setupWorld sets up a world with a simple movement system, message, and query.
func (s *ServerTestSuite) setupWorld(opts ...cardinal.WorldOption) {
	s.fixture = testutils.NewTestFixture(s.T(), nil, opts...)
	s.world = s.fixture.World
	err := cardinal.RegisterComponent[LocationComponent](s.world)
	s.Require().NoError(err)
	err = cardinal.RegisterMessage[MoveMsgInput, MoveMessageOutput](s.world, moveMsgName)
	s.Require().NoError(err)
	personaToPosition := make(map[string]types.EntityID)
	err = cardinal.RegisterSystems(s.world, func(context engine.Context) error {
		return cardinal.EachMessage[MoveMsgInput, MoveMessageOutput](context,
			func(tx message.TxData[MoveMsgInput]) (MoveMessageOutput, error) {
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
		func(wCtx engine.Context, req *QueryLocationRequest) (*QueryLocationResponse, error) {
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

// CreateRandomPersona Creates a random persona and returns it as a string
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

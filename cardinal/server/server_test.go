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
	"pkg.world.dev/world-engine/cardinal/server/handler"
	"pkg.world.dev/world-engine/cardinal/server/utils"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/message"
	"pkg.world.dev/world-engine/cardinal/world"
	"pkg.world.dev/world-engine/sign"
)

// Used for Registering message
type MoveMsgInput struct {
	Direction string
}

func (MoveMsgInput) Name() string { return "move" }

// Used for Registering message
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

	fixture *cardinal.TestCardinal

	privateKey *ecdsa.PrivateKey
	signerAddr string
	nonce      uint64
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

// TestCanClaimPersonaSendGameTxAndQueryGame tests that you can claim a persona, send a tx, and then query.
func (s *ServerTestSuite) TestCanClaimPersonaSendGameTxAndQueryGame() {
	s.setupWorld()
	s.fixture.DoTick()

	personaTag := s.CreateRandomPersona()

	s.submitTx(moveMsgName, personaTag, MoveMsgInput{Direction: "up"})

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

	comps := s.fixture.World().State().RegisteredComponents()
	msgs := s.fixture.World().RegisteredMessages()
	queries := s.fixture.World().RegisteredQuries()
	s.Require().Len(comps, len(result.Components))
	s.Require().Len(msgs, len(result.Messages))
	s.Require().Len(queries, len(result.Queries))

	// check that the component, message, query name are in the list
	for _, comp := range comps {
		assert.True(s.T(), slices.ContainsFunc(result.Components, func(field types.ComponentInfo) bool {
			return comp.Name == field.Name
		}))
	}
	for _, msg := range msgs {
		assert.True(s.T(), slices.ContainsFunc(result.Messages, func(field types.EndpointInfo) bool {
			return msg.Name == field.Name
		}))
	}
	for _, query := range queries {
		assert.True(s.T(), slices.ContainsFunc(result.Queries, func(field types.EndpointInfo) bool {
			return query.Name() == field.Name
		}))
	}

	assert.Equal(s.T(), s.fixture.World().Namespace(), result.Namespace)
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

	s.submitTxWithoutSig(message.DefaultGroup, moveMsgName, persona, msg)

	s.fixture.DoTick()
	s.nonce++

	// check the component was successfully updated, despite not using any signature data.
	res := s.fixture.Post("query/game/location", QueryLocationRequest{Persona: persona})

	var loc LocationComponent
	err := json.Unmarshal([]byte(s.readBody(res.Body)), &loc)
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
	err := world.RegisterQuery[SomeRequest, SomeResponse](
		s.fixture.World(),
		name,
		func(_ world.WorldContextReadOnly, _ *SomeRequest) (*SomeResponse, error) {
			called = true
			return &SomeResponse{}, nil
		},
		world.WithGroup[SomeRequest, SomeResponse](group),
	)
	s.Require().NoError(err)
	s.fixture.DoTick()
	res := s.fixture.Post(utils.GetQueryURL(group, name), SomeRequest{})
	s.Require().Equal(fiber.StatusOK, res.StatusCode)
	s.Require().True(called)
}

func (s *ServerTestSuite) TestMissingSignerAddressIsOKWhenSigVerificationIsDisabled() {
	s.setupWorld(cardinal.WithDisableSignatureVerification())
	s.fixture.DoTick()
	unclaimedPersona := "some-persona"
	// This persona tag does not have a signer address, but since signature verification is disabled it should
	// encounter no errors
	s.submitTx(moveMsgName, unclaimedPersona, MoveMsgInput{Direction: "up"})
}

func (s *ServerTestSuite) TestSignerAddressIsRequiredWhenSigVerificationIsEnabled() {
	t := s.T()
	// Signature verification is enabled
	s.setupWorld()
	s.fixture.DoTick()
	unclaimedPersona := "some-persona"
	payload := MoveMsgInput{Direction: "up"}
	tx, err := sign.NewTransaction(s.privateKey, unclaimedPersona, s.fixture.World().Namespace(), s.nonce, payload)
	assert.NilError(t, err)

	// This request should fail because signature verification is enabled, and we have not yet
	// registered the given personaTag
	res := s.fixture.Post(utils.GetTxURL(moveMsgName), tx)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
}

// Creates a transaction with the given message, and runs it in a tick.
func (s *ServerTestSuite) submitTx(name string, personaTag string, payload any) common.Hash {
	tx, err := sign.NewTransaction(s.privateKey, personaTag, s.fixture.World().Namespace(), s.nonce, payload)
	s.Require().NoError(err)

	res := s.fixture.Post(utils.GetTxURL(name), tx)
	resBody := s.readBody(res.Body)
	s.Require().Equal(fiber.StatusOK, res.StatusCode, resBody)

	var txResp handler.PostTransactionResponse
	err = json.Unmarshal([]byte(resBody), &txResp)
	s.Require().NoError(err)

	s.fixture.DoTick()
	s.nonce++

	return txResp.TxHash
}

func (s *ServerTestSuite) submitTxWithoutSig(group string, name string, personaTag string, payload any) {
	body, err := json.Marshal(payload)
	s.Require().NoError(err)

	tx := &sign.Transaction{
		PersonaTag: personaTag,
		Body:       body,
	}

	res := s.fixture.Post(utils.GetTxURL(name), tx)
	s.Require().Equal(fiber.StatusOK, res.StatusCode, s.readBody(res.Body))

	s.fixture.DoTick()
	s.nonce++
}

// Creates a persona with the specified tag.
func (s *ServerTestSuite) createPersona(personaTag string) {
	createPersonaTx := world.CreatePersona{
		PersonaTag: personaTag,
	}

	tx, err := sign.NewTransaction(s.privateKey, "foo", s.fixture.World().Namespace(), s.nonce, createPersonaTx)
	s.Require().NoError(err)

	res := s.fixture.Post(utils.GetTxURL("persona.create-persona"), tx)
	s.Require().Equal(fiber.StatusOK, res.StatusCode, s.readBody(res.Body))
	s.fixture.DoTick()
	s.nonce++
}

// setupWorld sets up a world with a simple movement system, message, and query.
func (s *ServerTestSuite) setupWorld(opts ...cardinal.CardinalOption) {
	s.fixture = cardinal.NewTestCardinal(s.T(), nil, opts...)

	err := world.RegisterComponent[LocationComponent](s.fixture.World())
	s.Require().NoError(err)

	err = world.RegisterMessage[MoveMsgInput](s.fixture.World())
	s.Require().NoError(err)

	personaToPosition := make(map[string]types.EntityID)
	err = world.RegisterSystems(s.fixture.World(), func(context world.WorldContext) error {
		return world.EachMessage[MoveMsgInput](context,
			func(tx world.Tx[MoveMsgInput]) (any, error) {
				posID, exists := personaToPosition[tx.Tx.PersonaTag]
				if !exists {
					id, err := world.Create(context, LocationComponent{})
					s.Require().NoError(err)
					personaToPosition[tx.Tx.PersonaTag] = id
					posID = id
				}
				var resultLocation LocationComponent
				err = world.UpdateComponent[LocationComponent](context, posID,
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

	err = world.RegisterQuery[QueryLocationRequest, QueryLocationResponse](
		s.fixture.World(),
		"location",
		func(wCtx world.WorldContextReadOnly, req *QueryLocationRequest) (*QueryLocationResponse, error) {
			locID, exists := personaToPosition[req.Persona]
			if !exists {
				return nil, fmt.Errorf("location for %q does not exists", req.Persona)
			}
			loc, err := world.GetComponent[LocationComponent](wCtx, locID)
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

	personaTag := string(result)
	s.createPersona(personaTag)

	return personaTag
}

type LocationComponent struct {
	X, Y uint64
}

func (LocationComponent) Name() string {
	return "location"
}

func (s *ServerTestSuite) TestDebugStateQuery() {
	s.setupWorld()

	const wantNumOfZeroLocation = 5
	err := world.RegisterInitSystems(s.fixture.World(), func(wCtx world.WorldContext) error {
		_, err := world.CreateMany(wCtx, wantNumOfZeroLocation, LocationComponent{})
		return err
	})
	assert.NilError(s.T(), err)

	s.fixture.DoTick()

	personaTag := s.CreateRandomPersona()

	// This will create 1 additional location for this particular persona tag
	s.submitTx(moveMsgName, personaTag, MoveMsgInput{Direction: "up"})

	res := s.fixture.Post("debug/state", handler.DebugStateRequest{})
	s.Require().Equal(res.StatusCode, 200)

	var results []types.EntityData
	s.Require().NoError(json.NewDecoder(res.Body).Decode(&results))

	numOfZeroLocation := 0
	numOfNonZeroLocation := 0
	for _, result := range results {
		comp := result.Components["location"]
		if comp == nil {
			continue
		}
		var loc LocationComponent
		s.Require().NoError(json.Unmarshal(comp, &loc))

		if loc.Y == 0 {
			numOfZeroLocation++
		} else {
			numOfNonZeroLocation++
		}
	}
	s.Require().Equal(numOfZeroLocation, wantNumOfZeroLocation)
	s.Require().Equal(numOfNonZeroLocation, 1)
}

func (s *ServerTestSuite) TestDebugStateQuery_NoState() {
	s.setupWorld()
	s.fixture.DoTick()

	res := s.fixture.Post("debug/state", handler.DebugStateRequest{})
	s.Require().Equal(res.StatusCode, 200)

	var results []types.EntityData
	s.Require().NoError(json.NewDecoder(res.Body).Decode(&results))

	s.Require().Equal(len(results), 0)
}

type fooIn struct{}

func (fooIn) Name() string { return "foo" }

type fooOut struct{ Y int }

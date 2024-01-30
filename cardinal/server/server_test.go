package server_test

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/persona/msg"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/suite"
	"github.com/swaggo/swag"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/server/handler"
	"pkg.world.dev/world-engine/cardinal/server/utils"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/cardinal/types/entity"
	"pkg.world.dev/world-engine/cardinal/types/message"
	"pkg.world.dev/world-engine/sign"
)

type ServerTestSuite struct {
	suite.Suite

	fixture *testutils.TestFixture
	world   *cardinal.World
	engine  *ecs.Engine

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
	personaTag := s.createRandomPersona()
	s.runTx(personaTag, MoveMessage, MoveMsgInput{Direction: "up"})
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
	msgs := s.engine.ListMessages()
	queries := s.engine.ListQueries()

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
	persona := s.createRandomPersona()
	s.createPersona(persona)
	msg := MoveMsgInput{Direction: "up"}
	msgBz, err := json.Marshal(msg)
	s.Require().NoError(err)
	tx := &sign.Transaction{
		PersonaTag: persona,
		Body:       msgBz,
	}
	url := "/tx/game/" + MoveMessage.Name()
	res := s.fixture.Post(url, tx)
	s.Require().Equal(fiber.StatusOK, res.StatusCode, s.readBody(res.Body))
	err = s.engine.Tick(context.Background())
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
	err := ecs.RegisterQuery[SomeRequest, SomeResponse](
		s.engine,
		name,
		func(eCtx engine.Context, req *SomeRequest) (*SomeResponse, error) {
			called = true
			return &SomeResponse{}, nil
		},
		ecs.WithCustomQueryGroup[SomeRequest, SomeResponse](group),
	)
	s.Require().NoError(err)
	s.fixture.DoTick()
	res := s.fixture.Post(utils.GetQueryURL(group, name), SomeRequest{})
	s.Require().Equal(fiber.StatusOK, res.StatusCode)
	s.Require().True(called)
}

// creates a transaction with the given message, and runs it in a tick.
func (s *ServerTestSuite) runTx(personaTag string, msg message.Message, payload any) {
	tx, err := sign.NewTransaction(s.privateKey, personaTag, s.engine.Namespace().String(), s.nonce, payload)
	s.Require().NoError(err)
	res := s.fixture.Post(utils.GetTxURL(msg.Group(), msg.Name()), tx)
	s.Require().Equal(fiber.StatusOK, res.StatusCode, s.readBody(res.Body))
	err = s.engine.Tick(context.Background())
	s.Require().NoError(err)
	s.nonce++
}

// creates a persona with the specified tag.
func (s *ServerTestSuite) createPersona(personaTag string) {
	createPersonaTx := msg.CreatePersona{
		PersonaTag:    personaTag,
		SignerAddress: s.signerAddr,
	}
	tx, err := sign.NewSystemTransaction(s.privateKey, s.engine.Namespace().String(), s.nonce, createPersonaTx)
	s.Require().NoError(err)
	res := s.fixture.Post(utils.GetTxURL("persona", "create-persona"), tx)
	s.Require().Equal(fiber.StatusOK, res.StatusCode, s.readBody(res.Body))
	err = s.engine.Tick(context.Background())
	s.Require().NoError(err)
	s.nonce++
}

// setupWorld sets up a world with a simple movement system, message, and query.
func (s *ServerTestSuite) setupWorld(opts ...cardinal.WorldOption) {
	s.fixture = testutils.NewTestFixture(s.T(), nil, opts...)
	s.world = s.fixture.World
	s.engine = s.fixture.Engine
	err := ecs.RegisterComponent[LocationComponent](s.engine)
	s.Require().NoError(err)
	err = s.engine.RegisterMessages(MoveMessage)
	s.Require().NoError(err)
	personaToPosition := make(map[string]entity.ID)
	err = s.engine.RegisterSystems(func(context engine.Context) error {
		MoveMessage.Each(context, func(tx ecs.TxData[MoveMsgInput]) (MoveMessageOutput, error) {
			posID, exists := personaToPosition[tx.Tx.PersonaTag]
			if !exists {
				id, err := ecs.Create(context, LocationComponent{})
				s.Require().NoError(err)
				personaToPosition[tx.Tx.PersonaTag] = id
				posID = id
			}
			var resultLocation LocationComponent
			err = ecs.UpdateComponent[LocationComponent](context, posID,
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
		return nil
	})
	assert.NilError(s.T(), err)
	err = cardinal.RegisterQuery[QueryLocationRequest, QueryLocationResponse](
		s.world,
		"location",
		func(eCtx engine.Context, req *QueryLocationRequest) (*QueryLocationResponse, error) {
			locID, exists := personaToPosition[req.Persona]
			if !exists {
				return nil, fmt.Errorf("location for %q does not exists", req.Persona)
			}
			loc, err := cardinal.GetComponent[LocationComponent](eCtx, locID)
			s.Require().NoError(err)

			return &QueryLocationResponse{*loc}, nil
		},
	)
}

// returns the body of an http response as string.
func (s *ServerTestSuite) readBody(body io.ReadCloser) string {
	buf, err := io.ReadAll(body)
	s.Require().NoError(err)
	return string(buf)
}

// createRandomPersona creates a random persona and returns it as a string
func (s *ServerTestSuite) createRandomPersona() string {
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

type MoveMsgInput struct {
	Direction string
}

type MoveMessageOutput struct {
	Location LocationComponent
}

var MoveMessage = ecs.NewMessageType[MoveMsgInput, MoveMessageOutput]("move")

type QueryLocationRequest struct {
	Persona string
}

type QueryLocationResponse struct {
	LocationComponent
}

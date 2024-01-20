package server_test

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/suite"
	"io"
	"math/rand"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/server"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/cardinal/types/entity"
	"pkg.world.dev/world-engine/cardinal/types/message"
	"pkg.world.dev/world-engine/sign"
	"testing"
	"time"
)

type ServerTestSuite struct {
	suite.Suite

	world  *cardinal.World
	engine *ecs.Engine
	server *testutils.TestServer

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
	s.setupWorld()
}

// TearDownTest runs after each test in the suite.
func (s *ServerTestSuite) TearDownTest() {
	s.server.Shutdown()
}

// TestCanClaimPersonaSendGameTxAndQueryGame tests that you can claim a persona, send a tx, and then query.
func (s *ServerTestSuite) TestCanClaimPersonaSendGameTxAndQueryGame() {
	s.server = testutils.NewTestServer(s.T(), s.engine, server.WithPrettyPrint())
	personaTag := s.createRandomPersona()
	s.runTx(personaTag, MoveMessage, MoveMsgInput{Direction: "up"})
	res := s.server.Post("query/game/location", QueryLocationRequest{Persona: personaTag})
	var loc LocationComponent
	err := json.Unmarshal([]byte(s.readBody(res.Body)), &loc)
	s.Require().NoError(err)
	s.Require().Equal(loc, LocationComponent{0, 1})
}

// TestCanListEndpoints tests the endpoints endpoint.
func (s *ServerTestSuite) TestCanListEndpoints() {
	s.server = testutils.NewTestServer(s.T(), s.engine, server.WithPrettyPrint())
	res := s.server.Get("/query/http/endpoints")
	var result server.EndpointsResult
	err := json.Unmarshal([]byte(s.readBody(res.Body)), &result)
	s.Require().NoError(err)
	msgs, err := s.engine.ListMessages()
	s.Require().NoError(err)
	queries := s.engine.ListQueries()

	s.Require().Len(msgs, len(result.TxEndpoints))
	s.Require().Len(queries, len(result.QueryEndpoints))
}

// TestCanSendTxWithoutSigVerification tests that you can submit a tx with just a persona and body when sig verification
// is disabled.
func (s *ServerTestSuite) TestCanSendTxWithoutSigVerification() {
	s.server = testutils.NewTestServer(s.T(), s.engine, server.DisableSignatureVerification(), server.WithPrettyPrint())
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
	res := s.server.Post(url, tx)
	s.Require().Equal(res.StatusCode, fiber.StatusOK, s.readBody(res.Body))
	err = s.engine.Tick(context.Background())
	s.Require().NoError(err)
	s.nonce++

	// check the component was successfully updated, despite not using any signature data.
	res = s.server.Post("query/game/location", QueryLocationRequest{Persona: persona})
	var loc LocationComponent
	err = json.Unmarshal([]byte(s.readBody(res.Body)), &loc)
	s.Require().NoError(err)
	s.Require().Equal(loc, LocationComponent{0, 1})
}

// creates a transaction with the given message, and runs it in a tick.
func (s *ServerTestSuite) runTx(personaTag string, msg message.Message, payload any) {
	tx, err := sign.NewTransaction(s.privateKey, personaTag, s.engine.Namespace().String(), s.nonce, payload)
	s.Require().NoError(err)
	var url string
	if msg.Path() != "" {
		url = msg.Path()
	} else {
		url = "/tx/game/" + msg.Name()
	}
	res := s.server.Post(url, tx)
	s.Require().Equal(res.StatusCode, fiber.StatusOK, s.readBody(res.Body))
	err = s.engine.Tick(context.Background())
	s.Require().NoError(err)
	s.nonce++
}

// creates a persona with the specified tag.
func (s *ServerTestSuite) createPersona(personaTag string) {
	createPersonaTx := ecs.CreatePersona{
		PersonaTag:    personaTag,
		SignerAddress: s.signerAddr,
	}
	tx, err := sign.NewSystemTransaction(s.privateKey, s.engine.Namespace().String(), s.nonce, createPersonaTx)
	s.Require().NoError(err)
	res := s.server.Post(ecs.CreatePersonaMsg.Path(), tx)
	s.Require().Equal(res.StatusCode, fiber.StatusOK, s.readBody(res.Body))
	err = s.engine.Tick(context.Background())
	s.Require().NoError(err)
	s.nonce++
}

// setupWorld sets up a world with a simple movement system, message, and query.
func (s *ServerTestSuite) setupWorld(opts ...cardinal.WorldOption) {
	s.world = testutils.NewTestWorld(s.T(), opts...)
	s.engine = s.world.Engine()
	err := ecs.RegisterComponent[LocationComponent](s.engine)
	s.Require().NoError(err)
	err = s.engine.RegisterMessages(MoveMessage)
	s.Require().NoError(err)
	personaToPosition := make(map[string]entity.ID)
	s.engine.RegisterSystem(func(context ecs.EngineContext) error {
		MoveMessage.Each(context, func(tx ecs.TxData[MoveMsgInput]) (MoveMessageOutput, error) {
			posID, exists := personaToPosition[tx.Tx.PersonaTag]
			if !exists {
				id, err := ecs.Create(context, LocationComponent{})
				s.Require().NoError(err)
				personaToPosition[tx.Tx.PersonaTag] = id
				posID = id
			}
			var resultLocation LocationComponent
			err = ecs.UpdateComponent[LocationComponent](context, posID, func(loc *LocationComponent) *LocationComponent {
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
	s.Require().NoError(s.engine.LoadGameState())
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

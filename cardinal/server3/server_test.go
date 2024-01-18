package server3_test

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/suite"
	"io"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/ecs"
	server "pkg.world.dev/world-engine/cardinal/server3"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/cardinal/types/entity"
	"pkg.world.dev/world-engine/sign"
	"testing"
)

type ServerTestSuite struct {
	suite.Suite

	w      *cardinal.World
	e      *ecs.Engine
	server *testutils.TestTransactionHandler

	privateKey *ecdsa.PrivateKey
	signerAddr string
	nonce      uint64
}

func TestServer(t *testing.T) {
	suite.Run(t, new(ServerTestSuite))
}

func (s *ServerTestSuite) SetupSuite() {
	var err error
	s.privateKey, err = crypto.GenerateKey()
	s.Require().NoError(err)
	s.signerAddr = crypto.PubkeyToAddress(s.privateKey.PublicKey).Hex()
}

func (s *ServerTestSuite) TestTransaction() {
	s.setupWorld()
	s.server = testutils.NewTestServer(s.T(), s.e, server.WithPrettyPrint())
	s.createPersona("tyler")
}

func (s *ServerTestSuite) createPersona(personaTag string) {
	createPersonaTx := ecs.CreatePersona{
		PersonaTag:    personaTag,
		SignerAddress: s.signerAddr,
	}
	tx, err := sign.NewSystemTransaction(s.privateKey, s.e.Namespace().String(), s.nonce, createPersonaTx)
	s.nonce++
	s.Require().NoError(err)
	res := s.server.Post(ecs.CreatePersonaMsg.Path(), tx)
	s.Require().Equal(res.StatusCode, fiber.StatusOK, s.readBody(res.Body))
	err = s.e.Tick(context.Background())
	s.Require().NoError(err)
}

func (s *ServerTestSuite) setupWorld(opts ...cardinal.WorldOption) {
	s.w = testutils.NewTestWorld(s.T(), opts...)
	s.e = s.w.Engine()
	err := ecs.RegisterComponent[LocationComponent](s.e)
	s.Require().NoError(err)
	err = s.e.RegisterMessages(MoveMessage)
	s.Require().NoError(err)
	personaToPosition := make(map[string]entity.ID)
	s.e.RegisterSystem(func(context ecs.EngineContext) error {
		MoveMessage.Each(context, func(tx ecs.TxData[MoveMsgInput]) (MoveMessageOutput, error) {
			posID, exists := personaToPosition[tx.Tx.PersonaTag]
			var loc LocationComponent
			if !exists {
				loc = LocationComponent{}
				id, err := ecs.Create(context, loc)
				s.Require().NoError(err)
				personaToPosition[tx.Tx.PersonaTag] = id
				posID = id
			} else {
				location, err := ecs.GetComponent[LocationComponent](context, posID)
				s.Require().NoError(err)
				loc = *location
			}
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

			err = ecs.SetComponent[LocationComponent](context, posID, &loc)
			s.Require().NoError(err)

			return MoveMessageOutput{}, nil
		})
		return nil
	})
	err = cardinal.RegisterQuery[QueryLocationRequest, QueryLocationResponse](
		s.w,
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
	s.Require().NoError(s.e.LoadGameState())
}

func (s *ServerTestSuite) readBody(body io.ReadCloser) string {
	buf, err := io.ReadAll(body)
	s.Require().NoError(err)
	return string(buf)
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

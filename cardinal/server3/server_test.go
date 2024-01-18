package server3_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
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

	w *cardinal.World
	e *ecs.Engine
}

func TestServer(t *testing.T) {
	suite.Run(t, new(ServerTestSuite))
}

func (s *ServerTestSuite) TestSomething() {
	s.setupWorld(cardinal.WithDisableSignatureVerification())
	srvr := testutils.NewTestServer(s.T(), s.e, server.DisableSignatureVerification())
	msg := &ecs.CreatePersona{PersonaTag: "tyler", SignerAddress: "0xfoobarbaz"}
	msgBz, err := json.Marshal(msg)
	s.Require().NoError(err)
	tx := &sign.Transaction{
		PersonaTag: "tyler",
		Namespace:  cardinal.DefaultNamespace,
		Nonce:      1,
		Signature:  "sfosdf",
		PublicKey:  "asdklgj",
		Hash:       common.HexToHash("0xfoobar"),
		Body:       msgBz,
	}
	res := srvr.Post("tx/game/"+ecs.CreatePersonaMsg.Name(), tx)
	s.Require().Equal(res.StatusCode, fiber.StatusOK)
	err = s.e.Tick(context.Background())
	s.Require().NoError(err)
	comp, err := ecs.GetComponent[ecs.SignerComponent](ecs.NewEngineContext(s.e), 0)
	s.Require().NoError(err)
	s.Require().Equal(comp.PersonaTag, msg.PersonaTag)
	s.Require().Equal(comp.SignerAddress, msg.SignerAddress)
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

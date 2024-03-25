package server_test

import (
	"encoding/json"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/server/handler"
)

func (s *ServerTestSuite) TestDebugStateQuery() {
	s.setupWorld()
	s.fixture.DoTick()
	const wantNumOfZeroLocation = 5

	wCtx := cardinal.NewWorldContext(s.world)
	_, err := cardinal.CreateMany(wCtx, wantNumOfZeroLocation, LocationComponent{})
	personaTag := s.CreateRandomPersona()
	moveMessage, ok := s.world.GetMessageByFullName("game." + moveMsgName)
	s.Require().True(ok)
	// This will create 1 additional location for this particular persona tag
	s.runTx(personaTag, moveMessage, MoveMsgInput{Direction: "up"})

	res := s.fixture.Post("debug/state", handler.DebugStateRequest{})
	s.Require().NoError(err)
	s.Require().Equal(res.StatusCode, 200)

	var results handler.DebugStateResponse
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

	var results handler.DebugStateResponse
	s.Require().NoError(json.NewDecoder(res.Body).Decode(&results))

	s.Require().Equal(len(results), 0)
}

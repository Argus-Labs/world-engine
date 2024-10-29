package main

import (
	"errors"
	"log"

	"github.com/argus-labs/world-engine/example/tester/game/comp"
	"github.com/argus-labs/world-engine/example/tester/game/msg"
	"github.com/argus-labs/world-engine/example/tester/game/query"
	"github.com/argus-labs/world-engine/example/tester/game/sys"
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/world"
)

func main() {
	c, w, err := cardinal.New()
	if err != nil {
		log.Fatal(err, eris.ToString(err, true))
	}

	err = errors.Join(
		world.RegisterComponent[comp.Location](w),
		world.RegisterComponent[comp.Player](w),
	)
	if err != nil {
		log.Fatal(err, eris.ToString(err, true))
	}

	err = errors.Join(
		world.RegisterMessage[msg.JoinInput](w),
		world.RegisterMessage[msg.MoveInput](w),
		world.RegisterMessage[msg.ErrorInput](w),
	)
	if err != nil {
		log.Fatal(err, eris.ToString(err, true))
	}

	err = world.RegisterQuery[query.LocationReq, query.LocationResp](w, "location", query.Location)
	if err != nil {
		log.Fatal(err, eris.ToString(err, true))
	}

	err = world.RegisterSystems(w, sys.Join, sys.Move, sys.Error)
	if err != nil {
		log.Fatal(err, eris.ToString(err, true))
	}

	err = c.Start()
	if err != nil {
		log.Fatal(err, eris.ToString(err, true))
	}
}

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
)

func main() {
	options := []cardinal.WorldOption{
		cardinal.WithReceiptHistorySize(10), //nolint:gomnd // fine for testing.
	}

	world, err := cardinal.NewWorld(options...)
	if err != nil {
		log.Fatal(err, eris.ToString(err, true))
	}
	err = errors.Join(
		cardinal.RegisterComponent[comp.Location](world),
		cardinal.RegisterComponent[comp.Player](world),
	)
	if err != nil {
		log.Fatal(err, eris.ToString(err, true))
	}
	err = cardinal.RegisterMessages(world, msg.JoinMsg, msg.MoveMsg)
	if err != nil {
		log.Fatal(err, eris.ToString(err, true))
	}
	err = query.RegisterLocationQuery(world)
	if err != nil {
		log.Fatal(err, eris.ToString(err, true))
	}
	err = cardinal.RegisterSystems(world, sys.Join, sys.Move)
	if err != nil {
		log.Fatal(err, eris.ToString(err, true))
	}

	err = world.StartGame()
	if err != nil {
		log.Fatal(err, eris.ToString(err, true))
	}
}

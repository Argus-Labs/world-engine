//nolint:testableexamples // can figure this out later.
package cardinal_test

import (
	"errors"

	"pkg.world.dev/world-engine/cardinal"
)

type MovePlayerMsg struct {
	DeltaX int
	DeltaY int
}

type MovePlayerResult struct {
	FinalX int
	FinalY int
}

var MoveMsg = cardinal.NewMessageType[MovePlayerMsg, MovePlayerResult]("move-player")

func ExampleMessageType() {
	world, err := cardinal.NewMockWorld()
	if err != nil {
		panic(err)
	}

	err = cardinal.RegisterMessages(world, MoveMsg)
	if err != nil {
		panic(err)
	}

	err = cardinal.RegisterSystems(world, func(wCtx cardinal.WorldContext) error {
		for _, tx := range MoveMsg.In(wCtx) {
			msg := tx.Msg()
			// handle the msg
			// ...

			// save the result
			MoveMsg.SetResult(wCtx, tx.Hash(), MovePlayerResult{
				FinalX: msg.DeltaX,
				FinalY: msg.DeltaY,
			})

			// optionally, add an error
			MoveMsg.AddError(wCtx, tx.Hash(), errors.New("some error"))
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	// The above system will be called during each game tick.

	err = world.StartGame()
	if err != nil {
		panic(err)
	}
}

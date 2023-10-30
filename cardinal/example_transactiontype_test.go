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

var MoveTx = cardinal.NewTransactionType[MovePlayerMsg, MovePlayerResult]("move-player")

func ExampleTransactionType() {
	world, err := cardinal.NewMockWorld()
	if err != nil {
		panic(err)
	}

	err = cardinal.RegisterTransactions(world, MoveTx)
	if err != nil {
		panic(err)
	}

	cardinal.RegisterSystems(world, func(wCtx cardinal.WorldContext) error {
		for _, tx := range MoveTx.In(wCtx) {
			msg := tx.Value()
			// handle the transaction
			// ...

			// save the result
			MoveTx.SetResult(wCtx, tx.Hash(), MovePlayerResult{
				FinalX: msg.DeltaX,
				FinalY: msg.DeltaY,
			})

			// optionally, add an error to the transaction
			MoveTx.AddError(wCtx, tx.Hash(), errors.New("some error"))
		}
		return nil
	})
	// The above system will be called during each game tick.

	err = world.StartGame()
	if err != nil {
		panic(err)
	}
}

//nolint:testableexamples // can figure this out later.
package cardinal_test

import (
	"errors"
	"fmt"

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

func ExampleMessageType() {
	world, err := cardinal.NewWorld(cardinal.WithMockRedis())
	if err != nil {
		panic(err)
	}

	err = cardinal.RegisterMessage[MovePlayerMsg, MovePlayerResult](world, "move-player")
	if err != nil {
		panic(err)
	}

	err = cardinal.RegisterSystems(world, func(wCtx cardinal.WorldContext) error {
		return cardinal.EachMessage[MovePlayerMsg, MovePlayerResult](wCtx,
			func(txData cardinal.TxData[MovePlayerMsg]) (MovePlayerResult, error) {
				// handle the transaction
				// ...

				if err := errors.New("some error from a function"); err != nil {
					// A returned non-nil error will be appended to this transaction's list of errors. Any existing
					// transaction result will not be modified.
					return MovePlayerResult{}, fmt.Errorf("problem processing transaction: %w", err)
				}

				// Returning a nil error implies this transaction handling was successful, so this transaction result
				// will be saved to the transaction receipt.
				return MovePlayerResult{
					FinalX: txData.Msg.DeltaX,
					FinalY: txData.Msg.DeltaY,
				}, nil
			})
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

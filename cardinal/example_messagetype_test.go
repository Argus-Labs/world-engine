//nolint:testableexamples // can figure this out later.
package cardinal_test

import (
	"errors"
	"fmt"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/types/engine"
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
	world, err := cardinal.NewMockWorld()
	if err != nil {
		panic(err)
	}

	err = cardinal.RegisterMessage[MovePlayerMsg, MovePlayerResult](world, "move-player")
	if err != nil {
		panic(err)
	}

	err = cardinal.RegisterSystems(world, func(wCtx engine.Context) error {
		MoveMsg, err := cardinal.GetMessage[MovePlayerMsg, MovePlayerResult](wCtx)
		if err != nil {
			return err
		}
		MoveMsg.Each(wCtx, func(txData message.TxData[MovePlayerMsg]) (MovePlayerResult, error) {
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

package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/persona/msg"
	"pkg.world.dev/world-engine/cardinal/receipt"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

// This file contains helper methods that should only be used in the context of running tests.

func TestingWorldToWorldContext(world *World) engine.Context {
	return NewWorldContext(world)
}

func (w *World) TestingGetTransactionReceiptsForTick(tick uint64) ([]receipt.Receipt, error) {
	return w.GetTransactionReceiptsForTick(tick)
}

func (w *World) TestingAddCreatePersonaTxToQueue(data msg.CreatePersona) {
	CreatePersonaMsg.AddToQueue(w, data)
}

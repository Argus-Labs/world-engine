package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/persona/msg"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

// This file contains helper methods that should only be used in the context of running tests.

func TestingWorldToWorldContext(world *World) engine.Context {
	return ecs.NewEngineContext(world.engine)
}

func (w *World) TestingGetTransactionReceiptsForTick(tick uint64) ([]Receipt, error) {
	return w.engine.GetTransactionReceiptsForTick(tick)
}

func (w *World) TestingAddCreatePersonaTxToQueue(data msg.CreatePersona) {
	msg.CreatePersonaMsg.AddToQueue(w.engine, data)
}

package cardinal

import "pkg.world.dev/world-engine/cardinal/ecs"

// This file contains helper methods that should only be used in the context of running tests.

func TestingWorldToWorldContext(world *World) WorldContext {
	ecsWorldCtx := ecs.NewWorldContext(world.implWorld)
	return &worldContext{implContext: ecsWorldCtx}
}

func TestingWorldContextToECSWorld(worldCtx WorldContext) *ecs.World {
	return worldCtx.getECSWorldContext().GetWorld()
}

func (w *World) GetTransactionReceiptsForTick(tick uint64) ([]Receipt, error) {
	return w.implWorld.GetTransactionReceiptsForTick(tick)
}

// The following type and function are exported temporarily pending a refactor of
// how Persona works with the different components of Cardinal
type CreatePersonaTransaction = ecs.CreatePersonaTransaction

func (w *World) AddCreatePersonaTxToQueue(data CreatePersonaTransaction) {
	ecs.CreatePersonaTx.AddToQueue(w.implWorld, data)
}

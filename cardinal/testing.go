package cardinal

import "pkg.world.dev/world-engine/cardinal/ecs"

// This file contains helper methods that should only be used in the context of running tests.

func TestingWorldToWorldContext(world *World) WorldContext {
	ecsWorldCtx := ecs.NewWorldContext(world.instance)
	return &worldContext{instance: ecsWorldCtx}
}

func TestingWorldContextToECSWorld(worldCtx WorldContext) *ecs.World {
	return worldCtx.Instance().GetWorld()
}

func (w *World) TestingGetTransactionReceiptsForTick(tick uint64) ([]Receipt, error) {
	return w.instance.GetTransactionReceiptsForTick(tick)
}

// The following type and function are exported temporarily pending a refactor of
// how Persona works with the different components of Cardinal.
type CreatePersonaTransaction = ecs.CreatePersona

func (w *World) TestingAddCreatePersonaTxToQueue(data CreatePersonaTransaction) {
	ecs.CreatePersonaMsg.AddToQueue(w.instance, data)
}

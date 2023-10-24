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

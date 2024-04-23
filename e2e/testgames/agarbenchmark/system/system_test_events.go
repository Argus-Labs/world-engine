package system

import (
	"pkg.world.dev/world-engine/cardinal"
)

func TestEventsSystem(wCtx cardinal.WorldContext) error {
	const startingTick = 200
	if wCtx.CurrentTick() < startingTick {
		return nil
	}
	TestNakamaEventNotificationLimit(wCtx, int(wCtx.CurrentTick()-startingTick+1))
	return nil
}

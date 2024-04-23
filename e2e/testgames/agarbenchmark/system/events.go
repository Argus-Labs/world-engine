package system

import (
	"fmt"

	"github.com/ByteArena/box2d"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

var numEventsSentThisTick = 0
var lastTick uint64

const maxEventsPerTick = 100 // Set this higher to break Nakama. 102 was the last breaking value, but try higher if needed.

func send(wCtx engine.Context, evtMsg string) {
	var currentTick = wCtx.CurrentTick()
	if lastTick != currentTick {
		lastTick = currentTick
		numEventsSentThisTick = 0
	}
	numEventsSentThisTick++
	if numEventsSentThisTick < maxEventsPerTick {
		wCtx.Logger().Debug().Msgf("Sending Event: %s", evtMsg)
		//wCtx.Logger().Info().Msgf("numEventsSentThisTick: %d", numEventsSentThisTick) // Uncomment this line when stress testing Nakama.
		evtMsg = fmt.Sprintf("{\"message\": \"%s\"}", evtMsg) // Wrapping this as a quick fix.  Expected format: {"message": "<the event string>"}
		_ = wCtx.EmitStringEvent(evtMsg)
	} else {
		if numEventsSentThisTick == maxEventsPerTick {
			evtMsg = fmt.Sprintf("error,Too many events sent this tick: %d Dropping event: %s", maxEventsPerTick, evtMsg)
			_ = wCtx.EmitStringEvent(evtMsg)
		}
		wCtx.Logger().Warn().Msgf("Too many events sent this tick. Dropping event: %s", evtMsg)
	}
}

func TestNakamaEventNotificationLimit(wCtx engine.Context, numEvents int) {
	wCtx.Logger().Info().Msgf("Sending %d events.", numEvents)
	for i := 0; i < numEvents; i++ {
		send(wCtx, fmt.Sprintf("test,event,%d", i))
	}
}

func SendError(wCtx engine.Context, errMsg string) {
	evtMsg := fmt.Sprintf("error,%s", errMsg)
	send(wCtx, evtMsg)
}

func SendPosition(wCtx engine.Context, entityID types.EntityID, personaTag string, position box2d.B2Vec2) {
	evtMsg := fmt.Sprintf("position,%d,%v,%f,%f", entityID, personaTag, position.X, position.Y)
	send(wCtx, evtMsg)
}

func SendBearing(wCtx engine.Context, entityID types.EntityID, personaTag string, bearing float64) {
	evtMsg := fmt.Sprintf("bearing,%d,%v,%f", entityID, personaTag, bearing)
	send(wCtx, evtMsg)
}

func SendLinearVelocity(wCtx engine.Context, entityID types.EntityID, personaTag string, linearVelocity box2d.B2Vec2) {
	evtMsg := fmt.Sprintf("linear-velocity,%d,%v,%f,%f", entityID, personaTag, linearVelocity.X, linearVelocity.Y)
	send(wCtx, evtMsg)
}

func SendHit(wCtx cardinal.WorldContext, entityID types.EntityID, targetEntityID types.EntityID, damage int, health int) {
	evtMsg := fmt.Sprintf("hit,%d,%d,%d,%d", entityID, targetEntityID, damage, health)
	send(wCtx, evtMsg)
}

func SendKill(wCtx cardinal.WorldContext, entityID types.EntityID, targetEntityID types.EntityID) {
	evtMsg := fmt.Sprintf("kill,%d,%d", entityID, targetEntityID)
	send(wCtx, evtMsg)
}

// Send a cull event to the client instead of a kill event because no player is responsible for the kill.
// This could be a 'despawn' event but that would require breaking the kill event into two events or adding needless ambiguity.
// If the need occurs 3x or more then we should consider a 'despawn' event.
func SendCull(wCtx cardinal.WorldContext, entityID types.EntityID) {
	evtMsg := fmt.Sprintf("cull,%d", entityID)
	send(wCtx, evtMsg)
}

func SendHitCoinpack(wCtx engine.Context, entityID types.EntityID, targetEntityID types.EntityID, remainingWealth int) {
	evtMsg := fmt.Sprintf("hit-coinpack,%d,%d,%d", entityID, targetEntityID, remainingWealth)
	send(wCtx, evtMsg)
}

func SendSpawnPlayer(wCtx engine.Context, entityID types.EntityID, personaTag string, p box2d.B2Vec2) {
	evtMsg := fmt.Sprintf("spawn-player,%d,%v,%f,%f", entityID, personaTag, p.X, p.Y)
	send(wCtx, evtMsg)
}

func SendSpawnCoin(wCtx engine.Context, entityID types.EntityID, position box2d.B2Vec2) {
	evtMsg := fmt.Sprintf("spawn-coin,%d,%f,%f", entityID, position.X, position.Y)
	send(wCtx, evtMsg)
}

func SendSpawnMedpack(wCtx engine.Context, entityID types.EntityID, position box2d.B2Vec2) {
	evtMsg := fmt.Sprintf("spawn-medpack,%d,%f,%f", entityID, position.X, position.Y)
	send(wCtx, evtMsg)
}

func SendCoinPickup(wCtx engine.Context, a types.EntityID, b types.EntityID, currCoin int) {
	evtMsg := fmt.Sprintf("coin-pickup,%d,%d,%d", a, b, currCoin)
	send(wCtx, evtMsg)
}

func SendMedpackPickup(wCtx engine.Context, a types.EntityID, b types.EntityID, health int) {
	evtMsg := fmt.Sprintf("medpack-pickup,%d,%d,%d", a, b, health)
	send(wCtx, evtMsg)
}

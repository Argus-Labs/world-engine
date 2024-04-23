package query

import (
	"fmt"
	"strings"

	comp "github.com/argus-labs/world-engine/example/tester/agarbenchmark/component"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"
)

type WorldStateRequest struct {
}

// Note: If we had a strong concept of ECS on the client side it might be interesting to segregate this by component type vs object type.
type WorldStateResponse struct {
	CurrentTick int64
	Players     string
	Coins       string
	Medpacks    string
	Weapons     string
}

func WorldState(wCtx cardinal.WorldContext, _ *WorldStateRequest) (*WorldStateResponse, error) {
	var bearing *comp.Bearing
	var health *comp.Health
	var level *comp.Level
	var linearVelocity *comp.LinearVelocity
	var pickup *comp.Pickup
	var player *comp.Player
	var position *comp.Position
	var score *comp.Score

	result := &WorldStateResponse{}
	result.CurrentTick = int64(wCtx.CurrentTick())
	sb := strings.Builder{}

	err := cardinal.NewSearch().
		Entity(
			filter.Contains(
				filter.Component[comp.Player](),
				filter.Component[comp.Health](),
				filter.Component[comp.Level](),
				filter.Component[comp.Score](),
				filter.Component[comp.Position](),
				filter.Component[comp.Bearing](),
				filter.Component[comp.LinearVelocity]())).
		Each(wCtx,
			func(entityID types.EntityID) bool {
				player, _ = cardinal.GetComponent[comp.Player](wCtx, entityID)
				health, _ = cardinal.GetComponent[comp.Health](wCtx, entityID)
				level, _ = cardinal.GetComponent[comp.Level](wCtx, entityID)
				score, _ = cardinal.GetComponent[comp.Score](wCtx, entityID)
				position, _ = cardinal.GetComponent[comp.Position](wCtx, entityID)
				bearing, _ = cardinal.GetComponent[comp.Bearing](wCtx, entityID)
				linearVelocity, _ = cardinal.GetComponent[comp.LinearVelocity](wCtx, entityID)
				sb.WriteString(fmt.Sprintf("%d,%v,%d,%d,%d,%f,%f,%f,%f,%f,", entityID, player.PersonaTag, health.Value, level.Value, score.Value, position.X, position.Y, bearing.Degrees, linearVelocity.X, linearVelocity.Y))
				return true
			},
		)
	if err != nil {
		return nil, fmt.Errorf("failed to execute search for player: %w", err)
	}

	result.Players = sb.String()
	sb.Reset()

	err = cardinal.NewSearch().
		Entity(
			filter.Contains(
				filter.Component[comp.Coin](),
				filter.Component[comp.Pickup](),
				filter.Component[comp.Position]())).
		Each(wCtx,
			func(entityID types.EntityID) bool {
				pickup, err = cardinal.GetComponent[comp.Pickup](wCtx, entityID)
				if err != nil {
					wCtx.Logger().Error().Msgf(err.Error())
					return false
				}
				position, err = cardinal.GetComponent[comp.Position](wCtx, entityID)
				if err != nil {
					wCtx.Logger().Error().Msgf(err.Error())
					return false
				}
				// Shouldn't be needed given the error checking above that was also just added, but doing it anyway since we don't have fast iterations.
				if pickup == nil {
					wCtx.Logger().Error().Msgf("Entity %d is missing a pickup component", entityID)
					return false
				}
				if position == nil {
					wCtx.Logger().Error().Msgf("Entity %d is missing a position component", entityID)
					return false
				}
				sb.WriteString(fmt.Sprintf("%d,%d,%f,%f,", entityID, pickup.ID, position.X, position.Y))
				return true
			},
		)
	if err != nil {
		return nil, fmt.Errorf("failed to execute search for coin entity: %w", err)
	}

	result.Coins = sb.String()
	sb.Reset()

	err = cardinal.NewSearch().Entity(
		filter.Contains(
			filter.Component[comp.Medpack](),
			filter.Component[comp.Pickup](),
			filter.Component[comp.Position]())).
		Each(wCtx,
			func(entityID types.EntityID) bool {
				pickup, _ = cardinal.GetComponent[comp.Pickup](wCtx, entityID)
				position, _ = cardinal.GetComponent[comp.Position](wCtx, entityID)
				sb.WriteString(fmt.Sprintf("%d,%d,%f,%f,", entityID, pickup.ID, position.X, position.Y))
				return true
			},
		)
	if err != nil {
		return nil, fmt.Errorf("failed to execute search for medpack entity: %w", err)
	}

	result.Medpacks = sb.String()
	sb.Reset()

	return result, nil
}

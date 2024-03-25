package handler

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"

	"pkg.world.dev/world-engine/cardinal/search/filter"
	servertypes "pkg.world.dev/world-engine/cardinal/server/types"
	"pkg.world.dev/world-engine/cardinal/types"
)

type DebugStateRequest struct{}

type debugStateElement struct {
	ID         types.EntityID             `json:"id"`
	Components map[string]json.RawMessage `json:"components" swaggertype:"object"`
}

type DebugStateResponse []*debugStateElement

// GetDebugState godoc
//
//	@Summary		Get information on all entities and components in world-engine
//	@Description	Displays the entire game state.
//	@Produce		application/json
//	@Success		200	{object}	DebugStateResponse
//	@Router			/debug/state [post]
func GetDebugState(provider servertypes.Provider) func(*fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		result := make(DebugStateResponse, 0)
		s := provider.Search(filter.All())
		var eachClosureErr error
		searchEachErr := s.Each(
			func(id types.EntityID) bool {
				var components []types.ComponentMetadata
				components, eachClosureErr = provider.StoreReader().GetComponentTypesForEntity(id)
				if eachClosureErr != nil {
					return false
				}
				resultElement := debugStateElement{
					ID:         id,
					Components: make(map[string]json.RawMessage),
				}
				for _, c := range components {
					var data json.RawMessage
					data, eachClosureErr = provider.StoreReader().GetComponentForEntityInRawJSON(c, id)
					if eachClosureErr != nil {
						return false
					}
					resultElement.Components[c.Name()] = data
				}
				result = append(result, &resultElement)
				return true
			},
		)
		if eachClosureErr != nil {
			return eachClosureErr
		}
		if searchEachErr != nil {
			return searchEachErr
		}

		return ctx.JSON(&result)
	}
}

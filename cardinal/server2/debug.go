package server

import (
	"encoding/json"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/types/component"

	"pkg.world.dev/world-engine/cardinal/types/entity"
)

type DebugStateElement struct {
	ID   entity.ID         `json:"id"`
	Data []json.RawMessage `json:"data"`
}

type DebugStateResponse = []*DebugStateElement

// register debug endpoints for swagger app.
func (handler *Handler) registerDebugHandler() {
	// request name not required. This handler doesn't use anything in the request.
	debugStateHandler := createQueryHandlerFromRequest[interface{}, DebugStateResponse](
		func(i *interface{}) (*DebugStateResponse, error) {
			result := make(DebugStateResponse, 0)
			search := ecs.NewSearch(filter.All())
			wCtx := ecs.NewReadOnlyEngineContext(handler.w)
			var eachClosureErr error
			searchEachErr := search.Each(
				wCtx, func(id entity.ID) bool {
					var components []component.ComponentMetadata
					components, eachClosureErr = wCtx.StoreReader().GetComponentTypesForEntity(id)
					if eachClosureErr != nil {
						return false
					}
					resultElement := DebugStateElement{
						ID:   id,
						Data: make([]json.RawMessage, 0),
					}
					for _, c := range components {
						var data json.RawMessage
						data, eachClosureErr = wCtx.StoreReader().GetComponentForEntityInRawJSON(c, id)
						if eachClosureErr != nil {
							return false
						}
						resultElement.Data = append(resultElement.Data, data)
					}
					result = append(result, &resultElement)
					return true
				},
			)
			if eachClosureErr != nil {
				return nil, eachClosureErr
			}
			if searchEachErr != nil {
				return nil, searchEachErr
			}

			return &result, nil
		},
	)

	handler.app.Get("/debug/state", debugStateHandler)
}

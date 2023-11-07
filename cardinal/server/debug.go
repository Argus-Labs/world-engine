package server

import (
	"encoding/json"

	"github.com/go-openapi/runtime/middleware/untyped"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
)

type DebugStateElement struct {
	ID   entity.ID         `json:"id"`
	Data []json.RawMessage `json:"data"`
}

type DebugStateResponse = []*DebugStateElement

// register debug endpoints for swagger server.
func (handler *Handler) registerDebugHandlerSwagger(api *untyped.API) {
	// request name not required. This handler doesn't use anything in the request.
	debugStateHandler :=
		createSwaggerQueryHandler[interface{}, DebugStateResponse](
			"", func(i *interface{}) (*DebugStateResponse, error) {
				result := make(DebugStateResponse, 0)
				search := ecs.NewSearch(filter.All())
				wCtx := ecs.NewReadOnlyWorldContext(handler.w)

				err := search.Each(wCtx, func(id entity.ID) bool {
					components, err := handler.w.StoreManager().GetComponentTypesForEntity(id)
					if err != nil {
						return false
					}
					resultElement := DebugStateElement{
						ID:   id,
						Data: make([]json.RawMessage, 0),
					}
					for _, c := range components {
						data, err := ecs.GetRawJSONOfComponent(handler.w, c, id)
						if err != nil {
							return false
						}
						resultElement.Data = append(resultElement.Data, data)
					}
					result = append(result, &resultElement)
					return true
				})
				if err != nil {
					return nil, err
				}

				return &result, nil
			})

	api.RegisterOperation("GET", "/debug/state", debugStateHandler)
}

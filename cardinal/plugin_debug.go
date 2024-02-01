package cardinal

import (
	"github.com/goccy/go-json"
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

type debugPlugin struct {
}

func newDebugPlugin() *debugPlugin {
	return &debugPlugin{}
}

var _ Plugin = &debugPlugin{}

func (p *debugPlugin) Register(world *World) error {
	err := p.RegisterQueries(world)
	if err != nil {
		return err
	}
	return nil
}

func (p *debugPlugin) RegisterQueries(world *World) error {
	err := RegisterQuery[debugStateRequest, debugStateResponse](world, "state",
		queryDebugState,
		WithCustomQueryGroup[debugStateRequest, debugStateResponse]("debug"))
	if err != nil {
		return err
	}
	return nil
}

type debugStateRequest struct{}

type debugStateElement struct {
	ID         entity.ID         `json:"id"`
	Components []json.RawMessage `json:"components" swaggertype:"array,object"`
}

type debugStateResponse []*debugStateElement

// queryDebugState godoc
//
//	@Summary		Get information on all entities and components in world-engine
//	@Description	Displays the entire game state.
//	@Produce		application/json
//	@Success		200	{object}	debugStateResponse
//	@Router			/query/debug/state [post]
func queryDebugState(ctx engine.Context, _ *debugStateRequest) (*debugStateResponse, error) {
	result := make(debugStateResponse, 0)
	s := NewSearch(ctx, filter.All())
	var eachClosureErr error
	searchEachErr := s.Each(
		func(id entity.ID) bool {
			var components []component.ComponentMetadata
			components, eachClosureErr = ctx.StoreReader().GetComponentTypesForEntity(id)
			if eachClosureErr != nil {
				return false
			}
			resultElement := debugStateElement{
				ID:         id,
				Components: make([]json.RawMessage, 0),
			}
			for _, c := range components {
				var data json.RawMessage
				data, eachClosureErr = ctx.StoreReader().GetComponentForEntityInRawJSON(c, id)
				if eachClosureErr != nil {
					return false
				}
				resultElement.Components = append(resultElement.Components, data)
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
}

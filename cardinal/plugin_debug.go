package cardinal

import (
	"github.com/goccy/go-json"
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
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
	err := RegisterQuery[DebugStateRequest, DebugStateResponse](world, "state",
		queryDebugState,
		WithCustomQueryGroup[DebugStateRequest, DebugStateResponse]("debug"))
	if err != nil {
		return err
	}
	return nil
}

type DebugStateRequest struct{}

type debugStateElement struct {
	ID         types.EntityID             `json:"id"`
	Components map[string]json.RawMessage `json:"components" swaggertype:"object"`
}

type DebugStateResponse []*debugStateElement

// queryDebugState godoc
//
//	@Summary		Get information on all entities and components in world-engine
//	@Description	Displays the entire game state.
//	@Produce		application/json
//	@Success		200	{object}	DebugStateResponse
//	@Router			/query/debug/state [post]
func queryDebugState(ctx engine.Context, _ *DebugStateRequest) (*DebugStateResponse, error) {
	result := make(DebugStateResponse, 0)
	s := NewSearch(ctx, filter.All())
	var eachClosureErr error
	searchEachErr := s.Each(
		func(id types.EntityID) bool {
			var components []types.ComponentMetadata
			components, eachClosureErr = ctx.StoreReader().GetComponentTypesForEntity(id)
			if eachClosureErr != nil {
				return false
			}
			resultElement := debugStateElement{
				ID:         id,
				Components: make(map[string]json.RawMessage),
			}
			for _, c := range components {
				var data json.RawMessage
				data, eachClosureErr = ctx.StoreReader().GetComponentForEntityInRawJSON(c, id)
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
		return nil, eachClosureErr
	}
	if searchEachErr != nil {
		return nil, searchEachErr
	}

	return &result, nil
}

package cardinal

import (
	"github.com/goccy/go-json"
	"pkg.world.dev/world-engine/cardinal/cql"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

type cqlPlugin struct {
}

func newCQLPlugin() *cqlPlugin {
	return &cqlPlugin{}
}

var _ Plugin = &cqlPlugin{}

func (p *cqlPlugin) Register(world *World) error {
	err := p.RegisterQueries(world)
	if err != nil {
		return err
	}
	return nil
}

func (p *cqlPlugin) RegisterQueries(world *World) error {
	err := RegisterQuery[cqlQueryRequest, cqlQueryResponse](world, "cql", queryCQL)
	if err != nil {
		return err
	}
	return nil
}

type cqlQueryRequest struct {
	CQL string
}

type cqlData struct {
	ID   types.EntityID    `json:"id"`
	Data []json.RawMessage `json:"data" swaggertype:"object"`
}

type cqlQueryResponse struct {
	Results []cqlData `json:"results"`
}

// queryCQL godoc
// @Summary		Query the ecs with CQL (cardinal query language)
// @Description	Query the ecs with CQL (cardinal query language)
// @Accept			application/json
// @Produce		application/json
// @Param			cql	body		cqlQueryRequest	true	"cql (cardinal query language)"
// @Success		200	{object}	cqlQueryResponse
// @Router			/query/game/cql [post]
func queryCQL(ctx engine.Context, req *cqlQueryRequest) (*cqlQueryResponse, error) {
	cqlString := req.CQL

	// getComponentByName is a wrapper function that casts component.ComponentMetadata from ctx.GetComponentByName
	// to component.Component
	getComponentByName := func(name string) (types.Component, error) {
		comp, err := ctx.GetComponentByName(name)
		if err != nil {
			return nil, err
		}
		return comp, nil
	}

	resultFilter, err := cql.Parse(cqlString, getComponentByName)
	if err != nil {
		return nil, err
	}
	result := make([]cqlData, 0)
	var eachError error
	searchErr := NewSearch(ctx, resultFilter).Each(
		func(id types.EntityID) bool {
			components, err := ctx.StoreReader().GetComponentTypesForEntity(id)
			if err != nil {
				eachError = err
				return false
			}
			resultElement := cqlData{
				ID:   id,
				Data: make([]json.RawMessage, 0),
			}

			for _, c := range components {
				data, err := ctx.StoreReader().GetComponentForEntityInRawJSON(c, id)
				if err != nil {
					eachError = err
					return false
				}
				resultElement.Data = append(resultElement.Data, data)
			}
			result = append(result, resultElement)
			return true
		},
	)
	if searchErr != nil {
		return nil, err
	}
	if eachError != nil {
		return nil, err
	}
	return &cqlQueryResponse{Results: result}, nil
}

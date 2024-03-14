package handler

import "C"
import (
	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v2"
	"pkg.world.dev/world-engine/cardinal/server/handler/cql"
	servertypes "pkg.world.dev/world-engine/cardinal/server/types"
	"pkg.world.dev/world-engine/cardinal/types"
)

type CQLQueryRequest struct {
	CQL string
}

type cqlData struct {
	ID   types.EntityID    `json:"id"`
	Data []json.RawMessage `json:"data" swaggertype:"object"`
}

type CQLQueryResponse struct {
	Results []cqlData `json:"results"`
}

// PostCQL godoc
// @Summary		Query the ecs with CQL (cardinal query language)
// @Description	Query the ecs with CQL (cardinal query language)
// @Accept		application/json
// @Produce		application/json
// @Param		cql	body		CQLQueryRequest	true	"cql (cardinal query language)"
// @Success		200	{object}	CQLQueryResponse
// @Router		/cql [post]
func PostCQL(provider servertypes.Provider) func(*fiber.Ctx) error { //nolint:gocognit // to refactor later
	return func(ctx *fiber.Ctx) error {
		req := new(CQLQueryRequest)
		if err := ctx.BodyParser(req); err != nil {
			return err
		}

		// getComponentByName is a wrapper function that casts component.ComponentMetadata from ctx.GetComponentByName
		// to types.Component
		getComponentByName := func(name string) (types.Component, error) {
			comp, err := provider.GetComponentByName(name)
			if err != nil {
				return nil, err
			}
			return comp, nil
		}

		// Parse the CQL string into a filter
		resultFilter, err := cql.Parse(req.CQL, getComponentByName)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}

		result := make([]cqlData, 0)
		var eachError error
		searchErr := provider.Search(resultFilter).Each(
			func(id types.EntityID) bool {
				components, err := provider.StoreReader().GetComponentTypesForEntity(id)
				if err != nil {
					eachError = err
					return false
				}
				resultElement := cqlData{
					ID:   id,
					Data: make([]json.RawMessage, 0),
				}

				for _, c := range components {
					data, err := provider.StoreReader().GetComponentForEntityInRawJSON(c, id)
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
			return fiber.NewError(fiber.StatusInternalServerError, searchErr.Error())
		}
		if eachError != nil {
			return fiber.NewError(fiber.StatusInternalServerError, eachError.Error())
		}

		return ctx.JSON(CQLQueryResponse{Results: result})
	}
}

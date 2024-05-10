package handler

import "C"

import (
	"github.com/gofiber/fiber/v2"

	"pkg.world.dev/world-engine/cardinal/server/handler/cql"
	servertypes "pkg.world.dev/world-engine/cardinal/server/types"
	"pkg.world.dev/world-engine/cardinal/types"
)

type CQLQueryRequest struct {
	CQL string
}

type CQLQueryResponse struct {
	Results []types.CqlData `json:"results"`
}

// PostCQL godoc
//
//	@Summary      Executes a CQL (Cardinal Query Language) query
//	@Description  Executes a CQL (Cardinal Query Language) query
//	@Accept       application/json
//	@Produce      application/json
//	@Param        cql  body      CQLQueryRequest   true  "CQL query to be executed"
//	@Success      200  {object}  CQLQueryResponse  "Results of the executed CQL query"
//	@Failure      400  {string}  string            "Invalid request parameters"
//	@Router       /cql [post]
func PostCQL(
	provider servertypes.ProviderWorld) func(*fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		req := new(CQLQueryRequest)
		if err := ctx.BodyParser(req); err != nil {
			return err
		}

		// getComponentByName is a wrapper function that casts component.ComponentMetadata from ctx.getComponentByName
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
		result, eachError, searchErr := provider.RunCQLSearch(resultFilter)
		if searchErr != nil {
			return fiber.NewError(fiber.StatusInternalServerError, searchErr.Error())
		}
		if eachError != nil {
			return fiber.NewError(fiber.StatusInternalServerError, eachError.Error())
		}

		return ctx.JSON(CQLQueryResponse{Results: result})
	}
}

package handler

import (
	"github.com/gofiber/fiber/v2"

	servertypes "pkg.world.dev/world-engine/cardinal/server/types"
	"pkg.world.dev/world-engine/cardinal/types"
)

type CQLQueryRequest struct {
	CQL string
}

type CQLQueryResponse struct {
	Results []types.EntityStateElement `json:"results"`
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
	world servertypes.ProviderWorld,
) func(*fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		req := new(CQLQueryRequest)
		if err := ctx.BodyParser(req); err != nil {
			return err
		}
		result, err := world.EvaluateCQL(req.CQL)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return ctx.JSON(CQLQueryResponse{Results: result})
	}
}

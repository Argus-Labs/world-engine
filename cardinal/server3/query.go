package server3

import (
	"github.com/gofiber/fiber/v2"
	"pkg.world.dev/world-engine/cardinal/ecs"
)

func (s *Server) registerQueryHandler() error {
	queries := s.eng.ListQueries()
	queryNameToQuery := make(map[string]ecs.Query)
	for _, q := range queries {
		queryNameToQuery[q.Name()] = q
	}

	s.app.Post("/query/game/:{query_type}", func(ctx *fiber.Ctx) error {
		queryName := ctx.Route().Name
		query, exists := queryNameToQuery[queryName]
		if !exists {
			return fiber.NewError(fiber.StatusNotFound, "no handler registered for "+queryName)
		}
		resBz, err := query.HandleQueryRaw(ecs.NewReadOnlyEngineContext(s.eng), ctx.Body())
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "encountered an error in query: ", err.Error())
		}
		ctx.Set("Content-Type", "application/json")
		return ctx.Send(resBz)
	})
	return nil
}

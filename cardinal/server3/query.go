package server3

import (
	"fmt"
	"github.com/gofiber/fiber/v2"
	"pkg.world.dev/world-engine/cardinal/ecs"
)

func (s *Server) registerQueryHandler(path string) error {
	queries := s.eng.ListQueries()
	queryNameToQuery := make(map[string]ecs.Query)
	customPathQuery := make(map[string]ecs.Query)
	for _, q := range queries {
		if q.Path() == "" {
			queryNameToQuery[q.Name()] = q
		} else {
			customPathQuery[q.Path()] = q
		}
	}

	// all user generated queries. i.e. queries made from using cardinal.NewQueryType. Queries are retrieved by using
	// the wildcard matcher.
	s.app.Post(path, makeQueryHandler(queryNameToQuery, s.eng, func(ctx *fiber.Ctx) string {
		return ctx.Params(s.queryWildCard)
	}))

	// all custom path ECS queries. Queries are retrieved by using the full path.
	for _, q := range customPathQuery {
		qry := q
		s.app.Post(qry.Path(), makeQueryHandler(customPathQuery, s.eng, func(ctx *fiber.Ctx) string {
			return ctx.Route().Path
		}))
	}
	return nil
}

type queryRetriever func(ctx *fiber.Ctx) string

func makeQueryHandler(queryNameToQuery map[string]ecs.Query, eng *ecs.Engine, qr queryRetriever) func(ctx *fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		queryName := qr(ctx)
		query, exists := queryNameToQuery[queryName]
		if !exists {
			return fiber.NewError(fiber.StatusNotFound, "no handler registered for "+queryName)
		}
		resBz, err := query.HandleQueryRaw(ecs.NewReadOnlyEngineContext(eng), ctx.Body())
		if err != nil {
			fmt.Println(err.Error())
			return fiber.NewError(fiber.StatusBadRequest, "encountered an error in query: "+err.Error())
		}
		ctx.Set("Content-Type", "application/json")
		return ctx.Send(resBz)
	}
}

package server

import (
	"github.com/gofiber/fiber/v2"
	"pkg.world.dev/world-engine/cardinal/ecs"
)

func (s *Server) registerQueryHandler(path string) {
	queries := s.eng.ListQueries()

	// separate the queries based on whether they have custom handlers or not.
	queryNameToQuery := make(map[string]ecs.Query)
	customPathQuery := make(map[string]ecs.Query)
	for _, q := range queries {
		if q.Path() == "" {
			queryNameToQuery[q.Name()] = q
		} else {
			customPathQuery[q.Path()] = q
		}
	}

	// setup a post request to handle all queries under a single wildcard matcher.
	s.app.Post(path, makeQueryHandler(queryNameToQuery, s.eng, func(ctx *fiber.Ctx) string {
		return ctx.Params(s.queryWildCard)
	}))

	// setup a handler for each query with a custom path.
	for _, q := range customPathQuery {
		qry := q
		s.app.Post(qry.Path(), makeQueryHandler(customPathQuery, s.eng, func(ctx *fiber.Ctx) string {
			return ctx.Route().Path
		}))
	}
}

// defines how a handler retrieves the query name from fiber context.
type queryRetriever func(ctx *fiber.Ctx) string

func makeQueryHandler(
	queryNameToQuery map[string]ecs.Query,
	eng *ecs.Engine,
	qr queryRetriever,
) func(ctx *fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		queryName := qr(ctx)
		query, exists := queryNameToQuery[queryName]
		if !exists {
			return fiber.NewError(fiber.StatusNotFound, "no handler registered for "+queryName)
		}
		resBz, err := query.HandleQueryRaw(ecs.NewReadOnlyEngineContext(eng), ctx.Body())
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "encountered an error in query: "+err.Error())
		}
		ctx.Set("Content-Type", "application/json")
		return ctx.Send(resBz)
	}
}

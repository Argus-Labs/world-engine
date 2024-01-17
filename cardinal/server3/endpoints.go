package server3

import "github.com/gofiber/fiber/v2"

// EndpointsResult result struct for /query/http/endpoints.
type EndpointsResult struct {
	TxEndpoints    []string `json:"txEndpoints"`
	QueryEndpoints []string `json:"queryEndpoints"`
}

func (s *Server) registerListEndpointsEndpoint(path string) error {
	txs, err := s.eng.ListMessages()
	if err != nil {
		return err
	}
	qrys := s.eng.ListQueries()

	res := EndpointsResult{
		TxEndpoints:    make([]string, 0, len(qrys)),
		QueryEndpoints: make([]string, 0, len(txs)),
	}

	for _, tx := range txs {
		res.TxEndpoints = append(res.TxEndpoints, s.txPrefix+tx.Name())
	}
	for _, q := range qrys {
		res.QueryEndpoints = append(res.QueryEndpoints, s.queryPrefix+q.Name())
	}

	s.app.Post(path, func(ctx *fiber.Ctx) error {
		return ctx.JSON(res)
	})

	return nil
}

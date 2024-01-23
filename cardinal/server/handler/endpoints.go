package handler

import (
	"github.com/gofiber/fiber/v2"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/types/message"
)

type EndpointsResult struct {
	TxEndpoints    []string `json:"txEndpoints"`
	QueryEndpoints []string `json:"queryEndpoints"`
}

func GetEndpoints(msgs []message.Message, queries []ecs.Query, defaultPrefixTx, defaultQueryPrefix string,
) func(*fiber.Ctx) error {
	res := EndpointsResult{
		TxEndpoints:    make([]string, 0, len(queries)),
		QueryEndpoints: make([]string, 0, len(msgs)),
	}

	for _, msg := range msgs {
		if msg.Path() == "" {
			res.TxEndpoints = append(res.TxEndpoints, defaultPrefixTx+msg.Name())
		} else {
			res.TxEndpoints = append(res.TxEndpoints, msg.Path())
		}
	}

	for _, q := range queries {
		if q.Path() == "" {
			res.QueryEndpoints = append(res.QueryEndpoints, defaultQueryPrefix+q.Name())
		} else {
			res.QueryEndpoints = append(res.QueryEndpoints, q.Path())
		}
	}

	return func(ctx *fiber.Ctx) error {
		return ctx.JSON(res)
	}
}

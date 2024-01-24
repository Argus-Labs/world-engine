package utils

import (
	"github.com/gofiber/fiber/v2"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/types/message"
)

const (
	DefaultPort = "4040"
	TxPrefix    = "/tx/"
	QueryPrefix = "/query/"
)

func GetQueryFromRouteParams(ctx *fiber.Ctx, queries map[string]map[string]ecs.Query) (ecs.Query, bool) {
	query, ok := queries[ctx.Params("group")][ctx.Params("name")]
	return query, ok
}

func GetMesssageFromRouteParams(ctx *fiber.Ctx, msgs map[string]map[string]message.Message) (message.Message, bool) {
	msg, ok := msgs[ctx.Params("group")][ctx.Params("name")]
	return msg, ok
}

func GetQueryURL(group string, name string) string {
	return QueryPrefix + group + "/" + name
}

func GetTxURL(group string, name string) string {
	return TxPrefix + group + "/" + name
}

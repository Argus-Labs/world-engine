package handler

import (
	"reflect"

	"github.com/gofiber/fiber/v2"

	servertypes "pkg.world.dev/world-engine/cardinal/server/types"
	"pkg.world.dev/world-engine/cardinal/server/utils"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

type GetWorldResponse = engine.GetWorldResponse

// GetWorld godoc
//
//	@Summary      Retrieves details of the game world
//	@Description  Contains the registered components, messages, queries, and namespace
//	@Accept       application/json
//	@Produce      application/json
//	@Success      200  {object}  GetWorldResponse  "Details of the game world"
//	@Failure      400  {string}  string            "Invalid request parameters"
//	@Router       /world [get]
func GetWorld(
	world servertypes.ProviderWorld,
	components []types.ComponentMetadata,
	messages []types.Message,
	namespace string,
) func(*fiber.Ctx) error {
	// Collecting name of all registered components
	comps := make([]engine.FieldDetail, 0, len(components))
	for _, component := range components {
		c, _ := component.Decode(component.GetSchema())
		comps = append(comps, engine.FieldDetail{
			Name:   component.Name(),
			Fields: types.GetFieldInformation(reflect.TypeOf(c)),
		})
	}

	// Collecting the structure of all messages
	messagesFields := make([]engine.FieldDetail, 0, len(messages))
	for _, message := range messages {
		// Extracting the fields of the message
		messagesFields = append(messagesFields, engine.FieldDetail{
			Name:   message.Name(),
			Fields: message.GetInFieldInformation(),
			URL:    utils.GetTxURL(message.Group(), message.Name()),
		})
	}

	// Collecting the structure of all queries

	return func(ctx *fiber.Ctx) error {
		return ctx.JSON(GetWorldResponse{
			Namespace:  namespace,
			Components: comps,
			Messages:   messagesFields,
			Queries:    world.BuildQueryFields(),
		})
	}
}

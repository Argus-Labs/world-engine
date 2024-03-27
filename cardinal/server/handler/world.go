package handler

import (
	"reflect"

	"github.com/gofiber/fiber/v2"

	"pkg.world.dev/world-engine/cardinal/server/utils"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

type GetWorldResponse struct {
	Namespace  string        `json:"namespace"`
	Components []FieldDetail `json:"components"` // list of component names
	Messages   []FieldDetail `json:"messages"`
	Queries    []FieldDetail `json:"queries"`
}

type FieldDetail struct {
	Name   string         `json:"name"`   // name of the message or query
	Fields map[string]any `json:"fields"` // variable name and type
	URL    string         `json:"url,omitempty"`
}

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
	components []types.ComponentMetadata, messages []types.Message,
	queries []engine.Query, namespace string,
) func(*fiber.Ctx) error {
	// Collecting name of all registered components
	comps := make([]FieldDetail, 0, len(components))
	for _, component := range components {
		c, _ := component.Decode(component.GetSchema())
		comps = append(comps, FieldDetail{
			Name:   component.Name(),
			Fields: types.GetFieldInformation(reflect.TypeOf(c)),
		})
	}

	// Collecting the structure of all messages
	messagesFields := make([]FieldDetail, 0, len(messages))
	for _, message := range messages {
		// Extracting the fields of the message
		messagesFields = append(messagesFields, FieldDetail{
			Name:   message.Name(),
			Fields: message.GetInFieldInformation(),
			URL:    utils.GetTxURL(message.Group(), message.Name()),
		})
	}

	// Collecting the structure of all queries
	queriesFields := make([]FieldDetail, 0, len(queries))
	for _, query := range queries {
		// Extracting the fields of the query
		queriesFields = append(queriesFields, FieldDetail{
			Name:   query.Name(),
			Fields: query.GetRequestFieldInformation(),
			URL:    utils.GetQueryURL(query.Group(), query.Name()),
		})
	}

	return func(ctx *fiber.Ctx) error {
		return ctx.JSON(GetWorldResponse{
			Namespace:  namespace,
			Components: comps,
			Messages:   messagesFields,
			Queries:    queriesFields,
		})
	}
}

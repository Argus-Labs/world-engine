package handler

import (
	"github.com/gofiber/fiber/v2"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

type GetDebugWorldResponse struct {
	Components []string      `json:"components"` // list of component names
	Messages   []FieldDetail `json:"messages"`
	Queries    []FieldDetail `json:"queries"`
}

type FieldDetail struct {
	Name   string         `json:"name"`   // name of the message or query
	Fields map[string]any `json:"fields"` // variable name and type
}

// GetDebugWorld godoc
//
//	@Summary		Get field information of registered components, messages, queries
//	@Description	Get field information of registered components, messages, queries
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200	{object}	GetFieldsResponse	"Field information of registered components, messages, queries"
//	@Failure		400	{string}	string				""
//	@Router			/debug/world [get]
func GetDebugWorld(components []types.ComponentMetadata, messages []types.Message,
	queries []engine.Query) func(*fiber.Ctx) error {
	// Collecting name of all registered components
	comps := make([]string, 0, len(components))
	for _, component := range components {
		comps = append(comps, component.Name())
	}

	// Collecting the structure of all messages
	messagesFields := make([]FieldDetail, 0, len(messages))
	for _, message := range messages {
		// Extracting the fields of the message
		messagesFields = append(messagesFields, FieldDetail{
			Name:   message.Name(),
			Fields: message.GetInFieldInformation(),
		})
	}

	// Collecting the structure of all queries
	queriesFields := make([]FieldDetail, 0, len(queries))
	for _, query := range queries {
		// Extracting the fields of the query
		queriesFields = append(queriesFields, FieldDetail{
			Name:   query.Name(),
			Fields: query.GetRequestFieldInformation(),
		})
	}

	return func(ctx *fiber.Ctx) error {
		return ctx.JSON(GetDebugWorldResponse{
			Components: comps,
			Messages:   messagesFields,
			Queries:    queriesFields,
		})
	}
}

package cardinal

import (
	"context"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/validate"
	"github.com/goccy/go-json"
	"github.com/invopop/jsonschema"
	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
)

// TODO: add tick log here.
type debugModule struct {
	world      *World
	server     *http.Server
	reflector  *jsonschema.Reflector
	commands   map[string]string // JSON schema strings
	events     map[string]string
	components map[string]string
}

func newDebugModule(world *World) debugModule {
	return debugModule{
		world:      world,
		commands:   make(map[string]string),
		events:     make(map[string]string),
		components: make(map[string]string),
		reflector: &jsonschema.Reflector{
			Anonymous:      true, // Don't add $id based on package path
			ExpandedStruct: true, // Inline the struct fields directly
		},
	}
}

func (d *debugModule) register(kind string, value schema.Serializable) error {
	if d == nil {
		return nil
	}

	var catalog map[string]string
	switch kind {
	case "command":
		catalog = d.commands
	case "event":
		catalog = d.events
	case "component":
		catalog = d.components
	default:
		panic("this is an internal function, this should never panic")
	}

	name := value.Name()
	if _, exists := catalog[name]; exists {
		return nil
	}

	jsonSchema := d.reflector.Reflect(value)
	data, err := json.Marshal(jsonSchema)
	if err != nil {
		return eris.Wrap(err, "failed to marshal json schema")
	}

	var schemaMap map[string]any
	if err := json.Unmarshal(data, &schemaMap); err != nil {
		return eris.Wrap(err, "failed to unmarshal json schema")
	}

	// Remove redundant fields.
	delete(schemaMap, "$schema")
	delete(schemaMap, "type")
	delete(schemaMap, "additionalProperties")

	// Re-marshal to get clean JSON string.
	cleanData, err := json.Marshal(schemaMap)
	if err != nil {
		return eris.Wrap(err, "failed to marshal cleaned schema")
	}

	catalog[name] = string(cleanData)
	return nil
}

// Init initializes and starts the connect server for the debug service.
func (d *debugModule) Init(addr string) {
	if d == nil {
		return
	}

	logger := d.world.tel.GetLogger("debug")

	mux := http.NewServeMux()
	mux.Handle(cardinalv1connect.NewDebugServiceHandler(d, connect.WithInterceptors(validate.NewInterceptor())))

	d.server = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Info().Str("addr", addr).Msg("Debug service initialized")

	go func() {
		_ = d.server.ListenAndServe()
	}()
}

// Shutdown gracefully shuts down the debug server.
func (d *debugModule) Shutdown(ctx context.Context) error {
	if d == nil || d.server == nil {
		return nil
	}
	return d.server.Shutdown(ctx)
}

var _ cardinalv1connect.DebugServiceHandler = (*debugModule)(nil)

// Introspect returns metadata about the registered types in the world.
func (d *debugModule) Introspect(
	_ context.Context,
	_ *connect.Request[cardinalv1.IntrospectRequest],
) (*connect.Response[cardinalv1.IntrospectResponse], error) {
	return connect.NewResponse(&cardinalv1.IntrospectResponse{
		Commands:   d.buildTypeSchemas(d.commands),
		Components: d.buildTypeSchemas(d.components),
		Events:     d.buildTypeSchemas(d.events),
	}), nil
}

// buildTypeSchemas converts the internal schema cache to proto TypeSchema messages.
func (d *debugModule) buildTypeSchemas(cache map[string]string) []*cardinalv1.TypeSchema {
	schemas := make([]*cardinalv1.TypeSchema, 0, len(cache))
	for name, schemaJSON := range cache {
		schemas = append(schemas, &cardinalv1.TypeSchema{
			Name:       name,
			SchemaJson: schemaJSON,
		})
	}
	return schemas
}

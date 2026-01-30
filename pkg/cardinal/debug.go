package cardinal

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/validate"
	"github.com/goccy/go-json"
	"github.com/invopop/jsonschema"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
)

type debugModule struct {
	world      *World
	server     *http.Server
	reflector  *jsonschema.Reflector
	commands   map[string]*structpb.Struct
	events     map[string]*structpb.Struct
	components map[string]*structpb.Struct
}

func newDebugModule(world *World) debugModule {
	return debugModule{
		world:      world,
		commands:   make(map[string]*structpb.Struct),
		events:     make(map[string]*structpb.Struct),
		components: make(map[string]*structpb.Struct),
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

	var catalog map[string]*structpb.Struct
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

	schemaStruct, err := structpb.NewStruct(schemaMap)
	if err != nil {
		return eris.Wrap(err, "failed to create struct from schema")
	}

	catalog[name] = schemaStruct
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
		Addr:    addr,
		Handler: mux,
	}

	logger.Info().Str("addr", addr).Msg("Debug service initialized")

	go d.server.ListenAndServe()
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
func (d *debugModule) buildTypeSchemas(cache map[string]*structpb.Struct) []*cardinalv1.TypeSchema {
	schemas := make([]*cardinalv1.TypeSchema, 0, len(cache))
	for name, schemaStruct := range cache {
		schemas = append(schemas, &cardinalv1.TypeSchema{
			Name:   name,
			Schema: schemaStruct,
		})
	}
	return schemas
}

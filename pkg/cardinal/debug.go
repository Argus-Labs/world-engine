package cardinal

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/goccy/go-json"
	"github.com/invopop/jsonschema"
	"github.com/rotisserie/eris"
)

type debugModule struct {
	world      *World
	reflector  *jsonschema.Reflector
	commands   map[string][]byte
	events     map[string][]byte
	components map[string][]byte
}

func newDebugModule(world *World) debugModule {
	return debugModule{
		world:      world,
		commands:   make(map[string][]byte),
		events:     make(map[string][]byte),
		components: make(map[string][]byte),
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

	var catalog map[string][]byte
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

	schema := d.reflector.Reflect(value)
	data, err := json.Marshal(schema)
	if err != nil {
		return eris.Wrap(err, "failed to marshal json schema")
	}

	if err != nil {
		return eris.Wrap(err, "failed to serialize type for debug module")
	}

	catalog[name] = data
	return nil
}

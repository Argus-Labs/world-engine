package introspect

import (
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/goccy/go-json"
	"github.com/invopop/jsonschema"
	"github.com/rotisserie/eris"
	orderedmap "github.com/wk8/go-ordered-map/v2"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/argus-labs/world-engine/pkg/immutable"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
)

// Kind identifies a category of registered world type.
type Kind uint8

const (
	// Command identifies command types.
	Command Kind = iota
	// Component identifies component types.
	Component
	// Event identifies event types.
	Event
)

// Catalog freezes registered payload metadata before the debug service starts.
type Catalog struct {
	finalized  bool
	registered [3]map[string]schema.Serializable
	types      [3][]*cardinalv1.TypeSchema
}

// NewCatalog creates an empty catalog.
func NewCatalog() *Catalog {
	c := &Catalog{}
	for i := range c.registered {
		c.registered[i] = make(map[string]schema.Serializable)
	}
	return c
}

// Register adds a type before the catalog is finalized.
func (c *Catalog) Register(kind Kind, value schema.Serializable) error {
	if c.finalized {
		return eris.New("introspection catalog is finalized")
	}
	if kind > Event {
		return eris.Errorf("unknown introspection kind: %d", kind)
	}
	name := value.Name()
	if _, exists := c.registered[kind][name]; !exists {
		c.registered[kind][name] = value
	}
	return nil
}

// Finalize builds metadata atomically and prevents further registration.
func (c *Catalog) Finalize() error {
	if c.finalized {
		return nil
	}
	var types [3][]*cardinalv1.TypeSchema
	for kind, registered := range c.registered {
		names := make([]string, 0, len(registered))
		for name := range registered {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			s, err := reflectSchema(registered[name])
			if err != nil {
				return eris.Wrap(err, "failed to inspect "+name)
			}
			types[kind] = append(types[kind], &cardinalv1.TypeSchema{Name: name, JsonSchema: s})
		}
	}
	c.types = types
	c.finalized = true
	return nil
}

// Commands returns the finalized command metadata.
func (c *Catalog) Commands() []*cardinalv1.TypeSchema { return c.types[Command] }

// Components returns the finalized component metadata.
func (c *Catalog) Components() []*cardinalv1.TypeSchema { return c.types[Component] }

// Events returns the finalized event metadata.
func (c *Catalog) Events() []*cardinalv1.TypeSchema { return c.types[Event] }

// reflectSchema describes ordinary MessagePack payloads, not arbitrary custom codecs.
// Definitions are local to each payload; references must resolve against that payload's root.
func reflectSchema(value any) (*structpb.Struct, error) {
	b := schemaBuilder{definitions: make(jsonschema.Definitions), seen: make(map[reflect.Type]string)}
	s := b.reflectType(reflect.TypeOf(value))
	if b.err != nil {
		return nil, eris.Wrap(b.err, "failed to reflect MessagePack schema")
	}
	s.Definitions = b.definitions
	data, err := json.Marshal(s)
	if err != nil {
		return nil, eris.Wrap(err, "failed to marshal JSON schema")
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, eris.Wrap(err, "failed to decode JSON schema")
	}
	out, err := structpb.NewStruct(fields)
	if err != nil {
		return nil, eris.Wrap(err, "failed to create JSON schema struct")
	}
	return out, nil
}

type schemaBuilder struct {
	definitions jsonschema.Definitions
	seen        map[reflect.Type]string
	err         error
}

const (
	arrayType   = "array"
	integerType = "integer"
	stringType  = "string"
)

func (b *schemaBuilder) reflectType(t reflect.Type) *jsonschema.Schema {
	if t == nil {
		return &jsonschema.Schema{}
	}
	if t.Kind() == reflect.Pointer {
		return nullable(b.reflectType(t.Elem()))
	}
	if elem, ok := immutable.SliceElem(t); ok {
		return &jsonschema.Schema{Type: arrayType, Items: b.reflectType(elem)}
	}
	switch t.Kind() { //nolint:exhaustive // Unsupported payload kinds produce an error below.
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &jsonschema.Schema{Type: integerType, Format: "int" + strconv.Itoa(t.Bits())}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &jsonschema.Schema{Type: integerType, Format: "uint" + strconv.Itoa(t.Bits())}
	case reflect.Float32, reflect.Float64:
		return &jsonschema.Schema{Type: "number"}
	case reflect.Bool:
		return &jsonschema.Schema{Type: "boolean"}
	case reflect.String:
		return &jsonschema.Schema{Type: stringType}
	case reflect.Interface:
		return &jsonschema.Schema{}
	case reflect.Struct:
		if name, ok := b.seen[t]; ok {
			return &jsonschema.Schema{Ref: "#/$defs/" + name}
		}
		name := "type" + strconv.Itoa(len(b.seen))
		b.seen[t] = name
		s := &jsonschema.Schema{
			Type: "object", Properties: orderedmap.New[string, *jsonschema.Schema](),
			AdditionalProperties: jsonschema.FalseSchema,
		}
		b.definitions[name] = s
		for _, f := range wireFields(t) {
			s.Properties.Set(f.name, b.reflectType(f.typ))
		}
		return &jsonschema.Schema{Ref: "#/$defs/" + name}
	case reflect.Array:
		// The domain codec encodes byte arrays as MessagePack binary, just like []byte.
		if t.Elem().Kind() == reflect.Uint8 {
			return &jsonschema.Schema{Type: stringType, ContentEncoding: "base64"}
		}
		n := uint64(t.Len()) //nolint:gosec // Array lengths cannot be negative.
		return &jsonschema.Schema{Type: arrayType, Items: b.reflectType(t.Elem()), MinItems: &n, MaxItems: &n}
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return nullable(&jsonschema.Schema{Type: stringType, ContentEncoding: "base64"})
		}
		return nullable(&jsonschema.Schema{Type: arrayType, Items: b.reflectType(t.Elem())})
	case reflect.Map:
		s := &jsonschema.Schema{Type: "object", AdditionalProperties: b.reflectType(t.Elem())}
		if t.Key().Kind() != reflect.String {
			key := b.reflectType(t.Key())
			if key.Type != integerType {
				b.err = eris.Errorf("unsupported MessagePack map key %s", t.Key())
			}
			s.Extras = map[string]any{"x-msgpack-key-format": key.Format}
		}
		return nullable(s)
	default:
		b.err = eris.Errorf("unsupported MessagePack schema type %s", t)
		return &jsonschema.Schema{}
	}
}

func nullable(s *jsonschema.Schema) *jsonschema.Schema {
	return &jsonschema.Schema{AnyOf: []*jsonschema.Schema{s, {Type: "null"}}}
}

type wireField struct {
	name string
	typ  reflect.Type
}

// Match vmihailenco's declaration-order embedding: renamed anonymous structs still
// auto-inline unless noinline is set or an earlier field conflicts. Explicit inline
// skips conflicting fields. JSON tags do not participate in MessagePack naming.
func wireFields(t reflect.Type) []wireField {
	var fields []wireField
	seen := make(map[string]bool)
	for f := range t.Fields() {
		tags := strings.Split(f.Tag.Get("msgpack"), ",")
		if tags[0] == "-" || (f.PkgPath != "" && !f.Anonymous) {
			continue
		}
		name := tags[0]
		if name == "" {
			name = f.Name
		}
		ft := f.Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		_, immutableSlice := immutable.SliceElem(ft)
		if f.Anonymous && ft.Kind() == reflect.Struct && !immutableSlice && !slices.Contains(tags[1:], "noinline") {
			nested := wireFields(ft)
			if canInline(nested, seen, slices.Contains(tags[1:], "inline")) {
				fields = appendUnshadowed(fields, nested, seen)
				continue
			}
		}
		fields = append(fields, wireField{name: name, typ: f.Type})
		seen[name] = true
	}
	return fields
}

func appendUnshadowed(fields, nested []wireField, seen map[string]bool) []wireField {
	for _, child := range nested {
		if !seen[child.name] {
			fields = append(fields, child)
			seen[child.name] = true
		}
	}
	return fields
}

func canInline(fields []wireField, seen map[string]bool, explicit bool) bool {
	for _, field := range fields {
		if seen[field.name] && !explicit {
			return false
		}
	}
	return true
}

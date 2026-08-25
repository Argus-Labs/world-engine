package introspect

import (
	"cmp"
	"reflect"
	"slices"
	"strings"

	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
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

// Catalog collects registered types before the debug service starts. Finalize freezes
// registration and builds the immutable protobuf metadata used by Introspect.
type Catalog struct {
	finalized bool

	// The three slots are commands, components, and events, indexed by Kind.
	registered [3]map[string]schema.Serializable
	// Finalized API metadata using the same Kind indexes.
	types [3][]*cardinalv1.TypeSchema
	// Shared serialized protobuf definitions returned by Introspect.
	descriptors []byte
}

// NewCatalog creates an empty catalog.
func NewCatalog() *Catalog {
	return &Catalog{
		registered: [3]map[string]schema.Serializable{
			make(map[string]schema.Serializable),
			make(map[string]schema.Serializable),
			make(map[string]schema.Serializable),
		},
	}
}

// Register adds a type before the catalog is finalized.
func (c *Catalog) Register(kind Kind, value schema.Serializable) error {
	if c.finalized {
		return eris.New("introspection catalog is finalized")
	}
	if kind > Event {
		return eris.Errorf("unknown introspection kind: %d", kind)
	}

	catalog := c.registered[kind]
	name := value.Name()
	if _, exists := catalog[name]; exists {
		return nil
	}

	catalog[name] = value
	return nil
}

// protoDescriptor returns the generated protobuf metadata for one registered type.
func protoDescriptor(value schema.Serializable) (protoreflect.MessageDescriptor, error) {
	describer, ok := value.(schema.ProtoDescriber)
	if !ok {
		return nil, eris.New("protobuf metadata is missing; regenerate wire code with the latest World CLI")
	}
	descriptor := describer.ProtoDescriptor()
	if descriptor == nil {
		return nil, eris.New("generated protobuf descriptor is nil")
	}
	if !hasCompleteDescriptor(descriptor.ParentFile()) {
		return nil, eris.Errorf("protobuf descriptor %q has unresolved imports", descriptor.ParentFile().Path())
	}
	return descriptor, nil
}

// hasCompleteDescriptor checks that a protobuf file and all its imports are available.
func hasCompleteDescriptor(file protoreflect.FileDescriptor) bool {
	imports := file.Imports()
	for i := range imports.Len() {
		dependency := imports.Get(i)
		if dependency.IsPlaceholder() || !hasCompleteDescriptor(dependency.FileDescriptor) {
			return false
		}
	}
	return true
}

// Finalize builds the metadata and prevents further registration.
func (c *Catalog) Finalize() error {
	if c.finalized {
		return nil
	}

	// Build everything first so a failure leaves the catalog unchanged and retryable.
	var types [3][]*cardinalv1.TypeSchema
	descriptors := make([]protoreflect.MessageDescriptor, 0)
	issues := make([]string, 0)
	for kind, registered := range c.registered {
		inspected := make(map[string]protoreflect.MessageDescriptor, len(registered))
		for name, value := range registered {
			descriptor, err := protoDescriptor(value)
			if err != nil {
				issues = append(issues, eris.Wrapf(err, "failed to inspect %s", name).Error())
				continue
			}
			inspected[name] = descriptor
			descriptors = append(descriptors, descriptor)
		}
		types[kind] = buildTypeSchemas(inspected, registered)
	}
	if len(issues) > 0 {
		slices.Sort(issues)
		return eris.Errorf("invalid protobuf metadata:\n  %s", strings.Join(issues, "\n  "))
	}

	descriptorSet, err := buildDescriptorSet(descriptors)
	if err != nil {
		return eris.Wrap(err, "failed to build introspection descriptor set")
	}
	c.types = types
	c.descriptors = descriptorSet
	c.finalized = true
	return nil
}

// Commands returns the registered command metadata.
func (c *Catalog) Commands() []*cardinalv1.TypeSchema {
	return c.schemas(Command)
}

// Components returns the registered component metadata.
func (c *Catalog) Components() []*cardinalv1.TypeSchema {
	return c.schemas(Component)
}

// Events returns the registered event metadata.
func (c *Catalog) Events() []*cardinalv1.TypeSchema {
	return c.schemas(Event)
}

func (c *Catalog) schemas(kind Kind) []*cardinalv1.TypeSchema {
	return c.types[kind]
}

// DescriptorSet returns the shared protobuf descriptor set.
func (c *Catalog) DescriptorSet() []byte {
	return c.descriptors
}

// buildTypeSchemas creates the sorted command, component, or event list returned by Introspect.
func buildTypeSchemas(
	inspected map[string]protoreflect.MessageDescriptor,
	values map[string]schema.Serializable,
) []*cardinalv1.TypeSchema {
	names := make([]string, 0, len(inspected))
	for name := range inspected {
		names = append(names, name)
	}
	slices.Sort(names)

	types := make([]*cardinalv1.TypeSchema, 0, len(names))
	for _, name := range names {
		types = append(types, &cardinalv1.TypeSchema{
			Name:             name,
			ProtoMessageName: string(inspected[name].FullName()),
			ArrayFields:      arrayFields(values[name]),
		})
	}
	return types
}

// arrayFields reports the shape of a type's multi-dimensional fixed-size array fields.
//
// A fixed array travels as one flat repeated field, which says everything in one dimension and not
// enough beyond it: 32 elements could be 4x8 or 8x4. One-dimensional arrays are therefore skipped —
// there is nothing to rebuild — and so is everything else, since only a fixed array has a shape that
// is known ahead of time rather than carried with the data.
//
// This is metadata for clients decoding types they do not know in advance. A client written against
// a known schema indexes the flat field directly and never needs it.
func arrayFields(value schema.Serializable) []*cardinalv1.ArrayField {
	if value == nil {
		return nil
	}
	t := reflect.TypeOf(value)
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}

	var out []*cardinalv1.ArrayField
	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue // never serialized, so never reconstructed
		}
		var dims []uint32
		for ft := field.Type; ft.Kind() == reflect.Array; ft = ft.Elem() {
			dims = append(dims, uint32(ft.Len())) //nolint:gosec // an array length is never negative
		}
		if len(dims) < 2 {
			continue // a flat repeated field is already unambiguous
		}
		out = append(out, &cardinalv1.ArrayField{Field: field.Name, Dims: dims})
	}
	return out
}

// buildDescriptorSet packages every referenced protobuf file and its imports into one validated bundle.
func buildDescriptorSet(messages []protoreflect.MessageDescriptor) ([]byte, error) {
	if len(messages) == 0 {
		return nil, nil
	}

	// Stable ordering makes the same catalog produce the same bytes.
	sortedMessages := slices.Clone(messages)
	slices.SortFunc(sortedMessages, func(a, b protoreflect.MessageDescriptor) int {
		return cmp.Compare(a.FullName(), b.FullName())
	})

	seen := make(map[string]bool)
	files := make([]*descriptorpb.FileDescriptorProto, 0, len(sortedMessages))
	var addFile func(protoreflect.FileDescriptor)
	addFile = func(file protoreflect.FileDescriptor) {
		// Add imports first and include each file only once.
		if seen[file.Path()] {
			return
		}
		seen[file.Path()] = true

		imports := file.Imports()
		for i := range imports.Len() {
			addFile(imports.Get(i).FileDescriptor)
		}
		files = append(files, protodesc.ToFileDescriptorProto(file))
	}

	for _, message := range sortedMessages {
		addFile(message.ParentFile())
	}

	set := &descriptorpb.FileDescriptorSet{File: files}
	if _, err := protodesc.NewFiles(set); err != nil {
		return nil, eris.Wrap(err, "invalid protobuf descriptor set")
	}
	return proto.MarshalOptions{Deterministic: true}.Marshal(set)
}

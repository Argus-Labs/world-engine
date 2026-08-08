package cardinal

import (
	"cmp"
	"slices"
	"sync"

	"github.com/goccy/go-json"
	"github.com/invopop/jsonschema"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
)

type introspectionKind uint8

const (
	introspectionCommand introspectionKind = iota
	introspectionComponent
	introspectionEvent
)

type introspectedType struct {
	schema     *structpb.Struct
	descriptor protoreflect.MessageDescriptor
}

// introspectionCatalog collects generated form and protobuf metadata before the debug service starts.
// Finalize freezes registration and builds the immutable response data used by Introspect.
type introspectionCatalog struct {
	mu          sync.RWMutex
	reflector   *jsonschema.Reflector
	finalized   bool
	registered  [3]map[string]introspectedType
	types       [3][]*cardinalv1.TypeSchema
	descriptors []byte
}

func newIntrospectionCatalog() *introspectionCatalog {
	return &introspectionCatalog{
		reflector: &jsonschema.Reflector{
			Anonymous:      true,
			ExpandedStruct: true,
		},
		registered: [3]map[string]introspectedType{
			make(map[string]introspectedType),
			make(map[string]introspectedType),
			make(map[string]introspectedType),
		},
	}
}

func (c *introspectionCatalog) Register(kind introspectionKind, value schema.Serializable) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.finalized {
		return eris.New("introspection catalog is finalized")
	}
	if kind > introspectionEvent {
		return eris.Errorf("unknown introspection kind: %d", kind)
	}

	catalog := c.registered[kind]
	name := value.Name()
	if _, exists := catalog[name]; exists {
		return nil
	}

	entry, err := c.inspect(value)
	if err != nil {
		return eris.Wrapf(err, "failed to inspect %s", name)
	}
	catalog[name] = entry
	return nil
}

func (c *introspectionCatalog) inspect(value schema.Serializable) (introspectedType, error) {
	var descriptor protoreflect.MessageDescriptor
	var data []byte
	if introspectable, ok := value.(schema.Introspectable); ok {
		descriptor = introspectable.ProtoDescriptor()
		if descriptor == nil {
			return introspectedType{}, eris.New("generated protobuf descriptor is nil")
		}
		data = introspectable.FormSchema()
	} else {
		jsonSchema := c.reflector.Reflect(value)
		var err error
		data, err = json.Marshal(jsonSchema)
		if err != nil {
			return introspectedType{}, eris.Wrap(err, "failed to marshal json schema")
		}
	}

	var schemaMap map[string]any
	if err := json.Unmarshal(data, &schemaMap); err != nil {
		return introspectedType{}, eris.Wrap(err, "failed to unmarshal json schema")
	}
	delete(schemaMap, "$schema")
	delete(schemaMap, "type")
	delete(schemaMap, "additionalProperties")

	schemaStruct, err := structpb.NewStruct(schemaMap)
	if err != nil {
		return introspectedType{}, eris.Wrap(err, "failed to create struct from schema")
	}
	return introspectedType{schema: schemaStruct, descriptor: descriptor}, nil
}

func (c *introspectionCatalog) Finalize() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.finalized {
		return nil
	}

	descriptors := make([]protoreflect.MessageDescriptor, 0)
	for kind, registered := range c.registered {
		c.types[kind] = buildTypeSchemas(registered)
		for _, entry := range registered {
			if entry.descriptor != nil {
				descriptors = append(descriptors, entry.descriptor)
			}
		}
	}

	descriptorSet, err := buildDescriptorSet(descriptors)
	if err != nil {
		return eris.Wrap(err, "failed to build introspection descriptor set")
	}
	c.descriptors = descriptorSet
	c.finalized = true
	return nil
}

func (c *introspectionCatalog) Finalized() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.finalized
}

func (c *introspectionCatalog) Commands() []*cardinalv1.TypeSchema {
	return c.schemas(introspectionCommand)
}

func (c *introspectionCatalog) Components() []*cardinalv1.TypeSchema {
	return c.schemas(introspectionComponent)
}

func (c *introspectionCatalog) Events() []*cardinalv1.TypeSchema {
	return c.schemas(introspectionEvent)
}

func (c *introspectionCatalog) schemas(kind introspectionKind) []*cardinalv1.TypeSchema {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return slices.Clone(c.types[kind])
}

func (c *introspectionCatalog) DescriptorSet() []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return slices.Clone(c.descriptors)
}

func buildTypeSchemas(registered map[string]introspectedType) []*cardinalv1.TypeSchema {
	names := make([]string, 0, len(registered))
	for name := range registered {
		names = append(names, name)
	}
	slices.Sort(names)

	types := make([]*cardinalv1.TypeSchema, 0, len(names))
	for _, name := range names {
		entry := registered[name]
		typeSchema := &cardinalv1.TypeSchema{Name: name, Schema: entry.schema}
		if entry.descriptor != nil {
			typeSchema.ProtoMessageName = string(entry.descriptor.FullName())
		}
		types = append(types, typeSchema)
	}
	return types
}

func buildDescriptorSet(messages []protoreflect.MessageDescriptor) ([]byte, error) {
	if len(messages) == 0 {
		return nil, nil
	}

	sortedMessages := slices.Clone(messages)
	slices.SortFunc(sortedMessages, func(a, b protoreflect.MessageDescriptor) int {
		return cmp.Compare(a.FullName(), b.FullName())
	})

	seen := make(map[string]bool)
	files := make([]*descriptorpb.FileDescriptorProto, 0, len(sortedMessages))
	var addFile func(protoreflect.FileDescriptor)
	addFile = func(file protoreflect.FileDescriptor) {
		if seen[file.Path()] {
			return
		}
		seen[file.Path()] = true

		imports := file.Imports()
		for i := range imports.Len() {
			dependency := imports.Get(i)
			if !dependency.IsPlaceholder() {
				addFile(dependency.FileDescriptor)
			}
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

package cardinal

import (
	"cmp"
	"slices"
	"sync"

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

// introspectionCatalog collects registered types before the debug service starts. Finalize freezes
// registration and builds the immutable form and protobuf metadata used by Introspect.
type introspectionCatalog struct {
	mu          sync.RWMutex
	finalized   bool
	registered  [3]map[string]schema.Serializable
	types       [3][]*cardinalv1.TypeSchema
	descriptors []byte
}

func newIntrospectionCatalog() *introspectionCatalog {
	return &introspectionCatalog{
		registered: [3]map[string]schema.Serializable{
			make(map[string]schema.Serializable),
			make(map[string]schema.Serializable),
			make(map[string]schema.Serializable),
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

	catalog[name] = value
	return nil
}

func (c *introspectionCatalog) inspect(value schema.Serializable) (introspectedType, error) {
	var descriptor protoreflect.MessageDescriptor
	if describer, ok := value.(schema.ProtoDescriber); ok {
		descriptor = describer.ProtoDescriptor()
		if descriptor == nil {
			return introspectedType{}, eris.New("generated protobuf descriptor is nil")
		}
		if err := validateDescriptorFields(value, descriptor); err != nil {
			return introspectedType{}, err
		}
	}

	schemaMap := buildFormSchema(value)
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

	var types [3][]*cardinalv1.TypeSchema
	descriptors := make([]protoreflect.MessageDescriptor, 0)
	for kind, registered := range c.registered {
		inspected := make(map[string]introspectedType, len(registered))
		for name, value := range registered {
			entry, err := c.inspect(value)
			if err != nil {
				return eris.Wrapf(err, "failed to inspect %s", name)
			}
			inspected[name] = entry
			if entry.descriptor != nil {
				descriptors = append(descriptors, entry.descriptor)
			}
		}
		types[kind] = buildTypeSchemas(inspected)
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

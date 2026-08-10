package introspect

import (
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// buildFormSchema converts a protobuf message definition into the JSON Schema used by clients to build forms.
func buildFormSchema(descriptor protoreflect.MessageDescriptor) map[string]any {
	if descriptor == nil {
		return map[string]any{}
	}

	b := formSchemaBuilder{
		defs:     make(map[string]any),
		building: make(map[protoreflect.FullName]bool),
	}
	root := b.messageSchema(descriptor)
	if len(b.defs) > 0 {
		root["$defs"] = b.defs
	}
	return root
}

// formSchemaBuilder tracks reusable nested message definitions while building one form schema.
type formSchemaBuilder struct {
	defs     map[string]any
	building map[protoreflect.FullName]bool
}

func (b *formSchemaBuilder) messageSchema(descriptor protoreflect.MessageDescriptor) map[string]any {
	properties := make(map[string]any, descriptor.Fields().Len())
	fields := descriptor.Fields()
	for i := range fields.Len() {
		field := fields.Get(i)
		properties[string(field.Name())] = b.fieldSchema(field)
	}

	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
}

func (b *formSchemaBuilder) fieldSchema(field protoreflect.FieldDescriptor) map[string]any {
	// JSON Schema represents a protobuf map as an object whose values share one schema.
	if field.IsMap() {
		return map[string]any{
			"type":                 "object",
			"additionalProperties": b.valueSchema(field.MapValue()),
		}
	}
	if field.IsList() {
		return map[string]any{"type": "array", "items": b.valueSchema(field)}
	}
	return b.valueSchema(field)
}

func (b *formSchemaBuilder) valueSchema(field protoreflect.FieldDescriptor) map[string]any {
	switch field.Kind() {
	case protoreflect.BoolKind:
		return map[string]any{"type": "boolean"}
	case protoreflect.EnumKind:
		values := field.Enum().Values()
		names := make([]any, values.Len())
		for i := range values.Len() {
			names[i] = string(values.Get(i).Name())
		}
		return map[string]any{"type": "string", "enum": names}
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return map[string]any{"type": "integer", "minimum": -2147483648, "maximum": 2147483647}
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return map[string]any{"type": "integer", "minimum": 0, "maximum": int64(4294967295)}
	// Use decimal strings because JavaScript numbers cannot safely represent all 64-bit integers.
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return map[string]any{"type": "string", "pattern": "^-?[0-9]+$"}
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return map[string]any{"type": "string", "pattern": "^[0-9]+$"}
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return protobufFloatSchema()
	case protoreflect.StringKind:
		return map[string]any{"type": "string"}
	// Mark bytes as Base64 so clients can distinguish them from ordinary strings.
	case protoreflect.BytesKind:
		return map[string]any{"type": "string", "contentEncoding": "base64"}
	case protoreflect.MessageKind:
		return b.nestedMessageSchema(field.Message())
	case protoreflect.GroupKind:
		// The SDK generator does not produce legacy proto2 groups.
		return map[string]any{}
	}
	return map[string]any{}
}

// nestedMessageSchema stores nested messages in $defs and returns a $ref.
func (b *formSchemaBuilder) nestedMessageSchema(descriptor protoreflect.MessageDescriptor) map[string]any {
	if descriptor == nil || descriptor.IsPlaceholder() {
		return map[string]any{}
	}
	// Timestamp is the only well-known type emitted by the SDK generator.
	if descriptor.FullName() == "google.protobuf.Timestamp" {
		return map[string]any{"type": "string", "format": "date-time"}
	}

	name := descriptor.FullName()
	key := string(name)

	// Build each definition once. The building check breaks recursive cycles.
	if _, exists := b.defs[key]; !exists && !b.building[name] {
		b.building[name] = true
		b.defs[key] = b.messageSchema(descriptor)
		delete(b.building, name)
	}
	return map[string]any{"$ref": "#/$defs/" + jsonPointerToken(key)}
}

// Protobuf JSON represents non-finite floats as strings.
func protobufFloatSchema() map[string]any {
	return map[string]any{"anyOf": []any{
		map[string]any{"type": "number"},
		map[string]any{"type": "string", "enum": []any{"NaN", "Infinity", "-Infinity"}},
	}}
}

// jsonPointerToken escapes a protobuf type name for use in a JSON Schema $ref.
func jsonPointerToken(value string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(value)
}

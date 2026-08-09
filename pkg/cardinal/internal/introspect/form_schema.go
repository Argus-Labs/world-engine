package introspect

import (
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

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

type formSchemaBuilder struct {
	defs     map[string]any
	building map[protoreflect.FullName]bool
}

func (b *formSchemaBuilder) messageSchema(descriptor protoreflect.MessageDescriptor) map[string]any {
	properties := make(map[string]any, descriptor.Fields().Len())
	required := make([]any, 0, descriptor.Fields().Len())
	fields := descriptor.Fields()
	for i := range fields.Len() {
		field := fields.Get(i)
		properties[string(field.Name())] = b.fieldSchema(field)
		if field.Cardinality() == protoreflect.Required {
			required = append(required, string(field.Name()))
		}
	}

	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func (b *formSchemaBuilder) fieldSchema(field protoreflect.FieldDescriptor) map[string]any {
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
		return map[string]any{"type": "integer", "minimum": 0, "maximum": 4294967295}
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return map[string]any{"type": "string", "pattern": "^-?[0-9]+$"}
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return map[string]any{"type": "string", "pattern": "^[0-9]+$"}
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return protobufFloatSchema()
	case protoreflect.StringKind:
		return map[string]any{"type": "string"}
	case protoreflect.BytesKind:
		return map[string]any{"type": "string", "contentEncoding": "base64"}
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return b.messageValueSchema(field.Message())
	}
	return map[string]any{}
}

func (b *formSchemaBuilder) messageValueSchema(descriptor protoreflect.MessageDescriptor) map[string]any {
	if descriptor == nil || descriptor.IsPlaceholder() {
		return map[string]any{}
	}
	if descriptor.FullName() == "google.protobuf.Timestamp" {
		return map[string]any{"type": "string", "format": "date-time"}
	}

	key := string(descriptor.FullName())
	if _, exists := b.defs[key]; !exists && !b.building[descriptor.FullName()] {
		b.building[descriptor.FullName()] = true
		b.defs[key] = b.messageSchema(descriptor)
		delete(b.building, descriptor.FullName())
	}
	return map[string]any{"$ref": "#/$defs/" + jsonPointerToken(key)}
}

func protobufFloatSchema() map[string]any {
	return map[string]any{"anyOf": []any{
		map[string]any{"type": "number"},
		map[string]any{"type": "string", "enum": []any{"NaN", "Infinity", "-Infinity"}},
	}}
}

func jsonPointerToken(value string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(value)
}

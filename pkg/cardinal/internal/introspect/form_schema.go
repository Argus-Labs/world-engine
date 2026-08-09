package introspect

import (
	"math"
	"reflect"
	"strings"
	"time"

	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// buildFormSchema describes the authored Go value shown by editor forms. Field names intentionally
// come from Go, not JSON tags, because the generated protobuf fields use the same names.
func buildFormSchema(value any, descriptor protoreflect.MessageDescriptor) map[string]any {
	t := reflect.TypeOf(value)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	b := formSchemaBuilder{
		defs:     make(map[string]any),
		building: make(map[string]bool),
	}
	var root map[string]any
	if t.Kind() == reflect.Struct {
		root = b.structSchema(t, descriptor, false)
	} else {
		root = b.typeSchema(t, nil, false)
	}
	if len(b.defs) > 0 {
		root["$defs"] = b.defs
	}
	return root
}

type formSchemaBuilder struct {
	defs     map[string]any
	building map[string]bool
}

func (b *formSchemaBuilder) structSchema(
	t reflect.Type,
	descriptor protoreflect.MessageDescriptor,
	jsonEncoded bool,
) map[string]any {
	properties := make(map[string]any)
	required := make([]any, 0, t.NumField())
	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		var wireField protoreflect.FieldDescriptor
		if descriptor != nil {
			wireField = descriptor.Fields().ByName(protoreflect.Name(field.Name))
		}
		properties[field.Name] = b.typeSchema(field.Type, wireField, jsonEncoded)
		if field.Type.Kind() != reflect.Pointer {
			required = append(required, field.Name)
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

func (b *formSchemaBuilder) typeSchema(
	t reflect.Type,
	wireField protoreflect.FieldDescriptor,
	jsonEncoded bool,
) map[string]any {
	if t == reflect.TypeFor[time.Time]() {
		return map[string]any{"type": "string", "format": "date-time"}
	}
	if !jsonEncoded && wireField != nil && !wireField.IsList() && !wireField.IsMap() &&
		wireField.Kind() == protoreflect.BytesKind && !isByteSlice(t) {
		return b.typeSchema(t, nil, true)
	}
	if t.Kind() == reflect.Pointer {
		return b.typeSchema(t.Elem(), wireField, jsonEncoded)
	}
	if schema := scalarFormSchema(t.Kind(), jsonEncoded); schema != nil {
		return schema
	}
	return b.compositeFormSchema(t, wireField, jsonEncoded)
}

func (b *formSchemaBuilder) compositeFormSchema(
	t reflect.Type,
	wireField protoreflect.FieldDescriptor,
	jsonEncoded bool,
) map[string]any {
	switch t.Kind() { //nolint:exhaustive // scalar kinds are handled before this call
	case reflect.Slice:
		if isByteSlice(t) {
			schema := map[string]any{"type": "string"}
			if !jsonEncoded {
				schema["contentEncoding"] = "base64"
			}
			return schema
		}
		return map[string]any{"type": "array", "items": b.typeSchema(t.Elem(), wireField, jsonEncoded)}
	case reflect.Array:
		return map[string]any{
			"type": "array", "items": b.typeSchema(t.Elem(), nil, jsonEncoded),
			"minItems": t.Len(), "maxItems": t.Len(),
		}
	case reflect.Map:
		var valueField protoreflect.FieldDescriptor
		if wireField != nil && wireField.IsMap() {
			valueField = wireField.MapValue()
		}
		return map[string]any{
			"type": "object", "additionalProperties": b.typeSchema(t.Elem(), valueField, jsonEncoded),
		}
	case reflect.Struct:
		if t.Name() == "" {
			return b.structSchema(t, messageDescriptor(wireField), jsonEncoded)
		}
		key := formDefinitionKey(t)
		if jsonEncoded {
			key += ".json"
		}
		if _, exists := b.defs[key]; !exists && !b.building[key] {
			b.building[key] = true
			b.defs[key] = b.structSchema(t, messageDescriptor(wireField), jsonEncoded)
			delete(b.building, key)
		}
		return map[string]any{"$ref": "#/$defs/" + jsonPointerToken(key)}
	case reflect.Invalid, reflect.Complex64, reflect.Complex128, reflect.Chan, reflect.Func,
		reflect.Interface, reflect.Pointer, reflect.Uintptr, reflect.UnsafePointer:
		// Values encoded as JSON inside protobuf bytes remain free-form.
		return map[string]any{}
	}
	return map[string]any{}
}

func scalarFormSchema(kind reflect.Kind, jsonEncoded bool) map[string]any {
	switch kind { //nolint:exhaustive // composite kinds are handled by compositeFormSchema
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int64:
		if jsonEncoded {
			return map[string]any{"type": "integer", "minimum": -9007199254740991, "maximum": 9007199254740991}
		}
		return map[string]any{"type": "string", "pattern": "^-?[0-9]+$"}
	case reflect.Uint, reflect.Uint64:
		if jsonEncoded {
			return map[string]any{"type": "integer", "minimum": 0, "maximum": 9007199254740991}
		}
		return map[string]any{"type": "string", "pattern": "^[0-9]+$"}
	case reflect.Int8:
		return map[string]any{"type": "integer", "minimum": -128, "maximum": 127}
	case reflect.Int16:
		return map[string]any{"type": "integer", "minimum": -32768, "maximum": 32767}
	case reflect.Uint8:
		return map[string]any{"type": "integer", "minimum": 0, "maximum": 255}
	case reflect.Uint16:
		return map[string]any{"type": "integer", "minimum": 0, "maximum": 65535}
	case reflect.Int32:
		return map[string]any{"type": "integer", "minimum": -2147483648, "maximum": 2147483647}
	case reflect.Uint32:
		return map[string]any{"type": "integer", "minimum": 0, "maximum": 4294967295}
	case reflect.Float32:
		if jsonEncoded {
			return map[string]any{"type": "number", "minimum": -math.MaxFloat32, "maximum": math.MaxFloat32}
		}
		return protobufFloatSchema()
	case reflect.Float64:
		if jsonEncoded {
			return map[string]any{"type": "number", "minimum": -math.MaxFloat64, "maximum": math.MaxFloat64}
		}
		return protobufFloatSchema()
	case reflect.String:
		return map[string]any{"type": "string"}
	default:
		return nil
	}
}

func protobufFloatSchema() map[string]any {
	return map[string]any{"anyOf": []any{
		map[string]any{"type": "number"},
		map[string]any{"type": "string", "enum": []any{"NaN", "Infinity", "-Infinity"}},
	}}
}

func isByteSlice(t reflect.Type) bool {
	return t.Kind() == reflect.Slice && t.Elem() == reflect.TypeFor[byte]()
}

func messageDescriptor(field protoreflect.FieldDescriptor) protoreflect.MessageDescriptor {
	if field != nil && field.Message() != nil {
		return field.Message()
	}
	return nil
}

func formDefinitionKey(t reflect.Type) string {
	name := t.Name()
	if t.PkgPath() != "" {
		name = t.PkgPath() + "." + name
	}
	return name
}

func jsonPointerToken(value string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(value)
}

func validateDescriptorFields(value any, descriptor protoreflect.MessageDescriptor) error {
	t := reflect.TypeOf(value)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return eris.Errorf("expected a struct, got %s", t)
	}

	fields := descriptor.Fields()
	exported := 0
	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		exported++
		if fields.ByName(protoreflect.Name(field.Name)) == nil {
			return eris.Errorf("protobuf message %s has no field for Go field %s", descriptor.FullName(), field.Name)
		}
	}
	if exported != fields.Len() {
		return eris.Errorf("Go type %s has %d exported fields but protobuf message %s has %d fields",
			t, exported, descriptor.FullName(), fields.Len())
	}
	return nil
}

package cardinal

import (
	"reflect"
	"strings"
	"time"

	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// buildFormSchema describes the authored Go value shown by editor forms. Field names intentionally
// come from Go, not JSON tags, because the generated protobuf fields use the same names.
func buildFormSchema(value any) map[string]any {
	t := reflect.TypeOf(value)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	b := formSchemaBuilder{
		defs:     make(map[string]any),
		building: make(map[reflect.Type]bool),
	}
	var root map[string]any
	if t.Kind() == reflect.Struct {
		root = b.structSchema(t)
	} else {
		root = b.typeSchema(t)
	}
	if len(b.defs) > 0 {
		root["$defs"] = b.defs
	}
	return root
}

type formSchemaBuilder struct {
	defs     map[string]any
	building map[reflect.Type]bool
}

func (b *formSchemaBuilder) structSchema(t reflect.Type) map[string]any {
	properties := make(map[string]any)
	required := make([]any, 0, t.NumField())
	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		properties[field.Name] = b.typeSchema(field.Type)
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

func (b *formSchemaBuilder) typeSchema(t reflect.Type) map[string]any {
	if t == reflect.TypeFor[time.Time]() {
		return map[string]any{"type": "string", "format": "date-time"}
	}
	if t.Kind() == reflect.Pointer {
		return b.typeSchema(t.Elem())
	}

	switch t.Kind() {
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Slice:
		if t.Elem() == reflect.TypeFor[byte]() {
			return map[string]any{"type": "string", "contentEncoding": "base64"}
		}
		return map[string]any{"type": "array", "items": b.typeSchema(t.Elem())}
	case reflect.Array:
		return map[string]any{
			"type": "array", "items": b.typeSchema(t.Elem()),
			"minItems": t.Len(), "maxItems": t.Len(),
		}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": b.typeSchema(t.Elem())}
	case reflect.Struct:
		if t.Name() == "" {
			return b.structSchema(t)
		}
		key := formDefinitionKey(t)
		if _, exists := b.defs[key]; !exists && !b.building[t] {
			b.building[t] = true
			b.defs[key] = b.structSchema(t)
			delete(b.building, t)
		}
		return map[string]any{"$ref": "#/$defs/" + key}
	case reflect.Invalid, reflect.Complex64, reflect.Complex128, reflect.Chan, reflect.Func,
		reflect.Interface, reflect.Pointer, reflect.UnsafePointer:
		// Values encoded as JSON inside protobuf bytes remain free-form.
		return map[string]any{}
	}
	return map[string]any{}
}

func formDefinitionKey(t reflect.Type) string {
	name := t.Name()
	if t.PkgPath() != "" {
		name = t.PkgPath() + "." + name
	}
	return strings.NewReplacer("/", "_", ".", "_", "-", "_").Replace(name)
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

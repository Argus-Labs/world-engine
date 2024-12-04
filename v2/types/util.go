package types

import "reflect"

// GetFieldInformation returns a map of the fields of a struct and their types.
func GetFieldInformation(t reflect.Type) map[string]any {
	if t.Kind() != reflect.Struct {
		return nil
	}

	fieldMap := make(map[string]any)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldName := field.Name

		// Check if the field has a json tag
		if tag := field.Tag.Get("json"); tag != "" {
			fieldName = tag
		}

		if field.Type.Kind() == reflect.Struct {
			fieldMap[fieldName] = GetFieldInformation(field.Type)
		} else {
			fieldMap[fieldName] = field.Type.String()
		}
	}

	return fieldMap
}

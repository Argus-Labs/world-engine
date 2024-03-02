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

		if field.Type.Kind() == reflect.Struct {
			fieldMap[field.Name] = GetFieldInformation(field.Type)
		} else {
			fieldMap[field.Name] = field.Type.Name()
		}
	}

	return fieldMap
}

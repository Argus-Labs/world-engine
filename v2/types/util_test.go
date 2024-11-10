package types

import (
	"reflect"
	"testing"

	"pkg.world.dev/world-engine/assert"
)

func TestGetFieldInformation(t *testing.T) {
	testCases := []struct {
		name  string
		value any
		want  map[string]any
	}{
		{
			name: "default field names",
			value: struct {
				Alpha int
				Beta  string
				Gamma float64
			}{},
			want: map[string]any{"Alpha": "int", "Beta": "string", "Gamma": "float64"},
		},
		{
			name: "json tagged fields",
			value: struct {
				Alpha int     `json:"aaaaa"`
				Beta  string  `json:"bbbbb"`
				Gamma float64 `json:"ggggg"`
			}{},
			want: map[string]any{"aaaaa": "int", "bbbbb": "string", "ggggg": "float64"},
		},
		{
			name: "nested fields",
			value: struct {
				Alpha struct {
					Beta struct {
						Gamma string
					} `json:"bbbbb"`
				}
			}{},
			want: map[string]any{
				"Alpha": map[string]any{
					"bbbbb": map[string]any{
						"Gamma": "string",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		fields := GetFieldInformation(reflect.TypeOf(tc.value))
		assert.DeepEqual(t, fields, tc.want)
	}
}

package ecs

import (
	"testing"
)

func TestIsAlphanumericWithUnderscore(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"abc123", true},
		{"ABC_123", true},
		{"123", true},
		{"abc 123", false}, // contains a space
		{"abc123 ", false}, // contains a trailing space
		{"abc@123", false}, // contains a special character
		{"", false},        // empty string
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := isAlphanumericWithUnderscore(test.input)
			if result != test.expected {
				t.Errorf("Expected %v, but got %v", test.expected, result)
			}
		})
	}
}

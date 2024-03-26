package credentials

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateKey(t *testing.T) {
	testCases := []struct {
		name        string
		input       string
		expectError bool
	}{
		{
			name:        "Valid lowercase (64 chars)",
			input:       "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz01",
			expectError: false,
		},
		{
			name:        "Valid uppercase (64 chars)",
			input:       "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ01",
			expectError: false,
		},
		{
			name:        "Valid mixed case (64 chars)",
			input:       "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ01",
			expectError: false,
		},
		{
			name:        "Valid digits only (64 chars)",
			input:       "0123456789012345678901234567890123456789012345678901234567890123",
			expectError: false,
		},
		{
			name:        "Valid letters only (64 chars)",
			input:       "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijkl",
			expectError: false,
		},
		{
			name:        "Empty string",
			input:       "",
			expectError: true,
		},
		{
			name:        "Too short (32 chars)",
			input:       "abcdefghijklmnopqrstuvwxyz012345",
			expectError: true,
		},
		{
			name:        "Too short (63 chars)",
			input:       "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz0",
			expectError: true,
		},
		{
			name:        "Too long (65 chars)",
			input:       "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz012",
			expectError: true,
		},
		{
			name:        "Contains underscore",
			input:       "abcdefghijklmnopqrstuvwxyz01234_6789abcdefghijklmnopqrstuvwxyz01",
			expectError: true,
		},
		{
			name:        "Contains hyphen",
			input:       "abcdefghijklmnopqrstuvwxyz-1234567890abcdefghijklmnopqrstuvwxyz0",
			expectError: true,
		},
		{
			name:        "Contains space",
			input:       "abcdefghijklmnopqrstuvwxyz 1234567890abcdefghijklmnopqrstuvwxyz0",
			expectError: true,
		},
		{
			name:        "Contains special characters",
			input:       "abcdefghijklmnopqrstuvwxyz!@#$%^abcdefghijklmnopqrstuvwxyz012345",
			expectError: true,
		},
		{
			name:        "Contains non-ASCII characters",
			input:       "abcdefghijklmnopqrstuvwxyzöäüéñçabcdefghijklmnopqrstuvwxyz012345",
			expectError: true,
		},
		{
			name:        "Unicode letters and digits",
			input:       "αβγδεζηθικλμνξοπρστυφχψωАБВГДЕЁЖабвгдежзийклмнопрстуфхцчшщъыьэюя",
			expectError: true,
		},
		{
			name:        "Leading space",
			input:       " abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz0",
			expectError: true,
		},
		{
			name:        "Trailing space",
			input:       "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz ",
			expectError: true,
		},
		{
			name:        "Leading underscore",
			input:       "_abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz",
			expectError: true,
		},
		{
			name:        "Trailing underscore",
			input:       "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz_",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ValidateKey(tc.input)
			if tc.expectError {
				assert.Error(t, result)
			} else {
				assert.Nil(t, result)
			}
		})
	}
}

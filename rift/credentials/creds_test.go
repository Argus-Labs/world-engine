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
		{"Valid lowercase", "abcdefghijklmnopqrstuvwxyz012345", false},
		{"Valid uppercase", "ABCDEFGHIJKLMNOPQRSTUVWXYZ012345", false},
		{"Valid mixed case", "abcdefghijklmnopqrstuvwxyz012345", false},
		{"Valid digits only", "01234567890123456789012345678901", false},
		{"Valid letters only", "abcdefghijklmnopqrstuvwxyzABCDEF", false},
		{"Empty string", "", true},
		{"Too short", "abcdefghijklmnopqrstuvwxyz01234", true},
		{"Too long", "abcdefghijklmnopqrstuvwxyz0123456", true},
		{"Contains underscore", "abcdefghijklmnopqrstuvwxyz01234_", true},
		{"Contains hyphen", "abcdefghijklmnopqrstuvwxyz-12345", true},
		{"Contains space", "abcdefghijklmnopqrstuvwxyz 12345", true},
		{"Contains special characters", "abcdefghijklmnopqrstuvwxyz!@#$%^", true},
		{"Contains non-ASCII characters", "abcdefghijklmnopqrstuvwxyzรถรครฉรฑรง", true},
		{"Unicode letters and digits", "αβγδεζηθικλμνξοπρστυφχψωАБВГДЕЁЖ", true},
		{"Leading space", " abcdefghijklmnopqrstuvwxyz012345", true},
		{"Trailing space", "abcdefghijklmnopqrstuvwxyz012345 ", true},
		{"Leading underscore", "_abcdefghijklmnopqrstuvwxyz012345", true},
		{"Trailing underscore", "abcdefghijklmnopqrstuvwxyz012345_", true},
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

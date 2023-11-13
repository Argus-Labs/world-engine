package types

import (
	"fmt"
	"testing"

	"gotest.tools/v3/assert"
)

func TestIsEmptyOrDefault(t *testing.T) {
	testCases := []struct {
		name     string
		pr       *PageRequest
		expected bool
	}{
		{
			name: "valid",
			pr: &PageRequest{
				Key:   []byte("foo"),
				Limit: 420,
			},
			expected: false,
		},
		{
			name:     "nil",
			pr:       nil,
			expected: true,
		},
		{
			name: "empty key should be ok",
			pr: &PageRequest{
				Key:   nil,
				Limit: 420,
			},
			expected: false,
		},
		{
			name:     "empty default struct should fail",
			pr:       &PageRequest{},
			expected: true,
		},
		{
			name: "key can be empty bytes when limit is supplied",
			pr: &PageRequest{
				Key:   []byte{},
				Limit: 420,
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			valid := IsEmptyOrDefault(tc.pr)
			assert.Equal(t,
				valid,
				tc.expected,
				fmt.Sprintf("expected test to return %t, got %t", tc.expected, valid),
			)
		})
	}
}

// TestExtractPageRequest checks the edge case where a key is supplied, but limit is 0 (results in nothing being sent).
func TestExtractPageRequest(t *testing.T) {
	pr := &PageRequest{
		Key:   []byte("hello"),
		Limit: 0,
	}
	_, limit := ExtractPageRequest(pr)
	assert.Equal(t, limit, DefaultPageRequestLimit)
}

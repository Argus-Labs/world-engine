package runtime_test

import (
	"errors"
	"testing"

	cardinalruntime "github.com/argus-labs/world-engine/pkg/cardinal/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContractMismatchError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		field    string
		expected any
		actual   any
		message  string
	}{
		{
			name:     "name",
			field:    "name",
			expected: "gameplay",
			actual:   "other",
			message:  `runtime contract mismatch: module name "other", want "gameplay"`,
		},
		{
			name:     "version",
			field:    "version",
			expected: "1.2.3",
			actual:   "2.0.0",
			message:  `runtime contract mismatch: module version "2.0.0", want "1.2.3"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := &cardinalruntime.ContractMismatchError{
				Field:    test.field,
				Expected: test.expected,
				Actual:   test.actual,
			}
			require.ErrorIs(t, err, cardinalruntime.ErrContractMismatch)

			var mismatch *cardinalruntime.ContractMismatchError
			require.True(t, errors.As(err, &mismatch))
			assert.Equal(t, test.field, mismatch.Field)
			assert.Equal(t, test.expected, mismatch.Expected)
			assert.Equal(t, test.actual, mismatch.Actual)
			assert.Equal(t, test.message, mismatch.Error())
		})
	}
}

func TestBufferSizeError(t *testing.T) {
	t.Parallel()

	err := &cardinalruntime.BufferSizeError{
		Operation: "tick",
		Required:  64,
		Provided:  16,
	}

	require.ErrorIs(t, err, cardinalruntime.ErrBufferTooSmall)

	var sizeErr *cardinalruntime.BufferSizeError
	require.True(t, errors.As(err, &sizeErr))
	assert.Equal(t, 64, sizeErr.Required)
	assert.Equal(t, 16, sizeErr.Provided)
	assert.Equal(t, "tick: output buffer too small: required 64 bytes, provided 16", err.Error())
}

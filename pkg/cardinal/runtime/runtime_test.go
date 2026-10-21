package runtime_test

import (
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
		{
			name:     "input type",
			field:    "input_type",
			expected: "game.v1.TickInput",
			actual:   "other.v1.Commands",
			message:  `runtime contract mismatch: module input type "other.v1.Commands", want "game.v1.TickInput"`,
		},
		{
			name:     "output type",
			field:    "output_type",
			expected: "game.v1.TickOutput",
			actual:   "other.v1.Events",
			message:  `runtime contract mismatch: module output type "other.v1.Events", want "game.v1.TickOutput"`,
		},
		{
			name:     "snapshot type",
			field:    "snapshot_type",
			expected: "game.v1.TickSnapshot",
			actual:   "other.v1.State",
			message:  `runtime contract mismatch: module snapshot type "other.v1.State", want "game.v1.TickSnapshot"`,
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
			require.ErrorAs(t, err, &mismatch)
			assert.Equal(t, test.field, mismatch.Field)
			assert.Equal(t, test.expected, mismatch.Expected)
			assert.Equal(t, test.actual, mismatch.Actual)
			assert.Equal(t, test.message, mismatch.Error())
		})
	}
}

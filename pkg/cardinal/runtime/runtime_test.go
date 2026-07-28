package runtime_test

import (
	"errors"
	"strings"
	"testing"

	cardinalruntime "github.com/argus-labs/world-engine/pkg/cardinal/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCapabilities(t *testing.T) {
	t.Parallel()

	capabilities := cardinalruntime.CapabilityTick | cardinalruntime.CapabilityQuery
	assert.True(t, capabilities.Has(cardinalruntime.CapabilityTick))
	assert.True(t, capabilities.Has(cardinalruntime.CapabilityTick|cardinalruntime.CapabilityQuery))
	assert.False(t, capabilities.Has(cardinalruntime.CapabilitySnapshot))
}

func TestContractRequirement(t *testing.T) {
	t.Parallel()

	schemaHash := [32]byte{1, 2, 3}
	contract := cardinalruntime.Contract{
		ABIVersion: cardinalruntime.ABIVersion,
		Capabilities: cardinalruntime.CapabilityTick |
			cardinalruntime.CapabilityStateless,
		SchemaHash: schemaHash,
		Name:       "gameplay",
		Version:    "1.2.3",
	}
	requirement := cardinalruntime.ContractRequirement{
		Name:         contract.Name,
		Version:      contract.Version,
		SchemaHash:   schemaHash,
		Capabilities: cardinalruntime.CapabilityTick,
	}

	require.NoError(t, requirement.Validate(contract))

	tests := []struct {
		name     string
		mutate   func(*cardinalruntime.Contract)
		field    string
		expected any
		actual   any
		message  string
	}{
		{
			name: "name",
			mutate: func(value *cardinalruntime.Contract) {
				value.Name = "other"
			},
			field:    "name",
			expected: "gameplay",
			actual:   "other",
			message:  `runtime contract mismatch: module name "other", want "gameplay"`,
		},
		{
			name: "version",
			mutate: func(value *cardinalruntime.Contract) {
				value.Version = "2.0.0"
			},
			field:    "version",
			expected: "1.2.3",
			actual:   "2.0.0",
			message:  `runtime contract mismatch: module version "2.0.0", want "1.2.3"`,
		},
		{
			name: "capabilities",
			mutate: func(value *cardinalruntime.Contract) {
				value.Capabilities = 0
			},
			field:    "capabilities",
			expected: cardinalruntime.CapabilityTick,
			actual:   cardinalruntime.Capabilities(0),
			message:  "runtime contract mismatch: module capabilities 0x0000000000000000 lack required 0x0000000000000002",
		},
		{
			name: "schema hash",
			mutate: func(value *cardinalruntime.Contract) {
				value.SchemaHash[0]++
			},
			field:    "schema_hash",
			expected: schemaHash,
			actual:   [32]byte{2, 2, 3},
			message: "runtime contract mismatch: module schema hash 020203" +
				strings.Repeat("00", 29) + ", want 010203" + strings.Repeat("00", 29),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			actual := contract
			test.mutate(&actual)
			err := requirement.Validate(actual)
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

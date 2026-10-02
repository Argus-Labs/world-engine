package command_test

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/require"
)

// TestNames_StableAndSorted pins the fix for bug surface #1 in the DST determinism report:
// command.Manager.Names() must return command names in a stable, sorted order across
// independent Manager instances, because the DST op-generation schedule (and the structurally
// identical E2E op-generation schedule) is built from Names() and consumed by the
// order-sensitive testutils.RandOpWeights (which shuffles the input and enables the first
// `numEnabled` items). Before the fix, Names() ranged over the catalog map (randomized by the
// Go runtime independent of the prng), so the enabled-command subset and weights varied run to
// run even under an identical TEST_SEED.
func TestNames_StableAndSorted(t *testing.T) {
	t.Parallel()

	const numInstances = 20
	const numCommands = 20

	// Build the expected (sorted) name list once.
	expected := make([]string, numCommands)
	for j := range numCommands {
		expected[j] = fmt.Sprintf("cmd_%02d", j)
	}

	var reference []string
	for i := range numInstances {
		m := command.NewManager()
		for j := range numCommands {
			name := fmt.Sprintf("cmd_%02d", j)
			_, err := m.Register(name, command.NewQueue[testutils.SimpleCommand]())
			require.NoError(t, err, "Register failed for %q on instance %d", name, i)
		}

		names := m.Names()
		require.Len(t, names, numCommands, "Names() returned wrong count on instance %d", i)

		if i == 0 {
			reference = names
		} else {
			require.Equal(t, reference, names,
				"Names() must return identical order across fresh Manager instances; "+
					"otherwise the DST op-generation schedule cannot be reproduced under TEST_SEED.")
		}
	}

	// Confirm the stable order is actually sorted (the actual contract).
	require.True(t, slices.IsSorted(reference),
		"Names() must return command names in sorted order; got %v", reference)
	require.Equal(t, expected, reference, "Names() returned an unexpected order")
}

// TestNames_EmptyAndSingle covers the boundary cases for sorting.
func TestNames_EmptyAndSingle(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		m := command.NewManager()
		require.Empty(t, m.Names())
	})

	t.Run("single", func(t *testing.T) {
		t.Parallel()
		m := command.NewManager()
		_, err := m.Register("solo", command.NewQueue[testutils.SimpleCommand]())
		require.NoError(t, err)
		require.Equal(t, []string{"solo"}, m.Names())
	})
}

// TestNames_InsertionOrderIrrelevant confirms that two Managers registering the same names in
// different orders produce the same (sorted) Names() output — i.e. the sort removes the
// dependence on registration order that the catalog map previously leaked.
func TestNames_InsertionOrderIrrelevant(t *testing.T) {
	t.Parallel()

	const numCommands = 8
	makeNames := func() []string {
		out := make([]string, numCommands)
		for j := range numCommands {
			out[j] = fmt.Sprintf("cmd_%02d", j)
		}
		return out
	}

	// Forward insertion.
	mForward := command.NewManager()
	for _, name := range makeNames() {
		_, err := mForward.Register(name, command.NewQueue[testutils.SimpleCommand]())
		require.NoError(t, err)
	}

	// Reverse insertion.
	mReverse := command.NewManager()
	reversed := makeNames()
	slices.Reverse(reversed)
	for _, name := range reversed {
		_, err := mReverse.Register(name, command.NewQueue[testutils.SimpleCommand]())
		require.NoError(t, err)
	}

	// Random insertion.
	prng := rand.New(rand.NewPCG(0x1234, 0x5678))
	mRandom := command.NewManager()
	shuffled := makeNames()
	prng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	for _, name := range shuffled {
		_, err := mRandom.Register(name, command.NewQueue[testutils.SimpleCommand]())
		require.NoError(t, err)
	}

	// All three must report identical (sorted) Names() output.
	require.Equal(t, mForward.Names(), mReverse.Names(),
		"Names() must be independent of registration order")
	require.Equal(t, mForward.Names(), mRandom.Names(),
		"Names() must be independent of registration order")
}

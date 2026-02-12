package testutils

import (
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/assert"
)

var Seed uint64 //nolint:gochecknoglobals // intentionally global for test reproducibility

func init() { //nolint:gochecknoinits // intentionally using init to set seed
	Seed = uint64(time.Now().UnixNano()) //nolint:gosec // it's ok
	if envSeed := os.Getenv("TEST_SEED"); envSeed != "" {
		parsed, err := strconv.ParseUint(envSeed, 0, 64)
		if err == nil { // Only set using the env if it's valid
			Seed = parsed
		}
	}
	fmt.Printf("to reproduce: TEST_SEED=0x%x\n", Seed) //nolint:forbidigo // just for testing
}

func NewRand(t *testing.T) *rand.Rand {
	t.Helper()
	return rand.New(rand.NewPCG(Seed, Seed)) //nolint:gosec // weak RNG is fine for tests
}

// RandMapKey returns a random key from a map. Panics if the map is empty.
func RandMapKey[K comparable, V any](r *rand.Rand, m map[K]V) K {
	idx := r.IntN(len(m))
	for k := range m {
		if idx == 0 {
			return k
		}
		idx--
	}
	panic("unreachable")
}

// OpWeights maps operation names to their weights.
type OpWeights = map[string]uint64

// RandOpWeights randomly selects a subset of operations and assigns random weights (1-100) to each.
// At minimum 1 operation is enabled, at most all operations are enabled.
func RandOpWeights(r *rand.Rand, ops []string) OpWeights {
	assert.That(len(ops) > 0, "you need multiple operations to randomize")

	// Randomly select how many operations to enable (1 to len(ops)).
	numEnabled := 1 + r.IntN(len(ops))

	// Shuffle and take the first numEnabled operations.
	shuffled := slices.Clone(ops)
	r.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	weights := make(map[string]uint64, numEnabled)
	for i := range numEnabled {
		weights[shuffled[i]] = uint64(1 + r.IntN(100)) //nolint:gosec // not gonna happen
	}
	return weights
}

// RandWeightedOp returns a random operation from a map, using each op's value as its weight.
func RandWeightedOp(r *rand.Rand, ops OpWeights) string {
	var total uint64
	for _, weight := range ops {
		total += weight
	}

	pick := r.Uint64N(total)
	for op, weight := range ops {
		if pick < weight {
			return op
		}
		pick -= weight
	}
	panic("unreachable")
}

// RandString generates a random alphanumeric string of the given length.
func RandString(r *rand.Rand, length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = chars[r.IntN(len(chars))]
	}
	return string(b)
}

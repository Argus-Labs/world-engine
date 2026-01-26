package testutils

import (
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"
	"testing"
	"time"
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

// WeightedOp is a constraint for operation types that use their value as the weight.
type WeightedOp interface {
	~uint8 | ~uint16 | ~uint32 | ~int
}

// RandWeightedOp returns a random operation from a slice, using each op's value as its weight.
func RandWeightedOp[T WeightedOp](r *rand.Rand, ops []T) T {
	var total int
	for _, op := range ops {
		total += int(op)
	}

	pick := r.IntN(total)
	for _, op := range ops {
		weight := int(op)
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

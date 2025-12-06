package testutils

import (
	"math/rand/v2"
	"os"
	"strconv"
	"time"
)

var Seed uint64 //nolint:gochecknoglobals // intentionally global for test reproducibility

func init() { //nolint:gochecknoinits // intentionally using init to set seed
	if envSeed := os.Getenv("TEST_SEED"); envSeed != "" {
		parsed, err := strconv.ParseUint(envSeed, 10, 64)
		if err == nil {
			Seed = parsed
			return
		}
	}
	Seed = uint64(time.Now().UnixNano()) //nolint:gosec // overflow is acceptable for test seeds
}

func NewRand() *rand.Rand {
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

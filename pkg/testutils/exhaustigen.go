package testutils

import "github.com/argus-labs/world-engine/pkg/assert"

// Gen is an exhaustive generator that iterates through all possible combinations of values.
//
// The implementation is tricky, refer to the post for details.
//
// On each iteration of `while (!g.done())` loop, Gen generates a sequence of numbers.
// Internally, it remembers this sequence  together with bounds the user requested:
//
// value:  3 1 4 4
// bound:  5 4 4 4
//
// To advance to the next iteration, Gen finds the smallest sequence of values which is larger than
// the current one, but still satisfies all the bounds. "Smallest" means that Gen tries to increment
// the rightmost number.
//
// In the above example, the last two "4"s already match the bound, so we can't increment them.
// However, we can increment the second number, "1", to get 3 2 4 4. This isn’t the smallest
// sequence though, 3 2 0 0 is be smaller. So, after incrementing the rightmost number possible,
// we zero the rest.
//
// See: <https://matklad.github.io/2021/11/07/generate-all-the-things.html>
type Gen struct {
	started bool
	v       [32]struct{ value, bound uint32 }
	p       int
	pMax    int
}

// NewGen creates a new exhaustive generator.
func NewGen() *Gen {
	return &Gen{
		started: false,
		v:       [32]struct{ value, bound uint32 }{},
		p:       0,
		pMax:    0,
	}
}

// Done returns true when all combinations have been exhausted.
func (g *Gen) Done() bool {
	if !g.started {
		g.started = true
		return false
	}
	i := g.pMax
	for i > 0 {
		i--
		if g.v[i].value < g.v[i].bound {
			g.v[i].value++
			g.pMax = i + 1
			g.p = 0
			return false
		}
	}
	return true
}

func (g *Gen) gen(bound uint32) uint32 {
	assert.That(g.p < len(g.v), "exhaustigen: exceeded maximum depth of 32")
	if g.p == g.pMax {
		g.v[g.p] = struct{ value, bound uint32 }{value: 0, bound: 0}
		g.pMax++
	}
	g.p++
	g.v[g.p-1].bound = bound
	return g.v[g.p-1].value
}

// Intn returns an int in range [0, bound] (inclusive).
func (g *Gen) Intn(bound int) int {
	return int(g.gen(uint32(bound))) //nolint:gosec // bound is expected to be small in tests
}

// Range returns an int in range [minVal, maxVal] (inclusive).
func (g *Gen) Range(minVal, maxVal int) int {
	assert.That(minVal < maxVal, "exhaustigen: min > max")
	return minVal + g.Intn(maxVal-minVal)
}

// Index returns a valid index into a slice of the given length.
func (g *Gen) Index(length int) int {
	assert.That(length > 0, "exhaustigen: empty slice")
	return g.Intn(length - 1)
}

// Bool returns an exhaustive boolean value.
func (g *Gen) Bool() bool {
	return g.Intn(1) == 1
}

// Shuffle shuffles the slice in-place, exhaustively generating all permutations.
func Shuffle[T any](g *Gen, slice []T) {
	if len(slice) <= 1 {
		return
	}
	for i := range len(slice) - 1 {
		j := g.Range(i, len(slice)-1)
		slice[i], slice[j] = slice[j], slice[i]
	}
}

// Pick returns an element from the slice.
func Pick[T any](g *Gen, slice []T) T {
	return slice[g.Index(len(slice))]
}

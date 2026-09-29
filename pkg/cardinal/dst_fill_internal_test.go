package cardinal

import (
	"reflect"
	"testing"

	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/require"
)

type fillProbeRow struct {
	Cells immutable.Slice[int32]
}

// fillProbe is the shape of a command payload that carries lists: ids, and rows that hold a Slice of
// their own, next to a plain field that must keep being filled as before.
type fillProbe struct {
	Ids   immutable.Slice[EntityID]
	Rows  immutable.Slice[fillProbeRow]
	Plain int32
}

// TestFillRandom_FillsImmutableSlices pins that DST's random filler reaches into an immutable.Slice.
// The detection is a heuristic on package path, type name and field shape; if a refactor of
// immutable stopped it matching, every Slice would arrive empty and DST would pass regardless, so
// this is the only place that failure shows. It also pins that a payload the filler cannot address
// fails loudly instead of the same silence.
func TestFillRandom_FillsImmutableSlices(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)
	live := []EntityID{7, 8, 9}

	var sawIDs, sawNested, sawPlain bool
	for range 64 {
		var probe fillProbe
		fillRandom(prng, reflect.ValueOf(&probe).Elem(), live)

		sawIDs = sawIDs || probe.Ids.Len() > 0
		for _, row := range probe.Rows.All() {
			sawNested = sawNested || row.Cells.Len() > 0
		}
		sawPlain = sawPlain || probe.Plain != 0
	}
	require.True(t, sawIDs, "Slice[EntityID] was never filled")
	require.True(t, sawNested, "a Slice inside a Slice element was never filled")
	require.True(t, sawPlain, "plain fields must still be filled")

	// A non-addressable Slice cannot take values, and the struct walk would pass it by in silence.
	require.Panics(t, func() {
		fillRandom(prng, reflect.ValueOf(immutable.Slice[int32]{}), live)
	}, "an unaddressable Slice must fail loudly")
}

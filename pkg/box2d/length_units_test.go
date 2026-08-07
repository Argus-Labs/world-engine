// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/core.c, include/box2d/constants.h.

package box2d_test

import (
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// Upstream derives several tolerances from the length unit with macros that
// re-read it at every use, so the Go port keeps them in variables that
// SetLengthUnitsPerMeter recomputes. Two properties matter here:
//
//  1. At the default unit of 1.0 every derived value must be bit-identical to
//     the compile-time constant it replaced. Anything else moves the golden
//     traces and breaks the package's determinism promise.
//  2. Setting unit k must scale each derived value by exactly k, and must leave
//     the dimensionless constants alone.
//
// The length unit is process-global state, so none of these tests may call
// t.Parallel: a parallel test would observe another test's unit. Sequential
// tests never overlap paused parallel ones, so running these without
// t.Parallel is what keeps the rest of the package isolated from them.

// withDefaultLengthUnitRestored registers the restore of the default length
// unit before the test can fail, so the default is put back even on failure or
// on a require abort.
func withDefaultLengthUnitRestored(t *testing.T) {
	t.Helper()

	t.Cleanup(func() { box2d.SetLengthUnitsPerMeter(1.0) })

	require.InDelta(t, 1.0, box2d.GetLengthUnitsPerMeter(), 0,
		"length unit must be the default at test start; a previous test leaked global state")
}

// requireSameBits compares two float64 values by bit pattern, which is stricter
// than == (it separates -0 from 0 and rejects any last-bit drift).
func requireSameBits(t *testing.T, name string, want, got float64) {
	t.Helper()

	require.Equal(t, math.Float64bits(want), math.Float64bits(got),
		"%s: want %v (bits %#016x), got %v (bits %#016x)",
		name, want, math.Float64bits(want), got, math.Float64bits(got))
}

// TestLengthScaledDefaultsAreBitIdentical pins the default tolerances to the
// exact values the package used when they were untyped constants. The wanted
// values below are written as untyped constant expressions, so the compiler
// folds them the same way it folded the originals.
func TestLengthScaledDefaultsAreBitIdentical(t *testing.T) {
	withDefaultLengthUnitRestored(t)

	requireSameBits(t, "LinearSlop", 0.005, box2d.LinearSlop)
	requireSameBits(t, "Huge", 100000.0, box2d.Huge)
	requireSameBits(t, "SpeculativeDistance", 4.0*0.005, box2d.SpeculativeDistance)
	requireSameBits(t, "ContactRecycleDistance", 10.0*0.005, box2d.ContactRecycleDistance)
	requireSameBits(t, "MaxAABBMargin", 0.05, box2d.MaxAABBMargin)
}

// TestLengthScaledDerivedExpressionsAreBitIdentical covers the expressions the
// collision code builds from LinearSlop. Those used to be folded at compile
// time and are now evaluated at run time, which is a separate rounding path, so
// each one is pinned against the constant expression it replaced.
func TestLengthScaledDerivedExpressionsAreBitIdentical(t *testing.T) {
	withDefaultLengthUnitRestored(t)

	slop := box2d.LinearSlop

	// manifold.go: slopBias and slopTenth.
	requireSameBits(t, "0.1*LinearSlop", 0.1*0.005, 0.1*slop)
	// hull.go: collinearity and welding tolerances.
	requireSameBits(t, "2.0*LinearSlop", 2.0*0.005, 2.0*slop)
	requireSameBits(t, "16.0*LinearSlop*LinearSlop", 16.0*0.005*0.005, 16.0*slop*slop)
}

// TestSetLengthUnitsPerMeterScalesDerivedQuantities checks the whole point of
// the setter: every length-scaled tolerance follows the unit by exactly the set
// factor, and the dimensionless constants do not move.
func TestSetLengthUnitsPerMeterScalesDerivedQuantities(t *testing.T) {
	withDefaultLengthUnitRestored(t)

	defaults := map[string]float64{
		"LinearSlop":             box2d.LinearSlop,
		"Huge":                   box2d.Huge,
		"SpeculativeDistance":    box2d.SpeculativeDistance,
		"ContactRecycleDistance": box2d.ContactRecycleDistance,
		"MaxAABBMargin":          box2d.MaxAABBMargin,
	}

	// A power of two, a non-power of two, and units well away from 1 in both
	// directions (millimetres and kilometres).
	for _, lengthUnits := range []float64{2.0, 0.5, 100.0, 1000.0, 0.001, 3.7} {
		box2d.SetLengthUnitsPerMeter(lengthUnits)

		requireSameBits(t, "GetLengthUnitsPerMeter", lengthUnits, box2d.GetLengthUnitsPerMeter())

		current := map[string]float64{
			"LinearSlop":             box2d.LinearSlop,
			"Huge":                   box2d.Huge,
			"SpeculativeDistance":    box2d.SpeculativeDistance,
			"ContactRecycleDistance": box2d.ContactRecycleDistance,
			"MaxAABBMargin":          box2d.MaxAABBMargin,
		}

		for name, base := range defaults {
			label := name + " at unit " + strconv.FormatFloat(lengthUnits, 'g', -1, 64)
			requireSameBits(t, label, base*lengthUnits, current[name])
		}

		// Dimensionless quantities are constants and must stay put.
		requireSameBits(t, "MaxRotation", 0.25*box2d.Pi, box2d.MaxRotation)
		requireSameBits(t, "AABBMarginFraction", 0.125, box2d.AABBMarginFraction)
		requireSameBits(t, "TimeToSleep", 0.5, box2d.TimeToSleep)
		require.Equal(t, -1, box2d.NullIndex)
		require.Equal(t, 24, box2d.GraphColorCount)
		require.Equal(t, 128, box2d.MaxWorlds)
		require.Equal(t, 64, box2d.MaxWorkers)
		require.Equal(t, 32, box2d.Alignment)
	}

	// Returning to the default restores the exact default bit patterns.
	box2d.SetLengthUnitsPerMeter(1.0)
	requireSameBits(t, "LinearSlop restored", 0.005, box2d.LinearSlop)
	requireSameBits(t, "Huge restored", 100000.0, box2d.Huge)
	requireSameBits(t, "SpeculativeDistance restored", 4.0*0.005, box2d.SpeculativeDistance)
	requireSameBits(t, "ContactRecycleDistance restored", 10.0*0.005, box2d.ContactRecycleDistance)
	requireSameBits(t, "MaxAABBMargin restored", 0.05, box2d.MaxAABBMargin)
}

// TestGetLengthUnitsPerMeterReturnsWhatWasSet is the round-trip check on the
// accessor pair.
func TestGetLengthUnitsPerMeterReturnsWhatWasSet(t *testing.T) {
	withDefaultLengthUnitRestored(t)

	for _, lengthUnits := range []float64{0.25, 1.0, 7.5, 1000.0} {
		box2d.SetLengthUnitsPerMeter(lengthUnits)
		requireSameBits(t, "GetLengthUnitsPerMeter", lengthUnits, box2d.GetLengthUnitsPerMeter())
	}
}

// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file include/box2d/constants.h, src/core.h.

package box2d

// NullIndex is B2_NULL_INDEX.
const NullIndex = -1

// SecretCookie is B2_SECRET_COOKIE.
const SecretCookie = 1152023

// Alignment is B2_ALIGNMENT.
const Alignment = 32

// MaxWorkers is B2_MAX_WORKERS.
const MaxWorkers = 64

// GraphColorCount is B2_GRAPH_COLOR_COUNT.
const GraphColorCount = 24

// MaxWorlds is B2_MAX_WORLDS.
const MaxWorlds = 128

// MaxRotation is B2_MAX_ROTATION (0.25f * B2_PI). An angle, so it does not
// scale with the length unit.
const MaxRotation = 0.25 * Pi

// AABBMarginFraction is B2_AABB_MARGIN_FRACTION. A dimensionless fraction of a
// shape extent, so it does not scale with the length unit.
const AABBMarginFraction = 0.125

// TimeToSleep is B2_TIME_TO_SLEEP. Seconds, so it does not scale with the
// length unit.
const TimeToSleep = 0.5

// The per-meter magnitudes of the length-scaled tolerances below. These are the
// literal factors upstream writes in each macro, evaluated as untyped constants
// so the compiler folds them exactly.
const (
	linearSlopPerMeter             = 0.005
	hugePerMeter                   = 100000.0
	speculativeDistancePerMeter    = 4.0 * linearSlopPerMeter
	contactRecycleDistancePerMeter = 10.0 * linearSlopPerMeter
	maxAABBMarginPerMeter          = 0.05
)

// The length-scaled tolerances. Upstream declares each of these as a macro that
// calls b2GetLengthUnitsPerMeter() at every use, for example
//
//	#define B2_LINEAR_SLOP ( 0.005f * b2GetLengthUnitsPerMeter() )
//
// so they follow b2SetLengthUnitsPerMeter. Go has no macros, so this port
// mirrors upstream's mutable global with package-level variables that
// SetLengthUnitsPerMeter recomputes. They are deliberately variables, not
// constants: freezing them would silently diverge from upstream collision, CCD
// and broad-phase behaviour for any caller that picks a non-meter length unit.
//
// Each variable is its per-meter constant multiplied by the current length
// unit. At the default unit of 1.0 that product is exact (IEEE-754 guarantees
// x*1.0 == x for finite x), so every default is bit-identical to the untyped
// constant it replaced and the golden traces are unaffected.
//
// # Warning
//
// This state is process-global, exactly as upstream's is. Set the length unit
// once during start-up, BEFORE creating any World — upstream documents the same
// contract ("This must be modified before any calls to Box2D"). Changing it
// while a simulation is live invalidates every length already stored in world
// state (AABBs, contact separations, joint frames) and voids the determinism
// guarantee of this package, because the same input sequence then produces
// different results depending on when the change occurred.
//
// Treat these as read-only outside SetLengthUnitsPerMeter.
var (
	// LinearSlop is B2_LINEAR_SLOP (0.005f * b2GetLengthUnitsPerMeter()).
	LinearSlop = linearSlopPerMeter * lengthUnitsPerMeter

	// Huge is B2_HUGE (100000.0f * b2GetLengthUnitsPerMeter()).
	Huge = hugePerMeter * lengthUnitsPerMeter

	// SpeculativeDistance is B2_SPECULATIVE_DISTANCE (4.0f * B2_LINEAR_SLOP).
	SpeculativeDistance = speculativeDistancePerMeter * lengthUnitsPerMeter

	// ContactRecycleDistance is B2_CONTACT_RECYCLE_DISTANCE (10.0f * B2_LINEAR_SLOP).
	ContactRecycleDistance = contactRecycleDistancePerMeter * lengthUnitsPerMeter

	// MaxAABBMargin is B2_MAX_AABB_MARGIN (0.05f * b2GetLengthUnitsPerMeter()).
	MaxAABBMargin = maxAABBMarginPerMeter * lengthUnitsPerMeter
)

// updateLengthScaledConstants re-evaluates the length-scaled tolerances for the
// given length unit. It reproduces the initializers above term for term, so
// passing the default unit of 1.0 restores the exact default bit patterns.
// SetLengthUnitsPerMeter is the only caller.
func updateLengthScaledConstants(lengthUnits float64) {
	LinearSlop = linearSlopPerMeter * lengthUnits
	Huge = hugePerMeter * lengthUnits
	SpeculativeDistance = speculativeDistancePerMeter * lengthUnits
	ContactRecycleDistance = contactRecycleDistancePerMeter * lengthUnits
	MaxAABBMargin = maxAABBMarginPerMeter * lengthUnits
}

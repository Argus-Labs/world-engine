// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/core.c, src/core.h, include/box2d/base.h.

package box2d

// assert mirrors B2_ASSERT. Assertions are compiled out when debugAsserts is
// false (the default build; the box2d_asserts build tag flips it — see
// core_asserts_off.go), but the condition is still EVALUATED at the call
// site unless the caller guards it. Upstream's release macro is `((void)0)`,
// which discards the expression unevaluated, so match it per site by cost:
// a bare assert(...) is fine when the condition inlines to nothing (field
// compares, IsValidFloat), but a condition that does real work — the
// multi-line validators (IsValidVec2/AABB/Rotation/Transform/Ray/Plane,
// ValidateHull) exceed the inline budget and survive as real calls in the
// release binary — must be wrapped in `if debugAsserts { ... }` so the
// compile-time const folds the whole evaluation away.
func assert(cond bool) {
	if debugAsserts && !cond {
		panic("box2d: assertion failed")
	}
}

// requireInitialized enforces the public API precondition that a definition
// struct was produced by its Default* constructor. It replaces the upstream
// b2_secretCookie/internalValue assert for the exported creation functions and
// is always enabled, independent of debugAsserts.
//
// It panics rather than returning an error on purpose: this reports a
// programmer error (a zero-value definition literal) rather than a runtime
// condition, it matches upstream's assert intent, and every creation function
// it guards returns only an id.
func requireInitialized(ok bool, defName, ctorName string) {
	if !ok {
		panic("box2d: " + defName + " was not initialized by " + ctorName +
			" (zero-value definition structs are not valid; see " + ctorName + ")")
	}
}

// requireValidDefField enforces a public API precondition on a single
// definition field whose bad value would silently corrupt the simulation
// instead of failing loudly. Like requireInitialized it is always enabled.
// requirement describes what the field must satisfy, in words.
func requireValidDefField(ok bool, defName, fieldName, requirement string) {
	if !ok {
		panic("box2d: " + defName + "." + fieldName + " is invalid: " + requirement)
	}
}

// Version mirrors b2Version from base.h.
type Version struct {
	Major    int
	Minor    int
	Revision int
}

// GetVersion returns the current Box2D version (upstream b2GetVersion).
func GetVersion() Version {
	return Version{
		Major:    3,
		Minor:    2,
		Revision: 0,
	}
}

// HashInit is the starting value for the djb2 hash (upstream B2_HASH_INIT).
const HashInit = 5381

// Hash is the simple djb2 hash function used for determinism testing (upstream b2Hash).
func Hash(hash uint32, data []byte) uint32 {
	result := hash
	for i := range data {
		result = (result << 5) + result + uint32(data[i])
	}

	return result
}

// lengthUnitsPerMeter is the port of upstream's b2_lengthUnitsPerMeter global.
// It is process-global mutable state on purpose: the value it holds is what the
// upstream tolerance macros read, and the port cannot reproduce their behaviour
// without it. Read it through GetLengthUnitsPerMeter; write it only through
// SetLengthUnitsPerMeter, which keeps the derived tolerances in constants.go in
// step.
var lengthUnitsPerMeter = 1.0

// SetLengthUnitsPerMeter allows the user to change the length units at runtime (upstream b2SetLengthUnitsPerMeter).
//
// It also recomputes the length-scaled tolerances declared in constants.go
// (LinearSlop, Huge, SpeculativeDistance, ContactRecycleDistance and
// MaxAABBMargin). Upstream defines those as macros that re-read the length unit
// at every use, so they must follow this setter; a Go port that froze them
// would give a caller with a non-meter length unit silently upstream-divergent
// collision, continuous-collision and broad-phase behaviour.
//
// # Warning
//
// The length unit and everything derived from it are process-global, as they
// are upstream. Call this once during start-up, BEFORE creating any World —
// upstream states the same contract, "This must be modified before any calls to
// Box2D". Calling it while a simulation is live invalidates in-flight state,
// because lengths already stored in a world (AABBs, contact separations, joint
// frames) were computed against the previous unit, and it voids this package's
// determinism guarantee: replaying the same inputs no longer reproduces the same
// results unless the change happens at the same point in the sequence. It is
// also not safe to call concurrently with any other use of the package.
func SetLengthUnitsPerMeter(lengthUnits float64) {
	assert(IsValidFloat(lengthUnits) && lengthUnits > 0.0)
	lengthUnitsPerMeter = lengthUnits
	updateLengthScaledConstants(lengthUnits)
}

// GetLengthUnitsPerMeter returns the current length units per meter (upstream b2GetLengthUnitsPerMeter).
func GetLengthUnitsPerMeter() float64 {
	return lengthUnitsPerMeter
}

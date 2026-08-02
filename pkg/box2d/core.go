// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/core.c, src/core.h, include/box2d/base.h.

package box2d

const debugAsserts = false

// assert mirrors B2_ASSERT. Assertions are compiled out when debugAsserts is false; the condition is still evaluated at the call site.
func assert(cond bool) {
	if debugAsserts && !cond {
		panic("box2d: assertion failed")
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

var lengthUnitsPerMeter = 1.0

// SetLengthUnitsPerMeter allows the user to change the length units at runtime (upstream b2SetLengthUnitsPerMeter).
func SetLengthUnitsPerMeter(lengthUnits float64) {
	assert(IsValidFloat(lengthUnits) && lengthUnits > 0.0)
	lengthUnitsPerMeter = lengthUnits
}

// GetLengthUnitsPerMeter returns the current length units per meter (upstream b2GetLengthUnitsPerMeter).
func GetLengthUnitsPerMeter() float64 {
	return lengthUnitsPerMeter
}

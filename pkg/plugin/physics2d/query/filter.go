package query

import "github.com/ByteArena/box2d"

// filterAllows reports whether fixture should be considered by a query. When f is nil, defaults
// match RaycastRequest: all category/mask bits (0xFFFF) and sensors excluded. Otherwise
// IncludeSensors gates sensors, and category/mask use the same bitwise rule as Box2D collision
// (mutual mask overlap with the fixture’s filter data).
func filterAllows(f *Filter, fixture *box2d.B2Fixture) bool {
	includeSensors := false
	cat := uint16(0xFFFF)
	mask := uint16(0xFFFF)
	if f != nil {
		includeSensors = f.IncludeSensors
		cat = f.CategoryBits
		mask = f.MaskBits
	}
	if !includeSensors && fixture.IsSensor() {
		return false
	}
	fd := fixture.GetFilterData()
	return (mask&fd.CategoryBits) != 0 && (cat&fd.MaskBits) != 0
}

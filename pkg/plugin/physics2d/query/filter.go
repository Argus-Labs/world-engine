package query

// Filter restricts which fixtures a query considers. A fixture passes when all of the
// following hold:
//   - (Filter.MaskBits & fixture.CategoryBits) != 0
//   - (Filter.CategoryBits & fixture.MaskBits) != 0
//   - if IncludeSensors is false, the fixture is not a sensor
//
// Fixture group index is not part of v1 query filtering; category/mask only.
//
// A nil *Filter uses CategoryBits and MaskBits ^uint64(0) and IncludeSensors false (solids only, all layers)
// for RaycastRequest, AABBOverlapRequest, and CircleSweepRequest.
type Filter struct {
	CategoryBits   uint64 `json:"category_bits"`
	MaskBits       uint64 `json:"mask_bits"`
	IncludeSensors bool   `json:"include_sensors"`
}

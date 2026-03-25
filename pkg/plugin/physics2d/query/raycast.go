package query

import (
	"github.com/ByteArena/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
)

// Filter restricts which fixtures a query considers. A fixture passes when all of the
// following hold:
//   - (Filter.MaskBits & fixture.CategoryBits) != 0
//   - (Filter.CategoryBits & fixture.MaskBits) != 0
//   - if IncludeSensors is false, the fixture is not a sensor
//
// Fixture group index is not part of v1 query filtering; category/mask only.
//
// A nil *Filter uses CategoryBits and MaskBits 0xFFFF and IncludeSensors false (solids only, all layers)
// for RaycastRequest, AABBOverlapRequest, and CircleSweepRequest.
type Filter struct {
	CategoryBits   uint16 `json:"category_bits"`
	MaskBits       uint16 `json:"mask_bits"`
	IncludeSensors bool   `json:"include_sensors"`
}

// RaycastRequest is a world-space segment cast from Origin toward End (inclusive segment; hit
// fraction is in [0,1] along Origin→End). The ray must have non-zero length.
type RaycastRequest struct {
	Origin component.Vec2 `json:"origin"`
	End    component.Vec2 `json:"end"`
	Filter *Filter        `json:"filter,omitempty"`
}

// RaycastResult is the closest hit along the segment, if any. When Hit is false, other fields are zero.
type RaycastResult struct {
	Hit        bool              `json:"hit"`
	Entity     cardinal.EntityID `json:"entity"`
	ShapeIndex int               `json:"shape_index"`
	Point      component.Vec2    `json:"point"`
	Normal     component.Vec2    `json:"normal"`
	Fraction   float64           `json:"fraction"`
}

// Raycast returns the closest fixture hit along [req.Origin, req.End], or Hit=false.
// World must be the plugin’s Box2D world (nil or zero-length segment yields Hit=false).
func Raycast(world *box2d.B2World, req RaycastRequest) RaycastResult {
	if world == nil {
		return RaycastResult{}
	}
	dx := req.End.X - req.Origin.X
	dy := req.End.Y - req.Origin.Y
	if dx*dx+dy*dy < raycastMinLengthSq {
		return RaycastResult{}
	}

	var best RaycastResult
	best.Hit = false
	bestFraction := 1.0

	p1 := box2d.MakeB2Vec2(req.Origin.X, req.Origin.Y)
	p2 := box2d.MakeB2Vec2(req.End.X, req.End.Y)

	cb := func(fixture *box2d.B2Fixture, point box2d.B2Vec2, normal box2d.B2Vec2, fraction float64) float64 {
		if !filterAllows(req.Filter, fixture) {
			return -1
		}
		entityID, bodyOK := BodyUserDataFrom(fixture.GetBody().GetUserData())
		if !bodyOK {
			return -1
		}
		_, shapeIndex, fixOK := FixtureUserDataFrom(fixture.GetUserData())
		if !fixOK {
			return -1
		}
		if fraction < bestFraction || !best.Hit {
			bestFraction = fraction
			best.Hit = true
			best.Entity = entityID
			best.ShapeIndex = shapeIndex
			best.Point = component.Vec2{X: point.X, Y: point.Y}
			best.Normal = component.Vec2{X: normal.X, Y: normal.Y}
			best.Fraction = fraction
		}
		return fraction
	}

	world.RayCast(cb, p1, p2)
	return best
}

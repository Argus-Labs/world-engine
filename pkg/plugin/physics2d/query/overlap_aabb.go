package query

import (
	"github.com/ByteArena/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
)

// AABBOverlapRequest finds fixtures whose shapes overlap the axis-aligned box [Min, Max] in world space
// (inclusive bounds on the query box). Min.X may be greater than Max.X; components are swapped per axis.
type AABBOverlapRequest struct {
	Min    component.Vec2 `json:"min"`
	Max    component.Vec2 `json:"max"`
	Filter *Filter        `json:"filter,omitempty"`
}

// AABBOverlapHit is one ECS shape that overlaps the query AABB after narrow-phase test.
type AABBOverlapHit struct {
	Entity     cardinal.EntityID `json:"entity"`
	ShapeIndex int               `json:"shape_index"`
}

// AABBOverlapResult lists distinct (Entity, ShapeIndex) pairs that overlap the query box.
type AABBOverlapResult struct {
	Hits []AABBOverlapHit `json:"hits"`
}

type aabbOverlapKey struct {
	Entity     cardinal.EntityID
	ShapeIndex int
}

// OverlapAABB returns distinct (Entity, ShapeIndex) pairs overlapping the query AABB (narrow-phase).
// World must be the plugin’s Box2D world.
func OverlapAABB(world *box2d.B2World, req AABBOverlapRequest) AABBOverlapResult {
	out := AABBOverlapResult{}
	if world == nil {
		return out
	}

	minX, maxX := req.Min.X, req.Max.X
	if minX > maxX {
		minX, maxX = maxX, minX
	}
	minY, maxY := req.Min.Y, req.Max.Y
	if minY > maxY {
		minY, maxY = maxY, minY
	}

	cx := 0.5 * (minX + maxX)
	cy := 0.5 * (minY + maxY)
	hx := 0.5 * (maxX - minX)
	hy := 0.5 * (maxY - minY)
	if hx <= 0 || hy <= 0 {
		return out
	}

	queryPoly := box2d.NewB2PolygonShape()
	queryPoly.SetAsBoxFromCenterAndAngle(hx, hy, box2d.MakeB2Vec2(cx, cy), 0)
	xfQuery := box2d.MakeB2Transform()
	xfQuery.SetIdentity()

	aabb := box2d.MakeB2AABB()
	aabb.LowerBound = box2d.MakeB2Vec2(minX, minY)
	aabb.UpperBound = box2d.MakeB2Vec2(maxX, maxY)

	seen := make(map[aabbOverlapKey]struct{})
	bp := &world.M_contactManager.M_broadPhase

	bp.Query(func(proxyID int) bool {
		ud := bp.GetUserData(proxyID)
		proxy, ok := ud.(*box2d.B2FixtureProxy)
		if !ok || proxy == nil || proxy.Fixture == nil {
			return true
		}
		fixture := proxy.Fixture
		if !filterAllows(req.Filter, fixture) {
			return true
		}
		entityID, bodyOK := BodyUserDataFrom(fixture.GetBody().GetUserData())
		if !bodyOK {
			return true
		}
		_, shapeIndex, shapeOK := FixtureUserDataFrom(fixture.GetUserData())
		if !shapeOK {
			return true
		}
		key := aabbOverlapKey{Entity: entityID, ShapeIndex: shapeIndex}
		if _, dup := seen[key]; dup {
			return true
		}

		xfB := fixture.GetBody().GetTransform()
		if !box2d.B2TestOverlapShapes(queryPoly, 0, fixture.GetShape(), proxy.ChildIndex, xfQuery, xfB) {
			return true
		}

		seen[key] = struct{}{}
		out.Hits = append(out.Hits, AABBOverlapHit{
			Entity:     entityID,
			ShapeIndex: shapeIndex,
		})
		return true
	}, aabb)

	return out
}

package query

import (
	"math"

	"github.com/ByteArena/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
)

// CircleSweepRequest sweeps a circle with center moving along the segment from Start to End in world space.
// Radius must be positive. MaxFraction is the TOI search bound in [0,1] along that segment; 0 means 1.0.
// A nil Filter uses the same defaults as RaycastRequest (all layers, solids only).
type CircleSweepRequest struct {
	Start       component.Vec2 `json:"start"`
	End         component.Vec2 `json:"end"`
	Radius      float64        `json:"radius"`
	Filter      *Filter        `json:"filter,omitempty"`
	MaxFraction float64        `json:"max_fraction"`
}

// CircleSweepResult is the closest first contact along the sweep, if any.
type CircleSweepResult struct {
	Hit        bool              `json:"hit"`
	Entity     cardinal.EntityID `json:"entity"`
	ShapeIndex int               `json:"shape_index"`
	Point      component.Vec2    `json:"point"`
	Normal     component.Vec2    `json:"normal"`
	Fraction   float64           `json:"fraction"`
}

func sweepTranslation(c0, c box2d.B2Vec2) box2d.B2Sweep {
	return box2d.B2Sweep{
		LocalCenter: box2d.MakeB2Vec2(0, 0),
		C0:          c0,
		C:           c,
		A0:          0,
		A:           0,
		Alpha0:      0,
	}
}

func circleSweepContact(
	circle *box2d.B2CircleShape,
	sweepA box2d.B2Sweep,
	fixture *box2d.B2Fixture,
	child int,
	t float64,
) (component.Vec2, component.Vec2) {
	xfA := box2d.MakeB2Transform()
	xfB := box2d.MakeB2Transform()
	sweepA.GetTransform(&xfA, t)
	sweepB := fixture.GetBody().M_sweep
	sweepB.GetTransform(&xfB, t)

	proxyA := box2d.MakeB2DistanceProxy()
	proxyA.Set(circle, 0)
	proxyB := box2d.MakeB2DistanceProxy()
	proxyB.Set(fixture.GetShape(), child)

	din := box2d.MakeB2DistanceInput()
	din.ProxyA = proxyA
	din.ProxyB = proxyB
	din.TransformA = xfA
	din.TransformB = xfB
	din.UseRadii = true
	cache := box2d.MakeB2SimplexCache()
	out := box2d.MakeB2DistanceOutput()
	box2d.B2Distance(&out, &cache, &din)

	nx := out.PointA.X - out.PointB.X
	ny := out.PointA.Y - out.PointB.Y
	d := math.Hypot(nx, ny)
	if d < 1e-10 {
		nx, ny = 0, 1
	} else {
		nx /= d
		ny /= d
	}
	return component.Vec2{X: out.PointA.X, Y: out.PointA.Y}, component.Vec2{X: nx, Y: ny}
}

// circleSweepScan holds per-query state for CircleSweep broad-phase TOI scan.
type circleSweepScan struct {
	world   *box2d.B2World
	req     CircleSweepRequest
	tMax    float64
	circle  *box2d.B2CircleShape
	sweepA  box2d.B2Sweep
	baseTOI box2d.B2TOIInput
	best    CircleSweepResult
}

func (s *circleSweepScan) broadPhaseCallback(proxyID int) bool {
	bp := &s.world.M_contactManager.M_broadPhase
	ud := bp.GetUserData(proxyID)
	proxy, ok := ud.(*box2d.B2FixtureProxy)
	if !ok || proxy == nil || proxy.Fixture == nil {
		return true
	}
	fixture := proxy.Fixture
	if !filterAllows(s.req.Filter, fixture) {
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

	toiIn := s.baseTOI
	toiIn.ProxyB = box2d.MakeB2DistanceProxy()
	toiIn.ProxyB.Set(fixture.GetShape(), proxy.ChildIndex)
	toiIn.SweepB = fixture.GetBody().M_sweep

	toiOut := box2d.MakeB2TOIOutput()
	box2d.B2TimeOfImpact(&toiOut, &toiIn)

	var frac float64
	switch toiOut.State {
	case box2d.B2TOIOutput_State.E_overlapped:
		frac = 0
	case box2d.B2TOIOutput_State.E_touching:
		frac = toiOut.T
	default:
		return true
	}

	if frac > s.tMax+circleSweepFracEpsilon {
		return true
	}
	if s.best.Hit && frac >= s.best.Fraction-circleSweepFracEpsilon {
		return true
	}

	pt, n := circleSweepContact(s.circle, s.sweepA, fixture, proxy.ChildIndex, frac)
	s.best = CircleSweepResult{
		Hit:        true,
		Entity:     entityID,
		ShapeIndex: shapeIndex,
		Point:      pt,
		Normal:     n,
		Fraction:   frac,
	}
	return true
}

// CircleSweep sweeps a circle along Start→End and returns the earliest TOI hit.
// World must be the plugin’s Box2D world.
func CircleSweep(world *box2d.B2World, req CircleSweepRequest) CircleSweepResult {
	if world == nil {
		return CircleSweepResult{}
	}
	dx := req.End.X - req.Start.X
	dy := req.End.Y - req.Start.Y
	if dx*dx+dy*dy < raycastMinLengthSq {
		return CircleSweepResult{}
	}
	if req.Radius <= 0 {
		return CircleSweepResult{}
	}
	tMax := req.MaxFraction
	if tMax <= 0 {
		tMax = 1
	}
	if tMax > 1 {
		tMax = 1
	}

	circle := box2d.NewB2CircleShape()
	circle.M_radius = req.Radius
	circle.M_p = box2d.MakeB2Vec2(0, 0)

	p0 := box2d.MakeB2Vec2(req.Start.X, req.Start.Y)
	p1 := box2d.MakeB2Vec2(req.End.X, req.End.Y)
	sweepA := sweepTranslation(p0, p1)

	proxyA := box2d.MakeB2DistanceProxy()
	proxyA.Set(circle, 0)

	r := req.Radius
	aabb := box2d.MakeB2AABB()
	aabb.LowerBound = box2d.MakeB2Vec2(
		math.Min(req.Start.X, req.End.X)-r,
		math.Min(req.Start.Y, req.End.Y)-r,
	)
	aabb.UpperBound = box2d.MakeB2Vec2(
		math.Max(req.Start.X, req.End.X)+r,
		math.Max(req.Start.Y, req.End.Y)+r,
	)

	baseTOI := box2d.MakeB2TOIInput()
	baseTOI.ProxyA = proxyA
	baseTOI.SweepA = sweepA
	baseTOI.TMax = tMax

	scan := &circleSweepScan{
		world:   world,
		req:     req,
		tMax:    tMax,
		circle:  circle,
		sweepA:  sweepA,
		baseTOI: baseTOI,
	}
	bp := &world.M_contactManager.M_broadPhase
	bp.Query(scan.broadPhaseCallback, aabb)
	return scan.best
}

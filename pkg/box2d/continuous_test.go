// Tests for the continuous-collision (CCD/bullet) port of Box2D v3.2.0
// src/solver.c (b2ContinuousQueryCallback, b2SolveContinuous and the bullet
// stage of b2Solve). External package tests: everything here goes through the
// public API.

package box2d_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// xorshift64 is a tiny deterministic PRNG so the randomized-but-hardcoded
// scenarios reproduce bit-identically on every run and architecture.
type xorshift64 struct {
	state uint64
}

func (x *xorshift64) next() uint64 {
	x.state ^= x.state << 13
	x.state ^= x.state >> 7
	x.state ^= x.state << 17
	return x.state
}

// rangeFloat returns a float64 in [lo, hi) derived from the top 53 bits of
// the generator state. The explicit float64 conversion keeps the product from
// fusing into an FMA (see math_fma.go) so golden inputs are bit-identical on
// every architecture.
func (x *xorshift64) rangeFloat(lo, hi float64) float64 {
	u := float64(x.next()>>11) / float64(uint64(1)<<53)
	return lo + float64((hi-lo)*u)
}

// makeWallWorld creates a zero-gravity world with a thin static wall centered
// at x = 0 (total thickness 0.1) spanning the given half height.
func makeWallWorld(t *testing.T, halfHeight float64) *box2d.World {
	t.Helper()

	def := box2d.DefaultWorldDef()
	def.Gravity = box2d.Vec2Zero
	w := box2d.NewWorld(&def)

	gd := box2d.DefaultBodyDef()
	ground := w.CreateBody(&gd)
	wall := box2d.MakeBox(0.05, halfHeight)
	sd := box2d.DefaultShapeDef()
	w.CreatePolygonShape(ground, &sd, &wall)

	return w
}

// spawnBulletCircle creates one fast circle body aimed at the wall. Negative
// group index -1 keeps the projectiles from colliding with each other so
// thousands can share one world on separate lanes.
func spawnBulletCircle(w *box2d.World, pos, vel box2d.Vec2, radius float64, bullet bool) box2d.BodyID {
	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bd.Position = pos
	bd.LinearVelocity = vel
	bd.IsBullet = bullet
	body := w.CreateBody(&bd)

	circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: radius}
	sd := box2d.DefaultShapeDef()
	sd.Density = 1.0
	sd.Filter.GroupIndex = -1
	w.CreateCircleShape(body, &sd, &circle)
	return body
}

// TestBulletNoTunnelThroughThinWall fires 10,000 bullet circles at roughly
// 200 m/s against a 0.1 m thick static wall with randomized-but-hardcoded
// start offsets and velocities. With continuous collision enabled, not one of
// them may end up past the wall's center plane: each must come to rest on the
// near side or in contact with the wall.
func TestBulletNoTunnelThroughThinWall(t *testing.T) {
	const bulletCount = 10000
	const radius = 0.05

	// One lane per bullet, 1 m apart, so projectiles never interact.
	w := makeWallWorld(t, float64(0.5*float64(bulletCount))+10.0)
	defer w.Destroy()

	rng := xorshift64{state: 0x9E3779B97F4A7C15}

	bodies := make([]box2d.BodyID, 0, bulletCount)
	starts := make([]box2d.Vec2, 0, bulletCount)
	vels := make([]box2d.Vec2, 0, bulletCount)
	for i := range bulletCount {
		lane := float64(i) - float64(0.5*float64(bulletCount))
		pos := box2d.Vec2{
			X: rng.rangeFloat(-6.0, -4.0),
			Y: lane + rng.rangeFloat(-0.2, 0.2),
		}
		vel := box2d.Vec2{
			X: rng.rangeFloat(150.0, 250.0),
			Y: rng.rangeFloat(-2.0, 2.0),
		}
		bodies = append(bodies, spawnBulletCircle(w, pos, vel, radius, true))
		starts = append(starts, pos)
		vels = append(vels, vel)
	}

	for range 30 {
		w.Step(1.0/60.0, 4)
	}

	tunneled := 0
	for i, body := range bodies {
		p := w.BodyPosition(body)
		if p.X >= 0.0 {
			tunneled++
			if tunneled <= 5 {
				t.Logf("tunnel event %d: start=%v vel=%v end=%v", i, starts[i], vels[i], p)
			}
		}
	}
	require.Zero(t, tunneled, "bullet bodies must never pass the wall center plane")
}

// TestFastNonBulletNoTunnelThroughStatic verifies that continuous collision
// versus static bodies applies to every fast dynamic body, not just bullets
// (upstream runs b2SolveContinuous for all fast bodies against the static
// tree).
func TestFastNonBulletNoTunnelThroughStatic(t *testing.T) {
	const count = 200
	const radius = 0.05

	w := makeWallWorld(t, float64(0.5*float64(count))+10.0)
	defer w.Destroy()

	rng := xorshift64{state: 0xDEADBEEFCAFEF00D}

	bodies := make([]box2d.BodyID, 0, count)
	for i := range count {
		lane := float64(i) - float64(0.5*float64(count))
		pos := box2d.Vec2{X: rng.rangeFloat(-6.0, -4.0), Y: lane}
		vel := box2d.Vec2{X: rng.rangeFloat(150.0, 250.0), Y: 0.0}
		bodies = append(bodies, spawnBulletCircle(w, pos, vel, radius, false))
	}

	for range 30 {
		w.Step(1.0/60.0, 4)
	}

	for i, body := range bodies {
		p := w.BodyPosition(body)
		require.Less(t, p.X, 0.0, "non-bullet fast body %d must not pass the static wall", i)
	}
}

// TestDisabledContinuousDoesTunnel is the control experiment: the exact
// anti-tunnel setup, but with continuous collision disabled, must tunnel for
// most projectiles. This proves the machinery in the other tests is doing the
// work rather than the discrete solver getting lucky.
func TestDisabledContinuousDoesTunnel(t *testing.T) {
	const count = 100
	const radius = 0.05

	w := makeWallWorld(t, float64(0.5*float64(count))+10.0)
	defer w.Destroy()
	w.EnableContinuous(false)

	rng := xorshift64{state: 0x9E3779B97F4A7C15}

	bodies := make([]box2d.BodyID, 0, count)
	for i := range count {
		lane := float64(i) - float64(0.5*float64(count))
		pos := box2d.Vec2{
			X: rng.rangeFloat(-6.0, -4.0),
			Y: lane + rng.rangeFloat(-0.2, 0.2),
		}
		vel := box2d.Vec2{
			X: rng.rangeFloat(150.0, 250.0),
			Y: rng.rangeFloat(-2.0, 2.0),
		}
		bodies = append(bodies, spawnBulletCircle(w, pos, vel, radius, true))
	}

	for range 30 {
		w.Step(1.0/60.0, 4)
	}

	tunneled := 0
	for _, body := range bodies {
		if w.BodyPosition(body).X > 1.0 {
			tunneled++
		}
	}
	require.Greater(t, tunneled, count/2,
		"with continuous disabled the thin wall must be porous to fast bodies")
}

// makePlateWorld creates a zero-gravity world with a thin heavy dynamic plate
// centered at x = 0 and returns the world plus the plate body.
func makePlateWorld() (*box2d.World, box2d.BodyID) {
	def := box2d.DefaultWorldDef()
	def.Gravity = box2d.Vec2Zero
	w := box2d.NewWorld(&def)

	pd := box2d.DefaultBodyDef()
	pd.Type = box2d.DynamicBody
	pd.Position = box2d.Vec2Zero
	plate := w.CreateBody(&pd)

	box := box2d.MakeBox(0.05, 2.0)
	sd := box2d.DefaultShapeDef()
	sd.Density = 50.0
	w.CreatePolygonShape(plate, &sd, &box)

	return w, plate
}

// TestBulletVersusThinDynamicPlate checks the upstream bullet semantics
// against dynamic bodies: only bullets get continuous collision versus the
// dynamic tree, so a bullet must be stopped by a thin dynamic plate while an
// equally fast non-bullet body passes straight through it.
func TestBulletVersusThinDynamicPlate(t *testing.T) {
	// The start offset is chosen so the discrete positions (3.333 m apart at
	// 200 m/s and 60 Hz) never land within contact range of the plate — a
	// non-bullet body can only be stopped by continuous collision, which it
	// must not receive versus a dynamic body.
	start := box2d.Vec2{X: -5.05, Y: 0.0}
	vel := box2d.Vec2{X: 200.0, Y: 0.0}

	// Bullet: continuous collision versus the dynamic plate stops it.
	{
		w, plate := makePlateWorld()
		bullet := spawnBulletCircle(w, start, vel, 0.1, true)

		for range 30 {
			w.Step(1.0/60.0, 4)
		}

		bulletX := w.BodyPosition(bullet).X
		plateX := w.BodyPosition(plate).X
		require.Less(t, bulletX, plateX,
			"bullet must stay on the near side of the dynamic plate it hit")
		w.Destroy()
	}

	// Non-bullet: no continuous collision versus dynamic bodies, so the fast
	// body tunnels through the thin plate exactly like upstream.
	{
		w, plate := makePlateWorld()
		body := spawnBulletCircle(w, start, vel, 0.1, false)

		for range 30 {
			w.Step(1.0/60.0, 4)
		}

		bodyX := w.BodyPosition(body).X
		plateX := w.BodyPosition(plate).X
		require.Greater(t, bodyX, plateX+1.0,
			"non-bullet fast body must not receive CCD versus a dynamic plate")
		w.Destroy()
	}
}

// TestSpeedCapClampsVelocity verifies the maximum linear speed clamp that
// feeds the isSpeedCapped bookkeeping: a body launched far above the world
// maximum linear speed must come out of the step clamped to it.
func TestSpeedCapClampsVelocity(t *testing.T) {
	def := box2d.DefaultWorldDef()
	def.Gravity = box2d.Vec2Zero
	def.MaximumLinearSpeed = 10.0
	w := box2d.NewWorld(&def)
	defer w.Destroy()

	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bd.Position = box2d.Vec2{X: 0.0, Y: 0.0}
	bd.LinearVelocity = box2d.Vec2{X: 100.0, Y: 0.0}
	body := w.CreateBody(&bd)

	circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.5}
	sd := box2d.DefaultShapeDef()
	sd.Density = 1.0
	w.CreateCircleShape(body, &sd, &circle)

	w.Step(1.0/60.0, 4)

	v := w.BodyLinearVelocity(body)
	speed := box2d.Length(v)
	require.LessOrEqual(t, speed, 10.0+1e-9, "velocity must be clamped to MaximumLinearSpeed")
	require.Greater(t, speed, 9.0, "clamp must not kill the velocity outright")
}

// TestBulletSlidesAlongChainWithoutSnagging drives a bullet along a chain of
// static segments under gravity. The chain one-sidedness early-out in the
// continuous query callback must keep the bullet from generating time of
// impact events against the internal chain vertices (ghost collisions), so
// the bullet has to keep sliding at essentially full speed.
func TestBulletSlidesAlongChainWithoutSnagging(t *testing.T) {
	def := box2d.DefaultWorldDef()
	def.Gravity = box2d.Vec2{X: 0.0, Y: -10.0}
	w := box2d.NewWorld(&def)
	defer w.Destroy()

	// Flat chain along y = 0. Points run from +x to -x so the one-sided
	// collision normal points up (CCW winding, normal to the right of the
	// segment direction). Internal vertices sit at x = 8, 4, 0, -4, -8.
	gd := box2d.DefaultBodyDef()
	ground := w.CreateBody(&gd)

	points := []box2d.Vec2{
		{X: 12.0, Y: 0.0},
		{X: 8.0, Y: 0.0},
		{X: 4.0, Y: 0.0},
		{X: 0.0, Y: 0.0},
		{X: -4.0, Y: 0.0},
		{X: -8.0, Y: 0.0},
		{X: -12.0, Y: 0.0},
	}
	cd := box2d.DefaultChainDef()
	cd.Points = points
	cd.Materials = []box2d.SurfaceMaterial{{Friction: 0.0}}
	w.CreateChain(ground, &cd)

	const radius = 0.25
	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bd.Position = box2d.Vec2{X: -10.0, Y: radius}
	bd.LinearVelocity = box2d.Vec2{X: 20.0, Y: 0.0}
	bd.IsBullet = true
	bullet := w.CreateBody(&bd)

	circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: radius}
	sd := box2d.DefaultShapeDef()
	sd.Density = 1.0
	sd.Material.Friction = 0.0
	w.CreateCircleShape(bullet, &sd, &circle)

	// One second of sliding: 20 m/s over the internal vertices at
	// x = -8, -4, 0, 4, 8.
	for range 60 {
		w.Step(1.0/60.0, 4)
	}

	p := w.BodyPosition(bullet)
	v := w.BodyLinearVelocity(bullet)

	// A snag on an internal vertex reflects or stops the body. Sliding
	// cleanly it must cross nearly the whole chain and keep its speed.
	require.Greater(t, p.X, 5.0, "bullet snagged on an internal chain vertex")
	require.InDelta(t, radius, p.Y, 0.5, "bullet must stay on the chain surface")
	require.Greater(t, v.X, 15.0, "bullet lost speed to ghost collisions")
}

// buildBulletStormScene fills a closed static box with high-speed bouncy
// bullets. Shared by the determinism test and the golden continuous test.
func buildBulletStormScene(w *box2d.World, count int) []box2d.BodyID {
	gd := box2d.DefaultBodyDef()
	ground := w.CreateBody(&gd)
	gsd := box2d.DefaultShapeDef()

	// Thin walls: floor, ceiling, left, right (0.1 thick).
	walls := []struct {
		center box2d.Vec2
		hx, hy float64
	}{
		{box2d.Vec2{X: 0.0, Y: -10.0}, 10.1, 0.05},
		{box2d.Vec2{X: 0.0, Y: 10.0}, 10.1, 0.05},
		{box2d.Vec2{X: -10.0, Y: 0.0}, 0.05, 10.1},
		{box2d.Vec2{X: 10.0, Y: 0.0}, 0.05, 10.1},
	}
	for i := range walls {
		wallBox := box2d.MakeOffsetBox(walls[i].hx, walls[i].hy, walls[i].center, box2d.RotIdentity)
		w.CreatePolygonShape(ground, &gsd, &wallBox)
	}

	rng := xorshift64{state: 0x0123456789ABCDEF}

	bodies := make([]box2d.BodyID, 0, count)
	for range count {
		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.Position = box2d.Vec2{
			X: rng.rangeFloat(-8.0, 8.0),
			Y: rng.rangeFloat(-8.0, 8.0),
		}
		bd.LinearVelocity = box2d.Vec2{
			X: rng.rangeFloat(-80.0, 80.0),
			Y: rng.rangeFloat(-80.0, 80.0),
		}
		bd.IsBullet = true
		body := w.CreateBody(&bd)

		circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.1}
		sd := box2d.DefaultShapeDef()
		sd.Density = 1.0
		sd.Material.Restitution = 1.0
		sd.Material.Friction = 0.0
		w.CreateCircleShape(body, &sd, &circle)
		bodies = append(bodies, body)
	}

	return bodies
}

// TestContinuousDeterminism runs the same bullet-heavy scene in two worlds
// for 300 steps and demands bit-identical state hashes every 50 steps. The
// serial bullet loop and the deterministic bulletBodies fill order make the
// continuous stage reproducible.
func TestContinuousDeterminism(t *testing.T) {
	def1 := box2d.DefaultWorldDef()
	w1 := box2d.NewWorld(&def1)
	defer w1.Destroy()

	def2 := box2d.DefaultWorldDef()
	w2 := box2d.NewWorld(&def2)
	defer w2.Destroy()

	bodies1 := buildBulletStormScene(w1, 30)
	bodies2 := buildBulletStormScene(w2, 30)

	for step := 1; step <= 300; step++ {
		w1.Step(1.0/60.0, 4)
		w2.Step(1.0/60.0, 4)

		if step%50 == 0 {
			h1 := hashWorldState(w1, bodies1)
			h2 := hashWorldState(w2, bodies2)
			require.Equal(t, fmt.Sprintf("%016x", h1), fmt.Sprintf("%016x", h2),
				"bullet-heavy worlds diverged at step %d", step)
		}
	}

	// No bullet may have escaped the box either — a divergent-but-equal pair
	// of tunneling worlds would still hash identically.
	for i, body := range bodies1 {
		p := w1.BodyPosition(body)
		require.Less(t, absDelta(p.X), 10.2, "bullet %d escaped the box (x)", i)
		require.Less(t, absDelta(p.Y), 10.2, "bullet %d escaped the box (y)", i)
	}
}

func absDelta(v float64) float64 {
	if v < 0.0 {
		return -v
	}
	return v
}

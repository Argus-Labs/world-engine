// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file
// src/physics_world.c (b2World_Explode and its ExplosionContext/ExplosionCallback
// helpers).
//
// Coordinate note: b2DefaultExplosionDef is NOT here. Upstream defines it next
// to the joint defaults in src/joint.c (b2ExplosionDef shipped with the joint
// API in v3.1), so this port keeps DefaultExplosionDef in joint.go to preserve
// the file-for-file mapping.
//
// DESIGN DEVIATION (approved, see world.go): upstream resolves the world
// through the global b2_worlds array via b2GetWorldFromId. This port has no
// world registry, so b2World_Explode becomes a method on *World and the
// receiver replaces the lookup. The `void* context` of the tree callback
// becomes an `any` value carrying *explosionContext.

package box2d

// explosionContext is the callback context for World.Explode
// (upstream struct ExplosionContext).
type explosionContext struct {
	world            *World
	position         Vec2
	radius           float64
	falloff          float64
	impulsePerLength float64
}

// explosionCallback is the dynamic tree callback used by World.Explode
// (upstream static ExplosionCallback).
func explosionCallback(proxyID int, userData uint64, context any) bool {
	_ = proxyID

	shapeID := int(userData)

	explosion, ok := context.(*explosionContext)
	assert(ok)
	if !ok {
		return false
	}
	world := explosion.world

	s := &world.shapes[shapeID]

	b := &world.bodies[s.bodyID]
	assert(b.bodyType == DynamicBody)

	transform := world.getBodyTransformQuick(b)

	var input DistanceInput
	input.ProxyA = makeShapeDistanceProxy(s)
	input.ProxyB = MakeProxy([]Vec2{explosion.position}, 1, 0.0)
	input.TransformA = transform
	input.TransformB = TransformIdentity
	input.UseRadii = true

	var cache SimplexCache
	output := ShapeDistance(&input, &cache, nil)

	radius := explosion.radius
	falloff := explosion.falloff
	if output.Distance > radius+falloff {
		return true
	}

	world.wakeBody(b)

	if b.setIndex != awakeSet {
		return true
	}

	closestPoint := output.PointA
	if output.Distance == 0.0 {
		localCentroid := getShapeCentroid(s)
		closestPoint = TransformPoint(transform, localCentroid)
	}

	direction := Sub(closestPoint, explosion.position)
	if LengthSquared(direction) > 100.0*epsilon*epsilon {
		direction = Normalize(direction)
	} else {
		direction = Vec2{X: 1.0, Y: 0.0}
	}

	localLine := InvRotateVector(transform.Q, LeftPerp(direction))
	perimeter := getShapeProjectedPerimeter(s, localLine)
	scale := 1.0
	if output.Distance > radius && falloff > 0.0 {
		scale = clampFloat((radius+falloff-output.Distance)/falloff, 0.0, 1.0)
	}

	magnitude := explosion.impulsePerLength * perimeter * scale
	impulse := MulSV(magnitude, direction)

	localIndex := b.localIndex
	set := &world.solverSets[awakeSet]
	state := &set.bodyStates[localIndex]
	sim := &set.bodySims[localIndex]
	state.linearVelocity = MulAdd(state.linearVelocity, sim.invMass, impulse)
	state.angularVelocity = mulAdd(sim.invInertia, Cross(Sub(closestPoint, sim.center), impulse),
		state.angularVelocity)

	return true
}

// Explode applies a radial explosion (upstream b2World_Explode).
//
// The explosion only affects dynamic body shapes whose category bits pass the
// definition mask bits. The impulse is proportional to the shape perimeter
// projected onto the plane perpendicular to the explosion direction, so larger
// shapes receive larger impulses. A negative ImpulsePerLength implodes.
func (w *World) Explode(explosionDef *ExplosionDef) {
	maskBits := explosionDef.MaskBits
	position := explosionDef.Position
	radius := explosionDef.Radius
	falloff := explosionDef.Falloff
	impulsePerLength := explosionDef.ImpulsePerLength

	assert(IsValidVec2(position))
	assert(IsValidFloat(radius) && radius >= 0.0)
	assert(IsValidFloat(falloff) && falloff >= 0.0)
	assert(IsValidFloat(impulsePerLength))

	assert(!w.locked)
	if w.locked {
		return
	}

	explosion := explosionContext{
		world:            w,
		position:         position,
		radius:           radius,
		falloff:          falloff,
		impulsePerLength: impulsePerLength,
	}

	var aabb AABB
	aabb.LowerBound.X = position.X - (radius + falloff)
	aabb.LowerBound.Y = position.Y - (radius + falloff)
	aabb.UpperBound.X = position.X + (radius + falloff)
	aabb.UpperBound.Y = position.Y + (radius + falloff)

	w.broadPhase.trees[DynamicBody].Query(aabb, maskBits, explosionCallback, &explosion)
}

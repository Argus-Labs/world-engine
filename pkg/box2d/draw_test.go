// Tests for the float64 port of b2World_Draw and its helpers
// (src/physics_world.c debug draw section, pkg/box2d/draw.go).

package box2d_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// drawRecord holds one recorded debug draw callback invocation. Only the
// fields a test needs are kept; the rest stay at their zero value.
type drawRecord struct {
	vertices []box2d.Vec2
	text     string
	p1       box2d.Vec2
	p2       box2d.Vec2
	xf       box2d.Transform
	radius   float64
	size     float64
	color    box2d.HexColor
}

// drawRecorder is a DebugDraw whose callbacks append to per-primitive slices.
type drawRecorder struct {
	polygons      []drawRecord
	solidPolygons []drawRecord
	circles       []drawRecord
	solidCircles  []drawRecord
	solidCapsules []drawRecord
	lines         []drawRecord
	transforms    []drawRecord
	points        []drawRecord
	strings       []drawRecord
}

// debugDraw returns a DebugDraw wired to this recorder. Every option is off;
// each test turns on exactly the flags it exercises.
func (r *drawRecorder) debugDraw() box2d.DebugDraw {
	draw := box2d.DefaultDebugDraw()
	draw.DrawShapes = false

	draw.DrawPolygonFcn = func(vertices []box2d.Vec2, color box2d.HexColor, _ any) {
		r.polygons = append(r.polygons, drawRecord{vertices: append([]box2d.Vec2(nil), vertices...), color: color})
	}
	draw.DrawSolidPolygonFcn = func(xf box2d.Transform, vertices []box2d.Vec2, radius float64,
		color box2d.HexColor, _ any,
	) {
		r.solidPolygons = append(r.solidPolygons, drawRecord{
			vertices: append([]box2d.Vec2(nil), vertices...),
			xf:       xf,
			radius:   radius,
			color:    color,
		})
	}
	draw.DrawCircleFcn = func(center box2d.Vec2, radius float64, color box2d.HexColor, _ any) {
		r.circles = append(r.circles, drawRecord{p1: center, radius: radius, color: color})
	}
	draw.DrawSolidCircleFcn = func(xf box2d.Transform, radius float64, color box2d.HexColor, _ any) {
		r.solidCircles = append(r.solidCircles, drawRecord{xf: xf, radius: radius, color: color})
	}
	draw.DrawSolidCapsuleFcn = func(p1, p2 box2d.Vec2, radius float64, color box2d.HexColor, _ any) {
		r.solidCapsules = append(r.solidCapsules, drawRecord{p1: p1, p2: p2, radius: radius, color: color})
	}
	draw.DrawLineFcn = func(p1, p2 box2d.Vec2, color box2d.HexColor, _ any) {
		r.lines = append(r.lines, drawRecord{p1: p1, p2: p2, color: color})
	}
	draw.DrawTransformFcn = func(xf box2d.Transform, _ any) {
		r.transforms = append(r.transforms, drawRecord{xf: xf})
	}
	draw.DrawPointFcn = func(p box2d.Vec2, size float64, color box2d.HexColor, _ any) {
		r.points = append(r.points, drawRecord{p1: p, size: size, color: color})
	}
	draw.DrawStringFcn = func(p box2d.Vec2, s string, color box2d.HexColor, _ any) {
		r.strings = append(r.strings, drawRecord{p1: p, text: s, color: color})
	}

	return draw
}

// The draw scene holds one shape of every drawable type plus one far-away box
// used for the drawing-bounds culling checks:
//
//	ground : static  polygon at (0, 0)
//	ball   : dynamic circle  at (0, 5)
//	pill   : dynamic capsule at (3, 5)
//	wall   : static  segment at (-8, 0)..(-8, 3)
//	far    : dynamic polygon at (1000, 1000)   (outside the culled bounds)
const (
	drawSceneNearPolygons = 1
	drawSceneNearCircles  = 1
	drawSceneNearCapsules = 1
	drawSceneNearSegments = 1
	drawSceneNearShapes   = drawSceneNearPolygons + drawSceneNearCircles + drawSceneNearCapsules + drawSceneNearSegments

	drawSceneAllPolygons = drawSceneNearPolygons + 1
	drawSceneAllShapes   = drawSceneNearShapes + 1
)

// drawSceneBounds is the culled view: it contains everything except the far
// box. The far box sits at (1000, 1000).
var drawSceneBounds = box2d.AABB{
	LowerBound: box2d.Vec2{X: -50.0, Y: -50.0},
	UpperBound: box2d.Vec2{X: 50.0, Y: 50.0},
}

// buildDrawWorld creates the draw scene. The world is never stepped, so every
// body keeps its creation transform.
func buildDrawWorld(t *testing.T) *box2d.World {
	t.Helper()

	worldDef := box2d.DefaultWorldDef()
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	shapeDef := box2d.DefaultShapeDef()

	groundDef := box2d.DefaultBodyDef()
	groundDef.Name = "ground"
	groundDef.Position = box2d.Vec2{X: 0.0, Y: 0.0}
	groundID := world.CreateBody(&groundDef)
	groundBox := box2d.MakeBox(10.0, 0.5)
	world.CreatePolygonShape(groundID, &shapeDef, &groundBox)

	ballDef := box2d.DefaultBodyDef()
	ballDef.Name = "ball"
	ballDef.Type = box2d.DynamicBody
	ballDef.Position = box2d.Vec2{X: 0.0, Y: 5.0}
	ballID := world.CreateBody(&ballDef)
	ball := box2d.Circle{Center: box2d.Vec2{X: 0.0, Y: 0.0}, Radius: 0.5}
	world.CreateCircleShape(ballID, &shapeDef, &ball)

	pillDef := box2d.DefaultBodyDef()
	pillDef.Name = "pill"
	pillDef.Type = box2d.DynamicBody
	pillDef.Position = box2d.Vec2{X: 3.0, Y: 5.0}
	pillID := world.CreateBody(&pillDef)
	pill := box2d.Capsule{
		Center1: box2d.Vec2{X: -0.5, Y: 0.0},
		Center2: box2d.Vec2{X: 0.5, Y: 0.0},
		Radius:  0.25,
	}
	world.CreateCapsuleShape(pillID, &shapeDef, &pill)

	wallDef := box2d.DefaultBodyDef()
	wallDef.Name = "wall"
	wallDef.Position = box2d.Vec2{X: -8.0, Y: 0.0}
	wallID := world.CreateBody(&wallDef)
	wall := box2d.Segment{Point1: box2d.Vec2{X: 0.0, Y: 0.0}, Point2: box2d.Vec2{X: 0.0, Y: 3.0}}
	world.CreateSegmentShape(wallID, &shapeDef, &wall)

	farDef := box2d.DefaultBodyDef()
	farDef.Name = "far"
	farDef.Type = box2d.DynamicBody
	farDef.Position = box2d.Vec2{X: 1000.0, Y: 1000.0}
	farID := world.CreateBody(&farDef)
	farBox := box2d.MakeBox(0.5, 0.5)
	world.CreatePolygonShape(farID, &shapeDef, &farBox)

	return world
}

// TestDrawShapesByType checks that every shape in the scene reaches the
// callback matching its type, with the geometry the scene created.
func TestDrawShapesByType(t *testing.T) {
	t.Parallel()

	world := buildDrawWorld(t)

	var recorder drawRecorder
	draw := recorder.debugDraw()
	draw.DrawShapes = true
	world.Draw(&draw)

	require.Len(t, recorder.solidPolygons, drawSceneAllPolygons)
	require.Len(t, recorder.solidCircles, drawSceneNearCircles)
	require.Len(t, recorder.solidCapsules, drawSceneNearCapsules)
	require.Len(t, recorder.lines, drawSceneNearSegments)

	// The ground box is the only 4-vertex polygon spanning +-10 in x.
	assert.Len(t, recorder.solidPolygons[0].vertices, 4)

	assert.InDelta(t, 0.5, recorder.solidCircles[0].radius, 0.0)
	assert.InDelta(t, 0.0, recorder.solidCircles[0].xf.P.X, 1e-12)
	assert.InDelta(t, 5.0, recorder.solidCircles[0].xf.P.Y, 1e-12)

	assert.InDelta(t, 0.25, recorder.solidCapsules[0].radius, 0.0)
	assert.InDelta(t, 2.5, recorder.solidCapsules[0].p1.X, 1e-12)
	assert.InDelta(t, 3.5, recorder.solidCapsules[0].p2.X, 1e-12)

	// The segment is drawn in world space.
	assert.InDelta(t, -8.0, recorder.lines[0].p1.X, 1e-12)
	assert.InDelta(t, 0.0, recorder.lines[0].p1.Y, 1e-12)
	assert.InDelta(t, -8.0, recorder.lines[0].p2.X, 1e-12)
	assert.InDelta(t, 3.0, recorder.lines[0].p2.Y, 1e-12)

	// Nothing else was drawn: every other option is off.
	assert.Empty(t, recorder.polygons)
	assert.Empty(t, recorder.circles)
	assert.Empty(t, recorder.transforms)
	assert.Empty(t, recorder.points)
	assert.Empty(t, recorder.strings)
}

// TestDrawShapesDisabled checks that the DrawShapes flag gates all shape
// rendering.
func TestDrawShapesDisabled(t *testing.T) {
	t.Parallel()

	world := buildDrawWorld(t)

	var recorder drawRecorder
	draw := recorder.debugDraw()
	world.Draw(&draw)

	assert.Empty(t, recorder.solidPolygons)
	assert.Empty(t, recorder.solidCircles)
	assert.Empty(t, recorder.solidCapsules)
	assert.Empty(t, recorder.lines)
}

// TestDrawBoundsCulling checks that shrinking DrawingBounds removes the
// off-screen body, and that the default (effectively infinite) bounds draw
// everything.
func TestDrawBoundsCulling(t *testing.T) {
	t.Parallel()

	world := buildDrawWorld(t)

	var culled drawRecorder
	culledDraw := culled.debugDraw()
	culledDraw.DrawShapes = true
	culledDraw.DrawingBounds = drawSceneBounds
	world.Draw(&culledDraw)

	culledShapes := len(culled.solidPolygons) + len(culled.solidCircles) + len(culled.solidCapsules) + len(culled.lines)
	assert.Equal(t, drawSceneNearShapes, culledShapes)
	assert.Len(t, culled.solidPolygons, drawSceneNearPolygons)

	// The default drawing bounds keep the upstream FLT_MAX magnitude, so
	// nothing is culled.
	var all drawRecorder
	allDraw := all.debugDraw()
	allDraw.DrawShapes = true
	world.Draw(&allDraw)

	allShapes := len(all.solidPolygons) + len(all.solidCircles) + len(all.solidCapsules) + len(all.lines)
	assert.Equal(t, drawSceneAllShapes, allShapes)
	assert.Len(t, all.solidPolygons, drawSceneAllPolygons)
	assert.Greater(t, allShapes, culledShapes, "the culled view must draw fewer shapes")
}

// TestDrawBoundsFlag checks that DrawBounds emits one gold AABB outline per
// visible shape.
func TestDrawBoundsFlag(t *testing.T) {
	t.Parallel()

	world := buildDrawWorld(t)

	var recorder drawRecorder
	draw := recorder.debugDraw()
	draw.DrawBounds = true
	draw.DrawingBounds = drawSceneBounds
	world.Draw(&draw)

	require.Len(t, recorder.polygons, drawSceneNearShapes)
	for _, record := range recorder.polygons {
		assert.Len(t, record.vertices, 4)
		assert.Equal(t, box2d.ColorGold, record.color)
	}
}

// TestDrawMass checks that DrawMass emits a centre-of-mass line, a transform
// and a mass label for each dynamic body, and nothing for static bodies.
func TestDrawMass(t *testing.T) {
	t.Parallel()

	world := buildDrawWorld(t)

	var recorder drawRecorder
	draw := recorder.debugDraw()
	draw.DrawMass = true
	draw.DrawingBounds = drawSceneBounds
	world.Draw(&draw)

	// The ball and the pill are the only dynamic bodies inside the bounds.
	const dynamicInBounds = 2

	assert.Len(t, recorder.lines, dynamicInBounds)
	assert.Len(t, recorder.transforms, dynamicInBounds)
	require.Len(t, recorder.strings, dynamicInBounds)

	for _, record := range recorder.strings {
		assert.Equal(t, box2d.ColorWhite, record.color)
		assert.Contains(t, record.text, ".", "the mass label keeps two decimals")
	}
}

// TestDrawBodyNames checks that DrawBodyNames labels every named body inside
// the drawing bounds.
func TestDrawBodyNames(t *testing.T) {
	t.Parallel()

	world := buildDrawWorld(t)

	var recorder drawRecorder
	draw := recorder.debugDraw()
	draw.DrawBodyNames = true
	draw.DrawingBounds = drawSceneBounds
	world.Draw(&draw)

	names := make([]string, 0, len(recorder.strings))
	for _, record := range recorder.strings {
		assert.Equal(t, box2d.ColorBlueViolet, record.color)
		names = append(names, record.text)
	}

	assert.ElementsMatch(t, []string{"ground", "ball", "pill", "wall"}, names)
}

// TestDrawIslands checks that DrawIslands emits one orange-red AABB outline
// per island of the awake dynamic bodies.
func TestDrawIslands(t *testing.T) {
	t.Parallel()

	world := buildDrawWorld(t)

	var recorder drawRecorder
	draw := recorder.debugDraw()
	draw.DrawIslands = true
	draw.DrawingBounds = drawSceneBounds
	world.Draw(&draw)

	// The ball and the pill are not touching, so they are separate islands.
	// Static bodies have no island.
	require.Len(t, recorder.polygons, 2)
	for _, record := range recorder.polygons {
		assert.Len(t, record.vertices, 4)
		assert.Equal(t, box2d.ColorOrangeRed, record.color)
	}
}

// buildContactDrawWorld drops a box onto a ground box and steps until the
// contact is touching, so the contact draw options have something to draw.
func buildContactDrawWorld(t *testing.T) *box2d.World {
	t.Helper()

	worldDef := box2d.DefaultWorldDef()
	// Keep the box awake so the contact stays in a constraint graph color.
	worldDef.EnableSleep = false
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	shapeDef := box2d.DefaultShapeDef()

	groundDef := box2d.DefaultBodyDef()
	groundID := world.CreateBody(&groundDef)
	groundBox := box2d.MakeBox(10.0, 0.5)
	world.CreatePolygonShape(groundID, &shapeDef, &groundBox)

	boxDef := box2d.DefaultBodyDef()
	boxDef.Type = box2d.DynamicBody
	boxDef.Position = box2d.Vec2{X: 0.0, Y: 1.0}
	boxID := world.CreateBody(&boxDef)
	box := box2d.MakeBox(0.5, 0.5)
	world.CreatePolygonShape(boxID, &shapeDef, &box)

	for range 60 {
		world.Step(1.0/60.0, 4)
	}

	require.Positive(t, world.Counters().ContactCount, "the scene must have a contact to draw")

	return world
}

// TestDrawContactPoints checks that each contact draw type emits one point per
// manifold point, and that DrawContactsNone emits nothing.
func TestDrawContactPoints(t *testing.T) {
	t.Parallel()

	world := buildContactDrawWorld(t)

	var none drawRecorder
	noneDraw := none.debugDraw()
	noneDraw.ContactDrawType = box2d.DrawContactsNone
	world.Draw(&noneDraw)
	assert.Empty(t, none.points)

	drawTypes := []box2d.ContactDrawType{
		box2d.DrawContactsClip,
		box2d.DrawContactsAnchorA,
		box2d.DrawContactsAnchorB,
		box2d.DrawContactsAverage,
	}

	for _, drawType := range drawTypes {
		var recorder drawRecorder
		draw := recorder.debugDraw()
		draw.ContactDrawType = drawType
		world.Draw(&draw)

		// A resting box on a flat ground has a two point manifold.
		assert.Len(t, recorder.points, 2, "contact draw type %d", drawType)
	}
}

// TestDrawContactNormals checks that DrawContactNormals adds one line and one
// separation label per contact point.
func TestDrawContactNormals(t *testing.T) {
	t.Parallel()

	world := buildContactDrawWorld(t)

	var recorder drawRecorder
	draw := recorder.debugDraw()
	draw.ContactDrawType = box2d.DrawContactsClip
	draw.DrawContactNormals = true
	world.Draw(&draw)

	pointCount := len(recorder.points)
	require.Positive(t, pointCount)
	assert.Len(t, recorder.lines, pointCount)
	assert.Len(t, recorder.strings, pointCount)

	for _, record := range recorder.lines {
		assert.Equal(t, box2d.ColorDimGray, record.color)
	}
}

// TestDrawContactForces checks the impulse and friction force annotations.
func TestDrawContactForces(t *testing.T) {
	t.Parallel()

	world := buildContactDrawWorld(t)

	var recorder drawRecorder
	draw := recorder.debugDraw()
	draw.ContactDrawType = box2d.DrawContactsClip
	draw.DrawContactForces = true
	draw.DrawFrictionForces = true
	draw.DrawContactFeatures = true
	world.Draw(&draw)

	pointCount := len(recorder.points)
	require.Positive(t, pointCount)

	// One impulse line and one friction line per contact point.
	assert.Len(t, recorder.lines, 2*pointCount)

	// One impulse label, one feature id label and one friction label per point.
	assert.Len(t, recorder.strings, 3*pointCount)

	impulseLines := 0
	frictionLines := 0
	for _, record := range recorder.lines {
		if record.color == box2d.ColorMagenta {
			impulseLines++
		}
		if record.color == box2d.ColorYellow {
			frictionLines++
		}
	}
	assert.Equal(t, pointCount, impulseLines)
	assert.Equal(t, pointCount, frictionLines)
}

// TestDrawGraphColors checks that DrawGraphColors switches contact points to
// the constraint graph palette.
func TestDrawGraphColors(t *testing.T) {
	t.Parallel()

	world := buildContactDrawWorld(t)

	var plain drawRecorder
	plainDraw := plain.debugDraw()
	plainDraw.ContactDrawType = box2d.DrawContactsClip
	world.Draw(&plainDraw)

	var graph drawRecorder
	graphDraw := graph.debugDraw()
	graphDraw.ContactDrawType = box2d.DrawContactsClip
	graphDraw.DrawGraphColors = true
	world.Draw(&graphDraw)

	require.NotEmpty(t, graph.points)
	require.Len(t, graph.points, len(plain.points))

	// A resting contact lives in a constraint graph color, so the graph
	// palette must replace the add/persist/speculative colors.
	differs := false
	for i, record := range graph.points {
		// The graph point size is 5 for a normal color, 7.5 for overflow.
		assert.True(t, record.size == 5.0 || record.size == 7.5, "unexpected graph color point size")
		if record.color != plain.points[i].color {
			differs = true
		}
	}
	assert.True(t, differs, "DrawGraphColors must change at least one contact point color")
}

// TestDrawEmptyWorld checks that drawing a world with no bodies is a no-op
// rather than a panic.
func TestDrawEmptyWorld(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	var recorder drawRecorder
	draw := recorder.debugDraw()
	draw.DrawShapes = true
	draw.DrawBounds = true
	draw.DrawMass = true
	draw.DrawBodyNames = true
	draw.DrawIslands = true
	draw.ContactDrawType = box2d.DrawContactsClip
	world.Draw(&draw)

	assert.Empty(t, recorder.solidPolygons)
	assert.Empty(t, recorder.polygons)
	assert.Empty(t, recorder.points)
	assert.Empty(t, recorder.strings)
}

// TestDrawJoints locks in the b2DrawJoint port: with DrawJoints enabled, a
// scene containing a revolute and a distance joint must emit joint drawing
// primitives (anchor lines/points at minimum), and DrawJointExtras must add
// to the output rather than replace it.
func TestDrawJoints(t *testing.T) {
	t.Parallel()

	def := box2d.DefaultWorldDef()
	w := box2d.NewWorld(&def)
	defer w.Destroy()

	ground := func() box2d.BodyID {
		bd := box2d.DefaultBodyDef()
		bd.Position = box2d.Vec2{X: 0, Y: 0}
		return w.CreateBody(&bd)
	}()

	makeBox := func(pos box2d.Vec2) box2d.BodyID {
		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.Position = pos
		body := w.CreateBody(&bd)
		sd := box2d.DefaultShapeDef()
		poly := box2d.MakeBox(0.25, 0.25)
		w.CreatePolygonShape(body, &sd, &poly)
		return body
	}

	swinger := makeBox(box2d.Vec2{X: 1, Y: 0})
	hanger := makeBox(box2d.Vec2{X: 0, Y: -2})

	rjd := box2d.DefaultRevoluteJointDef()
	rjd.Base.BodyIDA = ground
	rjd.Base.BodyIDB = swinger
	w.CreateRevoluteJoint(&rjd)

	djd := box2d.DefaultDistanceJointDef()
	djd.Base.BodyIDA = ground
	djd.Base.BodyIDB = hanger
	djd.Length = 2.0
	w.CreateDistanceJoint(&djd)

	w.Step(1.0/60.0, 4)

	var base drawRecorder
	drawBase := base.debugDraw()
	drawBase.DrawJoints = true
	w.Draw(&drawBase)
	baseTotal := len(base.lines) + len(base.points) + len(base.circles) +
		len(base.solidCircles) + len(base.transforms) + len(base.strings)
	require.Positive(t, baseTotal, "DrawJoints must emit primitives for revolute+distance joints")

	var off drawRecorder
	drawOff := off.debugDraw()
	drawOff.DrawJoints = false
	w.Draw(&drawOff)
	offTotal := len(off.lines) + len(off.points) + len(off.circles) +
		len(off.solidCircles) + len(off.transforms) + len(off.strings)
	require.Zero(t, offTotal, "no joint primitives when DrawJoints is off")

	var extras drawRecorder
	drawExtras := extras.debugDraw()
	drawExtras.DrawJoints = true
	drawExtras.DrawJointExtras = true
	w.Draw(&drawExtras)
	extrasTotal := len(extras.lines) + len(extras.points) + len(extras.circles) +
		len(extras.solidCircles) + len(extras.transforms) + len(extras.strings)
	require.GreaterOrEqual(t, extrasTotal, baseTotal, "extras must not reduce joint drawing output")
}

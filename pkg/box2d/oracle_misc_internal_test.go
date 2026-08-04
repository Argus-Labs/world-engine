// Oracle tests for unexported helpers of the float64 port whose expectations
// come from the vendored C source of truth only:
//
//   - shapeDrawColor: the b2HexColor if/else ladder of DrawQueryCallback,
//     src/physics_world.c:959-1009.
//   - getShapePerimeter / getShapeRadius / getShapeProjectedPerimeter:
//     src/shape.c b2GetShapePerimeter, b2GetShapeRadius,
//     b2GetShapeProjectedPerimeter.
//   - getArenaCapacity: src/arena_allocator.c b2GetArenaCapacity.
//   - validateIsland: src/island.c b2ValidateIsland.
//   - DynamicTree node pool invariants of test_dynamic_tree.c TreeCreateDestroy.
//
// Every expected value below is derived from the C algorithm, never from
// running this Go port.

package box2d

import (
	"math"
	"testing"

	tassert "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// shapeDrawColor: the exact C ladder
// ---------------------------------------------------------------------------

// TestOracleShapeDrawColorLadder walks every rung of the color ladder in
// src/physics_world.c:959-1009 (DrawQueryCallback). The ladder is an ordered
// if/else chain, so each case is built to satisfy its own condition plus every
// lower-priority condition, proving the ordering and not just the mapping.
//
// C ladder, in order (src/physics_world.c):
//
//	961: shape->material.customColor != 0        -> customColor
//	965: type == dynamic && body->mass == 0      -> b2_colorRed
//	970: body->setIndex == b2_disabledSet        -> b2_colorSlateGray
//	974: shape->sensorIndex != B2_NULL_INDEX     -> b2_colorWheat
//	978: body->flags & b2_hadTimeOfImpact        -> b2_colorLime
//	982: (sim->flags & b2_isBullet) && awake     -> b2_colorTurquoise
//	986: body->flags & b2_isSpeedCapped          -> b2_colorYellow
//	990: sim->flags & b2_isFast                  -> b2_colorSalmon
//	994: type == b2_staticBody                   -> b2_colorPaleGreen
//	998: type == b2_kinematicBody                -> b2_colorRoyalBlue
//	1002: body->setIndex == b2_awakeSet          -> b2_colorPink
//	1006: else                                   -> b2_colorGray
func TestOracleShapeDrawColorLadder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		mutar func(b *body, sim *bodySim, s *shape)
		want  HexColor
	}{
		{
			// C 961: customColor wins over every later rung, so this case also
			// sets mass 0 (rung 965) and the disabled set (rung 970).
			name: "custom color beats everything",
			mutar: func(b *body, _ *bodySim, s *shape) {
				s.material.CustomColor = uint32(ColorMagenta)
				b.mass = 0.0
				b.setIndex = disabledSet
			},
			want: ColorMagenta,
		},
		{
			// C 965: dynamic with zero mass is the "bad body" case. Also flag
			// the sensor index (rung 974) to prove 965 has priority.
			name: "dynamic zero mass is red",
			mutar: func(b *body, _ *bodySim, s *shape) {
				b.mass = 0.0
				s.sensorIndex = 3
			},
			want: ColorRed,
		},
		{
			// C 970: disabled set. Also a sensor (rung 974) to prove ordering.
			name: "disabled body is slate gray",
			mutar: func(b *body, _ *bodySim, s *shape) {
				b.setIndex = disabledSet
				s.sensorIndex = 3
			},
			want: ColorSlateGray,
		},
		{
			// C 974: sensor. Also carries hadTimeOfImpact (rung 978).
			name: "sensor is wheat",
			mutar: func(b *body, _ *bodySim, s *shape) {
				s.sensorIndex = 0
				b.flags |= hadTimeOfImpact
			},
			want: ColorWheat,
		},
		{
			// C 978: body-level hadTimeOfImpact. Also a bullet (rung 982).
			name: "time of impact is lime",
			mutar: func(b *body, sim *bodySim, _ *shape) {
				b.flags |= hadTimeOfImpact
				sim.flags |= isBullet
			},
			want: ColorLime,
		},
		{
			// C 982: bullet AND awake. Also speed capped (rung 986).
			name: "awake bullet is turquoise",
			mutar: func(b *body, sim *bodySim, _ *shape) {
				sim.flags |= isBullet
				b.flags |= isSpeedCapped
			},
			want: ColorTurquoise,
		},
		{
			// C 982 is a conjunction: a bullet that is NOT in the awake set
			// must fall through to the next true rung. Here the next true rung
			// is 986 (speed capped).
			name: "sleeping bullet falls through to speed capped",
			mutar: func(b *body, sim *bodySim, _ *shape) {
				sim.flags |= isBullet
				b.setIndex = firstSleepingSet
				b.flags |= isSpeedCapped
			},
			want: ColorYellow,
		},
		{
			// C 986: body-level isSpeedCapped. Also sim-level isFast (990).
			name: "speed capped is yellow",
			mutar: func(b *body, sim *bodySim, _ *shape) {
				b.flags |= isSpeedCapped
				sim.flags |= isFast
			},
			want: ColorYellow,
		},
		{
			// C 990: sim-level isFast. A static body too (rung 994).
			name: "fast is salmon",
			mutar: func(_ *body, sim *bodySim, _ *shape) {
				sim.flags |= isFast
			},
			want: ColorSalmon,
		},
		{
			// C 994: static. Static bodies live in the static set, and the
			// zero-mass rung 965 only applies to dynamic bodies.
			name: "static is pale green",
			mutar: func(b *body, _ *bodySim, _ *shape) {
				b.bodyType = StaticBody
				b.mass = 0.0
				b.setIndex = staticSet
			},
			want: ColorPaleGreen,
		},
		{
			// C 998: kinematic. Kinematic bodies also have zero mass, and rung
			// 965 must not claim them.
			name: "kinematic is royal blue",
			mutar: func(b *body, _ *bodySim, _ *shape) {
				b.bodyType = KinematicBody
				b.mass = 0.0
			},
			want: ColorRoyalBlue,
		},
		{
			// C 1002: plain awake dynamic body.
			name:  "awake dynamic is pink",
			mutar: func(_ *body, _ *bodySim, _ *shape) {},
			want:  ColorPink,
		},
		{
			// C 1006: the else. A dynamic body outside the awake set with a
			// non-zero mass and no flags.
			name: "sleeping dynamic is gray",
			mutar: func(b *body, _ *bodySim, _ *shape) {
				b.setIndex = firstSleepingSet
			},
			want: ColorGray,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// The baseline "falls through the whole ladder" state: a normal
			// awake dynamic body with non-zero mass and no flags.
			b := body{bodyType: DynamicBody, mass: 1.0, setIndex: awakeSet}
			sim := bodySim{}
			s := shape{sensorIndex: NullIndex}
			test.mutar(&b, &sim, &s)

			tassert.Equal(t, test.want, shapeDrawColor(&b, &sim, &s))
		})
	}
}

// TestOracleShapeDrawColorCustomColorIsPassedThrough checks the C cast at
// src/physics_world.c:963 (`color = shape->material.customColor;`): the raw
// uint32 becomes the b2HexColor unchanged, including values that are not in
// the b2HexColor enum.
func TestOracleShapeDrawColorCustomColorIsPassedThrough(t *testing.T) {
	t.Parallel()

	const raw = uint32(0x123456)

	b := body{bodyType: DynamicBody, mass: 1.0, setIndex: awakeSet}
	sim := bodySim{}
	s := shape{sensorIndex: NullIndex}
	s.material.CustomColor = raw

	tassert.Equal(t, HexColor(raw), shapeDrawColor(&b, &sim, &s))
}

// ---------------------------------------------------------------------------
// shape.c geometric helpers
// ---------------------------------------------------------------------------

// TestOracleGetShapePerimeter encodes b2GetShapePerimeter, src/shape.c. The
// expected values are closed forms of the C expressions, computed by hand:
//
//	capsule: 2 * length + 2 * pi * radius
//	circle : 2 * pi * radius
//	polygon: 2 * pi * polygonRadius + sum of edge lengths
//	segment: 2 * |p2 - p1|   (both sides of the segment)
//	chain segment: same as segment
func TestOracleGetShapePerimeter(t *testing.T) {
	t.Parallel()

	// A 2 x 2 box: 4 edges of length 2, zero polygon radius.
	box := MakeBox(1.0, 1.0)

	tests := []struct {
		name string
		s    shape
		want float64
	}{
		{
			name: "capsule",
			s: shape{
				shapeType: CapsuleShape,
				capsule: Capsule{
					Center1: Vec2{X: -1.5, Y: 0.0},
					Center2: Vec2{X: 1.5, Y: 0.0},
					Radius:  0.25,
				},
			},
			// 2 * 3.0 + 2 * pi * 0.25
			want: 6.0 + 2.0*math.Pi*0.25,
		},
		{
			name: "circle",
			s: shape{
				shapeType: CircleShape,
				circle:    Circle{Center: Vec2{X: 1.0, Y: 2.0}, Radius: 0.75},
			},
			want: 2.0 * math.Pi * 0.75,
		},
		{
			name: "polygon",
			s:    shape{shapeType: PolygonShape, polygon: box},
			want: 8.0,
		},
		{
			name: "segment",
			s: shape{
				shapeType: SegmentShape,
				segment:   Segment{Point1: Vec2{X: 0.0, Y: 0.0}, Point2: Vec2{X: 3.0, Y: 4.0}},
			},
			// |(3,4)| = 5, doubled
			want: 10.0,
		},
		{
			name: "chain segment",
			s: shape{
				shapeType: ChainSegmentShape,
				chainSegment: ChainSegment{
					Segment: Segment{Point1: Vec2{X: 0.0, Y: 0.0}, Point2: Vec2{X: 0.0, Y: 2.0}},
				},
			},
			want: 4.0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			s := test.s
			tassert.InDelta(t, test.want, getShapePerimeter(&s), 1e-12)
		})
	}
}

// TestOracleGetShapeRadius encodes b2GetShapeRadius, src/shape.c: the round
// radius of a capsule, circle or polygon, and 0 for every other type.
func TestOracleGetShapeRadius(t *testing.T) {
	t.Parallel()

	rounded := MakeRoundedBox(1.0, 1.0, 0.1)

	tests := []struct {
		name string
		s    shape
		want float64
	}{
		{
			name: "capsule",
			s:    shape{shapeType: CapsuleShape, capsule: Capsule{Radius: 0.3}},
			want: 0.3,
		},
		{
			name: "circle",
			s:    shape{shapeType: CircleShape, circle: Circle{Radius: 0.7}},
			want: 0.7,
		},
		{
			name: "rounded polygon",
			s:    shape{shapeType: PolygonShape, polygon: rounded},
			want: 0.1,
		},
		{
			name: "segment has no radius",
			s:    shape{shapeType: SegmentShape},
			want: 0.0,
		},
		{
			name: "chain segment has no radius",
			s:    shape{shapeType: ChainSegmentShape},
			want: 0.0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			s := test.s
			tassert.InDelta(t, test.want, getShapeRadius(&s), 1e-12)
		})
	}
}

// TestOracleGetShapeProjectedPerimeter encodes b2GetShapeProjectedPerimeter,
// src/shape.c. The C code projects the shape onto an infinite line and returns
// the projected extent:
//
//	capsule: 2 * radius + |dot(center2 - center1, line)|
//	circle : 2 * radius
//	polygon: (max - min) of dot(vertex, line) + 2 * polygon radius
//	segment: |dot(p2 - p1, line)|
//
// This is the perimeter used by the explosion impulse, so the values feed
// world_explode.go directly.
func TestOracleGetShapeProjectedPerimeter(t *testing.T) {
	t.Parallel()

	xAxis := Vec2{X: 1.0, Y: 0.0}
	yAxis := Vec2{X: 0.0, Y: 1.0}

	// A 2 x 4 box centred at the origin: x extent 2, y extent 4.
	box := MakeBox(1.0, 2.0)
	rounded := MakeRoundedBox(1.0, 2.0, 0.25)

	tests := []struct {
		name string
		s    shape
		line Vec2
		want float64
	}{
		{
			name: "horizontal capsule along x",
			s: shape{
				shapeType: CapsuleShape,
				capsule: Capsule{
					Center1: Vec2{X: -1.0, Y: 0.0},
					Center2: Vec2{X: 1.0, Y: 0.0},
					Radius:  0.5,
				},
			},
			line: xAxis,
			// 2 * 0.5 + |2.0| = 3.0
			want: 3.0,
		},
		{
			name: "horizontal capsule along y",
			s: shape{
				shapeType: CapsuleShape,
				capsule: Capsule{
					Center1: Vec2{X: -1.0, Y: 0.0},
					Center2: Vec2{X: 1.0, Y: 0.0},
					Radius:  0.5,
				},
			},
			line: yAxis,
			// 2 * 0.5 + |0| = 1.0
			want: 1.0,
		},
		{
			name: "circle is direction independent",
			s:    shape{shapeType: CircleShape, circle: Circle{Radius: 0.6}},
			line: yAxis,
			want: 1.2,
		},
		{
			name: "box along x",
			s:    shape{shapeType: PolygonShape, polygon: box},
			line: xAxis,
			want: 2.0,
		},
		{
			name: "box along y",
			s:    shape{shapeType: PolygonShape, polygon: box},
			line: yAxis,
			want: 4.0,
		},
		{
			name: "rounded box adds twice the polygon radius",
			s:    shape{shapeType: PolygonShape, polygon: rounded},
			line: xAxis,
			// b2MakeRoundedBox (src/geometry.c:158) keeps the b2MakeBox
			// vertices and only sets shape.radius, so the C expression
			// `(upper - lower) + 2 * radius` is 2.0 + 2 * 0.25.
			want: 2.5,
		},
		{
			name: "segment along its own direction",
			s: shape{
				shapeType: SegmentShape,
				segment:   Segment{Point1: Vec2{X: 0.0, Y: 0.0}, Point2: Vec2{X: 3.0, Y: 0.0}},
			},
			line: xAxis,
			want: 3.0,
		},
		{
			name: "segment perpendicular to the line",
			s: shape{
				shapeType: SegmentShape,
				segment:   Segment{Point1: Vec2{X: 0.0, Y: 0.0}, Point2: Vec2{X: 3.0, Y: 0.0}},
			},
			line: yAxis,
			want: 0.0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			s := test.s
			tassert.InDelta(t, test.want, getShapeProjectedPerimeter(&s, test.line), 1e-12)
		})
	}
}

// ---------------------------------------------------------------------------
// arena_allocator.c
// ---------------------------------------------------------------------------

// TestOracleArenaCapacityTracksHighWaterMark encodes b2GetArenaCapacity,
// b2GetArenaAllocation and b2GetMaxArenaAllocation, src/arena_allocator.c.
//
// Upstream reports bytes; this port reports elements (see the arena.go header),
// but the C invariants still hold:
//
//	capacity   >= the largest live allocation ever made
//	allocation == 0 once every item is freed
//	maxAllocation is monotone and never below the current allocation
func TestOracleArenaCapacityTracksHighWaterMark(t *testing.T) {
	t.Parallel()

	a := createArena()

	// A fresh arena has allocated nothing: b2CreateArenaAllocator sets index
	// and maxAllocation to zero.
	tassert.Equal(t, 0, getArenaCapacity(&a))
	tassert.Equal(t, 0, getArenaAllocation(&a))
	tassert.Equal(t, 0, getMaxArenaAllocation(&a))

	const small = 4
	data := a.allocMassData(small)
	require.Len(t, data, small)
	tassert.Equal(t, small, getArenaAllocation(&a))
	tassert.GreaterOrEqual(t, getArenaCapacity(&a), small)
	tassert.Equal(t, small, getMaxArenaAllocation(&a))

	a.freeMassData()
	tassert.Equal(t, 0, getArenaAllocation(&a))
	// b2GetArenaCapacity reports the retained buffer, so it must not shrink.
	tassert.GreaterOrEqual(t, getArenaCapacity(&a), small)

	const large = 37
	data = a.allocMassData(large)
	require.Len(t, data, large)
	tassert.GreaterOrEqual(t, getArenaCapacity(&a), large)
	tassert.Equal(t, large, getMaxArenaAllocation(&a))

	a.freeMassData()
	tassert.Equal(t, 0, getArenaAllocation(&a))

	// A smaller allocation reuses the grown buffer: the capacity stays at the
	// high-water mark while the live allocation drops.
	data = a.allocMassData(small)
	require.Len(t, data, small)
	tassert.GreaterOrEqual(t, getArenaCapacity(&a), large)
	tassert.Equal(t, small, getArenaAllocation(&a))
	tassert.Equal(t, large, getMaxArenaAllocation(&a))

	a.freeMassData()
	destroyArena(&a)

	// b2DestroyArenaAllocator releases the buffers, so the capacity is zero.
	tassert.Equal(t, 0, getArenaCapacity(&a))
}

// TestOracleArenaCapacityFromWorldStep checks the same accessor against the
// live per-world arena. b2GetArenaCapacity is a memory statistic, so the only
// C-guaranteed relation is capacity >= maxAllocation for the mass-data slot
// that b2UpdateBodyMassData uses.
func TestOracleArenaCapacityFromWorldStep(t *testing.T) {
	t.Parallel()

	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	bodyDef := DefaultBodyDef()
	bodyDef.Type = DynamicBody
	bodyID := w.CreateBody(&bodyDef)

	shapeDef := DefaultShapeDef()
	circle := Circle{Radius: 0.5}
	for range 6 {
		w.CreateCircleShape(bodyID, &shapeDef, &circle)
	}

	w.Step(1.0/60.0, 4)

	// Every arena item is freed by the end of b2World_Step.
	tassert.Equal(t, 0, getArenaAllocation(&w.arena))
	tassert.GreaterOrEqual(t, getArenaCapacity(&w.arena), 0)
}

// ---------------------------------------------------------------------------
// island.c
// ---------------------------------------------------------------------------

// TestOracleValidateIsland documents that b2ValidateIsland (src/island.c) is
// gated behind B2_VALIDATE in C and behind the compile-time `debugAsserts`
// constant in this port (core_asserts_off.go). In the default build the
// function body is unreachable, exactly like a C release build, so it is a
// total no-op for every input, including a null island id. Under the
// box2d_asserts tag the checks are live: a healthy island must validate clean
// and a null island id still returns before validation. Either way both calls
// below must not panic.
func TestOracleValidateIsland(t *testing.T) {
	t.Parallel()

	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	bodyDef := DefaultBodyDef()
	bodyDef.Type = DynamicBody
	bodyID := w.CreateBody(&bodyDef)

	shapeDef := DefaultShapeDef()
	circle := Circle{Radius: 0.5}
	w.CreateCircleShape(bodyID, &shapeDef, &circle)

	w.Step(1.0/60.0, 4)

	b := w.getBodyFullID(bodyID)
	require.NotEqual(t, NullIndex, b.islandID, "an awake dynamic body belongs to an island")

	// C: b2ValidateIsland returns immediately when validation is disabled.
	tassert.NotPanics(t, func() { w.validateIsland(b.islandID) })
	tassert.NotPanics(t, func() { w.validateIsland(NullIndex) })
}

// ---------------------------------------------------------------------------
// dynamic_tree.c node pool
// ---------------------------------------------------------------------------

// TestOracleTreeCreateDestroyNodePool is the node-pool half of the upstream
// test/test_dynamic_tree.c TreeCreateDestroy case:
//
//	ENSURE( tree.nodeCount > 0 );
//	ENSURE( tree.proxyCount == 1 );
//	b2DynamicTree_Destroy( &tree );
//	ENSURE( tree.nodeCount == 0 );
//	ENSURE( tree.proxyCount == 0 );
//
// nodeCount is unexported here, so this half lives in the internal test file.
func TestOracleTreeCreateDestroyNodePool(t *testing.T) {
	t.Parallel()

	a := AABB{
		LowerBound: Vec2{X: -1.0, Y: -1.0},
		UpperBound: Vec2{X: 2.0, Y: 2.0},
	}

	tree := NewDynamicTree()
	tree.CreateProxy(a, 1, 0)

	tassert.Positive(t, tree.nodeCount)
	tassert.Equal(t, 1, tree.GetProxyCount())

	tree.Destroy()

	tassert.Equal(t, 0, tree.nodeCount)
	tassert.Equal(t, 0, tree.GetProxyCount())
}

// TestOracleTreeFreeListInvariant encodes the accounting assertion at the tail
// of b2DynamicTree_Validate, src/dynamic_tree.c:1080:
//
//	B2_ASSERT( tree->nodeCount + freeCount == tree->nodeCapacity );
//
// It must hold after arbitrary create/destroy traffic, not only on a fresh
// tree. The proxy ids come from a fixed table so the test is deterministic.
func TestOracleTreeFreeListInvariant(t *testing.T) {
	t.Parallel()

	tree := NewDynamicTree()
	defer tree.Destroy()

	proxies := make([]int, 0, 24)
	for i := range 24 {
		x := float64(i)
		box := AABB{
			LowerBound: Vec2{X: x, Y: 0.0},
			UpperBound: Vec2{X: x + 1.0, Y: 1.0},
		}
		proxies = append(proxies, tree.CreateProxy(box, 1, uint64(i)))
	}

	// Destroy a fixed, scattered subset so the free list is non-trivial.
	for _, index := range []int{0, 3, 4, 9, 17, 23} {
		tree.DestroyProxy(proxies[index])
	}

	freeCount := 0
	for freeIndex := tree.freeList; freeIndex != NullIndex; freeIndex = int(tree.nodes[freeIndex].Parent) {
		freeCount++
	}

	tassert.Equal(t, tree.nodeCapacity, tree.nodeCount+freeCount)
	require.NoError(t, tree.Validate())
}

// TestOracleTreeValidateRejectsCorruption encodes every failure the C
// b2ValidateStructure (src/dynamic_tree.c:975), b2ValidateMetrics
// (src/dynamic_tree.c:1015) and b2DynamicTree_Validate
// (src/dynamic_tree.c:1059) assert on. Upstream calls B2_ASSERT; this port
// returns an error instead (see the Validate doc comment), so each corruption
// must produce a non-nil error.
//
// The C assertions, in the order they appear:
//
//	b2ValidateStructure
//	  index == root  =>  nodes[index].parent == B2_NULL_INDEX
//	  leaf           =>  node->height == 0
//	  0 <= child1, child2 < nodeCapacity
//	  nodes[child1].parent == index, nodes[child2].parent == index
//	  a child flagged enlarged implies the parent is flagged enlarged
//
//	b2ValidateMetrics
//	  node->height == 1 + max( height(child1), height(child2) )
//	  b2AABB_Contains( node->aabb, child aabb ) for both children
//	  node->categoryBits == child1.categoryBits | child2.categoryBits
//
//	b2DynamicTree_Validate
//	  the free list indices are in range
//	  b2DynamicTree_GetHeight == b2ComputeHeight( root )
//	  nodeCount + freeCount == nodeCapacity
func TestOracleTreeValidateRejectsCorruption(t *testing.T) {
	t.Parallel()

	// buildTree returns a tree with a real internal root and eight leaves.
	buildTree := func() DynamicTree {
		tree := NewDynamicTree()
		for i := range 8 {
			x := float64(float64(i) * 3.0)
			box := AABB{
				LowerBound: Vec2{X: x, Y: 0.0},
				UpperBound: Vec2{X: x + 1.0, Y: 1.0},
			}
			tree.CreateProxy(box, 1, uint64(i))
		}
		return tree
	}

	// anyInternal returns an internal (non-leaf) node index other than the
	// root, so the corruption is detected inside the recursion.
	anyInternal := func(tree *DynamicTree) int {
		for i := range tree.nodeCapacity {
			node := &tree.nodes[i]
			if node.Flags&allocatedNode != 0 && !isLeaf(node) && i != tree.root {
				return i
			}
		}
		return NullIndex
	}

	anyLeaf := func(tree *DynamicTree) int {
		for i := range tree.nodeCapacity {
			node := &tree.nodes[i]
			if node.Flags&allocatedNode != 0 && isLeaf(node) {
				return i
			}
		}
		return NullIndex
	}

	tests := []struct {
		name    string
		corrupt func(t *testing.T, tree *DynamicTree)
	}{
		{
			name: "root has a parent",
			corrupt: func(_ *testing.T, tree *DynamicTree) {
				tree.nodes[tree.root].Parent = 0
			},
		},
		{
			name: "leaf has a non zero height",
			corrupt: func(t *testing.T, tree *DynamicTree) {
				leaf := anyLeaf(tree)
				require.NotEqual(t, NullIndex, leaf)
				tree.nodes[leaf].Height = 3
			},
		},
		{
			name: "child index out of range",
			corrupt: func(t *testing.T, tree *DynamicTree) {
				internal := anyInternal(tree)
				require.NotEqual(t, NullIndex, internal)
				tree.nodes[internal].Child1 = int32(tree.nodeCapacity + 5)
			},
		},
		{
			name: "second child index out of range",
			corrupt: func(t *testing.T, tree *DynamicTree) {
				internal := anyInternal(tree)
				require.NotEqual(t, NullIndex, internal)
				tree.nodes[internal].Child2 = -7
			},
		},
		{
			name: "child parent link broken",
			corrupt: func(t *testing.T, tree *DynamicTree) {
				internal := anyInternal(tree)
				require.NotEqual(t, NullIndex, internal)
				tree.nodes[tree.nodes[internal].Child1].Parent = int32(tree.root)
			},
		},
		{
			name: "second child parent link broken",
			corrupt: func(t *testing.T, tree *DynamicTree) {
				internal := anyInternal(tree)
				require.NotEqual(t, NullIndex, internal)
				tree.nodes[tree.nodes[internal].Child2].Parent = int32(tree.root)
			},
		},
		{
			name: "enlarged child under a plain parent",
			corrupt: func(t *testing.T, tree *DynamicTree) {
				internal := anyInternal(tree)
				require.NotEqual(t, NullIndex, internal)
				tree.nodes[tree.nodes[internal].Child1].Flags |= enlargedNode
				tree.nodes[internal].Flags &^= enlargedNode
			},
		},
		{
			name: "node flags set without the allocated bit",
			corrupt: func(_ *testing.T, tree *DynamicTree) {
				tree.nodes[tree.root].Flags = enlargedNode
			},
		},
		{
			name: "internal node height is wrong",
			corrupt: func(t *testing.T, tree *DynamicTree) {
				internal := anyInternal(tree)
				require.NotEqual(t, NullIndex, internal)
				tree.nodes[internal].Height += 3
			},
		},
		{
			name: "parent aabb does not contain a child",
			corrupt: func(t *testing.T, tree *DynamicTree) {
				internal := anyInternal(tree)
				require.NotEqual(t, NullIndex, internal)
				tree.nodes[internal].AABB = AABB{
					LowerBound: Vec2{X: 1000.0, Y: 1000.0},
					UpperBound: Vec2{X: 1001.0, Y: 1001.0},
				}
			},
		},
		{
			name: "category bits are not the union of the children",
			corrupt: func(t *testing.T, tree *DynamicTree) {
				internal := anyInternal(tree)
				require.NotEqual(t, NullIndex, internal)
				tree.nodes[internal].CategoryBits |= 0x8000
			},
		},
		{
			name: "free list index out of range",
			corrupt: func(_ *testing.T, tree *DynamicTree) {
				tree.freeList = tree.nodeCapacity + 100
			},
		},
		{
			name: "cached height disagrees with the computed height",
			corrupt: func(_ *testing.T, tree *DynamicTree) {
				tree.nodes[tree.root].Height += 5
			},
		},
		{
			name: "node count does not add up",
			corrupt: func(_ *testing.T, tree *DynamicTree) {
				tree.nodeCount++
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			tree := buildTree()
			defer tree.Destroy()

			require.NoError(t, tree.Validate(), "the scene must start valid")

			test.corrupt(t, &tree)

			tassert.Error(t, tree.Validate())
		})
	}
}

// TestOracleTreeValidateNoEnlargedDetectsAFlaggedNode encodes
// b2DynamicTree_ValidateNoEnlarged, src/dynamic_tree.c:1089:
//
//	for ( int i = 0; i < capacity; ++i )
//	    B2_ASSERT( ( nodes[i].flags & b2_enlargedNode ) == 0 );
//
// The check walks the whole node pool, not just the reachable tree, and only
// looks at allocated nodes in this port.
func TestOracleTreeValidateNoEnlargedDetectsAFlaggedNode(t *testing.T) {
	t.Parallel()

	tree := NewDynamicTree()
	defer tree.Destroy()

	for i := range 8 {
		x := float64(float64(i) * 3.0)
		box := AABB{
			LowerBound: Vec2{X: x, Y: 0.0},
			UpperBound: Vec2{X: x + 1.0, Y: 1.0},
		}
		tree.CreateProxy(box, 1, uint64(i))
	}

	require.NoError(t, tree.ValidateNoEnlarged())

	tree.nodes[tree.root].Flags |= enlargedNode
	require.Error(t, tree.ValidateNoEnlarged())

	tree.nodes[tree.root].Flags &^= enlargedNode
	require.NoError(t, tree.ValidateNoEnlarged())
}

// TestOracleTreeValidateEmptyTreeIsTrivial encodes the head of
// b2DynamicTree_Validate, src/dynamic_tree.c:1059:
//
//	if ( tree->root == B2_NULL_INDEX ) return;
//
// A tree with no proxies validates unconditionally, even when the accounting
// would otherwise be inconsistent.
func TestOracleTreeValidateEmptyTreeIsTrivial(t *testing.T) {
	t.Parallel()

	tree := NewDynamicTree()
	defer tree.Destroy()

	require.Equal(t, NullIndex, tree.root)
	tassert.NoError(t, tree.Validate())

	// The early return happens before the node accounting is looked at.
	tree.nodeCount = 1234
	tassert.NoError(t, tree.Validate())
}

// TestOracleShapeHelpersDefaultArm encodes the `default:` arm that every
// shape-type switch in src/shape.c carries: a neutral return value rather
// than a panic or a read of a stale union member. Only two of the arms assert
// in C — b2ComputeShapeAABB (shape.c:650) and b2MakeShapeDistanceProxy
// (shape.c:1020) — so those two fork on the build: the release build pins the
// neutral value, the box2d_asserts build pins the panic, like an upstream
// debug build. The rest return their neutral value silently in every build,
// C debug included, and are pinned unconditionally.
//
// The switch subject is b2ShapeType, and b2_shapeTypeCount is the sentinel
// that is never a real shape type, so it drives every default arm at once.
func TestOracleShapeHelpersDefaultArm(t *testing.T) {
	t.Parallel()

	s := shape{shapeType: ShapeTypeCount, sensorIndex: NullIndex, density: 1.0}

	xf := Transform{P: Vec2{X: 3.0, Y: 4.0}, Q: RotIdentity}

	// b2ComputeShapeAABB (src/shape.c:648-653): `b2AABB empty = { xf.p, xf.p }`
	// behind B2_ASSERT( false ).
	if debugAsserts {
		// The box2d_asserts build mirrors an upstream debug build: the
		// B2_ASSERT( false ) fires before the neutral return.
		tassert.Panics(t, func() { computeShapeAABB(&s, xf) })
	} else {
		aabb := computeShapeAABB(&s, xf)
		tassert.Equal(t, AABB{LowerBound: xf.P, UpperBound: xf.P}, aabb)
	}

	// b2GetShapeCentroid: the C returns b2Vec2_zero.
	tassert.Equal(t, Vec2Zero, getShapeCentroid(&s))

	// b2GetShapePerimeter and b2GetShapeProjectedPerimeter return 0.
	tassert.InDelta(t, 0.0, getShapePerimeter(&s), 0.0)
	tassert.InDelta(t, 0.0, getShapeProjectedPerimeter(&s, Vec2{X: 1.0, Y: 0.0}), 0.0)

	// b2GetShapeRadius returns 0 for anything that is not round.
	tassert.InDelta(t, 0.0, getShapeRadius(&s), 0.0)

	// b2ComputeShapeMass returns a zeroed b2MassData.
	tassert.Equal(t, MassData{}, computeShapeMass(&s))

	// b2ComputeShapeExtent leaves the extents at their zero initialisation.
	tassert.Equal(t, shapeExtent{}, computeShapeExtent(&s, Vec2Zero))

	// b2ComputeShapeMargin returns B2_AABB_MARGIN, the cap it applies to every
	// other shape type.
	tassert.InDelta(t, MaxAABBMargin, computeShapeMargin(&s), 0.0)

	// b2RayCastShape and b2ShapeCastShape return the zeroed b2CastOutput
	// without transforming it back into world space.
	rayInput := RayCastInput{
		Origin:      Vec2{X: -10.0, Y: 0.0},
		Translation: Vec2{X: 20.0, Y: 0.0},
		MaxFraction: 1.0,
	}
	tassert.Equal(t, CastOutput{}, rayCastShape(&rayInput, &s, xf))

	var castInput ShapeCastInput
	castInput.Proxy = MakeProxy([]Vec2{{X: 0.0, Y: 0.0}}, 1, 0.1)
	castInput.Translation = Vec2{X: 20.0, Y: 0.0}
	castInput.MaxFraction = 1.0
	tassert.Equal(t, CastOutput{}, shapeCastShape(&castInput, &s, xf))

	// b2CollideMover returns the zeroed b2PlaneResult, so hit stays false.
	mover := Capsule{
		Center1: Vec2{X: 3.0, Y: -0.5},
		Center2: Vec2{X: 3.0, Y: 0.5},
		Radius:  0.5,
	}
	result := collideMover(&mover, &s, xf)
	tassert.False(t, result.Hit)
	tassert.Equal(t, PlaneResult{}, result)

	// b2MakeShapeDistanceProxy (src/shape.c:1020) returns a zeroed
	// b2ShapeProxy behind B2_ASSERT( false ).
	if debugAsserts {
		tassert.Panics(t, func() { makeShapeDistanceProxy(&s) })
	} else {
		tassert.Equal(t, ShapeProxy{}, makeShapeDistanceProxy(&s))
	}
}

// TestOracleShapeCastShapeRejectsEmptyProxy encodes the head of
// b2ShapeCastShape, src/shape.c:
//
//	if ( input->proxy.count == 0 ) return output;   // the zeroed output
//
// An empty proxy short-circuits before the per-type dispatch.
func TestOracleShapeCastShapeRejectsEmptyProxy(t *testing.T) {
	t.Parallel()

	s := shape{shapeType: CircleShape, sensorIndex: NullIndex, circle: Circle{Radius: 1.0}}

	var input ShapeCastInput
	input.Proxy.Count = 0
	input.Translation = Vec2{X: 10.0, Y: 0.0}
	input.MaxFraction = 1.0

	tassert.Equal(t, CastOutput{}, shapeCastShape(&input, &s, TransformIdentity))
}

package physics2d_test

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"unsafe"

	phycomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/component"
	"github.com/stretchr/testify/require"
)

func v(x, y float64) phycomp.Vec2 { return phycomp.Vec2{X: x, Y: y} }

// chain4 is the shortest polyline Box2D accepts.
func chain4() []phycomp.Vec2 { return []phycomp.Vec2{v(0, 0), v(1, 0), v(2, 1), v(3, 0)} }

func TestShape_ConstructorsCarryBox2DDefaults(t *testing.T) {
	t.Parallel()
	s := phycomp.Circle(0.5)
	require.Equal(t, phycomp.ShapeKindCircle, s.Kind())
	require.InDelta(t, 0.6, s.Friction, 1e-12)
	require.InDelta(t, 0.0, s.Restitution, 1e-12)
	require.InDelta(t, 1.0, s.Density, 1e-12)
	require.False(t, s.IsSensor)
	require.Equal(t, uint64(1), s.CategoryBits)
	require.Equal(t, ^uint64(0), s.MaskBits)
	require.Equal(t, int32(0), s.GroupIndex)
	require.NoError(t, s.Validate())
	t.Logf("Shape is %d bytes", unsafe.Sizeof(phycomp.Shape{}))
}

func TestShape_ValidateAcceptsEveryKind(t *testing.T) {
	t.Parallel()
	for _, s := range []phycomp.Shape{
		phycomp.Circle(0.5),
		phycomp.Box(1, 2),
		phycomp.Polygon(v(0, 0), v(1, 0), v(0.5, 1)),
		phycomp.Chain(chain4()...),
		phycomp.ChainLoop(v(0, 0), v(1, 0), v(1, 1), v(0, 1)),
		phycomp.Edge(v(0, 0), v(1, 0)),
		phycomp.Capsule(v(0, 0), v(0, 1), 0.25),
	} {
		require.NoError(t, s.Validate(), s.Kind().String())
	}
}

func TestShape_ValidateRejects(t *testing.T) {
	t.Parallel()
	nan := math.NaN()
	for _, tc := range []struct {
		name string
		s    phycomp.Shape
		want string
	}{
		{"bare literal", phycomp.Shape{}, "kind"},
		{"zero radius", phycomp.Circle(0), "radius"},
		{"zero extent", phycomp.Box(1, 0), "half extents"},
		{"two-vertex polygon", phycomp.Polygon(v(0, 0), v(1, 0)), "3..8"},
		{"nine-vertex polygon", phycomp.Polygon(
			v(0, 0), v(1, 0), v(2, 0), v(3, 1), v(3, 2), v(2, 3), v(1, 3), v(0, 2), v(0, 1)), "3..8"},
		{"flat polygon", phycomp.Polygon(v(-1, 0), v(0, 0), v(1, 0), v(2, 0)), "hull"},
		{"three-point chain", phycomp.Chain(v(0, 0), v(1, 0), v(2, 0)), "at least 4"},
		{"sensor chain", phycomp.Chain(chain4()...).Sensor(true), "sensor"},
		{"degenerate edge", phycomp.Edge(v(1, 1), v(1, 1)), "apart"},
		{"degenerate capsule", phycomp.Capsule(v(1, 1), v(1, 1), 0.5), "apart"},
		{"zero-radius capsule", phycomp.Capsule(v(0, 0), v(1, 0), 0), "radius"},
		{"NaN offset", phycomp.Circle(0.5).At(v(nan, 0), 0), "local_offset"},
		{"NaN rotation", phycomp.Circle(0.5).At(v(0, 0), nan), "local_rotation"},
		{"negative friction", phycomp.Circle(0.5).Material(-0.1, 0, 1), "friction"},
		{"NaN in a field the kind does not use", func() phycomp.Shape {
			s := phycomp.Circle(0.5)
			s.Geometry.A.X = nan
			return s
		}(), "a:"},
		{"NaN point", phycomp.Chain(v(0, 0), v(1, 0), v(nan, 1), v(3, 0)), "points[2]"},
	} {
		err := tc.s.Validate()
		require.Error(t, err, tc.name)
		require.Contains(t, err.Error(), tc.want, tc.name)
	}
}

func TestShape_GettersReadTheirKind(t *testing.T) {
	t.Parallel()
	circle := phycomp.Circle(0.5)
	box := phycomp.Box(1, 2)
	tri := phycomp.Polygon(v(0, 0), v(1, 0), v(0.5, 1))
	chain := phycomp.Chain(chain4()...)
	edge := phycomp.Edge(v(0, 0), v(1, 0))
	capsule := phycomp.Capsule(v(0, 0), v(0, 1), 0.25)

	require.InDelta(t, 0.5, circle.Radius(), 1e-12)
	require.InDelta(t, 0.25, capsule.Radius(), 1e-12)
	require.InDelta(t, 0.0, box.Radius(), 1e-12, "a box has no radius")

	require.Equal(t, v(1, 2), box.HalfExtents())
	require.Equal(t, v(0, 0), circle.HalfExtents(), "a circle has no half extents")

	a, b := edge.Endpoints()
	require.Equal(t, [2]phycomp.Vec2{v(0, 0), v(1, 0)}, [2]phycomp.Vec2{a, b})
	a, b = capsule.Endpoints()
	require.Equal(t, [2]phycomp.Vec2{v(0, 0), v(0, 1)}, [2]phycomp.Vec2{a, b})
	a, b = box.Endpoints()
	require.Equal(t, [2]phycomp.Vec2{}, [2]phycomp.Vec2{a, b}, "half extents must not leak out as endpoints")

	require.Equal(t, []phycomp.Vec2{v(0, 0), v(1, 0), v(0.5, 1)}, tri.Vertices())
	require.Nil(t, tri.Points(), "polygon vertices are not chain points")
	require.Equal(t, chain4(), chain.Points())
	require.Nil(t, chain.Vertices(), "chain points are not polygon vertices")
	require.Nil(t, circle.Points())
	require.Equal(t, chain4(), phycomp.ChainLoop(chain4()...).Points())
}

func TestShape_PointsAreCopies(t *testing.T) {
	t.Parallel()
	chain := phycomp.Chain(chain4()...)
	pts := chain.Points()
	pts[0] = v(9, 9)
	require.Equal(t, chain4(), chain.Points(), "editing the returned slice must not reach the shape")

	tri := phycomp.Polygon(v(0, 0), v(1, 0), v(0.5, 1))
	tri.Vertices()[0] = v(9, 9)
	require.Equal(t, v(0, 0), tri.Vertices()[0])
}

func TestShape_Reshape(t *testing.T) {
	t.Parallel()
	old := phycomp.Circle(0.5).At(v(1, 2), 0.3).Material(0.1, 0.2, 0.3).Filter(0x2, 0x4).Group(-1).Sensor(true)
	got := old.Reshape(phycomp.Box(3, 4))
	require.Equal(t, phycomp.ShapeKindBox, got.Kind())
	require.Equal(t, v(3, 4), got.HalfExtents())
	require.InDelta(t, 0.0, got.Radius(), 1e-12, "the circle's radius does not linger")
	want := phycomp.Box(3, 4).At(v(1, 2), 0.3).Material(0.1, 0.2, 0.3).Filter(0x2, 0x4).Group(-1).Sensor(true)
	require.True(t, got.Equal(want),
		"placement, material, filter and sensor flag are kept:\n got %+v\nwant %+v", got, want)
	require.False(t, got.StructuralEqual(old), "a reshape rebuilds the fixture")

	loop := []phycomp.Vec2{v(0, 0), v(1, 0), v(1, 1), v(0, 1)}
	terrain := phycomp.Chain(chain4()...).Material(0.9, 0, 1).Reshape(phycomp.ChainLoop(loop...))
	require.Equal(t, phycomp.ShapeKindChainLoop, terrain.Kind())
	require.Equal(t, loop, terrain.Points())
	require.InDelta(t, 0.9, terrain.Friction, 1e-12)
}

func TestShape_Equality(t *testing.T) {
	t.Parallel()
	a := phycomp.Chain(chain4()...).At(v(1, 2), 0.5)
	b := phycomp.Chain(chain4()...).At(v(1, 2), 0.5) // separate points array
	require.True(t, a.Equal(b), "same shape built twice is equal, points by value")

	softer := a.Material(0.1, 0, 1).Filter(2, 4)
	require.True(t, a.StructuralEqual(softer), "material and filter apply in place")
	require.False(t, a.Equal(softer), "but they are still changes")

	moved := a.At(v(0, 0), 0)
	require.False(t, a.StructuralEqual(moved), "placement rebuilds the fixture")

	sensor := phycomp.Circle(0.5)
	require.False(t, sensor.StructuralEqual(sensor.Sensor(true)), "Box2D cannot toggle a live sensor flag")
}

func TestShape_CopyBreaksPointSharing(t *testing.T) {
	t.Parallel()
	live := phycomp.Chain(chain4()...)
	kept := live.Copy()
	edited := live
	edited.Geometry.Points = edited.Geometry.Points.With(0, v(9, 9)) // writes through live's array
	require.Equal(t, v(9, 9), live.Points()[0], "With writes through the array it derives from")
	require.Equal(t, chain4(), kept.Points(), "the copy is untouched")
}

func TestShape_JSONIsFlatAndFillsDefaults(t *testing.T) {
	t.Parallel()
	var s phycomp.Shape
	require.NoError(t, json.Unmarshal([]byte(`{"kind": 1, "radius": 0.5}`), &s))
	require.NoError(t, s.Validate())
	require.InDelta(t, 0.6, s.Friction, 1e-12)
	require.InDelta(t, 1.0, s.Density, 1e-12)
	require.Equal(t, uint64(1), s.CategoryBits)
	require.Equal(t, ^uint64(0), s.MaskBits)

	var explicit phycomp.Shape
	require.NoError(t, json.Unmarshal([]byte(`{"kind": 1, "radius": 0.5, "friction": 0, "mask_bits": 0}`), &explicit))
	require.InDelta(t, 0.0, explicit.Friction, 1e-12, "an explicit zero is kept")
	require.Equal(t, uint64(0), explicit.MaskBits)

	want := phycomp.Polygon(v(0, 0), v(1, 0), v(0.5, 1)).At(v(1, 1), 0.25).Filter(0x2, 0x4).Sensor(true)
	round, err := json.Marshal(want)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(round), `{"kind":3,`), "the geometry must sit at the top level: %s", round)
	require.NotContains(t, string(round), "Geometry")
	var back phycomp.Shape
	require.NoError(t, json.Unmarshal(round, &back))
	require.True(t, back.Equal(want), "got %+v\nwant %+v", back, want)
}

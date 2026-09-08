package physics2d_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/immutable"

	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/stretchr/testify/require"
)

// TestShapeDef_DefaultsAndOptions: every constructor carries Box2D's default ShapeCommon, and
// the options set exactly what they say.
func TestShapeDef_DefaultsAndOptions(t *testing.T) {
	t.Parallel()

	defaults := physics.ShapeCommon{Friction: 0.6, Density: 1, CategoryBits: 1, MaskBits: ^uint64(0)}
	require.Equal(t, defaults, physics.Circle(0.5).Common)
	require.Equal(t, defaults, physics.Box(1, 2).Common)
	require.Equal(t, defaults, physics.Polygon(physics.Vec2{}, physics.Vec2{X: 1}, physics.Vec2{Y: 1}).Common)
	require.Equal(t, defaults, physics.Chain(physics.Vec2{}, physics.Vec2{X: 1}).Common)
	require.Equal(t, defaults, physics.ChainLoop(physics.Vec2{}, physics.Vec2{X: 1}).Common)
	require.Equal(t, defaults, physics.Edge(physics.Vec2{}, physics.Vec2{X: 1}).Common)
	require.Equal(t, defaults, physics.Capsule(physics.Vec2{}, physics.Vec2{X: 1}, 0.25).Common)

	d := physics.Box(1, 1).
		AsSensor().
		Material(0.1, 0.2, 0.3).
		Filter(0x2, 0x4).
		Group(-1)
	require.Equal(t, physics.ShapeCommon{
		IsSensor: true, Friction: 0.1, Restitution: 0.2, Density: 0.3,
		CategoryBits: 0x2, MaskBits: 0x4, GroupIndex: -1,
	}, d.Common)
	require.Equal(t, physics.BoxGeom{HalfExtents: physics.Vec2{X: 1, Y: 1}}, d.Geom)
}

func TestShapeDef_Geometry(t *testing.T) {
	t.Parallel()

	require.Equal(t, physics.CircleGeom{Radius: 0.5}, physics.Circle(0.5).Geom)
	line := []physics.Vec2{{X: 0, Y: 0}, {X: 1, Y: 0}}
	chain := physics.Chain(line...).Geom
	require.Equal(t, physics.ChainGeom{Points: immutable.SliceOf(line...)}, chain)
	require.Equal(t, physics.ChainGeom{Points: immutable.SliceOf(line...), Loop: true}, physics.ChainLoop(line...).Geom)
	line[0].X = 99
	require.InDelta(t, 0.0, chain.Points.At(0).X, 0, "constructor copies the points")
	require.Equal(t, physics.EdgeGeom{A: physics.Vec2{}, B: physics.Vec2{X: 1}},
		physics.Edge(physics.Vec2{}, physics.Vec2{X: 1}).Geom)
	require.Equal(t, physics.CapsuleGeom{A: physics.Vec2{}, B: physics.Vec2{X: 1}, Radius: 0.25},
		physics.Capsule(physics.Vec2{}, physics.Vec2{X: 1}, 0.25).Geom)

	tri := physics.Polygon(physics.Vec2{}, physics.Vec2{X: 1}, physics.Vec2{Y: 1}).Geom
	require.Equal(t, uint8(3), tri.Count)
	require.Equal(t, [physics.MaxPolygonVertices]physics.Vec2{{}, {X: 1}, {Y: 1}}, tri.Vertices)
	require.NoError(t, tri.Validate())

	var nine []physics.Vec2
	for i := range 9 {
		nine = append(nine, physics.Vec2{X: float64(i)})
	}
	require.Error(t, physics.Polygon(nine...).Geom.Validate(), "more than MaxPolygonVertices is rejected")
}

func TestShapeSlot_At(t *testing.T) {
	t.Parallel()
	slot := physics.Slot(9).At(physics.Vec2{X: 2, Y: 3}, 0.5)
	require.Equal(t, physics.ShapeSlot{Shape: 9, LocalOffset: physics.Vec2{X: 2, Y: 3}, LocalRotation: 0.5}, slot)
}

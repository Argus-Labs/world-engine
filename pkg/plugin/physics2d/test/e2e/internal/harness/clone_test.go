package harness_test

import (
	"testing"

	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"
	"github.com/stretchr/testify/require"
)

// A body read from ECS shares its shape array, and each chain's points array, with the stored
// component; With writes through them. Ctx.Body and the snapshot capture both promise a copy
// that later edits cannot reach, so CloneBody has to break both kinds of sharing.
func TestCloneBodyIsolatesShapes(t *testing.T) {
	t.Parallel()
	pts := []physics.Vec2{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 2, Y: 1}, {X: 3, Y: 0}}
	stored := physics.NewPhysicsBody2D(physics.BodyTypeStatic, physics.Circle(1), physics.Chain(pts...))

	edited := harness.CloneBody(stored)
	edited.Shapes = edited.Shapes.With(0, physics.Circle(9))
	require.InDelta(t, 1.0, stored.Shapes.At(0).Radius(), 1e-12,
		"editing the clone wrote through to the body it was cloned from")
	require.InDelta(t, 9.0, edited.Shapes.At(0).Radius(), 1e-12, "the clone did not take the edit")

	chain := edited.Shapes.At(1)
	chain.Geometry.Points = chain.Geometry.Points.With(0, physics.Vec2{X: 9, Y: 9}) // the getters hand out copies
	require.Equal(t, physics.Vec2{}, stored.Shapes.At(1).Points()[0],
		"editing the clone's points reached the original chain")
}

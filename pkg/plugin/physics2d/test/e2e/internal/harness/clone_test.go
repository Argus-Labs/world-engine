package harness_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	physcomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"
	"github.com/stretchr/testify/require"
)

// A body read from ECS shares its slot array with the stored component, and With writes
// through that array. Ctx.Body and the snapshot capture both promise a copy that later
// edits cannot reach, so CloneBody has to break the sharing.
func TestCloneBodyIsolatesShapes(t *testing.T) {
	t.Parallel()
	stored := physics.NewPhysicsBody2D(physics.BodyTypeStatic, physcomp.Ref(1), physcomp.Ref(2))

	edited := harness.CloneBody(stored)
	edited.Shapes = edited.Shapes.With(0, physcomp.Ref(9))

	require.Equal(t, cardinal.EntityID(1), stored.Shapes.At(0).Shape,
		"editing the clone wrote through to the body it was cloned from")
	require.Equal(t, cardinal.EntityID(9), edited.Shapes.At(0).Shape, "the clone did not take the edit")
}

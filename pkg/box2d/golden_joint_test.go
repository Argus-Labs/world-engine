// Cross-architecture determinism gate for the joint solver (stage E8). Each
// scene is stepped at 60Hz with 4 sub-steps; every step a djb2 hash is folded
// over the exact float64 bit patterns of all body transforms and velocities
// plus the contact and island counts (see golden_step_test.go). The golden
// file records the hash at every 30th step and the final step. Regenerate
// deliberately with:
//
//	BOX2D_UPDATE_GOLDEN=1 go test ./pkg/box2d/ -run TestGoldenJoint
//
// and commit the diff. A mismatch on one architecture only means an FMA or
// libm leak — see math_fma.go.

package box2d_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

const goldenJointStepCount = 240

// buildPendulumChainScene builds a chain of 5 boxes linked by revolute
// joints, hanging from a static anchor.
func buildPendulumChainScene(w *box2d.World) []box2d.BodyID {
	var bodies []box2d.BodyID

	gd := box2d.DefaultBodyDef()
	gd.Position = box2d.Vec2{X: 0.0, Y: 4.0}
	anchor := w.CreateBody(&gd)

	const linkCount = 5
	const halfLength = 0.5

	prev := anchor
	for i := range linkCount {
		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.Position = box2d.Vec2{X: (2.0*float64(i) + 1.0) * halfLength, Y: 4.0}
		link := w.CreateBody(&bd)

		linkBox := box2d.MakeBox(halfLength, 0.125)
		sd := box2d.DefaultShapeDef()
		sd.Density = 20.0
		w.CreatePolygonShape(link, &sd, &linkBox)

		jd := box2d.DefaultRevoluteJointDef()
		jd.Base.BodyIDA = prev
		jd.Base.BodyIDB = link
		if i == 0 {
			jd.Base.LocalFrameA.P = box2d.Vec2Zero
		} else {
			jd.Base.LocalFrameA.P = box2d.Vec2{X: halfLength, Y: 0.0}
		}
		jd.Base.LocalFrameB.P = box2d.Vec2{X: -halfLength, Y: 0.0}
		w.CreateRevoluteJoint(&jd)

		bodies = append(bodies, link)
		prev = link
	}

	return bodies
}

// buildSpringDistanceScene builds a column of circles connected by spring
// distance joints above a ground box, plus a rigid distance pair swinging
// sideways.
func buildSpringDistanceScene(w *box2d.World) []box2d.BodyID {
	var bodies []box2d.BodyID

	gd := box2d.DefaultBodyDef()
	gd.Position = box2d.Vec2{X: 0.0, Y: -10.0}
	ground := w.CreateBody(&gd)
	groundBox := box2d.MakeBox(50.0, 10.0)
	gsd := box2d.DefaultShapeDef()
	w.CreatePolygonShape(ground, &gsd, &groundBox)

	ad := box2d.DefaultBodyDef()
	ad.Position = box2d.Vec2{X: 0.0, Y: 8.0}
	anchor := w.CreateBody(&ad)

	prev := anchor
	for i := range 3 {
		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.Position = box2d.Vec2{X: 0.0, Y: 7.0 - float64(i)}
		ball := w.CreateBody(&bd)

		circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.25}
		sd := box2d.DefaultShapeDef()
		sd.Density = 1.0
		w.CreateCircleShape(ball, &sd, &circle)

		jd := box2d.DefaultDistanceJointDef()
		jd.Base.BodyIDA = prev
		jd.Base.BodyIDB = ball
		jd.Length = 1.0
		jd.EnableSpring = true
		jd.Hertz = 3.0
		jd.DampingRatio = 0.2
		jd.EnableLimit = true
		jd.MinLength = 0.5
		jd.MaxLength = 2.0
		w.CreateDistanceJoint(&jd)

		bodies = append(bodies, ball)
		prev = ball
	}

	// Rigid pair with sideways motion.
	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bd.Position = box2d.Vec2{X: 4.0, Y: 5.0}
	pairA := w.CreateBody(&bd)
	bd.Position = box2d.Vec2{X: 5.0, Y: 5.0}
	bd.LinearVelocity = box2d.Vec2{X: 0.0, Y: 3.0}
	pairB := w.CreateBody(&bd)

	circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.25}
	sd := box2d.DefaultShapeDef()
	sd.Density = 1.0
	w.CreateCircleShape(pairA, &sd, &circle)
	w.CreateCircleShape(pairB, &sd, &circle)

	jd := box2d.DefaultDistanceJointDef()
	jd.Base.BodyIDA = pairA
	jd.Base.BodyIDB = pairB
	jd.Length = 1.0
	w.CreateDistanceJoint(&jd)

	bodies = append(bodies, pairA, pairB)
	return bodies
}

func goldenJointScenes() []goldenSceneDef {
	return []goldenSceneDef{
		{"pendulum_chain", buildPendulumChainScene},
		{"spring_distance", buildSpringDistanceScene},
	}
}

func computeGoldenJoint() []goldenStepScene {
	return computeGoldenJointWorkers(0)
}

func computeGoldenJointWorkers(workerCount int) []goldenStepScene {
	return computeGoldenScenes(goldenJointScenes(), goldenJointStepCount, workerCount)
}

func TestGoldenJoint(t *testing.T) {
	path := filepath.Join("testdata", "golden_joint.json")

	got := computeGoldenJoint()

	if os.Getenv("BOX2D_UPDATE_GOLDEN") == "1" {
		data, err := json.MarshalIndent(got, "", "  ")
		require.NoError(t, err)
		data = append(data, '\n')
		require.NoError(t, os.WriteFile(path, data, 0o644))
		t.Logf("golden file updated: %s", path)
		return
	}

	data, err := os.ReadFile(path)
	require.NoError(t, err, "golden file missing — run with BOX2D_UPDATE_GOLDEN=1 to create it")

	var want []goldenStepScene
	require.NoError(t, json.Unmarshal(data, &want))

	require.Len(t, got, len(want), "scene count")
	for i := range want {
		require.Equal(t, want[i].Name, got[i].Name)
		require.Equal(t, want[i].Hashes, got[i].Hashes, "scene %s step hashes differ — joint determinism broken", want[i].Name)
	}
}

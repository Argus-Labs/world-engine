// Cross-architecture determinism gate for the motor and weld joints (stage
// E9a). Each scene is stepped at 60Hz with 4 sub-steps; every step a djb2 hash
// is folded over the exact float64 bit patterns of all body transforms and
// velocities plus the contact and island counts (see golden_step_test.go). The
// golden file records the hash at every 30th step and the final step.
// Regenerate deliberately with:
//
//	BOX2D_UPDATE_GOLDEN=1 go test ./pkg/box2d/ -run TestGoldenJoint2
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

const goldenJoint2StepCount = 240

// buildMotorDriverScene builds a motor-driven sled that pushes a row of loose
// boxes along the ground. The motor mixes both springs and both velocity
// motors so every branch of b2SolveMotorJoint is exercised.
func buildMotorDriverScene(w *box2d.World) []box2d.BodyID {
	var bodies []box2d.BodyID

	gd := box2d.DefaultBodyDef()
	gd.Position = box2d.Vec2{X: 0.0, Y: -1.0}
	ground := w.CreateBody(&gd)
	groundBox := box2d.MakeBox(50.0, 1.0)
	gsd := box2d.DefaultShapeDef()
	w.CreatePolygonShape(ground, &gsd, &groundBox)

	sd := box2d.DefaultBodyDef()
	sd.Type = box2d.DynamicBody
	sd.Position = box2d.Vec2{X: -3.0, Y: 0.5}
	sled := w.CreateBody(&sd)
	sledBox := box2d.MakeBox(0.5, 0.5)
	ssd := box2d.DefaultShapeDef()
	ssd.Density = 2.0
	w.CreatePolygonShape(sled, &ssd, &sledBox)
	bodies = append(bodies, sled)

	jd := box2d.DefaultMotorJointDef()
	jd.Base.BodyIDA = ground
	jd.Base.BodyIDB = sled
	jd.Base.LocalFrameA.P = box2d.Vec2{X: -3.0, Y: 1.5}
	jd.LinearVelocity = box2d.Vec2{X: 2.0, Y: 0.0}
	jd.MaxVelocityForce = 300.0
	jd.AngularVelocity = 0.0
	jd.MaxVelocityTorque = 80.0
	jd.LinearHertz = 3.0
	jd.LinearDampingRatio = 0.6
	jd.MaxSpringForce = 150.0
	jd.AngularHertz = 3.0
	jd.AngularDampingRatio = 0.6
	jd.MaxSpringTorque = 60.0
	w.CreateMotorJoint(&jd)

	for i := range 4 {
		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.Position = box2d.Vec2{X: float64(i) + 0.5, Y: 0.4}
		crate := w.CreateBody(&bd)

		crateBox := box2d.MakeBox(0.4, 0.4)
		csd := box2d.DefaultShapeDef()
		csd.Density = 1.0
		w.CreatePolygonShape(crate, &csd, &crateBox)
		bodies = append(bodies, crate)
	}

	return bodies
}

// buildWeldTowerScene builds a tower of boxes welded to each other and to a
// static base, tipped over by an impulse. The top three welds are soft so both
// the rigid and the spring paths of b2SolveWeldJoint run.
func buildWeldTowerScene(w *box2d.World) []box2d.BodyID {
	var bodies []box2d.BodyID

	gd := box2d.DefaultBodyDef()
	gd.Position = box2d.Vec2{X: 0.0, Y: -1.0}
	ground := w.CreateBody(&gd)
	groundBox := box2d.MakeBox(50.0, 1.0)
	gsd := box2d.DefaultShapeDef()
	w.CreatePolygonShape(ground, &gsd, &groundBox)

	const linkCount = 6
	const halfHeight = 0.5

	prev := ground
	for i := range linkCount {
		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.Position = box2d.Vec2{X: 0.0, Y: (2.0*float64(i) + 1.0) * halfHeight}
		link := w.CreateBody(&bd)

		linkBox := box2d.MakeBox(0.3, halfHeight)
		lsd := box2d.DefaultShapeDef()
		lsd.Density = 1.0
		w.CreatePolygonShape(link, &lsd, &linkBox)

		jd := box2d.DefaultWeldJointDef()
		jd.Base.BodyIDA = prev
		jd.Base.BodyIDB = link
		if i == 0 {
			jd.Base.LocalFrameA.P = box2d.Vec2{X: 0.0, Y: 1.0}
		} else {
			jd.Base.LocalFrameA.P = box2d.Vec2{X: 0.0, Y: halfHeight}
		}
		jd.Base.LocalFrameB.P = box2d.Vec2{X: 0.0, Y: -halfHeight}
		if i >= 3 {
			jd.LinearHertz = 5.0
			jd.LinearDampingRatio = 0.5
			jd.AngularHertz = 5.0
			jd.AngularDampingRatio = 0.5
		}
		w.CreateWeldJoint(&jd)

		bodies = append(bodies, link)
		prev = link
	}

	// Tip the tower sideways.
	w.ApplyBodyLinearImpulse(bodies[linkCount-1], box2d.Vec2{X: 12.0, Y: 0.0},
		box2d.Vec2{X: 0.0, Y: 2.0*float64(linkCount-1)*halfHeight + halfHeight}, true)

	return bodies
}

func goldenJoint2Scenes() []goldenSceneDef {
	return []goldenSceneDef{
		{"motor_driver", buildMotorDriverScene},
		{"weld_tower", buildWeldTowerScene},
	}
}

func computeGoldenJoint2() []goldenStepScene {
	return computeGoldenJoint2Workers(0)
}

func computeGoldenJoint2Workers(workerCount int) []goldenStepScene {
	return computeGoldenScenes(goldenJoint2Scenes(), goldenJoint2StepCount, workerCount)
}

func TestGoldenJoint2(t *testing.T) {
	path := filepath.Join("testdata", "golden_joint2.json")

	got := computeGoldenJoint2()

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
		require.Equal(t, want[i].Hashes, got[i].Hashes,
			"scene %s step hashes differ — motor/weld joint determinism broken", want[i].Name)
	}
}

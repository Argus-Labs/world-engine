// Cross-architecture determinism gate for the prismatic and wheel joint
// solvers (stage E9b). Each scene is stepped at 60Hz with 4 sub-steps; every
// step a djb2 hash is folded over the exact float64 bit patterns of all body
// transforms and velocities plus the contact and island counts (see
// golden_step_test.go). The golden file records the hash at every 30th step
// and the final step. Regenerate deliberately with:
//
//	BOX2D_UPDATE_GOLDEN=1 go test ./pkg/box2d/ -run TestGoldenJoint3
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

const goldenJoint3StepCount = 240

// buildPrismaticElevatorScene builds three prismatic sliders over a ground
// box: a motor-driven vertical elevator with limits, a vertical spring slider
// and a horizontal slider with limits only.
func buildPrismaticElevatorScene(w *box2d.World) []box2d.BodyID {
	var bodies []box2d.BodyID

	gd := box2d.DefaultBodyDef()
	gd.Position = box2d.Vec2{X: 0.0, Y: -10.0}
	ground := w.CreateBody(&gd)
	groundBox := box2d.MakeBox(50.0, 10.0)
	gsd := box2d.DefaultShapeDef()
	w.CreatePolygonShape(ground, &gsd, &groundBox)

	vertical := box2d.MakeRot(0.5 * box2d.Pi)

	// Motor-driven elevator with translation limits.
	{
		ad := box2d.DefaultBodyDef()
		ad.Position = box2d.Vec2{X: 0.0, Y: 6.0}
		anchor := w.CreateBody(&ad)

		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.Position = box2d.Vec2{X: 0.0, Y: 6.0}
		car := w.CreateBody(&bd)

		carBox := box2d.MakeBox(0.75, 0.25)
		sd := box2d.DefaultShapeDef()
		sd.Density = 2.0
		w.CreatePolygonShape(car, &sd, &carBox)

		jd := box2d.DefaultPrismaticJointDef()
		jd.Base.BodyIDA = anchor
		jd.Base.BodyIDB = car
		jd.Base.LocalFrameA.Q = vertical
		jd.Base.LocalFrameB.Q = vertical
		jd.EnableLimit = true
		jd.LowerTranslation = -4.0
		jd.UpperTranslation = 0.0
		jd.EnableMotor = true
		jd.MotorSpeed = -1.5
		jd.MaxMotorForce = 200.0
		w.CreatePrismaticJoint(&jd)

		bodies = append(bodies, car)
	}

	// Vertical spring slider driving to a target translation.
	{
		ad := box2d.DefaultBodyDef()
		ad.Position = box2d.Vec2{X: -4.0, Y: 6.0}
		anchor := w.CreateBody(&ad)

		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.Position = box2d.Vec2{X: -4.0, Y: 6.0}
		slider := w.CreateBody(&bd)

		sliderBox := box2d.MakeBox(0.5, 0.5)
		sd := box2d.DefaultShapeDef()
		sd.Density = 1.0
		w.CreatePolygonShape(slider, &sd, &sliderBox)

		jd := box2d.DefaultPrismaticJointDef()
		jd.Base.BodyIDA = anchor
		jd.Base.BodyIDB = slider
		jd.Base.LocalFrameA.Q = vertical
		jd.Base.LocalFrameB.Q = vertical
		jd.EnableSpring = true
		jd.Hertz = 2.0
		jd.DampingRatio = 0.3
		jd.TargetTranslation = -1.5
		w.CreatePrismaticJoint(&jd)

		bodies = append(bodies, slider)
	}

	// Horizontal slider bouncing between its limits.
	{
		ad := box2d.DefaultBodyDef()
		ad.Position = box2d.Vec2{X: 4.0, Y: 6.0}
		anchor := w.CreateBody(&ad)

		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.Position = box2d.Vec2{X: 4.0, Y: 6.0}
		bd.LinearVelocity = box2d.Vec2{X: 4.0, Y: 0.0}
		shuttle := w.CreateBody(&bd)

		shuttleBox := box2d.MakeBox(0.4, 0.4)
		sd := box2d.DefaultShapeDef()
		sd.Density = 1.0
		w.CreatePolygonShape(shuttle, &sd, &shuttleBox)

		jd := box2d.DefaultPrismaticJointDef()
		jd.Base.BodyIDA = anchor
		jd.Base.BodyIDB = shuttle
		jd.EnableLimit = true
		jd.LowerTranslation = -2.0
		jd.UpperTranslation = 2.0
		w.CreatePrismaticJoint(&jd)

		bodies = append(bodies, shuttle)
	}

	return bodies
}

// buildWheelSuspensionScene builds a two-wheeled chassis driving over a ground
// box; both corners use a sprung, limited and motorized wheel joint.
func buildWheelSuspensionScene(w *box2d.World) []box2d.BodyID {
	gd := box2d.DefaultBodyDef()
	gd.Position = box2d.Vec2{X: 0.0, Y: -1.0}
	ground := w.CreateBody(&gd)
	groundBox := box2d.MakeBox(50.0, 1.0)
	gsd := box2d.DefaultShapeDef()
	w.CreatePolygonShape(ground, &gsd, &groundBox)

	cd := box2d.DefaultBodyDef()
	cd.Type = box2d.DynamicBody
	cd.Position = box2d.Vec2{X: 0.0, Y: 1.0}
	chassis := w.CreateBody(&cd)

	chassisBox := box2d.MakeBox(1.5, 0.25)
	csd := box2d.DefaultShapeDef()
	csd.Density = 1.0
	w.CreatePolygonShape(chassis, &csd, &chassisBox)

	vertical := box2d.MakeRot(0.5 * box2d.Pi)
	bodies := []box2d.BodyID{chassis}

	for _, offset := range []float64{-1.0, 1.0} {
		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.Position = box2d.Vec2{X: offset, Y: 0.35}
		wheel := w.CreateBody(&bd)

		circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.35}
		sd := box2d.DefaultShapeDef()
		sd.Density = 2.0
		w.CreateCircleShape(wheel, &sd, &circle)

		jd := box2d.DefaultWheelJointDef()
		jd.Base.BodyIDA = chassis
		jd.Base.BodyIDB = wheel
		jd.Base.LocalFrameA.Q = vertical
		jd.Base.LocalFrameA.P = box2d.Vec2{X: offset, Y: -0.65}
		jd.Base.LocalFrameB.Q = vertical
		jd.EnableSpring = true
		jd.Hertz = 4.0
		jd.DampingRatio = 0.7
		jd.EnableLimit = true
		jd.LowerTranslation = -0.25
		jd.UpperTranslation = 0.25
		jd.EnableMotor = true
		jd.MotorSpeed = 8.0
		jd.MaxMotorTorque = 5.0
		w.CreateWheelJoint(&jd)

		bodies = append(bodies, wheel)
	}

	return bodies
}

func goldenJoint3Scenes() []goldenSceneDef {
	return []goldenSceneDef{
		{"prismatic_elevator", buildPrismaticElevatorScene},
		{"wheel_suspension", buildWheelSuspensionScene},
	}
}

func computeGoldenJoint3() []goldenStepScene {
	return computeGoldenJoint3Workers(0)
}

func computeGoldenJoint3Workers(workerCount int) []goldenStepScene {
	return computeGoldenScenes(goldenJoint3Scenes(), goldenJoint3StepCount, workerCount)
}

func TestGoldenJoint3(t *testing.T) {
	path := filepath.Join("testdata", "golden_joint3.json")

	got := computeGoldenJoint3()

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
			"scene %s step hashes differ — prismatic/wheel joint determinism broken", want[i].Name)
	}
}

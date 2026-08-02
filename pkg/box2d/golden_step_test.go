// Cross-architecture determinism gate for the solver core (stage E7). Each
// scene is stepped at 60Hz with 4 sub-steps; every step a djb2 hash is folded
// over the exact float64 bit patterns of all body transforms and velocities
// plus the contact and island counts. The golden file records the hash at
// every 30th step and the final step. Regenerate deliberately with:
//
//	BOX2D_UPDATE_GOLDEN=1 go test ./pkg/box2d/ -run TestGoldenStep
//
// and commit the diff. A mismatch on one architecture only means an FMA or
// libm leak — see math_fma.go.

package box2d_test

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

const goldenStepCount = 240

type goldenStepHash struct {
	Step int    `json:"step"`
	Hash string `json:"hash"`
}

type goldenStepScene struct {
	Name   string           `json:"name"`
	Hashes []goldenStepHash `json:"hashes"`
}

// djb2Fold folds one uint64 value into a djb2 hash, byte by byte
// (little-endian).
func djb2Fold(hash uint64, value uint64) uint64 {
	for i := 0; i < 8; i++ {
		b := byte(value >> (8 * i))
		hash = hash*33 + uint64(b)
	}
	return hash
}

// hashWorldState hashes all body states (position, rotation, velocities) in
// body creation order plus the contact and island counts.
func hashWorldState(w *box2d.World, bodies []box2d.BodyID) uint64 {
	hash := uint64(5381)
	for _, body := range bodies {
		p := w.BodyPosition(body)
		q := w.BodyRotation(body)
		v := w.BodyLinearVelocity(body)
		av := w.BodyAngularVelocity(body)

		hash = djb2Fold(hash, math.Float64bits(p.X))
		hash = djb2Fold(hash, math.Float64bits(p.Y))
		hash = djb2Fold(hash, math.Float64bits(q.C))
		hash = djb2Fold(hash, math.Float64bits(q.S))
		hash = djb2Fold(hash, math.Float64bits(v.X))
		hash = djb2Fold(hash, math.Float64bits(v.Y))
		hash = djb2Fold(hash, math.Float64bits(av))
	}

	counters := w.Counters()
	hash = djb2Fold(hash, uint64(counters.ContactCount))
	hash = djb2Fold(hash, uint64(counters.IslandCount))
	return hash
}

// buildPyramidScene builds a 6-row pyramid of boxes on the ground.
func buildPyramidScene(w *box2d.World) []box2d.BodyID {
	var bodies []box2d.BodyID

	gd := box2d.DefaultBodyDef()
	gd.Position = box2d.Vec2{X: 0.0, Y: -10.0}
	ground := w.CreateBody(&gd)
	groundBox := box2d.MakeBox(50.0, 10.0)
	gsd := box2d.DefaultShapeDef()
	w.CreatePolygonShape(ground, &gsd, &groundBox)

	const rows = 6
	h := 0.5
	for i := range rows {
		y := (2.0*float64(i)+1.0)*h + 0.02*float64(i)
		for j := i; j < rows; j++ {
			x := float64(i)*h + 2.0*h*float64(j-i) - h*float64(rows-i-1)

			bd := box2d.DefaultBodyDef()
			bd.Type = box2d.DynamicBody
			bd.Position = box2d.Vec2{X: x, Y: y}
			body := w.CreateBody(&bd)

			box := box2d.MakeBox(h, h)
			sd := box2d.DefaultShapeDef()
			sd.Density = 1.0
			w.CreatePolygonShape(body, &sd, &box)
			bodies = append(bodies, body)
		}
	}

	return bodies
}

// buildRainScene builds the mixed-shapes rain: circles, boxes and capsules
// falling onto a ground box and a segment ramp.
func buildRainScene(w *box2d.World) []box2d.BodyID {
	return buildMixedScene(w)
}

// buildRestitutionScene builds a row of balls with varying restitution.
func buildRestitutionScene(w *box2d.World) []box2d.BodyID {
	var bodies []box2d.BodyID

	gd := box2d.DefaultBodyDef()
	gd.Position = box2d.Vec2{X: 0.0, Y: -10.0}
	ground := w.CreateBody(&gd)
	groundBox := box2d.MakeBox(50.0, 10.0)
	gsd := box2d.DefaultShapeDef()
	w.CreatePolygonShape(ground, &gsd, &groundBox)

	restitutions := []float64{0.0, 0.2, 0.4, 0.6, 0.8}
	for i, r := range restitutions {
		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.Position = box2d.Vec2{X: -4.0 + 2.0*float64(i), Y: 6.0}
		ball := w.CreateBody(&bd)

		circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.5}
		sd := box2d.DefaultShapeDef()
		sd.Density = 1.0
		sd.Material.Restitution = r
		w.CreateCircleShape(ball, &sd, &circle)
		bodies = append(bodies, ball)
	}

	return bodies
}

func computeGoldenStep() []goldenStepScene {
	scenes := []struct {
		name  string
		build func(*box2d.World) []box2d.BodyID
	}{
		{"pyramid", buildPyramidScene},
		{"mixed_rain", buildRainScene},
		{"restitution_balls", buildRestitutionScene},
	}

	out := make([]goldenStepScene, 0, len(scenes))
	for _, scene := range scenes {
		def := box2d.DefaultWorldDef()
		w := box2d.NewWorld(&def)
		bodies := scene.build(w)

		var hashes []goldenStepHash
		for step := 1; step <= goldenStepCount; step++ {
			w.Step(1.0/60.0, 4)
			if step%30 == 0 || step == goldenStepCount {
				hash := hashWorldState(w, bodies)
				hashes = append(hashes, goldenStepHash{Step: step, Hash: fmt.Sprintf("%016x", hash)})
			}
		}

		w.Destroy()
		out = append(out, goldenStepScene{Name: scene.name, Hashes: hashes})
	}

	return out
}

func TestGoldenStep(t *testing.T) {
	path := filepath.Join("testdata", "golden_step.json")

	got := computeGoldenStep()

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
		require.Equal(t, want[i].Hashes, got[i].Hashes, "scene %s step hashes differ — solver determinism broken", want[i].Name)
	}
}

// Cross-architecture determinism gate for the continuous-collision stage
// (E11). A bullet-storm scene — high-speed bouncy bullets sealed in a thin
// static box, exercising the TOI sweeps, the bullet stage and the fast-body
// AABB path every step — is stepped at 60Hz with 4 sub-steps; every 30th step
// and the final step a djb2 hash is folded over the exact float64 bit
// patterns of all body transforms and velocities plus the contact and island
// counts. Regenerate deliberately with:
//
//	BOX2D_UPDATE_GOLDEN=1 go test ./pkg/box2d/ -run TestGoldenContinuous
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

const goldenContinuousStepCount = 240

func goldenContinuousScenes() []goldenSceneDef {
	return []goldenSceneDef{
		{"bullet_storm", func(w *box2d.World) []box2d.BodyID { return buildBulletStormScene(w, 40) }},
	}
}

func computeGoldenContinuous() []goldenStepScene {
	return computeGoldenContinuousWorkers(0)
}

func computeGoldenContinuousWorkers(workerCount int) []goldenStepScene {
	return computeGoldenScenes(goldenContinuousScenes(), goldenContinuousStepCount, workerCount)
}

func TestGoldenContinuous(t *testing.T) {
	path := filepath.Join("testdata", "golden_continuous.json")

	got := computeGoldenContinuous()

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
			"scene %s step hashes differ — continuous determinism broken", want[i].Name)
	}
}

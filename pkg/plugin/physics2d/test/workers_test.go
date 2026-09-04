package physics2d_test

// Config.Workers coverage for the physics2d wrapper. Before this file no test
// set Config.Workers at all, so a wrapper regression that dropped or
// mis-plumbed the value into box2d.WorldDef.WorkerCount (internal/runtime.go,
// internal/rebuild.go) was invisible.
//
//   - TestGoldenTraceWorkers replays every committed golden trace fixture
//     with Workers=4 and requires the same results as the committed goldens
//     (which the serial TestGoldenTrace also verifies) — the wrapper-level
//     byte-identical-for-every-worker-count gate.
//   - TestWorkersClampTicksWithoutPanic exercises the NewRuntime clamp for
//     out-of-range Workers values: negative and absurdly large configs must
//     register and tick, never panic (box2d.NewWorld itself rejects values
//     outside [0, MaxWorkers], so the clamp is load-bearing).
//
// Every other test keeps the serial default (makeWorld → Workers 0).

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGoldenTraceWorkers(t *testing.T) {
	t.Parallel()

	for _, sc := range goldenScenarios() {
		t.Run(sc.name, func(t *testing.T) {
			t.Parallel()

			got := runGoldenScenarioWorkers(t, sc, 4)

			path := filepath.Join("testdata", "golden_"+sc.name+".json")
			raw, err := os.ReadFile(path)
			require.NoErrorf(t, err, "missing golden fixture %s — TestGoldenTrace owns its creation", path)

			var want goldenTrace
			require.NoError(t, json.Unmarshal(raw, &want))
			compareGoldenTrace(t, want, got)
		})
	}
}

func TestWorkersClampTicksWithoutPanic(t *testing.T) {
	t.Parallel()

	for _, workers := range []int{-1, 1000} {
		t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) {
			t.Parallel()

			sc := goldenFallingCirclesFloor()
			w, _ := makeWorldWorkers(t, sc.gravity, workers)
			sc.setup(w)
			initCardinalECS(w)
			tickN(t, w, 30)
		})
	}
}

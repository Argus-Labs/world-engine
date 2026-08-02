// Cross-architecture determinism gate for the distance/GJK/TOI routines,
// mirroring golden_math_test.go. The golden file records exact float64 bit
// patterns produced on the machine that generated it; the test then requires
// bit-identical results everywhere the suite runs. Regenerate deliberately
// with:
//
//	BOX2D_UPDATE_GOLDEN=1 go test ./pkg/box2d/ -run TestGoldenDistance
//
// and commit the diff.

package box2d_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// distanceRNG is a hand-rolled 64-bit LCG (Knuth MMIX constants). The golden
// inputs must be reproducible bit-for-bit on every platform, so no math/rand.
type distanceRNG struct {
	state uint64
}

func (r *distanceRNG) next() uint64 {
	r.state = r.state*6364136223846793005 + 1442695040888963407
	return r.state
}

// uniform returns a float64 in [lo, hi) derived from exact integer arithmetic.
func (r *distanceRNG) uniform(lo, hi float64) float64 {
	u := float64(r.next()>>11) / float64(1<<53)
	return lo + (hi-lo)*u
}

func (r *distanceRNG) intn(n int) int {
	return int(r.next() % uint64(n))
}

func goldenProxy(r *distanceRNG) box2d.ShapeProxy {
	kind := r.intn(4)
	radius := 0.0
	if r.intn(2) == 1 {
		radius = r.uniform(0.05, 0.3)
	}

	switch kind {
	case 0:
		// Circle: single point with a guaranteed radius.
		center := box2d.Vec2{X: r.uniform(-1, 1), Y: r.uniform(-1, 1)}
		return box2d.MakeProxy([]box2d.Vec2{center}, 1, r.uniform(0.05, 0.5))
	case 1:
		// Segment or capsule.
		points := []box2d.Vec2{
			{X: r.uniform(-1, 1), Y: r.uniform(-1, 1)},
			{X: r.uniform(-1, 1), Y: r.uniform(-1, 1)},
		}
		return box2d.MakeProxy(points, 2, radius)
	case 2:
		// Triangle.
		s := r.uniform(0.2, 1.0)
		points := []box2d.Vec2{{X: -s, Y: 0}, {X: s, Y: 0}, {X: 0, Y: s}}
		return box2d.MakeProxy(points, 3, radius)
	default:
		// Box.
		hw := r.uniform(0.2, 1.0)
		hh := r.uniform(0.2, 1.0)
		points := []box2d.Vec2{
			{X: -hw, Y: -hh}, {X: hw, Y: -hh}, {X: hw, Y: hh}, {X: -hw, Y: hh},
		}
		return box2d.MakeProxy(points, 4, radius)
	}
}

func goldenTransform(r *distanceRNG) box2d.Transform {
	return box2d.Transform{
		P: box2d.Vec2{X: r.uniform(-2, 2), Y: r.uniform(-2, 2)},
		Q: box2d.MakeRot(r.uniform(-box2d.Pi, box2d.Pi)),
	}
}

func goldenSweep(r *distanceRNG) box2d.Sweep {
	return box2d.Sweep{
		LocalCenter: box2d.Vec2{X: r.uniform(-0.2, 0.2), Y: r.uniform(-0.2, 0.2)},
		C1:          box2d.Vec2{X: r.uniform(-2, 2), Y: r.uniform(-2, 2)},
		C2:          box2d.Vec2{X: r.uniform(-2, 2), Y: r.uniform(-2, 2)},
		Q1:          box2d.MakeRot(r.uniform(-box2d.Pi, box2d.Pi)),
		Q2:          box2d.MakeRot(r.uniform(-box2d.Pi, box2d.Pi)),
	}
}

// computeGoldenDistance evaluates ShapeDistance and TimeOfImpact over a table
// of 50 pseudo-fixed inputs (hardcoded LCG seed, no math/rand) and returns
// the full outputs as bit patterns.
func computeGoldenDistance() []goldenCase {
	rng := &distanceRNG{state: 0x9E3779B97F4A7C15}

	var distBits []float64
	var toiBits []float64
	for i := range 50 {
		proxyA := goldenProxy(rng)
		proxyB := goldenProxy(rng)

		distanceInput := box2d.DistanceInput{
			ProxyA:     proxyA,
			ProxyB:     proxyB,
			TransformA: goldenTransform(rng),
			TransformB: goldenTransform(rng),
			UseRadii:   i%2 == 0,
		}
		var cache box2d.SimplexCache
		distanceOutput := box2d.ShapeDistance(&distanceInput, &cache, nil)
		distBits = append(distBits,
			distanceOutput.Distance,
			distanceOutput.PointA.X, distanceOutput.PointA.Y,
			distanceOutput.PointB.X, distanceOutput.PointB.Y,
			distanceOutput.Normal.X, distanceOutput.Normal.Y,
		)

		toiInput := box2d.TOIInput{
			ProxyA:      proxyA,
			ProxyB:      proxyB,
			SweepA:      goldenSweep(rng),
			SweepB:      goldenSweep(rng),
			MaxFraction: 1.0,
		}
		toiOutput := box2d.TimeOfImpact(&toiInput)
		toiBits = append(toiBits,
			float64(toiOutput.State),
			toiOutput.Fraction,
			toiOutput.Point.X, toiOutput.Point.Y,
			toiOutput.Normal.X, toiOutput.Normal.Y,
		)
	}

	return []goldenCase{
		{Name: "shape_distance_table", Bits: f64bits(distBits...)},
		{Name: "time_of_impact_table", Bits: f64bits(toiBits...)},
	}
}

func TestGoldenDistance(t *testing.T) {
	t.Parallel()

	got := computeGoldenDistance()
	path := filepath.Join("testdata", "golden_distance.json")

	if os.Getenv("BOX2D_UPDATE_GOLDEN") == "1" {
		data, err := json.MarshalIndent(got, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, append(data, '\n'), 0o644))
		t.Logf("updated %s", path)
		return
	}

	data, err := os.ReadFile(path)
	require.NoError(t, err, "golden file missing — run with BOX2D_UPDATE_GOLDEN=1 to create it")

	var want []goldenCase
	require.NoError(t, json.Unmarshal(data, &want))
	require.Len(t, got, len(want), "golden case count mismatch")

	for i := range want {
		require.Equal(t, want[i].Name, got[i].Name)
		require.Equal(t, want[i].Bits, got[i].Bits,
			"bit mismatch in %q: results are not bit-identical to the golden generation — likely an FMA or libm determinism leak (see math_fma.go)",
			want[i].Name)
	}
}

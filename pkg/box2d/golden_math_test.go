// Cross-architecture determinism gate for the math foundations. The golden
// file records exact float64 bit patterns produced on the machine that
// generated it; the test then requires bit-identical results everywhere the
// suite runs (amd64, arm64, any OS). Regenerate deliberately with:
//
//	BOX2D_UPDATE_GOLDEN=1 go test ./pkg/box2d/ -run TestGoldenMath
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

type goldenCase struct {
	Name string   `json:"name"`
	Bits []string `json:"bits"`
}

func f64bits(vals ...float64) []string {
	out := make([]string, len(vals))
	for i, v := range vals {
		out[i] = fmt.Sprintf("%016x", math.Float64bits(v))
	}
	return out
}

func computeGoldenMath() []goldenCase {
	var cases []goldenCase

	// Atan2 over a fixed grid including axes and extreme ratios.
	atan2Inputs := [][2]float64{
		{0, 0}, {1, 0}, {-1, 0}, {0, 1}, {0, -1},
		{1, 1}, {-1, 1}, {1, -1}, {-1, -1},
		{0.001, 1000}, {1000, 0.001}, {-3.7, 2.9}, {12345.678, -0.001},
		{2.2250738585072014e-308, 1}, {1, 1.7976931348623157e+308},
	}
	var atan2Bits []float64
	for _, in := range atan2Inputs {
		atan2Bits = append(atan2Bits, box2d.Atan2(in[0], in[1]))
	}
	cases = append(cases, goldenCase{Name: "atan2", Bits: f64bits(atan2Bits...)})

	// ComputeCosSin over fixed angles including large magnitudes.
	angles := []float64{
		0, 0.5, -0.5, 1.5707, -1.5707, 3.1415, -3.1415,
		6.5, -6.5, 100.25, -100.25, 1e6, -1e6, 0.7853981633974483,
	}
	var csBits []float64
	for _, a := range angles {
		cs := box2d.ComputeCosSin(a)
		csBits = append(csBits, cs.Cosine, cs.Sine)
	}
	cases = append(cases, goldenCase{Name: "cos_sin", Bits: f64bits(csBits...)})

	// Long dependent chain: rotation integration amplifies any FMA
	// difference into visible divergence.
	q := box2d.RotIdentity
	for i := range 10000 {
		q = box2d.IntegrateRotation(q, 0.001+float64(float64(i%7)*1e-5))
	}
	cases = append(cases, goldenCase{Name: "integrate_rotation_chain", Bits: f64bits(q.C, q.S)})

	// Vector pipeline chain: rotate/transform/normalize feedback loop.
	v := box2d.Vec2{X: 1.0, Y: 0.0}
	xf := box2d.Transform{P: box2d.Vec2{X: 0.123, Y: -0.456}, Q: box2d.MakeRot(0.777)}
	for range 10000 {
		v = box2d.TransformPoint(xf, v)
		v = box2d.MulSV(1.0/(1.0+box2d.Length(v)), v)
		v = box2d.MulAdd(v, 0.25, box2d.RotateVector(xf.Q, box2d.LeftPerp(v)))
	}
	cases = append(cases, goldenCase{Name: "vector_pipeline_chain", Bits: f64bits(v.X, v.Y)})

	// NLerp / angular velocity sequence.
	q1 := box2d.MakeRot(-2.5)
	q2 := box2d.MakeRot(1.75)
	var nlerpBits []float64
	for t := 0.0; t <= 1.0; t += 0.125 {
		n := box2d.NLerp(q1, q2, t)
		nlerpBits = append(nlerpBits, n.C, n.S, box2d.ComputeAngularVelocity(q1, n, 60.0))
	}
	cases = append(cases, goldenCase{Name: "nlerp_angular_velocity", Bits: f64bits(nlerpBits...)})

	// Solve22 / inverse on ill-conditioned matrices.
	var matBits []float64
	mats := []box2d.Mat22{
		{CX: box2d.Vec2{X: 1e8, Y: 1}, CY: box2d.Vec2{X: 1, Y: 1e-8}},
		{CX: box2d.Vec2{X: 3, Y: -7}, CY: box2d.Vec2{X: 2.5, Y: 11}},
	}
	for _, m := range mats {
		x := box2d.Solve22(m, box2d.Vec2{X: 0.375, Y: -12.25})
		inv := box2d.GetInverse22(m)
		matBits = append(matBits, x.X, x.Y, inv.CX.X, inv.CX.Y, inv.CY.X, inv.CY.Y)
	}
	cases = append(cases, goldenCase{Name: "mat22", Bits: f64bits(matBits...)})

	// SpringDamper trajectory.
	position, velocity := 1.0, 0.0
	const h = 1.0 / 60.0
	for range 1000 {
		velocity = box2d.SpringDamper(7.5, 0.3, position, velocity, h)
		position += float64(h * velocity)
	}
	cases = append(cases, goldenCase{Name: "spring_damper", Bits: f64bits(position, velocity)})

	return cases
}

func TestGoldenMath(t *testing.T) {
	t.Parallel()

	got := computeGoldenMath()
	path := filepath.Join("testdata", "golden_math.json")

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

// TEMPORARY diagnostic for the amd64 vs arm64 TestGoldenMath divergence in
// "vector_pipeline_chain". Always passes; logs checkpoint hashes every 100
// iterations plus the MakeRot(0.777) rotation bits so a single CI run on
// amd64 can be diffed against a local arm64 run to bisect where the
// divergence appears. DELETE before merging.
package box2d_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

func TestDiagnoseVectorPipelineChain(t *testing.T) {
	q := box2d.MakeRot(0.777)
	t.Logf("MakeRot(0.777) = C:%016x S:%016x", math.Float64bits(q.C), math.Float64bits(q.S))

	cs := box2d.ComputeCosSin(0.777)
	t.Logf("ComputeCosSin(0.777) = Cosine:%016x Sine:%016x", math.Float64bits(cs.Cosine), math.Float64bits(cs.Sine))

	v := box2d.Vec2{X: 1.0, Y: 0.0}
	xf := box2d.Transform{P: box2d.Vec2{X: 0.123, Y: -0.456}, Q: q}

	for i := 1; i <= 10000; i++ {
		v = box2d.TransformPoint(xf, v)
		v = box2d.MulSV(1.0/(1.0+box2d.Length(v)), v)
		v = box2d.MulAdd(v, 0.25, box2d.RotateVector(xf.Q, box2d.LeftPerp(v)))

		if i <= 30 || i%500 == 0 {
			t.Logf("iter=%5d X:%016x Y:%016x", i, math.Float64bits(v.X), math.Float64bits(v.Y))
		}
	}

	fmt.Println("final", v)
}

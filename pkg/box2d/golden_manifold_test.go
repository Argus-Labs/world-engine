// Cross-architecture determinism gate for the manifold generation routines,
// mirroring golden_math_test.go. The golden file records exact float64 bit
// patterns produced on the machine that generated it; the test then requires
// bit-identical results everywhere the suite runs. Regenerate deliberately
// with:
//
//	BOX2D_UPDATE_GOLDEN=1 go test ./pkg/box2d/ -run TestGoldenManifold
//
// and commit the diff.

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

type goldenManifoldCase struct {
	Name string   `json:"name"`
	Bits []string `json:"bits"`
}

// manifoldBits flattens a manifold into a deterministic string encoding:
// point count, normal bit patterns, and per active point the id plus the bit
// patterns of clipPoint, anchorA, anchorB, and separation.
func manifoldBits(m box2d.Manifold) []string {
	bits := []string{fmt.Sprintf("count:%d", m.PointCount)}
	bits = append(bits, f64bits(m.Normal.X, m.Normal.Y)...)
	for i := 0; i < m.PointCount; i++ {
		p := m.Points[i]
		bits = append(bits, fmt.Sprintf("id:%04x", p.ID))
		bits = append(bits, f64bits(
			p.ClipPoint.X, p.ClipPoint.Y,
			p.AnchorA.X, p.AnchorA.Y,
			p.AnchorB.X, p.AnchorB.Y,
			p.Separation,
		)...)
	}
	return bits
}

//nolint:maintidx // straight-line table of golden configurations.
func computeGoldenManifolds() []goldenManifoldCase {
	var cases []goldenManifoldCase
	add := func(name string, m box2d.Manifold) {
		cases = append(cases, goldenManifoldCase{Name: name, Bits: manifoldBits(m)})
	}
	mkXF := func(px, py, angle float64) box2d.Transform {
		return box2d.Transform{P: box2d.Vec2{X: px, Y: py}, Q: box2d.MakeRot(angle)}
	}

	circleSmall := box2d.Circle{Center: box2d.Vec2{X: 0.1, Y: -0.05}, Radius: 0.25}
	circleBig := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.5}
	capsuleH := box2d.Capsule{Center1: box2d.Vec2{X: -0.5, Y: 0}, Center2: box2d.Vec2{X: 0.5, Y: 0}, Radius: 0.2}
	capsuleV := box2d.Capsule{Center1: box2d.Vec2{X: 0, Y: -0.4}, Center2: box2d.Vec2{X: 0, Y: 0.4}, Radius: 0.15}
	box := box2d.MakeBox(0.5, 0.5)
	boxWide := box2d.MakeBox(1.0, 0.25)
	roundedBox := box2d.MakeRoundedBox(0.4, 0.4, 0.1)
	square := box2d.MakeSquare(0.35)
	segment := box2d.Segment{Point1: box2d.Vec2{X: -1, Y: 0}, Point2: box2d.Vec2{X: 1, Y: 0}}
	segmentDiag := box2d.Segment{Point1: box2d.Vec2{X: -1, Y: -0.5}, Point2: box2d.Vec2{X: 1, Y: 0.5}}
	chainFlat := box2d.ChainSegment{
		Ghost1:  box2d.Vec2{X: -2, Y: 0},
		Segment: box2d.Segment{Point1: box2d.Vec2{X: -1, Y: 0}, Point2: box2d.Vec2{X: 1, Y: 0}},
		Ghost2:  box2d.Vec2{X: 2, Y: 0},
	}
	chainConvex := box2d.ChainSegment{
		Ghost1:  box2d.Vec2{X: -2, Y: 1},
		Segment: box2d.Segment{Point1: box2d.Vec2{X: -1, Y: 0}, Point2: box2d.Vec2{X: 1, Y: 0}},
		Ghost2:  box2d.Vec2{X: 2, Y: 1},
	}
	chainConcave := box2d.ChainSegment{
		Ghost1:  box2d.Vec2{X: -2, Y: -1},
		Segment: box2d.Segment{Point1: box2d.Vec2{X: -1, Y: 0}, Point2: box2d.Vec2{X: 1, Y: 0}},
		Ghost2:  box2d.Vec2{X: 2, Y: -1},
	}

	// b2CollideCircles
	add("circles_overlap", box2d.CollideCircles(&circleBig, mkXF(0, 0, 0), &circleSmall, mkXF(0.6, 0.1, 0.3)))
	add("circles_touch", box2d.CollideCircles(&circleBig, mkXF(-0.2, 0.4, 1.1), &circleBig, mkXF(0.75, 0.3, -0.4)))
	add("circles_speculative", box2d.CollideCircles(&circleBig, mkXF(0, 0, 0), &circleBig, mkXF(1.01, 0, 0)))
	add("circles_separated", box2d.CollideCircles(&circleBig, mkXF(0, 0, 0), &circleBig, mkXF(2.5, 0, 0)))

	// b2CollideCapsuleAndCircle
	add("capsule_circle_interior", box2d.CollideCapsuleAndCircle(&capsuleH, mkXF(0, 0, 0.2), &circleSmall, mkXF(0.1, 0.5, 0)))
	add("capsule_circle_end", box2d.CollideCapsuleAndCircle(&capsuleH, mkXF(0, 0, 0), &circleSmall, mkXF(0.75, 0.25, 0.5)))
	add("capsule_circle_separated", box2d.CollideCapsuleAndCircle(&capsuleH, mkXF(0, 0, 0), &circleSmall, mkXF(0, 3, 0)))

	// b2CollidePolygonAndCircle
	add("polygon_circle_face", box2d.CollidePolygonAndCircle(&box, mkXF(0, 0, 0), &circleSmall, mkXF(0, 0.75, 0.1)))
	add("polygon_circle_vertex", box2d.CollidePolygonAndCircle(&box, mkXF(0, 0, 0), &circleSmall, mkXF(0.62, 0.66, 0)))
	add("polygon_circle_inside", box2d.CollidePolygonAndCircle(&box, mkXF(0, 0, 0), &circleSmall, mkXF(0, 0.1, 0)))
	add("polygon_circle_rounded", box2d.CollidePolygonAndCircle(&roundedBox, mkXF(0.2, -0.1, 0.7), &circleBig, mkXF(0.3, 0.9, 0)))
	add("polygon_circle_separated", box2d.CollidePolygonAndCircle(&box, mkXF(0, 0, 0), &circleSmall, mkXF(0, 2, 0)))

	// b2CollideCapsules
	add("capsules_parallel", box2d.CollideCapsules(&capsuleH, mkXF(0, 0, 0), &capsuleH, mkXF(0.1, 0.35, 0)))
	add("capsules_parallel_rot", box2d.CollideCapsules(&capsuleH, mkXF(0.3, -0.2, 0.25), &capsuleH, mkXF(0.35, 0.2, 0.25)))
	add("capsules_cross", box2d.CollideCapsules(&capsuleH, mkXF(0, 0, 0), &capsuleV, mkXF(0.2, 0.7, 0)))
	add("capsules_end_end", box2d.CollideCapsules(&capsuleH, mkXF(0, 0, 0), &capsuleH, mkXF(1.3, 0.05, 0)))
	add("capsules_oblique", box2d.CollideCapsules(&capsuleH, mkXF(0, 0, 0), &capsuleH, mkXF(0.4, 0.3, 0.6)))
	add("capsules_speculative", box2d.CollideCapsules(&capsuleH, mkXF(0, 0, 0), &capsuleH, mkXF(0, 0.41, 0)))
	add("capsules_separated", box2d.CollideCapsules(&capsuleH, mkXF(0, 0, 0), &capsuleH, mkXF(0, 1.5, 0)))

	// b2CollideSegmentAndCapsule
	add("segment_capsule_parallel", box2d.CollideSegmentAndCapsule(&segment, mkXF(0, 0, 0), &capsuleH, mkXF(0, 0.15, 0)))
	add("segment_capsule_diag", box2d.CollideSegmentAndCapsule(&segmentDiag, mkXF(0, 0, 0), &capsuleH, mkXF(0, 0.25, 0.2)))
	add("segment_capsule_separated", box2d.CollideSegmentAndCapsule(&segment, mkXF(0, 0, 0), &capsuleH, mkXF(0, 2, 0)))

	// b2CollidePolygonAndCapsule
	add("polygon_capsule_face", box2d.CollidePolygonAndCapsule(&box, mkXF(0, 0, 0), &capsuleH, mkXF(0, 0.65, 0)))
	add("polygon_capsule_rot", box2d.CollidePolygonAndCapsule(&boxWide, mkXF(0, 0, -0.1), &capsuleV, mkXF(0.5, 0.7, 0.3)))

	// b2CollidePolygons
	add("polygons_face_overlap", box2d.CollidePolygons(&box, mkXF(0, 0, 0), &box, mkXF(0, 0.98, 0)))
	add("polygons_face_slide", box2d.CollidePolygons(&box, mkXF(0, 0, 0), &box, mkXF(0.35, 0.98, 0)))
	add("polygons_rotated", box2d.CollidePolygons(&box, mkXF(0, 0, 0.1), &square, mkXF(0.2, 0.8, -0.35)))
	add("polygons_rounded", box2d.CollidePolygons(&roundedBox, mkXF(0, 0, 0), &roundedBox, mkXF(0.1, 0.95, 0.05)))
	add("polygons_vertex_vertex", box2d.CollidePolygons(&box, mkXF(0, 0, 0), &box, mkXF(1.005, 1.005, 0)))
	add("polygons_speculative", box2d.CollidePolygons(&box, mkXF(0, 0, 0), &box, mkXF(0, 1.01, 0)))
	add("polygons_wide_narrow", box2d.CollidePolygons(&boxWide, mkXF(0, 0, 0), &box, mkXF(0.75, 0.7, 0.4)))
	add("polygons_separated", box2d.CollidePolygons(&box, mkXF(0, 0, 0), &box, mkXF(0, 1.5, 0)))

	// b2CollideSegmentAndCircle
	add("segment_circle_interior", box2d.CollideSegmentAndCircle(&segment, mkXF(0, 0, 0), &circleSmall, mkXF(0, 0.25, 0)))
	add("segment_circle_end", box2d.CollideSegmentAndCircle(&segment, mkXF(0, 0, 0.15), &circleBig, mkXF(1.2, 0.4, 0)))
	add("segment_circle_separated", box2d.CollideSegmentAndCircle(&segment, mkXF(0, 0, 0), &circleSmall, mkXF(0, 2, 0)))

	// b2CollideSegmentAndPolygon
	add("segment_polygon_face", box2d.CollideSegmentAndPolygon(&segment, mkXF(0, 0, 0), &box, mkXF(0, 0.45, 0)))
	add("segment_polygon_diag", box2d.CollideSegmentAndPolygon(&segmentDiag, mkXF(0, 0, 0), &square, mkXF(0.3, 0.4, 0.2)))
	add("segment_polygon_separated", box2d.CollideSegmentAndPolygon(&segment, mkXF(0, 0, 0), &box, mkXF(0, 2, 0)))

	// b2CollideChainSegmentAndCircle
	add("chain_circle_interior", box2d.CollideChainSegmentAndCircle(&chainFlat, mkXF(0, 0, 0), &circleSmall, mkXF(0, -0.2, 0)))
	add("chain_circle_wrong_side", box2d.CollideChainSegmentAndCircle(&chainFlat, mkXF(0, 0, 0), &circleSmall, mkXF(0, 0.4, 0)))
	add("chain_circle_ghost_reject", box2d.CollideChainSegmentAndCircle(&chainFlat, mkXF(0, 0, 0), &circleBig, mkXF(-1.1, -0.3, 0)))
	add("chain_circle_convex_admit", box2d.CollideChainSegmentAndCircle(&chainConvex, mkXF(0, 0, 0), &circleBig, mkXF(-1.1, -0.3, 0)))

	// b2CollideChainSegmentAndCapsule (fresh cache per call, as the narrow
	// phase does on a cold contact).
	chainCapsule := func(seg *box2d.ChainSegment, xfA box2d.Transform, capsule *box2d.Capsule, xfB box2d.Transform) box2d.Manifold {
		var cache box2d.SimplexCache
		return box2d.CollideChainSegmentAndCapsule(seg, xfA, capsule, xfB, &cache)
	}
	add("chain_capsule_parallel", chainCapsule(&chainFlat, mkXF(0, 0, 0), &capsuleH, mkXF(0, -0.15, 0)))
	add("chain_capsule_oblique", chainCapsule(&chainFlat, mkXF(0, 0, 0), &capsuleH, mkXF(0.4, -0.3, 0.5)))
	add("chain_capsule_concave", chainCapsule(&chainConcave, mkXF(0, 0, 0), &capsuleH, mkXF(-0.8, -0.2, 0)))
	add("chain_capsule_separated", chainCapsule(&chainFlat, mkXF(0, 0, 0), &capsuleH, mkXF(0, -2, 0)))

	// b2CollideChainSegmentAndPolygon
	chainPolygon := func(seg *box2d.ChainSegment, xfA box2d.Transform, poly *box2d.Polygon, xfB box2d.Transform) box2d.Manifold {
		var cache box2d.SimplexCache
		return box2d.CollideChainSegmentAndPolygon(seg, xfA, poly, xfB, &cache)
	}
	add("chain_polygon_face", chainPolygon(&chainFlat, mkXF(0, 0, 0), &box, mkXF(0, -0.4, 0)))
	add("chain_polygon_rot", chainPolygon(&chainFlat, mkXF(0, 0, 0), &square, mkXF(0.3, -0.35, 0.3)))
	add("chain_polygon_speculative", chainPolygon(&chainFlat, mkXF(0, 0, 0), &box, mkXF(0, -0.51, 0)))
	add("chain_polygon_convex_end", chainPolygon(&chainConvex, mkXF(0, 0, 0), &square, mkXF(1.1, -0.3, 0.2)))
	add("chain_polygon_concave", chainPolygon(&chainConcave, mkXF(0, 0, 0), &box, mkXF(-0.6, -0.45, 0)))
	add("chain_polygon_wrong_side", chainPolygon(&chainFlat, mkXF(0, 0, 0), &box, mkXF(0, 0.6, 0)))

	return cases
}

func TestGoldenManifold(t *testing.T) {
	t.Parallel()

	got := computeGoldenManifolds()
	path := filepath.Join("testdata", "golden_manifold.json")

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

	var want []goldenManifoldCase
	require.NoError(t, json.Unmarshal(data, &want))
	require.Len(t, got, len(want), "golden case count mismatch")

	for i := range want {
		require.Equal(t, want[i].Name, got[i].Name)
		require.Equal(t, want[i].Bits, got[i].Bits,
			"bit mismatch in %q: results are not bit-identical to the golden generation — likely an FMA or libm determinism leak (see math_fma.go)",
			want[i].Name)
	}
}

// TestGoldenManifoldDeterminism runs the full golden configuration table twice
// in-process and requires identical manifolds, including inactive fields.
func TestGoldenManifoldDeterminism(t *testing.T) {
	t.Parallel()

	first := computeGoldenManifolds()
	second := computeGoldenManifolds()
	require.Equal(t, first, second)

	// Also compare full manifold structs (not just the serialized bits) for a
	// pair of representative configurations.
	box := box2d.MakeBox(0.5, 0.5)
	capsule := box2d.Capsule{Center1: box2d.Vec2{X: -0.5, Y: 0}, Center2: box2d.Vec2{X: 0.5, Y: 0}, Radius: 0.2}
	xfA := box2d.Transform{P: box2d.Vec2{X: 0.1, Y: -0.2}, Q: box2d.MakeRot(0.4)}
	xfB := box2d.Transform{P: box2d.Vec2{X: 0.15, Y: 0.55}, Q: box2d.MakeRot(-0.3)}

	m1 := box2d.CollidePolygons(&box, xfA, &box, xfB)
	m2 := box2d.CollidePolygons(&box, xfA, &box, xfB)
	require.Equal(t, m1, m2)

	c1 := box2d.CollideCapsules(&capsule, xfA, &capsule, xfB)
	c2 := box2d.CollideCapsules(&capsule, xfA, &capsule, xfB)
	require.Equal(t, c1, c2)

	// Guard against NaN leaking into any produced manifold.
	for _, cse := range first {
		for _, b := range cse.Bits {
			var bits uint64
			if _, err := fmt.Sscanf(b, "%016x", &bits); err == nil && len(b) == 16 {
				require.False(t, math.IsNaN(math.Float64frombits(bits)), "NaN in case %s", cse.Name)
			}
		}
	}
}

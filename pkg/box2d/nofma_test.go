// Determinism gate: the compiled package must contain no fused
// multiply-add instructions on any architecture.
//
// pkg/box2d promises bit-identical results across architectures. An FMA
// computes a*b+c with a single rounding, so any expression the compiler is
// allowed to fuse produces different bits on a target that has FMA (arm64
// always; amd64 only at GOAMD64=v3+) than on one that does not. Reviewing
// source for this is unreliable: fusion is legal whenever an *unrounded*
// product reaches a +/-, including when it arrives through a variable, a
// struct field, or a function parameter that is later inlined. Only an
// explicit float64() conversion on the value forces the intermediate
// rounding that forbids fusion (Go spec, "Floating point operators").
//
// So instead of auditing expressions, this test asks the compiler: build the
// package for FMA-capable targets and assert the emitted assembly contains
// no FMA instruction. That is a complete check — if none is emitted, no
// fusion-induced divergence is possible.
//
// When this fails, wrap the products feeding the reported line in float64().
// Rounding the *consumer's* operands works too and is often the smaller fix,
// since it survives inlining (see mulAdd/dot2 in math_fma.go).

package box2d_test

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// fmaMnemonic matches the fused multiply-add instructions Go emits.
// arm64: FMADDD/FMSUBD/FNMADDD/FNMSUBD. amd64 (GOAMD64=v3): VFMADD*/VFMSUB*.
var fmaMnemonic = regexp.MustCompile(`\b(FN?M(ADD|SUB)[DS]|VFN?M(ADD|SUB)[0-9]*[SP]?[DS])\b`)

func TestNoFusedMultiplyAdd(t *testing.T) {
	t.Parallel()

	targets := []struct {
		name string
		env  []string
	}{
		// arm64 always has FMA and the compiler fuses whenever allowed.
		{name: "arm64", env: []string{"GOARCH=arm64", "GOOS=linux"}},
		// amd64 gains FMA at v3; the guarantee must hold there too.
		{name: "amd64/v3", env: []string{"GOARCH=amd64", "GOOS=linux", "GOAMD64=v3"}},
	}

	for _, target := range targets {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()

			// -S writes the generated assembly to stderr. -a forces a rebuild
			// so a cached package cannot hide a regression.
			cmd := exec.Command("go", "build", "-a", "-gcflags=-S", ".")
			cmd.Env = append(cmd.Environ(), target.env...)
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "compiling for %s failed:\n%s", target.name, out)

			var offenders []string
			for _, line := range strings.Split(string(out), "\n") {
				if fmaMnemonic.MatchString(line) {
					offenders = append(offenders, strings.TrimSpace(line))
				}
			}

			require.Empty(t, offenders,
				"%s: found %d fused multiply-add instruction(s); results will not be "+
					"bit-identical against targets without FMA. Round the products feeding "+
					"these lines with float64(...). Offenders:\n%s",
				target.name, len(offenders), strings.Join(offenders, "\n"))
		})
	}
}

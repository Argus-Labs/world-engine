package sdkgen

import (
	"reflect"
	"testing"

	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/stretchr/testify/require"
)

// TestImmutablePkgPathMatchesEngine binds the path discovery matches on to where the type actually
// lives. engineSliceElem compares it exactly, so if pkg/immutable ever moves and this constant does
// not follow, the generator stops recognising Slice and starts reporting every Slice field as a
// value-only violation — with no compile error anywhere to say why.
//
// The tests in engine_slice_test.go cannot catch that on their own: their shim is written to a
// hardcoded path too, so a stale constant and a stale shim agree with each other and keep passing.
// This is the only assertion tied to the real package.
func TestImmutablePkgPathMatchesEngine(t *testing.T) {
	t.Parallel()

	require.Equal(t, reflect.TypeFor[immutable.Slice[int]]().PkgPath(), immutablePkgPath)
}

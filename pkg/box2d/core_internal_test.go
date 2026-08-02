// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/core.c, src/core.h, include/box2d/base.h.

package box2d

import (
	"testing"

	tassert "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHash_GoldenValues(t *testing.T) {
	t.Parallel()

	require.Equal(t, uint32(5381), Hash(HashInit, []byte{}))
	require.Equal(t, uint32(177670), Hash(HashInit, []byte("a")))
	require.Equal(t, uint32(261238937), Hash(HashInit, []byte("hello")))
	require.Equal(t, uint32(279393645), Hash(HashInit, []byte("world")))

	// Chained hash: feed "hello" then "world" should equal hashing "helloworld".
	h := Hash(HashInit, []byte("hello"))
	require.Equal(t, Hash(HashInit, []byte("helloworld")), Hash(h, []byte("world")))
}

func TestGetVersion(t *testing.T) {
	t.Parallel()

	v := GetVersion()
	tassert.Equal(t, 3, v.Major)
	tassert.Equal(t, 2, v.Minor)
	tassert.Zero(t, v.Revision)
}

func TestLengthUnitsPerMeter(t *testing.T) {
	t.Parallel()

	require.InDelta(t, 1.0, GetLengthUnitsPerMeter(), 0)

	SetLengthUnitsPerMeter(2.0)
	tassert.InDelta(t, 2.0, GetLengthUnitsPerMeter(), 0)

	// Restore the default so other tests are not affected.
	SetLengthUnitsPerMeter(1.0)
}

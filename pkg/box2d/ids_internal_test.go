// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file include/box2d/id.h.

package box2d

import (
	"testing"

	tassert "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorldID_PackUnpack(t *testing.T) {
	t.Parallel()

	id := WorldID{index1: 42, generation: 7}
	x := PackWorldID(id)
	require.Equal(t, id, UnpackWorldID(x))

	null := WorldID{}
	require.True(t, null.IsNull())
	require.False(t, null.IsNonNull())
	require.Equal(t, WorldID{}, UnpackWorldID(PackWorldID(null)))
}

func TestBodyID_PackUnpack(t *testing.T) {
	t.Parallel()

	id := BodyID{index1: 42, world0: 3, generation: 5}
	x := PackBodyID(id)
	require.Equal(t, id, UnpackBodyID(x))

	negative := BodyID{index1: -1, world0: 7, generation: 9}
	require.Equal(t, negative, UnpackBodyID(PackBodyID(negative)))

	null := BodyID{}
	require.True(t, null.IsNull())
	require.False(t, null.IsNonNull())
}

func TestShapeID_PackUnpack(t *testing.T) {
	t.Parallel()

	id := ShapeID{index1: 12345, world0: 2, generation: 99}
	x := PackShapeID(id)
	require.Equal(t, id, UnpackShapeID(x))

	null := ShapeID{}
	require.True(t, null.IsNull())
	require.False(t, null.IsNonNull())
}

func TestChainID_PackUnpack(t *testing.T) {
	t.Parallel()

	id := ChainID{index1: 999, world0: 1, generation: 12}
	x := PackChainID(id)
	require.Equal(t, id, UnpackChainID(x))

	null := ChainID{}
	require.True(t, null.IsNull())
	require.False(t, null.IsNonNull())
}

func TestJointID_PackUnpack(t *testing.T) {
	t.Parallel()

	id := JointID{index1: -123, world0: 5, generation: 255}
	x := PackJointID(id)
	require.Equal(t, id, UnpackJointID(x))

	null := JointID{}
	require.True(t, null.IsNull())
	require.False(t, null.IsNonNull())
}

func TestID_IsNullAndIsNonNull(t *testing.T) {
	t.Parallel()

	tassert.True(t, WorldID{}.IsNull())
	tassert.False(t, WorldID{index1: 1}.IsNull())
	tassert.True(t, WorldID{index1: 1}.IsNonNull())

	tassert.True(t, BodyID{}.IsNull())
	tassert.False(t, BodyID{index1: -1}.IsNull())
	tassert.True(t, BodyID{index1: -1}.IsNonNull())
}

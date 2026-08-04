// Cross-world id ownership tests — the halves whose observable behavior
// depends on the box2d_asserts build tag.
//
// Every Destroy* function rejects an id minted by another world with
// `assert(false); return`, before touching any state. Without the tag the
// assert compiles out and the call is silently ignored
// (TestDestroyIgnoresForeignIDs); with the tag the same call panics before any
// mutation (TestDestroyPanicsOnForeignIDsWithAsserts). Both files pin that the
// foreign world's objects survive either way.

//go:build !box2d_asserts

package box2d_test

import (
	"testing"

	tassert "github.com/stretchr/testify/assert"
)

func TestDestroyIgnoresForeignIDs(t *testing.T) {
	t.Parallel()

	a := buildIdentityScene(t)
	b := buildIdentityScene(t)

	before := b.world.BodyPosition(b.body)
	beforeCounters := b.world.Counters()

	// Every destructive call below names a live slot in b, so before owner
	// tokens existed each of these silently destroyed b's object.
	b.world.DestroyJoint(a.joint, true)
	b.world.DestroyChain(a.chain)
	b.world.DestroyShape(a.shape, true)
	b.world.DestroyBody(a.body)

	// b is untouched.
	tassert.True(t, b.world.IsBodyValid(b.body))
	tassert.True(t, b.world.IsShapeValid(b.shape))
	tassert.True(t, b.world.IsChainValid(b.chain))
	tassert.True(t, b.world.IsJointValid(b.joint))
	tassert.Equal(t, before, b.world.BodyPosition(b.body))
	tassert.Equal(t, beforeCounters, b.world.Counters())

	// ...and so is a: the call was ignored, not forwarded.
	tassert.True(t, a.world.IsBodyValid(a.body))
	tassert.True(t, a.world.IsShapeValid(a.shape))
	tassert.True(t, a.world.IsChainValid(a.chain))
	tassert.True(t, a.world.IsJointValid(a.joint))

	// The owning world still destroys its own body.
	a.world.DestroyBody(a.body)
	tassert.False(t, a.world.IsBodyValid(a.body))
}

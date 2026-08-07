// Cross-world id ownership tests — box2d_asserts half. See
// world_identity_noasserts_test.go for the pairing and the release-build half.

//go:build box2d_asserts

package box2d_test

import (
	"testing"

	tassert "github.com/stretchr/testify/assert"
)

// TestDestroyPanicsOnForeignIDsWithAsserts pins the debug-build shape of the
// foreign-id reject: every Destroy* hits `assert(false)` before touching any
// state, so the call panics and both worlds' objects survive.
func TestDestroyPanicsOnForeignIDsWithAsserts(t *testing.T) {
	t.Parallel()

	a := buildIdentityScene(t)
	b := buildIdentityScene(t)

	before := b.world.BodyPosition(b.body)
	beforeCounters := b.world.Counters()

	tassert.Panics(t, func() { b.world.DestroyJoint(a.joint, true) })
	tassert.Panics(t, func() { b.world.DestroyChain(a.chain) })
	tassert.Panics(t, func() { b.world.DestroyShape(a.shape, true) })
	tassert.Panics(t, func() { b.world.DestroyBody(a.body) })

	// The assert fires before any mutation, so b is untouched...
	tassert.True(t, b.world.IsBodyValid(b.body))
	tassert.True(t, b.world.IsShapeValid(b.shape))
	tassert.True(t, b.world.IsChainValid(b.chain))
	tassert.True(t, b.world.IsJointValid(b.joint))
	tassert.Equal(t, before, b.world.BodyPosition(b.body))
	tassert.Equal(t, beforeCounters, b.world.Counters())

	// ...and so is a: the call was rejected, not forwarded.
	tassert.True(t, a.world.IsBodyValid(a.body))
	tassert.True(t, a.world.IsShapeValid(a.shape))
	tassert.True(t, a.world.IsChainValid(a.chain))
	tassert.True(t, a.world.IsJointValid(a.joint))

	// The owning world still destroys its own body.
	a.world.DestroyBody(a.body)
	tassert.False(t, a.world.IsBodyValid(a.body))
}

package errors_test

import (
	"context"
	"errors"
	"testing"

	"github.com/rotisserie/eris"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgerrors "github.com/argus-labs/world-engine/cli/internal/errors"
)

func TestShouldPrint_Silent(t *testing.T) {
	t.Parallel()

	s := pkgerrors.NewSilent(errors.New("user cancelled"))
	require.Error(t, s)
	assert.True(t, pkgerrors.IsSilent(s))
	assert.False(t, pkgerrors.ShouldPrint(s))
}

func TestShouldPrint_NonSilent(t *testing.T) {
	t.Parallel()

	err := eris.New("visible error")
	assert.True(t, pkgerrors.ShouldPrint(err))
}

// TestIsSilent_ThroughErrorsJoin reproduces the regression: kong's Context.Run always
// wraps a command's returned error with errors.Join, whose *joinError exposes inner
// errors via Unwrap() []error. eris.As only walks Unwrap() error and is blind to that
// shape, so a SilentError attached by a command is hidden after kong dispatch.
func TestIsSilent_ThroughErrorsJoin(t *testing.T) {
	t.Parallel()

	base := errors.New("user cancelled")
	silent := pkgerrors.NewSilent(base)
	joined := errors.Join(silent, nil) // exact shape kong returns (runErr, nil hook err)

	assert.True(t, pkgerrors.IsSilent(joined), "SilentError must be detected through errors.Join")
	assert.False(
		t,
		pkgerrors.ShouldPrint(joined),
		"ShouldPrint must be false for a silent error wrapped by errors.Join",
	)
}

// TestShouldPrint_NonSilentThroughErrorsJoin guards against over-suppression: a plain
// (non-silent) error wrapped by errors.Join must still be printable.
func TestShouldPrint_NonSilentThroughErrorsJoin(t *testing.T) {
	t.Parallel()

	plain := eris.New("real failure")
	joined := errors.Join(plain, nil)

	assert.False(t, pkgerrors.IsSilent(joined))
	assert.True(t, pkgerrors.ShouldPrint(joined))
}

// TestIsSilent_ThroughErisWrap ensures the fix does not regress the prior eris.As
// behavior for a SilentError wrapped only by eris.Wrap (no errors.Join). stdlib
// errors.As walks eris.wrapError's Unwrap() error the same way eris.As did.
func TestIsSilent_ThroughErisWrap(t *testing.T) {
	t.Parallel()

	silent := pkgerrors.NewSilent(context.Canceled)
	wrapped := eris.Wrap(silent, "initial shard build")

	assert.True(t, pkgerrors.IsSilent(wrapped))
	assert.False(t, pkgerrors.ShouldPrint(wrapped))
}

// TestIsSilent_ThroughErisWrapThenErrorsJoin mirrors the production chain for world
// start: a command returns eris.Wrap(NewSilent(ctx.Canceled), "initial shard build")
// and kong then wraps that with errors.Join. The SilentError must still be found.
func TestIsSilent_ThroughErisWrapThenErrorsJoin(t *testing.T) {
	t.Parallel()

	silent := pkgerrors.NewSilent(context.Canceled)
	wrapped := eris.Wrap(silent, "initial shard build")
	joined := errors.Join(wrapped, nil)

	assert.True(t, pkgerrors.IsSilent(joined), "SilentError must be detected through eris.Wrap + errors.Join")
	assert.False(t, pkgerrors.ShouldPrint(joined))
}

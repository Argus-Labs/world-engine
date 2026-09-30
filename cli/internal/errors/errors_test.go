package errors_test

import (
	"context"
	"errors"
	"fmt"
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

// Regression: kong returns errors.Join(runErr, hookErr), which eris.As can't
// see into, so every Ctrl+C was printed and sent to Sentry.
func TestShouldPrint_SilentInsideKongJoin(t *testing.T) {
	t.Parallel()

	s := eris.Wrap(pkgerrors.NewSilent(errors.New("user cancelled")), "cluster start")
	assert.False(t, pkgerrors.ShouldPrint(errors.Join(s, nil)))
}

func TestJoinFailures(t *testing.T) {
	t.Parallel()

	real1 := eris.New("cluster failed")
	real2 := eris.New("build failed")
	silent := pkgerrors.NewSilent(errors.New("canceled"))

	require.NoError(t, pkgerrors.JoinFailures(nil, nil))
	assert.Same(t, silent, pkgerrors.JoinFailures(nil, silent), "only a cancel: stay silent")

	got := pkgerrors.JoinFailures(real1, silent)
	assert.True(t, pkgerrors.ShouldPrint(got), "a cancel must not hide a real failure")
	require.ErrorIs(t, got, real1)

	got = pkgerrors.JoinFailures(real1, real2)
	require.ErrorIs(t, got, real1)
	require.ErrorIs(t, got, real2)

	// Regression: shard A failed, then Ctrl+C hit shard B. The join read as
	// canceled, so A's error was never printed.
	canceled := fmt.Errorf("redeploy shard b: %w", context.Canceled)
	got = pkgerrors.JoinFailures(real1, canceled)
	require.ErrorIs(t, got, real1)
	require.NotErrorIs(t, got, context.Canceled)
	assert.Same(t, canceled, pkgerrors.JoinFailures(nil, canceled), "only a cancel: stay a cancel")
}

// TestIsSilent_ThroughErrorsJoin is the regression test for the kong-wrapping bug.
// kong wraps command return errors with errors.Join (Unwrap() []error), and eris.As
// v0.5.4 only traverses Unwrap() error — so a SilentError returned from a command
// (e.g. spinner.Run on Ctrl+C) was invisibly printed and sent to Sentry. The fix
// uses standard errors.As which supports Unwrap() []error since Go 1.20.
func TestIsSilent_ThroughErrorsJoin(t *testing.T) {
	t.Parallel()

	silent := pkgerrors.NewSilent(eris.New("cancelled build"))
	joined := errors.Join(silent)

	require.Error(t, joined)
	assert.True(t, pkgerrors.IsSilent(joined),
		"IsSilent must see through errors.Join — eris.As cannot, errors.As can")
	assert.False(t, pkgerrors.ShouldPrint(joined),
		"a joined SilentError must not be printed")
}

// TestIsSilent_ThroughErisWrapAndJoin matches the exact E2E chain: spinner.Run returns
// NewSilent(opErr) where opErr is eris.Wrap(execExitError, "ensure buf image"); kong
// then wraps that in errors.Join. Both layers must be traversed for IsSilent to fire.
func TestIsSilent_ThroughErisWrapAndJoin(t *testing.T) {
	t.Parallel()

	inner := eris.Wrap(errors.New("signal: killed"), "docker build buf image")
	silent := pkgerrors.NewSilent(eris.Wrap(inner, "ensure buf image"))
	joined := errors.Join(silent)

	require.Error(t, joined)
	assert.True(t, pkgerrors.IsSilent(joined),
		"IsSilent must traverse eris.Wrap + errors.Join to find the SilentError")
	assert.False(t, pkgerrors.ShouldPrint(joined))
}

// TestIsSilent_NonSilentThroughJoin guards against false positives: a joined non-silent
// error must still be detected as non-silent (printable).
func TestIsSilent_NonSilentThroughJoin(t *testing.T) {
	t.Parallel()

	err := eris.New("genuine failure")
	joined := errors.Join(err)

	require.Error(t, joined)
	assert.False(t, pkgerrors.IsSilent(joined))
	assert.True(t, pkgerrors.ShouldPrint(joined))
}

// TestIsSilent_ThroughErisWrap confirms the standard errors.As still walks the eris
// single-unwrap chain (Unwrap() error) that eris.Wrap produces.
func TestIsSilent_ThroughErisWrap(t *testing.T) {
	t.Parallel()

	silent := pkgerrors.NewSilent(eris.New("cancelled"))
	wrapped := eris.Wrap(silent, "outer wrap")

	require.Error(t, wrapped)
	assert.True(t, pkgerrors.IsSilent(wrapped),
		"IsSilent must traverse eris.Wrap's Unwrap() error chain")
}

// TestIsSilent_NilEdgeCases guards the nil-safety paths.
func TestIsSilent_NilEdgeCases(t *testing.T) {
	t.Parallel()

	assert.False(t, pkgerrors.IsSilent(nil))
	// ShouldPrint(nil) returns true (!IsSilent(nil) == !false == true); main.go
	// guards with `if err != nil` so ShouldPrint is never called on nil in practice.
	assert.True(t, pkgerrors.ShouldPrint(nil))
	assert.Nil(t, pkgerrors.NewSilent(nil))
}

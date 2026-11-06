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

// A raw [context.Canceled] (Ctrl+C) is an expected user action, not a failure,
// so ShouldPrint must return false — aligning with JoinFailures, which treats a
// cancel as "not a failure". Before the fix, ShouldPrint only checked for
// *SilentError and a plain cancel was printed and Sentry-captured.
func TestShouldPrint_ContextCanceled(t *testing.T) {
	t.Parallel()

	assert.False(t, pkgerrors.ShouldPrint(context.Canceled))
}

// The cancel that leaks from the pre-dashboard Docker calls is eris-wrapped
// (e.g. eris.Wrap(context.Canceled, "list containers for project ...")), not a
// bare [context.Canceled]. ShouldPrint must see through eris and treat it as
// non-printable — this is the exact error form that reached main.go's gate.
func TestShouldPrint_ErisWrappedCancel(t *testing.T) {
	t.Parallel()

	canceled := eris.Wrap(context.Canceled, `list containers for project "demo"`)
	assert.True(t, errors.Is(canceled, context.Canceled), "sanity: eris wrap preserves context.Canceled")
	assert.False(t, pkgerrors.ShouldPrint(canceled))
}

// Kong wraps every command error in [errors.Join](runErr, hookErr); a cancel that
// escapes through that join must still be recognised as non-printable.
func TestShouldPrint_CancelInsideKongJoin(t *testing.T) {
	t.Parallel()

	canceled := eris.Wrap(context.Canceled, "check world status")
	assert.False(t, pkgerrors.ShouldPrint(errors.Join(canceled, nil)))
}

// A SilentError wrapping a cancel is already non-printable via IsSilent; the
// cancel check must not interfere — both paths agree it should not print.
func TestShouldPrint_SilentWrappingCancel(t *testing.T) {
	t.Parallel()

	s := pkgerrors.NewSilent(context.Canceled)
	assert.False(t, pkgerrors.ShouldPrint(s))
}

// JoinFailures drops a cancel when there are real failures, so its result
// contains only the real failures and ShouldPrint must return true — a cancel
// must never hide a real failure.
func TestShouldPrint_RealFailureJoinedWithCancelViaJoinFailures(t *testing.T) {
	t.Parallel()

	realErr := eris.New("cluster failed")
	canceled := fmt.Errorf("redeploy shard b: %w", context.Canceled)
	got := pkgerrors.JoinFailures(realErr, canceled)
	require.ErrorIs(t, got, realErr)
	assert.NotErrorIs(t, got, context.Canceled, "JoinFailures must drop the cancel")
	assert.True(t, pkgerrors.ShouldPrint(got), "a real failure must still print")
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

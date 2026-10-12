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

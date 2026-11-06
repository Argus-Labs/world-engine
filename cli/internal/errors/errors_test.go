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

// A cancel (Ctrl+C) is an expected user action, not a failure, so ShouldPrint
// must return false for each form a cancel can take reaching this gate.
func TestShouldPrint_CancelSuppressed(t *testing.T) {
	t.Parallel()

	canceled := eris.Wrap(context.Canceled, `list containers for project "demo"`)
	cases := []struct {
		name string
		err  error
	}{
		{"bare", context.Canceled},
		{"eris-wrapped", canceled},
		{"inside kong join", errors.Join(canceled, nil)},
		{"silent wrapping cancel", pkgerrors.NewSilent(context.Canceled)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.False(t, pkgerrors.ShouldPrint(tc.err))
		})
	}
}

// A SilentError inside kong's errors.Join stays non-printable: kong wraps every
// command error this way, which eris.As can't see into.
func TestShouldPrint_SilentInsideKongJoin(t *testing.T) {
	t.Parallel()

	s := eris.Wrap(pkgerrors.NewSilent(errors.New("user cancelled")), "cluster start")
	assert.False(t, pkgerrors.ShouldPrint(errors.Join(s, nil)))
}

// ShouldPrint uses [errors.Is], which walks every branch of an [errors.Join], so a
// join carrying both a real failure and a cancel is suppressed wholesale. This
// differs from JoinFailures, which classifies whole arguments: a cancel passed as
// its own argument is dropped while a real failure passed separately is kept, but
// a single [errors.Join] of the two is treated as cancellation there too.
func TestShouldPrint_MixedJoinSuppressesWholeJoin(t *testing.T) {
	t.Parallel()

	realErr := eris.New("cluster failed")
	joined := errors.Join(realErr, context.Canceled)
	assert.False(t, pkgerrors.ShouldPrint(joined),
		"a join carrying a cancel is suppressed even with a real failure")
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
	assert.True(t, pkgerrors.ShouldPrint(got), "a real failure must still print")
	assert.Same(t, canceled, pkgerrors.JoinFailures(nil, canceled), "only a cancel: stay a cancel")
}

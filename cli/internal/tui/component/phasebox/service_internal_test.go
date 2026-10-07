package phasebox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	errorspkg "github.com/argus-labs/world-engine/cli/internal/errors"
)

// headless runs fn against a dashboard backed by a real program with no
// terminal, and returns the final model.
func headless(t *testing.T, fn func(d *Dashboard)) Model {
	t.Helper()
	p := tea.NewProgram(newModel(nil),
		tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler())
	go func() {
		fn(&Dashboard{p: p, ctx: context.Background()})
		p.Quit()
	}()
	final, err := p.Run()
	require.NoError(t, err)
	m, ok := final.(Model)
	require.True(t, ok)
	return m
}

// A box row is one truncated line, so an error's text must never land in the
// box; it's printed in full after the dashboard instead.
func TestFailureShowsStatusNotErrorText(t *testing.T) {
	t.Parallel()

	const errText = "build error for game-shard: exit code: 1"
	m := headless(t, func(d *Dashboard) {
		_ = d.Run("Build",
			func(_ context.Context, sess Session) error {
				sess.Fail("a", "game-shard", errors.New(errText))
				sess.Fail("b", "lobby-shard", fmt.Errorf("build: %w", context.Canceled))
				return errors.New(errText)
			},
			func(time.Duration) string { return "built" },
		)
	})

	view := m.View()
	require.NotContains(t, view, "build error", "not even truncated")
	require.Contains(t, view, "game-shard  failed")
	require.Contains(t, view, "lobby-shard  canceled", "a sibling stopped by the failure isn't a failure")
	require.Contains(t, view, "see error below")
}

func TestCanceledRunIsSilent(t *testing.T) {
	t.Parallel()

	var runErr error
	m := headless(t, func(d *Dashboard) {
		runErr = d.Run("Build",
			func(context.Context, Session) error { return fmt.Errorf("build: %w", context.Canceled) },
			func(time.Duration) string { return "built" },
		)
	})

	require.True(t, errorspkg.IsSilent(runErr))
	require.Contains(t, m.View(), "✗ canceled")
}

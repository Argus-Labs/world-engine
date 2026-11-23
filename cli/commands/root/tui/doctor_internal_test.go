package tui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/internal/dependency"
)

// Err must be nil only after every check passed, so `world doctor` exits 0 only
// then. Quitting before the checks finish never confirmed the dependencies.
func TestWorldDoctorModel_ErrUnlessAllChecksPass(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		msg     tea.Msg
		wantErr string // empty: Err must be nil
	}{
		{"all checks passed", dependency.CheckResult{}, ""},
		{"a check failed", dependency.CheckResult{Err: errors.New("docker missing")}, "docker missing"},
		{"ctrl+c before the checks finished", tea.KeyMsg{Type: tea.KeyCtrlC}, "interrupted"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			next, _ := NewWorldDoctorModel().Update(tc.msg)
			m, ok := next.(WorldDoctorModel)
			require.True(t, ok, "Update should return a WorldDoctorModel")

			if tc.wantErr == "" {
				require.NoError(t, m.Err())
			} else {
				require.ErrorContains(t, m.Err(), tc.wantErr)
			}
		})
	}
}

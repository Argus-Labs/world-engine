package root

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errorspkg "github.com/argus-labs/world-engine/cli/internal/errors"
)

// A setup that creates no project must return an error, so main exits non-zero
// for scripts and CI. The wizard has already rendered the message, so the error
// is silent rather than printed a second time. Neither case reaches the network:
// the template is rejected before cloning, and the clone step fails at mkdir.
func TestSetupCmd_FailureReturnsError(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "existing")
	require.NoError(t, os.Mkdir(existing, 0o755))

	neverCreated := filepath.Join(t.TempDir(), "never-created")

	cases := []struct {
		name    string
		cmd     SetupCmd
		wantErr string
	}{
		{"invalid template", SetupCmd{Directory: neverCreated, Template: "no-such-template"}, "unknown template"},
		{"existing directory", SetupCmd{Directory: existing, Template: "bare"}, "already exists"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			done := make(chan error, 1)
			go func() { done <- tc.cmd.Run(&Cmd{}) }()

			select {
			case err := <-done:
				require.ErrorContains(t, err, tc.wantErr)
				assert.False(t, errorspkg.ShouldPrint(err), "the wizard already rendered the error")
			case <-time.After(30 * time.Second):
				t.Fatal("setup did not exit")
			}
		})
	}

	assert.NoDirExists(t, neverCreated)
}

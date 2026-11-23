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

// fakeDoctorPath replaces PATH with stub git, go, and docker binaries that exit
// with the given status, so the doctor's checks run without the real tools.
func fakeDoctorPath(t *testing.T, exitCodes map[string]string) {
	t.Helper()

	bin := t.TempDir()

	for name, code := range exitCodes {
		script := "#!/bin/sh\nexit " + code + "\n"
		require.NoError(t, os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755))
	}

	t.Setenv("PATH", bin)
}

// A failed dependency check must return an error, so main exits non-zero for
// scripts and CI. The doctor has already rendered the failures, so the error is
// silent rather than printed a second time.
func TestDoctorCmd_ExitStatusFollowsChecks(t *testing.T) {
	cases := []struct {
		name      string
		exitCodes map[string]string
		wantErr   string // empty: Run must return nil
	}{
		{"docker missing", map[string]string{"git": "0", "go": "0", "docker": "1"}, `"Docker" failed`},
		{"all installed", map[string]string{"git": "0", "go": "0", "docker": "0"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakeDoctorPath(t, tc.exitCodes)

			done := make(chan error, 1)
			go func() { done <- (&DoctorCmd{}).Run() }()

			select {
			case err := <-done:
				if tc.wantErr == "" {
					require.NoError(t, err)
					return
				}

				require.ErrorContains(t, err, tc.wantErr)
				assert.False(t, errorspkg.ShouldPrint(err), "the doctor already rendered the failures")
			case <-time.After(30 * time.Second):
				t.Fatal("doctor did not exit")
			}
		})
	}
}

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectPinsOtherCLI(t *testing.T) {
	t.Parallel()

	const tool = "\ntool github.com/argus-labs/world-engine/cli/cmd/world\n"
	tests := []struct {
		name  string
		goMod string // "" writes no go.mod
		want  bool
	}{
		{"outside a module", "", false},
		{"project without the tool", "module ex/game\n\nrequire github.com/argus-labs/world-engine v0.17.0\n", false},
		{
			"project pins this release",
			"module ex/game\n\nrequire github.com/argus-labs/world-engine v0.18.0\n" + tool,
			false,
		},
		{
			"project pins another release",
			"module ex/game\n\nrequire github.com/argus-labs/world-engine v0.17.0\n" + tool,
			true,
		},
		{
			"project replaces world-engine",
			"module ex/game\n\nrequire github.com/argus-labs/world-engine v0.18.0\n" + tool +
				"\nreplace github.com/argus-labs/world-engine => ../world-engine\n",
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if tt.goMod != "" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(tt.goMod), 0o600))
			}
			sub := filepath.Join(dir, "shards", "game")
			require.NoError(t, os.MkdirAll(sub, 0o755))

			assert.Equal(t, tt.want, projectPinsOtherCLI(sub, "v0.18.0"))
		})
	}
}

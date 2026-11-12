package modfile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/modfile"
)

func TestFindModuleRoot(t *testing.T) {
	t.Parallel()

	t.Run("finds go.mod in current directory", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/project"), 0o600))

		root, ok := modfile.FindModuleRoot(dir)
		require.True(t, ok, "expected to find go.mod root")
		assert.Equal(t, dir, root)
	})

	t.Run("finds go.mod in parent directory", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		projectDir := filepath.Join(root, "project")
		nestedDir := filepath.Join(projectDir, "subdir", "nested")
		require.NoError(t, os.MkdirAll(nestedDir, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(projectDir, "go.mod"),
			[]byte("module example.com/project"),
			0o600,
		))

		foundRoot, ok := modfile.FindModuleRoot(nestedDir)
		require.True(t, ok, "expected to find go.mod root from nested directory")
		assert.Equal(t, projectDir, foundRoot)
	})

	t.Run("returns false when no go.mod exists", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		gotRoot, ok := modfile.FindModuleRoot(dir)
		assert.False(t, ok)
		assert.Empty(t, gotRoot)
	})
}

func TestModuleReplace(t *testing.T) {
	t.Parallel()

	modulePath := "github.com/argus-labs/world-engine"

	// writeGoMod writes content to dir/go.mod so each case can point ModuleReplace at a fresh module root.
	writeGoMod := func(t *testing.T, dir, content string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0o600))
	}

	t.Run("returns not found when there is a require but no replace", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeGoMod(
			t,
			dir,
			"module example.com/mygame\n\ngo 1.22\n\nrequire github.com/argus-labs/world-engine v0.8.0\n",
		)
		ver, found, err := modfile.ModuleReplace(dir, modulePath)
		require.NoError(t, err)
		assert.False(t, found, "expected not found when there is no replace")
		assert.Equal(t, "", ver)
	})

	t.Run("returns the fork version for a versioned fork replace", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeGoMod(
			t,
			dir,
			"module example.com/mygame\n\ngo 1.22\n\nrequire github.com/argus-labs/world-engine v0.11.2\n\n"+
				"replace github.com/argus-labs/world-engine => github.com/myfork/world-engine v0.18.0\n",
		)
		ver, found, err := modfile.ModuleReplace(dir, modulePath)
		require.NoError(t, err)
		require.True(t, found, "expected to find the fork replace")
		assert.Equal(t, "v0.18.0", ver)
	})

	t.Run("returns empty version for a path-only local replace", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeGoMod(
			t,
			dir,
			"module example.com/mygame\n\ngo 1.22\n\nrequire github.com/argus-labs/world-engine v0.8.0\n\n"+
				"replace github.com/argus-labs/world-engine => ../world-engine-local\n",
		)
		ver, found, err := modfile.ModuleReplace(dir, modulePath)
		require.NoError(t, err)
		require.True(t, found, "expected to find the local replace")
		assert.Equal(t, "", ver)
	})

	t.Run("matches a versioned-old replace form", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeGoMod(
			t,
			dir,
			"module example.com/mygame\n\ngo 1.22\n\nrequire github.com/argus-labs/world-engine v1.0.0\n\n"+
				"replace github.com/argus-labs/world-engine v1.0.0 => github.com/argus-labs/world-engine v2.0.0\n",
		)
		ver, found, err := modfile.ModuleReplace(dir, modulePath)
		require.NoError(t, err)
		require.True(t, found, "expected to find the versioned-old replace")
		assert.Equal(t, "v2.0.0", ver)
	})

	t.Run("ignores replaces of other modules", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeGoMod(
			t,
			dir,
			"module example.com/mygame\n\ngo 1.22\n\nrequire github.com/argus-labs/world-engine v0.8.0\n\n"+
				"replace github.com/some/other => github.com/some/other v3.0.0\n",
		)
		ver, found, err := modfile.ModuleReplace(dir, modulePath)
		require.NoError(t, err)
		assert.False(t, found, "expected not found for an unrelated replace")
		assert.Equal(t, "", ver)
	})

	t.Run("returns error when go.mod does not exist", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		_, _, err := modfile.ModuleReplace(dir, modulePath)
		require.Error(t, err, "expected error when go.mod does not exist")
	})

	t.Run("returns error for malformed go.mod", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeGoMod(t, dir, "not a valid go.mod {{{}}}")
		_, _, err := modfile.ModuleReplace(dir, modulePath)
		require.Error(t, err, "expected error for malformed go.mod")
	})
}

func TestModuleDependencyVersion(t *testing.T) {
	t.Parallel()

	modulePath := "github.com/argus-labs/world-engine"

	t.Run("detects version from require", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		goMod := `module example.com/mygame

go 1.22

require github.com/argus-labs/world-engine v0.8.0
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o600))

		ver, found, err := modfile.ModuleDependencyVersion(dir, modulePath)
		require.NoError(t, err)
		require.True(t, found, "expected to find world-engine version")
		assert.Equal(t, "v0.8.0", ver)
	})

	t.Run("detects version from replace directive", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		goMod := `module example.com/mygame

go 1.22

require github.com/argus-labs/world-engine v0.8.0

replace github.com/argus-labs/world-engine => github.com/argus-labs/world-engine v0.9.0
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o600))

		ver, found, err := modfile.ModuleDependencyVersion(dir, modulePath)
		require.NoError(t, err)
		require.True(t, found, "expected to find world-engine version")
		assert.Equal(t, "v0.9.0", ver)
	})

	t.Run("falls back to require when replace has no version", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		goMod := `module example.com/mygame

go 1.22

require github.com/argus-labs/world-engine v0.8.0

replace github.com/argus-labs/world-engine => ../world-engine-local
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o600))

		ver, found, err := modfile.ModuleDependencyVersion(dir, modulePath)
		require.NoError(t, err)
		require.True(t, found, "expected to find world-engine version")
		assert.Equal(t, "v0.8.0", ver)
	})

	t.Run("returns not found when module not present", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		goMod := `module example.com/mygame

go 1.22

require github.com/some/other v1.0.0
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o600))

		_, found, err := modfile.ModuleDependencyVersion(dir, modulePath)
		require.NoError(t, err)
		assert.False(t, found, "expected not found when module is absent")
	})

	t.Run("returns error when go.mod does not exist", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		_, _, err := modfile.ModuleDependencyVersion(dir, modulePath)
		require.Error(t, err, "expected error when go.mod does not exist")
	})

	t.Run("returns error for malformed go.mod", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("not a valid go.mod {{{}}}"), 0o600))

		_, _, err := modfile.ModuleDependencyVersion(dir, modulePath)
		require.Error(t, err, "expected error for malformed go.mod")
	})
}

func TestIsOlderThan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ver      string
		baseline string
		want     bool
	}{
		{"older version", "v0.8.0", "v0.9.1", true},
		{"same version", "v0.9.1", "v0.9.1", false},
		{"newer version", "v1.0.0", "v0.9.1", false},
		{"older minor", "v0.9.0", "v0.9.1", true},
		{"invalid ver", "not-semver", "v0.9.1", false},
		{"invalid baseline", "v0.9.0", "not-semver", false},
		{"both invalid", "bad", "also-bad", false},
		{"prerelease older", "v0.9.1-rc1", "v0.9.1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := modfile.IsOlderThan(tt.ver, tt.baseline)
			assert.Equal(t, tt.want, got)
		})
	}
}

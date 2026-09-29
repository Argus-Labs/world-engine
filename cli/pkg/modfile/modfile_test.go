package modfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindModuleRoot(t *testing.T) {
	t.Parallel()

	t.Run("finds go.mod in current directory", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/project"), 0o600); err != nil {
			t.Fatalf("failed to write go.mod: %v", err)
		}

		root, ok := FindModuleRoot(dir)
		if !ok {
			t.Fatalf("expected to find go.mod root")
		}
		if root != dir {
			t.Fatalf("expected root %q, got %q", dir, root)
		}
	})

	t.Run("finds go.mod in parent directory", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		projectDir := filepath.Join(root, "project")
		nestedDir := filepath.Join(projectDir, "subdir", "nested")
		if err := os.MkdirAll(nestedDir, 0o755); err != nil {
			t.Fatalf("failed to create nested directories: %v", err)
		}
		if err := os.WriteFile(
			filepath.Join(projectDir, "go.mod"),
			[]byte("module example.com/project"),
			0o600,
		); err != nil {
			t.Fatalf("failed to write go.mod: %v", err)
		}

		foundRoot, ok := FindModuleRoot(nestedDir)
		if !ok {
			t.Fatalf("expected to find go.mod root from nested directory")
		}
		if foundRoot != projectDir {
			t.Fatalf("expected root %q, got %q", projectDir, foundRoot)
		}
	})

	t.Run("returns false when no go.mod exists", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		gotRoot, ok := FindModuleRoot(dir)
		if ok || gotRoot != "" {
			t.Fatalf("expected no go.mod root, got ok=%v root=%q", ok, gotRoot)
		}
	})
}

func TestModuleReplace(t *testing.T) {
	t.Parallel()

	modulePath := "github.com/argus-labs/world-engine"

	// writeGoMod writes content to dir/go.mod so each case can point ModuleReplace at a fresh module root.
	writeGoMod := func(t *testing.T, dir, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0o600); err != nil {
			t.Fatalf("failed to write go.mod: %v", err)
		}
	}

	t.Run("returns not found when there is a require but no replace", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeGoMod(t, dir, "module example.com/mygame\n\ngo 1.22\n\nrequire github.com/argus-labs/world-engine v0.8.0\n")
		ver, found, err := ModuleReplace(dir, modulePath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found {
			t.Fatal("expected not found when there is no replace")
		}
		if ver != "" {
			t.Fatalf("expected empty version, got %q", ver)
		}
	})

	t.Run("returns the fork version for a versioned fork replace", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeGoMod(t, dir, "module example.com/mygame\n\ngo 1.22\n\nrequire github.com/argus-labs/world-engine v0.11.2\n\n"+
			"replace github.com/argus-labs/world-engine => github.com/myfork/world-engine v0.18.0\n")
		ver, found, err := ModuleReplace(dir, modulePath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found {
			t.Fatal("expected to find the fork replace")
		}
		if ver != "v0.18.0" {
			t.Fatalf("expected v0.18.0 from the fork replace, got %q", ver)
		}
	})

	t.Run("returns empty version for a path-only local replace", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeGoMod(t, dir, "module example.com/mygame\n\ngo 1.22\n\nrequire github.com/argus-labs/world-engine v0.8.0\n\n"+
			"replace github.com/argus-labs/world-engine => ../world-engine-local\n")
		ver, found, err := ModuleReplace(dir, modulePath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found {
			t.Fatal("expected to find the local replace")
		}
		if ver != "" {
			t.Fatalf("expected empty version for a path-only replace, got %q", ver)
		}
	})

	t.Run("matches a versioned-old replace form", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeGoMod(t, dir, "module example.com/mygame\n\ngo 1.22\n\nrequire github.com/argus-labs/world-engine v1.0.0\n\n"+
			"replace github.com/argus-labs/world-engine v1.0.0 => github.com/argus-labs/world-engine v2.0.0\n")
		ver, found, err := ModuleReplace(dir, modulePath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found {
			t.Fatal("expected to find the versioned-old replace")
		}
		if ver != "v2.0.0" {
			t.Fatalf("expected v2.0.0, got %q", ver)
		}
	})

	t.Run("ignores replaces of other modules", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeGoMod(t, dir, "module example.com/mygame\n\ngo 1.22\n\nrequire github.com/argus-labs/world-engine v0.8.0\n\n"+
			"replace github.com/some/other => github.com/some/other v3.0.0\n")
		ver, found, err := ModuleReplace(dir, modulePath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found {
			t.Fatal("expected not found for an unrelated replace")
		}
		if ver != "" {
			t.Fatalf("expected empty version, got %q", ver)
		}
	})

	t.Run("returns error when go.mod does not exist", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		_, _, err := ModuleReplace(dir, modulePath)
		if err == nil {
			t.Fatal("expected error when go.mod does not exist")
		}
	})

	t.Run("returns error for malformed go.mod", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeGoMod(t, dir, "not a valid go.mod {{{}}}")
		_, _, err := ModuleReplace(dir, modulePath)
		if err == nil {
			t.Fatal("expected error for malformed go.mod")
		}
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
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o600); err != nil {
			t.Fatalf("failed to write go.mod: %v", err)
		}

		ver, found, err := ModuleDependencyVersion(dir, modulePath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found {
			t.Fatal("expected to find world-engine version")
		}
		if ver != "v0.8.0" {
			t.Fatalf("expected v0.8.0, got %q", ver)
		}
	})

	t.Run("detects version from replace directive", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		goMod := `module example.com/mygame

go 1.22

require github.com/argus-labs/world-engine v0.8.0

replace github.com/argus-labs/world-engine => github.com/argus-labs/world-engine v0.9.0
`
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o600); err != nil {
			t.Fatalf("failed to write go.mod: %v", err)
		}

		ver, found, err := ModuleDependencyVersion(dir, modulePath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found {
			t.Fatal("expected to find world-engine version")
		}
		if ver != "v0.9.0" {
			t.Fatalf("expected v0.9.0 from replace, got %q", ver)
		}
	})

	t.Run("falls back to require when replace has no version", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		goMod := `module example.com/mygame

go 1.22

require github.com/argus-labs/world-engine v0.8.0

replace github.com/argus-labs/world-engine => ../world-engine-local
`
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o600); err != nil {
			t.Fatalf("failed to write go.mod: %v", err)
		}

		ver, found, err := ModuleDependencyVersion(dir, modulePath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found {
			t.Fatal("expected to find world-engine version")
		}
		if ver != "v0.8.0" {
			t.Fatalf("expected v0.8.0 to fallback to require, got %q", ver)
		}
	})

	t.Run("returns not found when module not present", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		goMod := `module example.com/mygame

go 1.22

require github.com/some/other v1.0.0
`
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o600); err != nil {
			t.Fatalf("failed to write go.mod: %v", err)
		}

		_, found, err := ModuleDependencyVersion(dir, modulePath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found {
			t.Fatal("expected not found when module is absent")
		}
	})

	t.Run("returns error when go.mod does not exist", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		_, _, err := ModuleDependencyVersion(dir, modulePath)
		if err == nil {
			t.Fatal("expected error when go.mod does not exist")
		}
	})

	t.Run("returns error for malformed go.mod", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("not a valid go.mod {{{}}}"), 0o600); err != nil {
			t.Fatalf("failed to write go.mod: %v", err)
		}

		_, _, err := ModuleDependencyVersion(dir, modulePath)
		if err == nil {
			t.Fatal("expected error for malformed go.mod")
		}
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
			got := IsOlderThan(tt.ver, tt.baseline)
			if got != tt.want {
				t.Fatalf("IsOlderThan(%q, %q) = %v, want %v", tt.ver, tt.baseline, got, tt.want)
			}
		})
	}
}

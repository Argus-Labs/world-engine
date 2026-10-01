package modfile

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rotisserie/eris"
	gomod "golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"
)

// FindModuleRoot resolves the Go module root using `go env GOMOD`.
func FindModuleRoot(startDir string) (string, bool) {
	if startDir == "" {
		return "", false
	}

	cmd := exec.Command("go", "env", "GOMOD")
	cmd.Dir = startDir
	modPathBytes, err := cmd.Output()
	if err != nil {
		return "", false
	}

	modPath := strings.TrimSpace(string(modPathBytes))
	if modPath == "" || modPath == "N/A" || modPath == "off" {
		return "", false
	}
	if filepath.Clean(modPath) == filepath.Clean(os.DevNull) {
		return "", false
	}
	if filepath.Base(modPath) != "go.mod" {
		return "", false
	}
	if _, err := os.Stat(modPath); err != nil {
		return "", false
	}

	return filepath.Dir(modPath), true
}

// ModuleReplace reports whether modulePath is replaced in moduleRoot's go.mod and returns the
// replacement's New.Version. found is true when any replace directive matches modulePath by its
// old path — covering the same-path (version bump), fork, versioned-old, and path-only (local)
// replace forms. newVersion is the version the backend actually builds against for a versioned
// replace, and "" for a path-only local replace (e.g. "=> ./local"), whose effective version cannot
// be determined from go.mod alone.
//
// ModuleDependencyVersion falls back to the require line for a path-only replace, which is
// indistinguishable from a plain, un-replaced dependency — use ModuleReplace to tell them apart so a
// path-only replace can skip a version comparison rather than compare a stale require hint.
func ModuleReplace(moduleRoot, modulePath string) (newVersion string, found bool, err error) {
	data, err := os.ReadFile(filepath.Join(moduleRoot, "go.mod"))
	if err != nil {
		return "", false, eris.Wrap(err, "failed to read go.mod")
	}

	f, err := gomod.Parse("go.mod", data, nil)
	if err != nil {
		return "", false, eris.Wrap(err, "failed to parse go.mod")
	}

	for _, r := range f.Replace {
		if r.Old.Path == modulePath {
			return r.New.Version, true, nil
		}
	}
	return "", false, nil
}

// ModuleDependencyVersion returns the version for modulePath in the go.mod file
// for moduleRoot. Replace directives take precedence over require directives.
// If a replace exists without a version, it falls back to the require entry.
func ModuleDependencyVersion(moduleRoot, modulePath string) (string, bool, error) {
	data, err := os.ReadFile(filepath.Join(moduleRoot, "go.mod"))
	if err != nil {
		return "", false, eris.Wrap(err, "failed to read go.mod")
	}

	f, err := gomod.Parse("go.mod", data, nil)
	if err != nil {
		return "", false, eris.Wrap(err, "failed to parse go.mod")
	}

	for _, r := range f.Replace {
		if r.Old.Path == modulePath {
			if r.New.Version != "" {
				return r.New.Version, true, nil
			}
		}
	}

	for _, r := range f.Require {
		if r.Mod.Path == modulePath {
			return r.Mod.Version, true, nil
		}
	}

	return "", false, nil
}

// IsOlderThan returns true if ver is a valid semver version strictly less than
// baseline. Returns false if either version is invalid.
func IsOlderThan(ver, baseline string) bool {
	return semver.IsValid(ver) && semver.IsValid(baseline) && semver.Compare(ver, baseline) < 0
}

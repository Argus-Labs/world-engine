package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"

	gomod "golang.org/x/mod/modfile"

	"github.com/argus-labs/world-engine/cli/pkg/modfile"
	"github.com/argus-labs/world-engine/cli/pkg/version"
)

const (
	worldEngineModule = "github.com/argus-labs/world-engine"
	worldTool         = "github.com/argus-labs/world-engine/cli/cmd/world"

	// envDelegated marks a process started by a hand-off, so it runs the command instead of handing off again.
	envDelegated = "WORLD_CLI_DELEGATED"
)

// delegateToProjectTool hands the command to `go tool world` when the working directory is in a project
// that pins a different World CLI than this binary. A globally installed `world` is only an entry point:
// the CLI ships in the world-engine module, so a project pins its CLI through its World Engine version.
// It returns when this binary should run the command itself.
func delegateToProjectTool(args []string) {
	if os.Getenv(envDelegated) != "" {
		return
	}
	dir, err := os.Getwd()
	if err != nil || !projectPinsOtherCLI(dir, version.WorldEngine()) {
		return
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		return
	}
	runGoTool(goBin, append([]string{"go", "tool", "world"}, args...), append(os.Environ(), envDelegated+"=1"))
}

// projectPinsOtherCLI reports whether the Go module containing dir declares the world tool at a World
// Engine other than running. A replaced world-engine counts as other: its effective version is unknown.
func projectPinsOtherCLI(dir, running string) bool {
	root, ok := modfile.FindModuleRoot(dir)
	if !ok {
		return false
	}
	path := filepath.Join(root, "go.mod")
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	f, err := gomod.Parse(path, data, nil)
	if err != nil || !slices.ContainsFunc(f.Tool, func(t *gomod.Tool) bool { return t.Path == worldTool }) {
		return false
	}
	if slices.ContainsFunc(f.Replace, func(r *gomod.Replace) bool { return r.Old.Path == worldEngineModule }) {
		return true
	}
	return !slices.ContainsFunc(f.Require, func(r *gomod.Require) bool {
		return r.Mod.Path == worldEngineModule && r.Mod.Version == running
	})
}

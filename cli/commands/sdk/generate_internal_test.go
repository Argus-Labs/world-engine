package sdk

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureStdout captures [os.Stdout] output produced by fn and returns it as a string. It reassigns the
// process-global [os.Stdout], so callers MUST NOT run in parallel (t.Parallel) while a capture is active.
// Mirrors the helper used by apps/world-cli/internal/printer and internal/logger internal tests.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w //nolint:reassign // capturing stdout in tests

	fn()

	_ = w.Close()
	os.Stdout = orig //nolint:reassign // restore stdout
	buf, _ := io.ReadAll(r)
	_ = r.Close()
	return string(buf)
}

var ansiRegexp = regexp.MustCompile("\x1b\\[[0-9;?]*[a-zA-Z]")

// stripANSI removes ANSI escape sequences (e.g. lipgloss styling) so assertions match the visible text.
func stripANSI(s string) string {
	return ansiRegexp.ReplaceAllString(s, "")
}

// writeGoMod writes content to dir/go.mod so a temp dir can stand in for a backend module root.
func writeGoMod(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}
}

// TestClassifyEngineCheck exercises the replace-aware decision behind printEngineCheck. It uses fresh
// temp dirs as module roots; classifyEngineCheck reads go.mod directly (no subprocess, no printer), so
// these subtests are parallel-safe. Each subtest guards a distinct branch: a versioned fork replace
// must be judged by its replace version (not the stale require hint), and a path-only local replace
// must skip rather than warn on the require hint.
func TestClassifyEngineCheck(t *testing.T) {
	t.Parallel()

	const mod = worldEngineModule
	const target = "v0.16.4"

	t.Run("plain require at the target version is compatible (boundary: == must not be Old)", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeGoMod(t, dir, "module ex/mygame\n\ngo 1.22\n\nrequire "+mod+" v0.16.4\n")
		outcome, ver := classifyEngineCheck(target, dir)
		assert.Equal(t, engineCheckOK, outcome)
		assert.Equal(t, "v0.16.4", ver)
	})

	t.Run("plain require newer than target is compatible", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeGoMod(t, dir, "module ex/mygame\n\ngo 1.22\n\nrequire "+mod+" v0.20.0\n")
		outcome, ver := classifyEngineCheck(target, dir)
		assert.Equal(t, engineCheckOK, outcome)
		assert.Equal(t, "v0.20.0", ver)
	})

	t.Run("plain require older than target warns with the require version", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeGoMod(t, dir, "module ex/mygame\n\ngo 1.22\n\nrequire "+mod+" v0.11.2\n")
		outcome, ver := classifyEngineCheck(target, dir)
		assert.Equal(t, engineCheckOld, outcome)
		assert.Equal(t, "v0.11.2", ver)
	})

	t.Run(
		"versioned fork replace newer than target is compatible by the replace version, not the require hint",
		func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeGoMod(t, dir, "module ex/mygame\n\ngo 1.22\n\nrequire "+mod+" v0.11.2\n\n"+
				"replace "+mod+" => github.com/myfork/world-engine v0.18.0\n")
			outcome, ver := classifyEngineCheck(target, dir)
			assert.Equal(t, engineCheckOK, outcome)
			assert.Equal(t, "v0.18.0", ver)
		},
	)

	t.Run(
		"versioned fork replace older than target warns on the replace version, not the require hint",
		func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeGoMod(t, dir, "module ex/mygame\n\ngo 1.22\n\nrequire "+mod+" v0.11.2\n\n"+
				"replace "+mod+" => github.com/myfork/world-engine v0.12.0\n")
			outcome, ver := classifyEngineCheck(target, dir)
			assert.Equal(t, engineCheckOld, outcome)
			assert.Equal(t, "v0.12.0", ver)
		},
	)

	t.Run("path-only local replace skips rather than warns on the stale require hint", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeGoMod(t, dir, "module ex/mygame\n\ngo 1.22\n\nrequire "+mod+" v0.11.2\n\n"+
			"replace "+mod+" => ./local\n")
		outcome, ver := classifyEngineCheck(target, dir)
		assert.Equal(t, engineCheckNoComparableVersion, outcome)
		assert.Equal(t, "", ver)
	})

	t.Run("no world-engine dependency is not found", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeGoMod(t, dir, "module ex/mygame\n\ngo 1.22\n\nrequire github.com/some/other v1.0.0\n")
		outcome, ver := classifyEngineCheck(target, dir)
		assert.Equal(t, engineCheckNotFound, outcome)
		assert.Equal(t, "", ver)
	})

	t.Run("a replace of a different module is ignored and the require is used", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeGoMod(t, dir, "module ex/mygame\n\ngo 1.22\n\nrequire "+mod+" v0.11.2\n\n"+
			"replace github.com/some/other => github.com/some/other v2.0.0\n")
		outcome, ver := classifyEngineCheck(target, dir)
		assert.Equal(t, engineCheckOld, outcome)
		assert.Equal(t, "v0.11.2", ver)
	})

	t.Run("missing go.mod is a read error", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		outcome, ver := classifyEngineCheck(target, dir)
		assert.Equal(t, engineCheckReadError, outcome)
		assert.Equal(t, "", ver)
	})

	t.Run("malformed go.mod is a read error", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeGoMod(t, dir, "not a valid go.mod {{{}}}")
		outcome, ver := classifyEngineCheck(target, dir)
		assert.Equal(t, engineCheckReadError, outcome)
		assert.Equal(t, "", ver)
	})

	t.Run("invalid target is no-target", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeGoMod(t, dir, "module ex/mygame\n\ngo 1.22\n\nrequire "+mod+" v0.11.2\n")
		outcome, ver := classifyEngineCheck("", dir)
		assert.Equal(t, engineCheckNoTarget, outcome)
		assert.Equal(t, "", ver)
	})

	t.Run("pseudo-version replace skips rather than warns (version not meaningfully comparable)", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeGoMod(t, dir, "module ex/mygame\n\ngo 1.22\n\nrequire "+mod+" v0.11.2\n\n"+
			"replace "+mod+" => github.com/myfork/world-engine v0.0.0-20240101000000-abcdefabcdef\n")
		// A pseudo-version replace has no meaningful release version to compare against the CLI target,
		// so the check skips (engineCheckNoComparableVersion) rather than producing a misleading old/ok verdict.
		outcome, ver := classifyEngineCheck(target, dir)
		assert.Equal(t, engineCheckNoComparableVersion, outcome)
		assert.Equal(t, "", ver)
	})

	t.Run("pseudo-version require skips rather than warns", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeGoMod(t, dir, "module ex/mygame\n\ngo 1.22\n\nrequire "+mod+
			" v0.0.0-20240101000000-abcdefabcdef\n")
		// A pseudo-version pins a commit, so it carries no API level to compare — the same reason a
		// path-only replace skips. Reaching it through require rather than replace does not change that.
		outcome, ver := classifyEngineCheck(target, dir)
		assert.Equal(t, engineCheckNoComparableVersion, outcome)
		assert.Equal(t, "", ver)
	})
}

// TestPrintEngineCheckMessage covers the formatting/wiring from outcome to printed text, one case per
// outcome. These tests capture [os.Stdout], so they MUST NOT run in parallel.
func TestPrintEngineCheckMessage(t *testing.T) {
	const target = "v0.16.4"

	capture := func(outcome engineCheckOutcome, ver string) string {
		return stripANSI(captureStdout(t, func() { printEngineCheckMessage(outcome, target, ver) }))
	}

	t.Run("no target is silent", func(t *testing.T) {
		assert.Empty(t, capture(engineCheckNoTarget, ""))
	})

	t.Run("no module prints the locate-module skip line", func(t *testing.T) {
		out := capture(engineCheckNoModule, "")
		assert.Contains(t, out, "could not locate backend module")
		assert.Contains(t, out, "skipping compatibility check")
	})

	t.Run("no comparable version prints the skip line", func(t *testing.T) {
		out := capture(engineCheckNoComparableVersion, "")
		assert.Contains(t, out, "no comparable version")
		assert.Contains(t, out, "skipping compatibility check")
	})

	t.Run("not found prints the not-a-direct-dependency skip line", func(t *testing.T) {
		out := capture(engineCheckNotFound, "")
		assert.Contains(t, out, "not a direct dependency")
		assert.Contains(t, out, "skipping compatibility check")
	})

	t.Run("read error prints the could-not-read skip line", func(t *testing.T) {
		out := capture(engineCheckReadError, "")
		assert.Contains(t, out, "could not read backend go.mod")
		assert.Contains(t, out, "skipping compatibility check")
	})

	t.Run("old warns naming both versions and the go get advice", func(t *testing.T) {
		out := capture(engineCheckOld, "v0.11.2")
		assert.Contains(t, out, "v0.11.2")
		assert.Contains(t, out, target)
		assert.Contains(t, out, "older than")
		assert.Contains(t, out, "won't compile")
		assert.Contains(t, out, "go get "+worldEngineModule+"@"+target)
	})

	t.Run("compatible prints the success line naming both versions", func(t *testing.T) {
		out := capture(engineCheckOK, "v0.18.0")
		assert.Contains(t, out, "v0.18.0")
		assert.Contains(t, out, target)
		assert.Contains(t, out, "compatible")
	})
}

// TestPrintEngineCheckForTarget exercises the end-to-end wiring (FindModuleRoot -> classify -> print)
// against temp-dir backend modules with a controlled target. These tests capture [os.Stdout], so they MUST
// NOT run in parallel.
func TestPrintEngineCheckForTarget(t *testing.T) {
	const target = "v0.16.4"

	run := func(t *testing.T, goMod string) string {
		t.Helper()
		dir := t.TempDir()
		if goMod != "" {
			writeGoMod(t, dir, goMod)
		}
		return stripANSI(captureStdout(t, func() { printEngineCheckForTarget(dir, target) }))
	}

	t.Run("versioned fork replace newer than target prints compatible (regression for the bug)", func(t *testing.T) {
		out := run(t, "module ex/mygame\n\ngo 1.22\n\nrequire "+worldEngineModule+" v0.11.2\n\n"+
			"replace "+worldEngineModule+" => github.com/myfork/world-engine v0.18.0\n")
		assert.Contains(t, out, "compatible")
		assert.Contains(t, out, "v0.18.0")
		assert.NotContains(t, out, "won't compile")
		assert.NotContains(t, out, "older than")
	})

	t.Run("path-only local replace skips rather than warns", func(t *testing.T) {
		out := run(t, "module ex/mygame\n\ngo 1.22\n\nrequire "+worldEngineModule+" v0.11.2\n\n"+
			"replace "+worldEngineModule+" => ./local\n")
		assert.Contains(t, out, "no comparable version")
		assert.Contains(t, out, "skipping compatibility check")
		assert.NotContains(t, out, "older than")
		assert.NotContains(t, out, "won't compile")
	})

	t.Run("plain require older than target warns (no regression for the happy warn path)", func(t *testing.T) {
		out := run(t, "module ex/mygame\n\ngo 1.22\n\nrequire "+worldEngineModule+" v0.11.2\n")
		assert.Contains(t, out, "older than")
		assert.Contains(t, out, "v0.11.2")
		assert.Contains(t, out, "go get "+worldEngineModule+"@"+target)
	})

	t.Run("invalid target stays silent and does not shell out to FindModuleRoot", func(t *testing.T) {
		dir := t.TempDir()
		writeGoMod(t, dir, "module ex/mygame\n\ngo 1.22\n\nrequire "+worldEngineModule+" v0.11.2\n")
		out := captureStdout(t, func() { printEngineCheckForTarget(dir, "") })
		assert.Empty(t, out)
	})

	t.Run("directory with no go.mod prints could not locate backend module", func(t *testing.T) {
		out := run(t, "")
		assert.Contains(t, out, "could not locate backend module")
	})
}

// TestPrintEngineCheckSkipsDevelopmentBuild pins that a world-cli built without a world-engine release
// (tests, or a checkout) prints no verdict instead of comparing against its "main" target. Captures
// [os.Stdout], so it MUST NOT run in parallel.
func TestPrintEngineCheckSkipsDevelopmentBuild(t *testing.T) {
	dir := t.TempDir()
	writeGoMod(t, dir, "module ex/mygame\n\ngo 1.22\n\nrequire "+worldEngineModule+" v0.11.2\n")

	assert.Empty(t, strings.TrimSpace(stripANSI(captureStdout(t, func() { printEngineCheck(dir) }))))
}

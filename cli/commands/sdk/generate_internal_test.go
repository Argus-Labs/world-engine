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

	"github.com/argus-labs/world-engine/cli/internal/sdkgen"
)

// captureStdout captures os.Stdout output produced by fn and returns it as a string. It reassigns the
// process-global os.Stdout, so callers MUST NOT run in parallel (t.Parallel) while a capture is active.
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
// outcome. These tests capture os.Stdout, so they MUST NOT run in parallel.
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
// against temp-dir backend modules with a controlled target. These tests capture os.Stdout, so they MUST
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
// os.Stdout, so it MUST NOT run in parallel.
func TestPrintEngineCheckSkipsDevelopmentBuild(t *testing.T) {
	dir := t.TempDir()
	writeGoMod(t, dir, "module ex/mygame\n\ngo 1.22\n\nrequire "+worldEngineModule+" v0.11.2\n")

	assert.Empty(t, strings.TrimSpace(stripANSI(captureStdout(t, func() { printEngineCheck(dir) }))))
}

// writeRel writes content to filepath.Join(root, rel), creating parent dirs.
func writeRel(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
}

// genTaggedWire is a wire.gen.go carrying the generator's own build tag, the way RenderGoWire stamps it.
const genTaggedWire = "// Code generated by world sdk generate. DO NOT EDIT.\n\n//go:build !sdkgen\n\npackage p\n"

// TestCleanStaleWire verifies the generator removes its own previously-written wire.gen.go from source
// packages that no longer declare any wire type, while keeping live ones and hand-written files of the
// same name. emitWireBridges only rewrites dirs that still have messages, so a package that lost its last
// wire type is absent from the live set; cleanStaleWire walks the tree for tagged wire.gen.go files and
// removes the orphans, gated on the build tag so a hand-written file is never touched.
func TestCleanStaleWire(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRel(t, root, "alpha/wire.gen.go", genTaggedWire)     // live this run
	writeRel(t, root, "beta/wire.gen.go", genTaggedWire)      // stale this run
	writeRel(t, root, "gamma/wire.gen.go", "package gamma\n") // hand-written (no tag) — must survive
	writeRel(t, root, "sub/delta/wire.gen.go", genTaggedWire) // stale, nested — must be removed

	alphaDir := filepath.Join(root, "alpha")
	require.NoError(t, cleanStaleWire(root, []sdkgen.Message{{Dir: alphaDir}}))

	for _, rel := range []string{"alpha/wire.gen.go", "gamma/wire.gen.go"} {
		_, err := os.Stat(filepath.Join(root, rel))
		assert.NoErrorf(t, err, "%s should still exist", rel)
	}
	for _, rel := range []string{"beta/wire.gen.go", "sub/delta/wire.gen.go"} {
		_, err := os.Stat(filepath.Join(root, rel))
		assert.Truef(t, os.IsNotExist(err), "%s should have been removed (stale), got err=%v", rel, err)
	}
}

// TestCleanStaleWire_MissingRootIsNotAnError pins that a source root that does not exist is tolerated
// rather than aborting generation, matching removeGlobRecursive's behaviour on a missing output dir.
func TestCleanStaleWire_MissingRootIsNotAnError(t *testing.T) {
	t.Parallel()
	assert.NoError(t, cleanStaleWire(filepath.Join(t.TempDir(), "does-not-exist"), nil))
}

// genCardinalShim stands in for cardinal's registration API. Mirrors the sdkgen test shim: every wire
// role is generic method on *World, so discovery reads which one a type is instantiated with.
const genCardinalShim = `package cardinal

type World struct{}

func (*World) RegisterCommand[T any]()     {}
func (*World) RegisterEvent[T any]()       {}
func (*World) RegisterComponent[T any]()   {}
func (*World) RegisterSystemEvent[T any]() {}
`

// genTestGoMod is a self-contained module that resolves world-engine locally (no network).
const genTestGoMod = `module sdkgentest

go 1.27

require github.com/argus-labs/world-engine v0.0.0

replace github.com/argus-labs/world-engine => ./worldengine
`

// genImmutableShim stands in for world-engine's pkg/immutable at the real import path.
const genImmutableShim = `package immutable

type Slice[T any] struct{ items []T }

func SliceOf[T any](items ...T) Slice[T] { return Slice[T]{items: items} }
`

// setupGenFixture lays out a two-package backend (alpha + beta, each with one command) under a temp
// module so Discover + emitWireBridges can run against it. Returns the temp root.
func setupGenFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeRel(t, root, "go.mod", genTestGoMod)
	writeRel(t, root, "worldengine/go.mod", "module github.com/argus-labs/world-engine\n\ngo 1.27\n")
	writeRel(t, root, "worldengine/pkg/immutable/slice.go", genImmutableShim)
	writeRel(t, root, "cardinal/cardinal.go", genCardinalShim)
	writeRel(t, root, "alpha/alpha.go", `package alpha

import "sdkgentest/cardinal"

type AlphaCmd struct{ X int32 }

func (AlphaCmd) Name() string { return "alpha" }

func Setup(w *cardinal.World) { w.RegisterCommand[AlphaCmd]() }
`)
	writeRel(t, root, "beta/beta.go", `package beta

import "sdkgentest/cardinal"

type BetaCmd struct{ Y int32 }

func (BetaCmd) Name() string { return "beta" }

func Setup(w *cardinal.World) { w.RegisterCommand[BetaCmd]() }
`)
	return root
}

// TestEmitWireBridges_StaleRemovedAfterLastTypeRemoved exercises the real Discover → cleanStaleWire +
// emitWireBridges path: after a source package loses its only wire type, the stale wire.gen.go the previous
// run wrote into it is removed, while a package that still declares a wire type keeps its file. This is
// the only test that verifies m.Dir from real discovery (absolute, from go/packages file paths) aligns
// with cleanStaleWire's absolute-root join — a unit test constructing its own paths cannot catch that.
func TestEmitWireBridges_StaleRemovedAfterLastTypeRemoved(t *testing.T) {
	t.Parallel()

	root := setupGenFixture(t)
	betaWire := filepath.Join(root, "beta", sdkgen.WireFileName)
	alphaWire := filepath.Join(root, "alpha", sdkgen.WireFileName)

	// Run 1: both packages declare a command, so both get a wire.gen.go.
	res1, err := sdkgen.Discover(root)
	require.NoError(t, err, "Discover (run 1)")
	res1.ResolveGen("sdkgentest/gen")
	require.NoError(t, emitWireBridges(res1.Messages), "emitWireBridges (run 1)")
	for _, p := range []string{alphaWire, betaWire} {
		_, err := os.Stat(p)
		require.NoErrorf(t, err, "run 1 should write %s", p)
	}

	// beta loses its only wire type.
	require.NoError(t, os.WriteFile(filepath.Join(root, "beta/beta.go"), []byte("package beta\n"), 0o644))

	// Run 2: alpha still has a command; beta does not. cleanStaleWire + emitWireBridges together must
	// remove beta's orphaned wire.gen.go and rewrite alpha's.
	res2, err := sdkgen.Discover(root)
	require.NoError(t, err, "Discover (run 2)")
	res2.ResolveGen("sdkgentest/gen")
	require.NoError(t, cleanStaleWire(root, res2.Messages))
	require.NoError(t, emitWireBridges(res2.Messages))

	_, err = os.Stat(betaWire)
	assert.Truef(t, os.IsNotExist(err), "beta/wire.gen.go should be removed after cleanStaleWire, got err=%v", err)

	_, err = os.Stat(alphaWire)
	assert.NoErrorf(t, err, "alpha/wire.gen.go should still exist (alpha still declares a wire type)")
}

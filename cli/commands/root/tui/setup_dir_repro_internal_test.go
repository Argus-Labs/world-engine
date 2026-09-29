package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
	"github.com/argus-labs/world-engine/cli/pkg/version"
	"github.com/argus-labs/world-engine/cli/pkg/worldscaffold"
)

// initLocalTemplateRepo creates a self-contained local git repository that
// looks like a World Engine template: it has a valid world.toml (so
// worldscaffold.InstantiateTemplate's setProject step succeeds) and a go file.
// The repo is tagged with version.WorldEngine() (e.g. "main" for a checkout or
// pseudo-version test build) so the scaffolder's tag-then-branch clone resolves
// to it without any network access. It returns the repository's directory path.
func initLocalTemplateRepo(t *testing.T) string {
	t.Helper()

	repoDir := t.TempDir()
	repo, err := gogit.PlainInit(repoDir, false)
	require.NoError(t, err)

	// A valid world.toml: InstantiateTemplate rewrites its `project` field via
	// toml.WriteProject, which validates the result, so it must satisfy the
	// toml schema (organization + project + at least one shard).
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "world.toml"), []byte(
		`organization = "test-org"
project = "starter-game"

[[shards]]
id = "game"
`), 0o600))

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "main.go"),
		[]byte("package game\n"), 0o600))

	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add(".")
	require.NoError(t, err)
	head, err := wt.Commit("init", &gogit.CommitOptions{
		Author: &object.Signature{Name: "t", Email: "t@example.com", When: time.Now()},
	})
	require.NoError(t, err)

	// InstantiateTemplate clones with ReferenceName = tag <version> first, then
	// falls back to branch <version>. A tag satisfies the first attempt.
	_, err = repo.CreateTag(version.WorldEngine(), head, nil)
	require.NoError(t, err)

	return repoDir
}

// chdirTemp returns a fresh temp directory and chdir's the process into it,
// restoring the original working directory when the test finishes (via
// t.Chdir's cleanup). It is used to make "the project lands under the CWD"
// assertions concrete.
func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	return dir
}

// parseDirectoryArg runs the real Kong "path" mapper (the same one SetupCmd's
// Directory argument uses) over the supplied argument and returns the
// resulting (absolutized) directory value.
func parseDirectoryArg(t *testing.T, arg string) string {
	t.Helper()
	var cli struct {
		Directory string `arg:"" optional:"" type:"path"`
	}
	parser, err := kong.New(&cli)
	require.NoError(t, err)
	_, err = parser.Parse([]string{arg})
	require.NoError(t, err)
	return cli.Directory
}

// readWorldTomlProject loads the project field from a world.toml file.
func readWorldTomlProject(path string) (string, error) {
	cfg, err := worldtoml.LoadFile(path)
	if err != nil {
		return "", err
	}
	return cfg.Project, nil
}

// TestCloneTemplateCmd_CreatesAtUserSuppliedPath is the regression test for the
// bug where `world setup /some/parent/my-game` created the project at
// <CWD>/my-game instead of the user-supplied path. It drives the real Kong
// "path" mapper, builds the real WorldSetupModel, points it at a local git
// fixture, and invokes the real cloneTemplateCmd — then checks the filesystem
// to confirm the project lands at the user-supplied absolute path.
func TestCloneTemplateCmd_CreatesAtUserSuppliedPath(t *testing.T) {
	workDir := chdirTemp(t)  // process CWD
	elsewhere := t.TempDir() // the parent the user wants the project under
	absTarget := filepath.Join(elsewhere, "my-game")

	dir := parseDirectoryArg(t, absTarget)
	require.Equal(t, absTarget, dir, "Kong \"path\" mapper must deliver the absolute path unchanged")

	repoDir := initLocalTemplateRepo(t)

	m := NewWorldSetupModel(dir, "DEV", "")
	m.selectedTemplate = &worldscaffold.GameTemplate{URL: repoDir, Subdir: ""}

	msg := m.cloneTemplateCmd()()
	require.IsType(t, CloneFinishedMsg{}, msg)
	require.NoError(t, msg.(CloneFinishedMsg).Err)

	// The project must exist at the user-supplied absolute path.
	info, err := os.Stat(absTarget)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	// The project must NOT be created under the process working directory.
	_, err = os.Stat(filepath.Join(workDir, "my-game"))
	require.Error(t, err, "nothing should be created under the CWD")

	// The user's parent directory should contain exactly the project dir.
	entries, err := os.ReadDir(elsewhere)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "my-game", entries[0].Name())

	// The scaffolder derives the project name from the target's base, so the
	// world.toml project field should be the trailing segment of the path.
	project, err := readWorldTomlProject(filepath.Join(absTarget, "world.toml"))
	require.NoError(t, err)
	assert.Equal(t, "my-game", project)
}

// TestCloneTemplateCmd_NoDirectoryFallback_CreatesUnderCWD guards the
// no-argument interactive path: when no Directory argument is supplied the
// model keeps targetDir empty and cloneTemplateCmd falls back to the name the
// user typed into projectNameInput, creating the project under the CWD just as
// it did before the fix.
func TestCloneTemplateCmd_NoDirectoryFallback_CreatesUnderCWD(t *testing.T) {
	workDir := chdirTemp(t)

	repoDir := initLocalTemplateRepo(t)

	m := NewWorldSetupModel("", "DEV", "")
	require.Empty(t, m.targetDir, "no-arg path must leave targetDir empty")
	m.selectedTemplate = &worldscaffold.GameTemplate{URL: repoDir, Subdir: ""}
	// Simulate the user typing a bare shard name and pressing Enter.
	m.projectNameInput.SetValue("my-game")

	msg := m.cloneTemplateCmd()()
	require.IsType(t, CloneFinishedMsg{}, msg)
	require.NoError(t, msg.(CloneFinishedMsg).Err)

	info, err := os.Stat(filepath.Join(workDir, "my-game"))
	require.NoError(t, err)
	require.True(t, info.IsDir())

	// An absolute parented path was never supplied, so nothing else should exist.
	entries, err := os.ReadDir(workDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

// TestNewWorldSetupModel_PreservesDirectoryPath checks that the model retains
// the full Kong-expanded directory on targetDir while projectNameInput carries
// only the trailing segment for display/validation.
func TestNewWorldSetupModel_PreservesDirectoryPath(t *testing.T) {
	workDir := chdirTemp(t)
	absTarget := filepath.Join(workDir, "nested", "my-game")
	require.NoError(t, os.MkdirAll(filepath.Dir(absTarget), 0o755))

	dir := parseDirectoryArg(t, absTarget)
	m := NewWorldSetupModel(dir, "DEV", "")

	assert.Equal(t, dir, m.targetDir, "targetDir must retain the full path")
	assert.Equal(t, "my-game", m.projectNameInput.Value(), "input carries only the base name")
	assert.Equal(t, dir, m.targetDirectory(), "targetDirectory() returns the user-supplied path")
}

// TestNewWorldSetupModel_NoDirectory_TargetDirEmpty checks the no-argument
// interactive path: targetDir stays empty and targetDirectory() falls back to
// whatever the user types into projectNameInput.
func TestNewWorldSetupModel_NoDirectory_TargetDirEmpty(t *testing.T) {
	m := NewWorldSetupModel("", "DEV", "")
	assert.Empty(t, m.targetDir)
	assert.Empty(t, m.targetDirectory(), "empty input -> empty target")

	m.projectNameInput.SetValue("my-game")
	assert.Equal(t, "my-game", m.targetDirectory(), "falls back to the typed name")
	assert.Equal(t, "./my-game", m.displayDir(), "display path for the no-arg path is ./<name>")
}

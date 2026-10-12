package worldscaffold

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type SetupCloneTestSuite struct {
	suite.Suite
}

func TestSetupCloneTestSuite(t *testing.T) {
	suite.Run(t, new(SetupCloneTestSuite))
}

// -----------------------------------------------------------------------------
// cloneTemplate
// -----------------------------------------------------------------------------

func (s *SetupCloneTestSuite) TestInstantiateTemplate_DirectoryExists() {
	t := s.T()

	existingDir := filepath.Join(t.TempDir(), "exists")
	require.NoError(t, os.MkdirAll(existingDir, 0o755))

	// Will fail before git is called due to directory check
	err := InstantiateTemplate(context.Background(),
		"https://example.com/repo.git", "v0.0.1", existingDir, "")

	require.Error(t, err)
	assert.Equal(t, "Project named '"+existingDir+"' already exists in this directory, "+
		"please change the directory or use another name", err.Error())
}

// -----------------------------------------------------------------------------
// Scaffold
// -----------------------------------------------------------------------------

func (s *SetupCloneTestSuite) TestScaffold_DirectoryExists() {
	t := s.T()

	existingDir := filepath.Join(t.TempDir(), "exists")
	require.NoError(t, os.MkdirAll(existingDir, 0o755))

	// Scaffold clones first, so it fails on the existing-directory guard before
	// any network or toolchain call.
	tmpl := GameTemplate{
		Name:        "Basic Example",
		ShortName:   "basic",
		Description: "Simple example demonstrating core Cardinal concepts",
		URL:         "https://example.com/repo.git",
		Subdir:      "",
	}
	err := Scaffold(context.Background(), tmpl, existingDir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

// -----------------------------------------------------------------------------
// copyTemplateDirectory
// -----------------------------------------------------------------------------

func (s *SetupCloneTestSuite) TestCopyTemplateDirectory_SkipsGitAndGitkeep() {
	t := s.T()

	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")

	// Setup source with .git and .gitkeep
	require.NoError(t, os.MkdirAll(filepath.Join(src, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, ".git", "config"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(src, ".gitkeep"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(src, "main.go"), []byte("package main"), 0o644))

	require.NoError(t, copyTemplateDirectory(src, dst))

	// .git and .gitkeep should be skipped
	assert.NoDirExists(t, filepath.Join(dst, ".git"))
	assert.NoFileExists(t, filepath.Join(dst, ".gitkeep"))
	assert.FileExists(t, filepath.Join(dst, "main.go"))
}

func (s *SetupCloneTestSuite) TestCopyTemplateDirectory_PreservesSubdirectories() {
	t := s.T()

	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")

	// Create nested structure
	require.NoError(t, os.MkdirAll(filepath.Join(src, "pkg", "component"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "pkg", "component", "health.go"),
		[]byte("package component"), 0o644))

	require.NoError(t, copyTemplateDirectory(src, dst))

	assert.FileExists(t, filepath.Join(dst, "pkg", "component", "health.go"))
}

// -----------------------------------------------------------------------------
// skipGitArtifacts
// -----------------------------------------------------------------------------

func (s *SetupCloneTestSuite) TestSkipGitArtifacts_SkipsGitDir() {
	t := s.T()

	info, err := os.Stat(t.TempDir())
	require.NoError(t, err)

	skip, err := skipGitArtifacts(info, "/path/to/.git", "")
	require.NoError(t, err)
	assert.True(t, skip)
}

func (s *SetupCloneTestSuite) TestSkipGitArtifacts_SkipsGitkeep() {
	t := s.T()

	// Create a temp file to get file info
	tmpFile := filepath.Join(t.TempDir(), ".gitkeep")
	require.NoError(t, os.WriteFile(tmpFile, []byte(""), 0o644))

	info, err := os.Stat(tmpFile)
	require.NoError(t, err)

	skip, err := skipGitArtifacts(info, tmpFile, "")
	require.NoError(t, err)
	assert.True(t, skip)
}

func (s *SetupCloneTestSuite) TestSkipGitArtifacts_AllowsRegularFiles() {
	t := s.T()

	tmpFile := filepath.Join(t.TempDir(), "main.go")
	require.NoError(t, os.WriteFile(tmpFile, []byte("package main"), 0o644))

	info, err := os.Stat(tmpFile)
	require.NoError(t, err)

	skip, err := skipGitArtifacts(info, tmpFile, "")
	require.NoError(t, err)
	assert.False(t, skip)
}

// -----------------------------------------------------------------------------
// createGoModIn
// -----------------------------------------------------------------------------

func (s *SetupCloneTestSuite) TestCreateGoModIn_CreatesWhenMissing() {
	t := s.T()
	dir := t.TempDir()

	err := createGoModIn(dir, "example.com/mymod", "v0.18.0")
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	require.NoError(t, err)
	assert.Equal(t, "module example.com/mymod\n\ngo 1.27.1\n\n"+
		"require github.com/argus-labs/world-engine v0.18.0\n\n"+
		"tool github.com/argus-labs/world-engine/cli/cmd/world\n", string(data))
}

func (s *SetupCloneTestSuite) TestCreateGoModIn_SkipsWhenExists() {
	t := s.T()
	dir := t.TempDir()
	goModPath := filepath.Join(dir, "go.mod")
	require.NoError(t, os.WriteFile(goModPath, []byte("module keep.me\n"), 0o644))

	err := createGoModIn(dir, "example.com/ignored", "v0.18.0")
	require.NoError(t, err)

	data, err := os.ReadFile(goModPath)
	require.NoError(t, err)
	assert.Equal(t, "module keep.me\n", string(data))
}

// -----------------------------------------------------------------------------
// rewriteImportsIn
// -----------------------------------------------------------------------------

func (s *SetupCloneTestSuite) TestRewriteImportsIn_RewritesExampleImports() {
	t := s.T()
	dir := t.TempDir()

	src := `package x

import (
	c "github.com/argus-labs/world-engine/pkg/cardinal"
	otherworld "github.com/argus-labs/world-engine/pkg/template/basic/other_world"
)
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.go"), []byte(src), 0o644))

	err := rewriteImportsIn(dir, "example.com/mymod")
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "file.go"))
	require.NoError(t, err)
	out := string(data)

	// Engine import preserved
	assert.Contains(t, out, `"github.com/argus-labs/world-engine/pkg/cardinal"`)
	// Example import rewritten
	assert.Contains(t, out, `"example.com/mymod/other_world"`)
}

func (s *SetupCloneTestSuite) TestRewriteImportsIn_PreservesUnrelatedImports() {
	t := s.T()
	dir := t.TempDir()

	src := `package x

import "fmt"

func main() { fmt.Println("hello") }
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.go"), []byte(src), 0o644))

	err := rewriteImportsIn(dir, "example.com/mymod")
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "file.go"))
	require.NoError(t, err)
	assert.Equal(t, src, string(data)) // unchanged
}

func (s *SetupCloneTestSuite) TestRewriteImportsIn_SkipsGitDirectory() {
	t := s.T()
	dir := t.TempDir()

	// Create a .git directory with a Go file (should be skipped)
	gitDir := filepath.Join(dir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "hooks.go"), []byte(`package hooks
import "github.com/argus-labs/world-engine/pkg/template/basic/other_world"
`), 0o644))

	// Run rewrite
	err := rewriteImportsIn(dir, "example.com/mymod")
	require.NoError(t, err)

	// .git file should be unchanged
	data, err := os.ReadFile(filepath.Join(gitDir, "hooks.go"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "world-engine/pkg/template") // Not rewritten
}

// -----------------------------------------------------------------------------
// prettifyGoToolError
// -----------------------------------------------------------------------------

func (s *SetupCloneTestSuite) TestPrettifyGoToolError_PrivateModule() {
	t := s.T()

	err := prettifyGoToolError("fatal: could not read Username: terminal prompts disabled")

	assert.Contains(t, err.Error(), "unable to fetch private module")
}

func (s *SetupCloneTestSuite) TestPrettifyGoToolError_SumDBLookup() {
	t := s.T()

	err := prettifyGoToolError("verifying module: sum.golang.org/lookup failed")

	assert.Contains(t, err.Error(), "unable to fetch private module")
}

func (s *SetupCloneTestSuite) TestPrettifyGoToolError_TruncatesMultiline() {
	t := s.T()

	err := prettifyGoToolError("first line error\nsecond line\nthird line")

	assert.Equal(t, "go mod tidy failed: first line error", err.Error())
}

func (s *SetupCloneTestSuite) TestPrettifyGoToolError_SingleLine() {
	t := s.T()

	err := prettifyGoToolError("some error message")

	assert.Equal(t, "go mod tidy failed: some error message", err.Error())
}

// A failure partway through instantiation must remove the partially-created
// target directory so the name stays retryable (uses a local git repo whose
// template has no world.toml, so it fails at the world.toml step after the
// directory has already been created).
func (s *SetupCloneTestSuite) TestInstantiateTemplate_CleansUpPartialOnFailure() {
	t := s.T()

	repoDir := t.TempDir()
	repo, err := gogit.PlainInit(repoDir, false)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, "tmpl"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "tmpl", "main.go"), []byte("package main\n"), 0o600))
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add(".")
	require.NoError(t, err)
	head, err := wt.Commit("init", &gogit.CommitOptions{
		Author: &object.Signature{Name: "t", Email: "t@example.com", When: time.Now()},
	})
	require.NoError(t, err)
	_, err = repo.CreateTag("v0.0.1", head, nil)
	require.NoError(t, err)

	target := filepath.Join(t.TempDir(), "proj")
	err = InstantiateTemplate(context.Background(), repoDir, "v0.0.1", target, "tmpl")
	require.Error(t, err) // no world.toml in the template
	assert.Contains(t, err.Error(), "world.toml")
	assert.NoDirExists(t, target) // partial output cleaned up
}

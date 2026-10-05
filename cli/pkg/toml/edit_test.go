package toml_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	toml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// templateWorldTOML mirrors the world.toml shipped by the world-engine templates.
const templateWorldTOML = `# World Engine Configuration
# All commented-out keys are optional with sensible defaults.

# These fields are your project identifier.
organization = "organization"
project = "project"

# Shard definitions
# Each shard's ` + "`id`" + ` must match its directory name under shards/.

[[shards]]
id = "game"
# log_level = "info"
`

func TestSetProject_ReplacesValue(t *testing.T) {
	t.Parallel()

	out := toml.SetProject(templateWorldTOML, "my-game")
	want := strings.Replace(templateWorldTOML, `project = "project"`, `project = "my-game"`, 1)

	assert.Equal(t, want, out)
}

func TestSetProject_ReplacesOnlyFirstAssignment(t *testing.T) {
	t.Parallel()

	const src = "project = \"first\"\nproject = \"second\"\n"
	const want = "project = \"my-game\"\nproject = \"second\"\n"

	assert.Equal(t, want, toml.SetProject(src, "my-game"))
}

func TestSetProject_NoProjectLine(t *testing.T) {
	t.Parallel()

	const noProject = "organization = \"organization\"\n[[shards]]\nid = \"game\"\n"
	assert.Equal(t, noProject, toml.SetProject(noProject, "my-game"))
}

// The replacement must be substituted literally. regexp.ReplaceAllString would
// expand `$name`/`${name}` in the project as capture-group references (there are
// none), silently truncating the value — e.g. "my$game" would become "my", which
// is still a *valid* project name and so would slip past validation.
func TestSetProject_DollarIsLiteral(t *testing.T) {
	t.Parallel()

	const src = "organization = \"org\"\nproject = \"project\"\n"
	for _, project := range []string{"my$game", "a$1b", "cost$", "a${x}b", "$$"} {
		out := toml.SetProject(src, project)
		assert.Contains(t, out, fmt.Sprintf("project = %q", project), "project %q", project)
	}
}

func TestWriteProject_SetsProject(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, toml.FileName)
	require.NoError(t, os.WriteFile(path, []byte(templateWorldTOML), 0o600))

	require.NoError(t, toml.WriteProject(path, "starter-game"))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	out := string(data)

	assert.Contains(t, out, `project = "starter-game"`)
	assert.NotContains(t, out, `project = "project"`)
	assert.Contains(t, out, `id = "game"`) // shards + comments preserved

	// The rewritten file must still parse and validate.
	cfg, err := toml.LoadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "starter-game", cfg.Project)
}

func TestWriteProject_MissingFile(t *testing.T) {
	t.Parallel()

	err := toml.WriteProject(filepath.Join(t.TempDir(), toml.FileName), "starter-game")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "world.toml")
}

// WriteProject must reject a world.toml with no project line (SetProject can't set
// it) instead of silently writing an invalid file.
func TestWriteProject_RejectsMissingProject(t *testing.T) {
	t.Parallel()

	const noProject = "organization = \"org\"\n\n[[shards]]\nid = \"game\"\n"
	dir := t.TempDir()
	path := filepath.Join(dir, toml.FileName)
	require.NoError(t, os.WriteFile(path, []byte(noProject), 0o600))

	err := toml.WriteProject(path, "my-game")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")
}

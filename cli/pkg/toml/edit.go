package toml

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/rotisserie/eris"
)

// projectFieldRE matches a `project = ...` assignment line. [ \t] rather than \s
// keeps the match on one line.
var projectFieldRE = regexp.MustCompile(`(?m)^[ \t]*project[ \t]*=.*$`)

// SetProject replaces the first project assignment in a world.toml document,
// which the World Engine templates define at the top level. It edits the text so
// comments and layout survive; a typed round-trip would drop every comment. If no
// project line exists the content is returned unchanged, and callers should
// validate afterwards (see WriteProject).
func SetProject(content, project string) string {
	match := projectFieldRE.FindStringIndex(content)
	if match == nil {
		return content
	}
	return content[:match[0]] + fmt.Sprintf("project = %q", project) + content[match[1]:]
}

// WriteProject reads the world.toml at path, sets its project field to project,
// validates the result, and writes it back. It fails without writing if the file
// is missing or the edit yields an invalid world.toml (e.g. there was no project
// line to set).
func WriteProject(path, project string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return eris.Wrap(err, "failed to read world.toml")
	}

	updated := SetProject(string(data), project)

	// Validate before committing: catches a project we failed to set, rather than
	// writing a placeholder or otherwise invalid world.toml.
	if _, err := Load(strings.NewReader(updated)); err != nil {
		return eris.Wrap(err, "world.toml is invalid after setting project")
	}

	// 0600 satisfies gosec G306; world.toml is not sensitive, but stricter perms
	// are acceptable.
	if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
		return eris.Wrap(err, "failed to write world.toml")
	}
	return nil
}

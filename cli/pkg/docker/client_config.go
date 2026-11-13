package docker

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// NewClientConfig builds a service.Config by reading world.toml from projectDir.
// projectDir may be relative; it is resolved to an absolute path against the
// process's working directory so the world.toml lookup and the Docker build
// context (cfg.RootDir) never silently target the wrong project. This
// protects MCP callers, where a relative world_path would otherwise be
// resolved against the long-lived server process's cwd.
func NewClientConfig(projectDir string, debug bool) (*service.Config, error) {
	abs, err := filepath.Abs(projectDir)
	if err != nil {
		return nil, eris.Wrap(err, "resolve project dir")
	}

	worldToml, err := worldtoml.LoadDir(abs)
	if err != nil {
		userMsg := fmt.Sprintf("Cannot find %s in %s", worldtoml.FileName, abs)
		if !eris.Is(err, os.ErrNotExist) {
			userMsg = fmt.Sprintf("Failed to read %s", worldtoml.FileName)
		}

		return nil, eris.Wrap(
			err,
			userMsg+": ensure projectDir points to a valid World Engine project directory",
		)
	}

	cfg := &service.Config{
		RootDir:   abs,
		Debug:     debug,
		WorldToml: worldToml,
	}

	cfg.Project = worldToml.Project
	cfg.NATSURL = service.NatsURL(worldToml.Project)

	return cfg, nil
}

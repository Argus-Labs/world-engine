package docker

import (
	"fmt"
	"os"

	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// NewClientConfig builds a service.Config by reading world.toml from projectDir.
// projectDir must be an absolute path to the World Engine project root.
func NewClientConfig(projectDir string, debug bool) (*service.Config, error) {
	worldToml, err := worldtoml.LoadDir(projectDir)
	if err != nil {
		userMsg := fmt.Sprintf("Cannot find %s in %s", worldtoml.FileName, projectDir)
		if !eris.Is(err, os.ErrNotExist) {
			userMsg = fmt.Sprintf("Failed to read %s", worldtoml.FileName)
		}

		return nil, eris.Wrap(
			err,
			userMsg+": ensure projectDir points to a valid World Engine project directory",
		)
	}

	cfg := &service.Config{
		RootDir:   projectDir,
		Debug:     debug,
		WorldToml: worldToml,
	}

	cfg.Project = worldToml.Project
	cfg.NATSURL = service.NatsURL(worldToml.Project)

	return cfg, nil
}

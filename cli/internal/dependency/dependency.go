package dependency

import (
	"errors"
	"os/exec"

	"github.com/rotisserie/eris"
)

// CmdFactory creates a new [exec.Cmd] for each check.
// This is necessary because [exec.Cmd] can only be Run() once.
type CmdFactory func() *exec.Cmd

//nolint:gochecknoglobals // Predefined dependencies
var (
	Git = Dependency{
		Name:       "Git",
		CmdFactory: func() *exec.Cmd { return exec.Command("git", "--version") },
		Help: `Git is required to clone the starter-game-template.
Learn how to install Git: https://github.com/git-guides/install-git`,
	}
	Go = Dependency{
		Name:       "Go",
		CmdFactory: func() *exec.Cmd { return exec.Command("go", "version") },
		Help: `Go is required to build and run World Engine game shards.
Learn how to install Go: https://go.dev/doc/install`,
	}
	Docker = Dependency{
		Name:       "Docker",
		CmdFactory: func() *exec.Cmd { return exec.Command("docker", "--version") },
		Help: `Docker is required to build and run World Engine game shards.
Learn how to install Docker: https://docs.docker.com/engine/install/`,
	}
	DockerDaemon = Dependency{
		Name:       "Docker daemon is running",
		CmdFactory: func() *exec.Cmd { return exec.Command("docker", "info") },
		Help: `Docker daemon needs to be running.
If you use Docker Desktop, make sure that you have ran it`,
	}
	AlwaysFail = Dependency{
		Name:       "Always fails",
		CmdFactory: func() *exec.Cmd { return exec.Command("false") },
		Help:       `This dependency check will always fail. It can be used for testing.`,
	}
)

type Dependency struct {
	Name       string
	CmdFactory CmdFactory
	Help       string
}

func (d Dependency) Check() error {
	cmd := d.CmdFactory()
	if err := cmd.Run(); err != nil {
		return eris.Wrapf(err, "dependency check for %q failed", d.Name)
	}
	return nil
}

func Check(deps ...Dependency) error {
	errs := make([]error, 0, len(deps))
	for _, dep := range deps {
		errs = append(errs, dep.Check())
	}
	return errors.Join(errs...)
}

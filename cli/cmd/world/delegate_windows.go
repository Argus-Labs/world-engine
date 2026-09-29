//go:build windows

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
)

// runGoTool runs argv with this process's terminal and exits with its status. Windows has no exec, so
// the child runs alongside this process. It returns only if the child fails to start.
func runGoTool(goBin string, argv, env []string) {
	cmd := exec.CommandContext(
		context.Background(),
		goBin,
		argv[1:]...) // #nosec G204 G702 -- argv is `go tool world` plus the user's own arguments
	cmd.Stdin, cmd.Stdout, cmd.Stderr, cmd.Env = os.Stdin, os.Stdout, os.Stderr, env
	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		os.Exit(0)
	case errors.As(err, &exitErr):
		os.Exit(exitErr.ExitCode())
	}
}

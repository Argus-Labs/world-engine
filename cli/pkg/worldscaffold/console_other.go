//go:build !windows

package worldscaffold

import "os/exec"

// hideConsoleWindow is a no-op outside Windows, where a child process cannot pop
// a console window.
func hideConsoleWindow(_ *exec.Cmd) {}

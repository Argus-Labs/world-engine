//go:build !windows

package main

import "syscall"

// runGoTool replaces this process with argv, so the project's CLI owns the terminal, signals, and exit
// status. It returns only if the exec fails, leaving this binary to run the command.
func runGoTool(goBin string, argv, env []string) {
	_ = syscall.Exec(goBin, argv, env) // #nosec G702 -- argv is `go tool world` plus the user's own arguments
}

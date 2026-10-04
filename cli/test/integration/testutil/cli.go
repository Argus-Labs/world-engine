//go:build integration

// Package testutil provides helper functions for integration tests.
package testutil

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

// CLIResult holds the result of a CLI command execution.
type CLIResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

// RunCLI executes the world CLI binary with the given arguments.
func RunCLI(ctx context.Context, binary string, args ...string) CLIResult {
	return RunCLIWithEnv(ctx, binary, nil, args...)
}

// RunCLIWithEnv executes the world CLI binary with the given arguments and environment variables.
func RunCLIWithEnv(ctx context.Context, binary string, env map[string]string, args ...string) CLIResult {
	cmd := exec.CommandContext(ctx, binary, args...)

	// Inherit current environment and add custom vars
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return CLIResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Err:      err,
	}
}

// RunCLIInDir executes the world CLI binary in a specific directory.
func RunCLIInDir(ctx context.Context, binary, dir string, args ...string) CLIResult {
	return RunCLIInDirWithEnv(ctx, binary, dir, nil, args...)
}

// BackgroundProcess represents a CLI process running in the background.
type BackgroundProcess struct {
	cmd    *exec.Cmd
	stdout *bytes.Buffer
	stderr *bytes.Buffer
	done   chan error
}

// Wait waits for the background process to complete and returns the result.
func (p *BackgroundProcess) Wait() CLIResult {
	err := <-p.done

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return CLIResult{
		Stdout:   p.stdout.String(),
		Stderr:   p.stderr.String(),
		ExitCode: exitCode,
		Err:      err,
	}
}

// Kill terminates the background process.
func (p *BackgroundProcess) Kill() error {
	if p.cmd.Process != nil {
		return p.cmd.Process.Kill()
	}
	return nil
}

// RunCLIBackground starts the CLI binary in the background with stdin input.
// Returns a BackgroundProcess that can be killed or waited on.
// This is useful for long-running commands like `world start`.
func RunCLIBackground(ctx context.Context, binary, dir, stdin string, args ...string) (*BackgroundProcess, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()

	// Provide stdin input
	cmd.Stdin = strings.NewReader(stdin)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	proc := &BackgroundProcess{
		cmd:    cmd,
		stdout: &stdout,
		stderr: &stderr,
		done:   make(chan error, 1),
	}

	// Wait for process in goroutine
	go func() {
		proc.done <- cmd.Wait()
	}()

	return proc, nil
}

// RunCLIWithStdin executes the world CLI binary with stdin input in a specific directory.
// This is useful for interactive commands that require user input.
func RunCLIWithStdin(ctx context.Context, binary, dir, stdin string, args ...string) CLIResult {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()

	// Provide stdin input
	cmd.Stdin = strings.NewReader(stdin)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return CLIResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Err:      err,
	}
}

// RunCLIInDirWithEnv executes the world CLI binary in a specific directory with environment variables.
func RunCLIInDirWithEnv(ctx context.Context, binary, dir string, env map[string]string, args ...string) CLIResult {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir

	// Inherit current environment and add custom vars
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return CLIResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Err:      err,
	}
}

// Success returns true if the command exited with code 0.
func (r CLIResult) Success() bool {
	return r.ExitCode == 0
}

// OutputContains checks if stdout or stderr contains the given substring.
func (r CLIResult) OutputContains(substr string) bool {
	return strings.Contains(r.Stdout, substr) || strings.Contains(r.Stderr, substr)
}

// StdoutContains checks if stdout contains the given substring.
func (r CLIResult) StdoutContains(substr string) bool {
	return strings.Contains(r.Stdout, substr)
}

// StderrContains checks if stderr contains the given substring.
func (r CLIResult) StderrContains(substr string) bool {
	return strings.Contains(r.Stderr, substr)
}

// DefaultTimeout returns the default timeout for CLI operations.
func DefaultTimeout() time.Duration {
	return 2 * time.Minute
}

// ShortTimeout returns a shorter timeout for quick operations.
func ShortTimeout() time.Duration {
	return 30 * time.Second
}

// LongTimeout returns a longer timeout for slow operations.
func LongTimeout() time.Duration {
	return 5 * time.Minute
}

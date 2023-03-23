package main

import (
	"os"

	"github.com/magefile/mage/sh"
)

// RunCmdV is a helper function that returns a function that runs the given
// command with the given arguments.
func RunCmdV(cmd string, args ...string) func(args ...string) error {
	return func(args2 ...string) error {
		return sh.RunV(cmd, append(args, args2...)...)
	}
}

// Executes a function in a given directory.
func ExecuteInDirectory(dir string, f func(args ...string) error, withArgs bool) error {
	rootCwd, _ := os.Getwd()
	// Change to the directory where the contracts are.
	if err := os.Chdir(dir); err != nil {
		return err
	}
	// Run the command
	if withArgs {
		if err := f(dir); err != nil {
			return err
		}
	} else {
		if err := f(); err != nil {
			return err
		}
	}

	// Go back to the starting directory.
	if err := os.Chdir(rootCwd); err != nil {
		return err
	}
	return nil
}

func ExecuteForAllModules(dirs []string, f func(args ...string) error, withArgs bool) error {
	for _, dir := range dirs {
		if err := ExecuteInDirectory(dir, f, withArgs); err != nil {
			return err
		}
	}
	return nil
}

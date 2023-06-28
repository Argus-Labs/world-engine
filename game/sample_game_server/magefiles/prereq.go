//go:build mage

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/magefile/mage/sh"
)

const (
	goprivateEnv = "GOPRIVATE"
	goprivateURL = "github.com/argus-labs/world-engine"
)

func checkPrereq(verbose bool) error {
	var errs []error

	// check runs the given verification function, and only prints success information
	// if the verbose flag is set to true.
	check := func(msg string, fn func() error) {
		if verbose {
			fmt.Printf("%-30s", "Checking "+msg+"...")
		}
		if err := fn(); err != nil {
			errs = append(errs, err)
		} else if verbose {
			fmt.Println("success")
		}
	}

	check(goprivateEnv, func() error {
		env := os.Getenv(goprivateEnv)
		if !strings.Contains(env, goprivateURL) {
			return fmt.Errorf("the env variable %q should contain %q", goprivateEnv, goprivateURL)
		}
		return nil
	})

	check("Docker", func() error {
		if _, err := sh.Output("docker", "-v"); err != nil {
			return fmt.Errorf("docker is not installed: %v", err)
		}
		return nil
	})

	check("Docker compose", func() error {
		if _, err := sh.Output("docker", "compose", "version"); err != nil {
			return fmt.Errorf("docker compose is not installed: %v", err)
		}
		return nil
	})

	check("Docker daemon", func() error {
		if _, err := sh.Output("docker", "info"); err != nil {
			return fmt.Errorf("docker daemon is not running: %v", err)
		}
		return nil
	})

	check("Git", func() error {
		if _, err := sh.Output("git", "version"); err != nil {
			return fmt.Errorf("git is not installed: %v", err)
		}
		return nil
	})

	return errors.Join(errs...)
}

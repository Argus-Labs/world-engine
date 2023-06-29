//go:build mage

package main

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/magefile/mage/sh"
)

const (
	goprivateEnv            = "GOPRIVATE"
	goprivateURLArgusLabs   = "github.com/argus-labs"
	goprivateURLWorldEngine = goprivateURLArgusLabs + "/world-engine"
)

// allOutput runs the command and returns the stdout and stderr. Nothing is printed to stdout and stderr.
func allOutput(cmd string, args ...string) (out string, err error) {
	outWriter, errWriter := &bytes.Buffer{}, &bytes.Buffer{}

	_, err = sh.Exec(nil, outWriter, errWriter, cmd, args...)
	if err != nil {
		return "", err
	}
	if errWriter.Len() > 0 {
		err = errors.New(errWriter.String())
	}
	return outWriter.String(), err
}

func checkPrereq(verbose bool) error {
	var errCount int

	// check runs the given verification function, and only prints success information
	// if the verbose flag is set to true.
	check := func(msg string, fn func() error) {
		if verbose {
			fmt.Printf("%-30s", "Checking "+msg+"...")
		}
		if err := fn(); err != nil {
			errCount++
			fmt.Println("FAILURE")
			fmt.Println("  ", err.Error())
		} else if verbose {
			fmt.Println("success")
		}
	}

	check(goprivateEnv, func() error {
		out, err := allOutput("go", "env", goprivateEnv)
		if err != nil {
			return fmt.Errorf("problem getting env variable %q", goprivateEnv)
		} else if !strings.Contains(out, goprivateURLArgusLabs) {
			return fmt.Errorf("the env variable %q should contain %q or %q", goprivateEnv, goprivateURLArgusLabs, goprivateURLWorldEngine)
		}
		return nil
	})

	check("Docker", func() error {
		if _, err := allOutput("docker", "-v"); err != nil {
			return fmt.Errorf("docker is not installed: %v", err)
		}
		return nil
	})

	check("Docker compose", func() error {
		if _, err := allOutput("docker", "compose", "version"); err != nil {
			return fmt.Errorf("docker compose is not installed: %v", err)
		}
		return nil
	})

	check("Docker daemon", func() error {
		if _, err := allOutput("docker", "info"); err != nil {
			return fmt.Errorf("docker daemon is not running: %v", err)
		}
		return nil
	})

	check("Git", func() error {
		if _, err := allOutput("git", "version"); err != nil {
			return fmt.Errorf("git is not installed: %v", err)
		}
		return nil
	})

	if errCount > 0 {
		return fmt.Errorf("encountered %d errors when checking prerequisites", errCount)
	}
	return nil
}

//go:build mage

package main

import (
	"os"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

// Check verifies that various prerequisites are installed or configured on your machine
func Check() error {
	return checkPrereq(true)
}

// Stop stops Nakama and the game server.
func Stop() error {
	return sh.Run("docker", "compose", "stop")
}

// Restart restarts ONLY the game server.
func Restart() error {
	if err := sh.Run("docker", "compose", "stop", "server"); err != nil {
		return err
	}
	if err := sh.Run("docker", "compose", "up", "server", "--build", "-d"); err != nil {
		return err
	}
	return nil
}

// Start starts Nakama and the game server
func Start() error {
	mg.Deps(mg.F(checkPrereq, mg.Verbose()))

	if err := prepareDir("server"); err != nil {
		return err
	}
	if err := prepareDir("nakama"); err != nil {
		return err
	}
	if err := sh.RunV("docker", "compose", "up", "--build"); err != nil {
		return err
	}
	return nil
}

func prepareDir(dir string) error {
	if err := os.Chdir(dir); err != nil {
		return err
	}
	if err := sh.Rm("./vendor"); err != nil {
		return err
	}
	if err := sh.Run("go", "mod", "tidy"); err != nil {
		return err
	}
	if err := sh.Run("go", "mod", "vendor"); err != nil {
		return err
	}
	if err := os.Chdir(".."); err != nil {
		return err
	}
	return nil
}

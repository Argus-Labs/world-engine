//go:build mage

package main

import (
	"os"
	"path/filepath"

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
	mg.Deps(exitMagefilesDir)
	if err := sh.Run("docker", "compose", "stop", "server"); err != nil {
		return err
	}
	if err := sh.Run("docker", "compose", "up", "server", "--build", "-d"); err != nil {
		return err
	}
	return nil
}

// Nakama starts just the Nakama server. The game server needs to be started some other way.
func Nakama() error {
	mg.Deps(exitMagefilesDir)
	if err := prepareDir("nakama"); err != nil {
		return err
	}
	env := map[string]string{
		"CARDINAL_ADDR": "http://host.docker.internal:3333",
	}
	if err := sh.RunWithV(env, "docker", "compose", "up", "--build", "nakama"); err != nil {
		return err
	}
	return nil
}

// Start starts Nakama and the game server
func Start() error {
	mg.Deps(exitMagefilesDir)
	if err := prepareDir("server"); err != nil {
		return err
	}
	if err := prepareDir("nakama"); err != nil {
		return err
	}
	if err := sh.RunV("docker", "compose", "up", "--build", "server", "nakama"); err != nil {
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

func exitMagefilesDir() error {
	curr, err := os.Getwd()
	if err != nil {
		return err
	}
	curr = filepath.Base(curr)
	if curr == "magefiles" {
		if err := os.Chdir(".."); err != nil {
			return err
		}

	}
	return nil
}

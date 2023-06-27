//go:build mage

package main

import (
	"os"

	"github.com/magefile/mage/sh"
)

func Stop() error {
	return sh.Run("docker", "compose", "stop")
}

func Restart() error {
	if err := sh.Run("docker", "compose", "stop", "server"); err != nil {
		return err
	}
	if err := sh.Run("docker", "compose", "up", "server", "--build", "-d"); err != nil {
		return err
	}
	return nil
}

func Start() error {
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
	if err := sh.Run("rm", "-rf", "vendor/"); err != nil {
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

package main

import (
	"fmt"
	"os"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

type Nakama mg.Namespace

func (n Nakama) Test() error {
	for _, dir := range []string{
		"./internal/test_nakama",
		"./internal/e2e/tester/cardinal",
		"./relay/nakama",
	} {
		if err := prepareDir(dir); err != nil {
			return err
		}
	}
	env := map[string]string{
		"ENABLE_ADAPTER": "false",
	}

	return sh.RunWithV(env, "docker", "compose", "up",
		"--build", "--abort-on-container-exit",
		"--exit-code-from", "test_nakama",
		"--attach", "test_nakama")
}

func prepareDirs(dirs ...string) error {
	for _, d := range dirs {
		if err := prepareDir(d); err != nil {
			return fmt.Errorf("failed to prepare dir %s: %w", d, err)
		}
	}
	return nil
}

func prepareDir(dir string) error {
	originDir, err := os.Getwd()
	if err != nil {
		return err
	}
	if err = os.Chdir(dir); err != nil {
		return err
	}
	if err = sh.Rm("./vendor"); err != nil {
		return err
	}
	if err = sh.Run("go", "mod", "tidy"); err != nil {
		return err
	}
	if err = sh.Run("go", "mod", "vendor"); err != nil {
		return err
	}
	return os.Chdir(originDir)
}

//go:build mage

package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/magefile/mage/sh"
)

// Copy copies this sample project to the <target> directory and initializes it with 'go mod init <modulePath>'.
// The module path parameter should be set to your code's repository. See https://golang.org/ref/mod#go-mod-init
// for more info about go mod.
func Copy(target, modulePath string) error {
	if err := os.MkdirAll(target, os.ModePerm); err != nil {
		return err
	}
	walkErr := filepath.Walk(".", func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(path, "go.mod") {
			return nil
		}
		if strings.HasSuffix(path, "go.sum") {
			return nil
		}
		if strings.Contains(path, "vendor") {
			return nil
		}
		if info.IsDir() {
			if err := os.MkdirAll(filepath.Join(target, path), os.ModePerm); err != nil {
				return err
			}
			return nil
		}
		source := filepath.Join(".", path)
		dest := filepath.Join(target, path)
		if err := sh.Copy(dest, source); err != nil {
			return err
		}

		return nil
	})
	if walkErr != nil {
		return walkErr
	}

	if err := os.Chdir(target); err != nil {
		return err
	}

	if err := goModInit(modulePath, "nakama"); err != nil {
		return err
	}
	if err := goModInit(modulePath, "server"); err != nil {
		return err
	}
	if err := goModInit(modulePath, "magefiles"); err != nil {
		return err
	}

	return nil
}

func goModInit(modulePath, component string) error {
	if err := os.Chdir(component); err != nil {
		return err
	}
	if err := sh.Run("go", "mod", "init", modulePath+"/"+component); err != nil {
		return err
	}
	if err := sh.Run("go", "mod", "tidy"); err != nil {
		return err
	}
	if err := os.Chdir(".."); err != nil {
		return err
	}
	return nil
}

package main

import (
	"os"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

type Sidecar mg.Namespace

func (Sidecar) Gen() error {
	LogGreen("Building sidecar proto files...")

	if err := os.Chdir("chain"); err != nil {
		return err
	}

	return sh.Run("./proto/sidecar/scripts/bufgen.sh")
}

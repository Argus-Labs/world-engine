package main

import (
	"os"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

type Cosmos mg.Namespace

var (
	protoImageName    = "ghcr.io/cosmos/proto-builder"
	protoImageVersion = "0.12.1"
	protoDir          = "chain/proto"

	// Variables and Helpers.
	production = false
	statically = false

	//// Commands.
	//dockerBuild = RunCmdV("docker", "build", "--rm=false")
	//
	//// Variables.
	//baseDockerPath  = "./cosmos/"
	//beradDockerPath = baseDockerPath + "Dockerfile"
	//imageName       = "argus-cosmos"
	//// testImageVersion       = "e2e-test-dev".
	//goVersion              = "1.20.2"
	//debianStaticImage      = "gcr.io/distroless/static-debian11"
	//golangAlpine           = "golang:1.20-alpine3.17"
	//precompileContractsDir = "./cosmos/precompile/contracts/solidity"
)

// Build builds the rollup.
func (Cosmos) Build() error {
	LogGreen("Building rollup...")
	cmd := "argusd"
	args := []string{
		generateBuildTags(),
		generateLinkerFlags(production, statically),
		"-o", generateOutDirectory(cmd),
	}
	if err := os.Chdir("chain"); err != nil {
		return err
	}
	command := "cmd/argusd/main.go"
	c := sh.RunCmd("go", "build")
	return c(append(args, command)...)
}

func (Cosmos) ProtoGen() error {
	LogGreen("Generating proto files...")
	if err := os.Chdir("chain"); err != nil {
		return err
	}

	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	dockerArgs := []string{
		"run", "--rm", "-v", dir + ":/workspace",
		"--workdir", "/workspace",
		protoImageName + ":" + protoImageVersion,
		"sh", "./proto/cosmos/scripts/protocgen.sh",
	}

	return sh.Run("docker", dockerArgs...)
}

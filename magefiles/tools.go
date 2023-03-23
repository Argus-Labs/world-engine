package main

import (
	"github.com/magefile/mage/sh"
)

var (
	// Commands.
	goInstall  = RunCmdV("go", "install", "-mod=readonly")
	goBuild    = RunCmdV("go", "build", "-mod=readonly")
	goRun      = RunCmdV("go", "run")
	goGenerate = RunCmdV("go", "generate")
	goModTidy  = RunCmdV("go", "mod", "tidy")
	goWorkSync = RunCmdV("go", "work", "sync")

	// Directories.
	outdir = "../bin"

	// Tools.
	gitDiff = sh.RunCmd("git", "diff", "--stat", "--exit-code", ".",
		"':(exclude)*.mod' ':(exclude)*.sum'")

	// Dependencies.
	moq = "github.com/matryer/moq"

	moduleDirs = []string{"contracts", "eth", "cosmos", "playground", "magefiles", "lib"}
)

package main

import (
	"strings"

	"github.com/magefile/mage/sh"
)

var (
	sdkRepo        = "github.com/cosmos/cosmos-sdk"
	version        = "0.0.0"
	commit, _      = sh.Output("git", "log", "-1", "--format='%H'")
	defaultDB      = "pebbledb"
	ledgerEnabled  = true
	appName        = "argus"
	executableName = "argusd"
)

// generateOutDirectory returns the output directory for a given command.
func generateOutDirectory(cmd string) string {
	return outdir + "/" + cmd
}

// generateBuildTags returns the build tags to be used when building the binary.
func generateBuildTags() string {
	tags := []string{defaultDB}
	if ledgerEnabled {
		tags = append(tags, "ledger")
	}
	return "-tags='" + strings.Join(tags, " ") + "'"
}

// generateLinkerFlags returns the linker flags to be used when building the binary.
func generateLinkerFlags(production, statically bool) string {
	baseFlags := []string{
		"-X ", sdkRepo + "/version.Name=" + executableName,
		" -X ", sdkRepo + "/version.AppName=" + appName,
		" -X ", sdkRepo + "/version.Version=" + version,
		" -X ", sdkRepo + "/version.Commit=" + commit,
		// TODO: Refactor versioning more broadly.
		// " \"-X " + sdkRepo + "/version.BuildTags=" + strings.Join(generateBuildTags(), ",") +
		" -X ", sdkRepo + "/version.DBBackend=" + defaultDB,
	}

	if production {
		baseFlags = append(baseFlags, "-w", "-s")
	}

	if statically {
		baseFlags = append(
			baseFlags,
			"-linkmode=external",
			"-extldflags",
			"\"-Wl,-z,muldefs -static\"",
		)
	}

	return "-ldflags=" + strings.Join(baseFlags, " ")
}

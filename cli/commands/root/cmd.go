package root

import (
	"os"
	"runtime/debug"

	"github.com/alecthomas/kong"

	"github.com/argus-labs/world-engine/cli/commands/debugger"
	"github.com/argus-labs/world-engine/cli/commands/mcp"
	"github.com/argus-labs/world-engine/cli/commands/sdk"
)

// These variables are overridden by ldflags during build.
// Example:
// go build -ldflags "-X github.com/argus-labs/world-engine/cli/commands/root.PosthogAPIKey=<POSTHOG_API_KEY> -X github.com/argus-labs/world-engine/cli/commands/root.SentryDsn=<SENTRY_DSN> -X github.com/argus-labs/world-engine/cli/commands/root.AppVersion=<VERSION>"
//
//nolint:gochecknoglobals // Only used for ldflags
var (
	PosthogAPIKey string
	SentryDsn     string
	AppVersion    string
)

type Cmd struct {
	Setup   *SetupCmd        `cmd:"" group:"Getting Started:"   help:"Setup a new World Engine project"`
	Docs    *DocsCmd         `cmd:"" group:"Getting Started:"   help:"Open the World CLI documentation"`
	Doctor  *DoctorCmd       `cmd:"" group:"Getting Started:"   help:"Check your development environment"`
	Start   *StartCmd        `cmd:"" group:"Cardinal Commands:" help:"Launch your Cardinal game environment"`
	Stop    *StopCmd         `cmd:"" group:"Cardinal Commands:" help:"Gracefully shut down your Cardinal game environment"`
	Purge   *PurgeCmd        `cmd:"" group:"Cardinal Commands:" help:"Reset your Cardinal game shard to a clean state by removing all data and containers"`
	Reload  *ReloadCmd       `cmd:"" group:"Cardinal Commands:" help:"Rebuild and roll Cardinal shards in the running world"`
	Logs    *LogsCmd         `cmd:"" group:"Cardinal Commands:" help:"View and tail logs for shards + platform components"`
	Debug   *debugger.Cmd    `cmd:"" group:"Debugger Commands:" help:"Debug commands for Cardinal shards"                                                  aliases:"db"`
	SDK     *sdk.Cmd         `cmd:"" group:"SDK Commands:"      help:"Generate the typed SDK (Go + C#) from backend wire types"                                         name:"sdk"`
	Build   *BuildCmd        `cmd:""                            help:"Build Cardinal shard images without starting containers (CI use)"                                                hidden:""`
	MCP     *mcp.Cmd         `cmd:""                            help:"MCP server for AI tools (internal)"                                                                              hidden:""`
	Version kong.VersionFlag `                                  help:"Show the version of the CLI"                                                                      name:"version"`
	Verbose bool             `                                  help:"Enable World CLI Debug logs"                                                                                               short:"v"`
}

// GetEnv resolves runtime environment.
func (c *Cmd) GetEnv() string {
	env := "unknown env"

	if AppVersion != "" {
		env = "PROD"
		if os.Getenv("WORLD_CLI_ENV") != "" {
			env = os.Getenv("WORLD_CLI_ENV")
		}
		return env
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		if os.Getenv("WORLD_CLI_ENV") != "" {
			return os.Getenv("WORLD_CLI_ENV")
		}
		return env
	}

	if info.Main.Version == "(devel)" {
		env = "LOCAL"
	} else {
		env = "PROD"
	}

	if os.Getenv("WORLD_CLI_ENV") != "" {
		env = os.Getenv("WORLD_CLI_ENV")
	}

	return env
}

// GetVersion resolves CLI version.
func (c *Cmd) GetVersion() string {
	if AppVersion != "" {
		return AppVersion
	}

	version := "unknown version"
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}

	if info.Main.Version == "(devel)" {
		version = "v0.0.1-dev"
	} else {
		version = info.Main.Version
	}

	return version
}

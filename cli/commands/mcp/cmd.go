package mcp

import (
	"context"
	"os"

	"github.com/rotisserie/eris"
	"golang.org/x/term"

	errorspkg "github.com/argus-labs/world-engine/cli/internal/errors"
	"github.com/argus-labs/world-engine/cli/internal/printer"
	"github.com/argus-labs/world-engine/cli/internal/telemetry"

	"github.com/mark3labs/mcp-go/server"
)

// Cmd runs the MCP server.
// Hidden from help - only called by AI tools like Cursor/Claude.
// Shows setup instructions if run from terminal.
type Cmd struct{}

// Run executes the MCP command.
func (c *Cmd) Run() error {
	telemetry.PosthogCaptureEvent("mcp-command", map[string]any{})

	// Check if stdin is a terminal (human running directly)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		printSetupInstructions()
		return nil
	}

	// Not a terminal - MCP client is calling us, run stdio server
	srv := NewServer()

	if err := server.ServeStdio(srv); err != nil {
		if eris.Is(err, context.Canceled) {
			return errorspkg.NewSilent(err)
		}
		return eris.Wrap(err, "Failed to serve MCP over stdio")
	}

	return nil
}

// printSetupInstructions shows how to configure MCP clients.
func printSetupInstructions() {
	printer.Infoln("")
	printer.Infoln("World Engine MCP Server - AI tooling for World Engine")
	printer.Infoln("")
	printer.Infoln("This provides AI assistants with tools to debug and inspect your World Engine environment.")
	printer.Infoln("")
	printer.Infoln("To connect your MCP-compatible AI tool, configure it to run:")
	printer.Infoln("")
	printer.Infoln("  Command: world")
	printer.Infoln("  Args:    mcp")
}

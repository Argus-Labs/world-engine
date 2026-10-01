package mcp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/guumaster/logsymbols"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rotisserie/eris"
)

// sdkGenerateTimeout bounds the whole generation. The first run builds the
// codegen toolchain image, which the CLI itself warns "may take a few minutes";
// later runs are seconds.
const sdkGenerateTimeout = 10 * time.Minute

// SdkGenerateInput is the structured input for the sdk_generate tool.
type SdkGenerateInput struct {
	Source   string `json:"source"              jsonschema_description:"Backend directory to scan for commands, events, and components — the shard's source dir, e.g. /path/to/world/shards/game."`
	GoOut    string `json:"go_out,omitempty"    jsonschema_description:"Directory to write generated Go into; must sit inside the backend's Go module. Defaults to <source>/gen when no other output is set. When cs_out is also passed go_out is NOT defaulted: omit it for a client-only C# run (no wire layer), or set it explicitly to regenerate the wire layer alongside the C# SDK."`
	CsOut    string `json:"cs_out,omitempty"    jsonschema_description:"Directory to write the generated C# client SDK into. Omit unless a client SDK is wanted."`
	ProtoOut string `json:"proto_out,omitempty" jsonschema_description:"Debugging only, slated for deprecation. Directory to persist the generated .proto schema into, for reading the wire contract by hand; nothing in generation reads it. Requires go_out."`
}

// SdkGenerateOutput is the structured output for the sdk_generate tool.
type SdkGenerateOutput struct {
	Source string `json:"source"           jsonschema_description:"Backend directory that was scanned"`
	GoOut  string `json:"go_out,omitempty" jsonschema_description:"Where generated Go was written"`
	CsOut  string `json:"cs_out,omitempty" jsonschema_description:"Where generated C# was written"`
	Report string `json:"report"           jsonschema_description:"The generator's own report: discovered counts, plus any warnings and orphaned types (declared with Name() but never registered, so not generated)"`
	Status string `json:"status"           jsonschema_description:"Human-readable result summary"`
}

// registerSdkGenerateTool registers the sdk_generate tool — the MCP counterpart
// of `world sdk generate`.
func registerSdkGenerateTool(srv *server.MCPServer) {
	sdkGenerateTool := mcp.NewTool(
		"sdk_generate",
		mcp.WithDescription(
			"Regenerate a shard's wire layer from its Go source — the MCP counterpart of "+
				"`world sdk generate`. Run this after adding, renaming, or changing the fields of any "+
				"command, event, component, or system event: the generated code is what gives those types "+
				"their proto conversions, so until it is regenerated the shard will not compile and a "+
				"reload will fail with errors like 'does not satisfy ecs.Component (missing method "+
				"AppendWire)' or '(missing method MarshalWire)'. Also the fix when a build complains about a stale or missing generated "+
				"package. Generation type-checks the backend first, so it reports authoring violations "+
				"(a field type that cannot be expressed in proto) and orphaned types instead of emitting "+
				"broken code; nothing is written when it finds blocking issues. Run it before reload, not "+
				"after — reload compiles the source this produces. Once that reload lands, run introspect "+
				"again: the type changes that forced the regeneration also changed the schemas it reports.",
		),
		mcp.WithInputSchema[SdkGenerateInput](),
		mcp.WithOutputSchema[SdkGenerateOutput](),
	)
	srv.AddTool(sdkGenerateTool, strictToolHandler(sdkGenerateHandler))
}

// sdkGenerateHandler shells out to this same binary's `sdk generate`. The
// generator writes a progress spinner and a formatted report to the terminal,
// which cannot be emitted inline: this process speaks JSON-RPC over stdio, and
// stray output would corrupt the stream. Running it as a child captures that
// output on a pipe instead, and reuses the CLI's implementation verbatim rather
// than maintaining a second copy of the codegen orchestration.
func sdkGenerateHandler(
	ctx context.Context,
	_ mcp.CallToolRequest,
	args SdkGenerateInput,
) (SdkGenerateOutput, error) {
	source := absPath(args.Source)
	if source == "" {
		return SdkGenerateOutput{}, eris.New(
			"source is required (the backend directory to scan, e.g. <world>/shards/game)")
	}

	goOut, csOut, protoOut := sdkGenerateArgs(source, args)

	exe, err := os.Executable()
	if err != nil {
		return SdkGenerateOutput{}, eris.Wrap(err, "locate the world binary to run sdk generate")
	}

	cmdArgs := sdkGenerateCmdArgs(source, goOut, csOut, protoOut)

	ctx, cancel := ensureDeadline(ctx, sdkGenerateTimeout)
	defer cancel()

	out, runErr := exec.CommandContext(ctx, exe, cmdArgs...).CombinedOutput()
	report := cleanTerminalOutput(out)
	// `world sdk generate` exits 0 even when it refuses to generate (a backend
	// that doesn't type-check, a --go-out outside any module), so the exit status
	// alone would report those failures as success. Its printed report is the
	// reliable signal: a failure always carries printer's error symbol.
	if runErr != nil || reportFailed(report) {
		// The report names the offending types and why, so it is far more useful
		// to the caller than an exit status.
		return SdkGenerateOutput{}, eris.Errorf("sdk generate failed: %s", report)
	}

	return SdkGenerateOutput{
		Source: source,
		GoOut:  goOut,
		CsOut:  csOut,
		Report: report,
		Status: sdkGenerateStatus(goOut, csOut),
	}, nil
}

// sdkGenerateArgs resolves the output paths and defaults go_out to <source>/gen.
// cs_out alone stays a client-only C# run (generate.go's resolveGoPackage
// supports it), so the default only fires when neither output is named.
func sdkGenerateArgs(source string, args SdkGenerateInput) (string, string, string) {
	goOut, csOut, protoOut := absPath(args.GoOut), absPath(args.CsOut), absPath(args.ProtoOut)
	if goOut == "" && csOut == "" {
		goOut = filepath.Join(source, "gen")
	}
	return goOut, csOut, protoOut
}

// sdkGenerateCmdArgs builds the `world sdk generate` argv, omitting any flag
// whose output was not requested (an unset --go-out selects client-only).
func sdkGenerateCmdArgs(source, goOut, csOut, protoOut string) []string {
	cmdArgs := []string{"sdk", "generate", source}
	for _, f := range []struct{ flag, value string }{
		{"--go-out", goOut}, {"--cs-out", csOut}, {"--proto-out", protoOut},
	} {
		if f.value != "" {
			cmdArgs = append(cmdArgs, f.flag+"="+f.value)
		}
	}
	return cmdArgs
}

// sdkGenerateStatus summarises what actually ran, so a client-only run never
// claims wire regeneration. An empty goOut means cs_out was set: the defaulting
// fills goOut whenever cs_out is empty.
func sdkGenerateStatus(goOut, csOut string) string {
	switch {
	case goOut != "" && csOut != "":
		return "regenerated the wire layer and the C# client SDK; rebuild or reload the shard to pick it up"
	case goOut != "":
		return "regenerated the wire layer; rebuild or reload the shard to pick it up"
	default:
		return "generated the C# client SDK"
	}
}

// reportFailed reports whether the generator printed an error. printer marks
// those lines with logsymbols.Error, so this tracks the CLI rather than a
// hardcoded glyph.
func reportFailed(report string) bool {
	marker := string(logsymbols.Error)
	for line := range strings.SplitSeq(report, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), marker) {
			return true
		}
	}
	return false
}

// absPath resolves an optional caller-supplied path; "" means not given.
func absPath(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return ""
	}
	return abs
}

// cleanTerminalOutput turns captured CLI output into something readable in a
// tool result: the spinner redraws its line with carriage returns and colours
// everything, so strip the escapes, keep only the last frame of each redrawn
// line, and drop the blank runs that leaves behind.
func cleanTerminalOutput(raw []byte) string {
	lines := make([]string, 0, 16)
	for line := range strings.SplitSeq(ansi.Strip(string(raw)), "\n") {
		// A redrawn line carries every frame separated by \r; only the last stuck.
		if idx := strings.LastIndex(line, "\r"); idx >= 0 {
			line = line[idx+1:]
		}
		if trimmed := strings.TrimRight(line, " \t"); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return strings.Join(lines, "\n")
}

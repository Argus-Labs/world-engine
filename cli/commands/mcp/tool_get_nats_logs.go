package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
)

// GetNatsLogsInput is the structured input for the get_nats_logs tool.
type GetNatsLogsInput struct {
	TailLines int    `json:"tail_lines,omitempty" jsonschema_description:"Maximum number of log lines to return (0 or unset defaults to 2000, capped at 10000)"`
	Project   string `json:"project,omitempty"    jsonschema_description:"World project whose NATS to read (defaults to the world.toml in the working directory)"`
}

// GetNatsLogsOutput is the structured output for get_nats_logs.
type GetNatsLogsOutput struct {
	Component string `json:"component"`
	Logs      string `json:"logs"`
}

// registerGetNatsLogsTool registers the get_nats_logs tool.
func registerGetNatsLogsTool(srv *server.MCPServer) {
	getNatsLogsTool := mcp.NewTool(
		"get_nats_logs",
		mcp.WithDescription(
			"Fetch recent logs from the project's NATS container. Requires a running world.",
		),
		mcp.WithInputSchema[GetNatsLogsInput](),
		mcp.WithOutputSchema[GetNatsLogsOutput](),
	)
	srv.AddTool(getNatsLogsTool, strictToolHandler(getNatsLogsHandler))
}

// getNatsLogsHandler fetches recent logs from the project's NATS container.
func getNatsLogsHandler(
	ctx context.Context,
	_ mcp.CallToolRequest,
	args GetNatsLogsInput,
) (GetNatsLogsOutput, error) {
	project, err := resolveProject(args.Project)
	if err != nil {
		return GetNatsLogsOutput{}, err
	}
	logs, err := collectContainerLogs(ctx, project, service.NatsContainerName(project), clampTail(args.TailLines))
	if err != nil {
		return GetNatsLogsOutput{}, eris.Wrap(err, "failed to fetch NATS logs")
	}
	return GetNatsLogsOutput{Component: "nats", Logs: logs}, nil
}

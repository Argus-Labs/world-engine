package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rotisserie/eris"
)

// GetNatsLogsInput is the structured input for the get_nats_logs tool.
type GetNatsLogsInput struct {
	TailLines int `json:"tail_lines,omitempty" jsonschema_description:"Maximum number of log lines to return (0 or unset defaults to 2000, capped at 10000)"`
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
			"Fetch recent logs from the NATS pod in the local k8s cluster. Requires a running cluster.",
		),
		mcp.WithInputSchema[GetNatsLogsInput](),
		mcp.WithOutputSchema[GetNatsLogsOutput](),
	)
	srv.AddTool(getNatsLogsTool, strictToolHandler(getNatsLogsHandler))
}

// getNatsLogsHandler fetches recent logs from the NATS platform pod via the
// kube-apiserver (the operator can't reach the nats namespace).
func getNatsLogsHandler(
	ctx context.Context,
	_ mcp.CallToolRequest,
	args GetNatsLogsInput,
) (GetNatsLogsOutput, error) {
	logs, err := collectPlatformLogs(ctx, "nats", clampTail(args.TailLines))
	if err != nil {
		return GetNatsLogsOutput{}, eris.Wrap(err, "failed to fetch NATS logs")
	}
	return GetNatsLogsOutput{Component: "nats", Logs: logs}, nil
}

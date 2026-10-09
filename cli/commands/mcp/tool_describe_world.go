package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// DescribeWorldInput is the (empty) input for describe_world; it reads the
// Docker, not a local path.
type DescribeWorldInput struct {
	Project string `json:"project,omitempty" jsonschema_description:"World project whose namespace to read (defaults to the world.toml in the working directory)"`
}

// DescribeWorldOutput summarizes the worlds running on local Docker.
type DescribeWorldOutput struct {
	Worlds []DescribedWorld `json:"worlds" jsonschema_description:"Worlds currently deployed on local Docker"`
}

// DescribedWorld is one deployed world and its shards.
type DescribedWorld struct {
	Organization string   `json:"organization"`
	Project      string   `json:"project"`
	Shards       []string `json:"shards"`
}

// registerDescribeWorldTool registers the describe_world tool. It lists the
// project's deployed shards, read from its container labels.
func registerDescribeWorldTool(srv *server.MCPServer) {
	describeWorldTool := mcp.NewTool(
		"describe_world",
		mcp.WithDescription(
			"List the world deployed for a project on local Docker (organization, project, and shards), "+
				"read from its container labels. Requires a running world.",
		),
		// No WithInputSchema: the schema it generates for an empty struct has no "properties", which
		// some MCP hosts reject on an object parameter. NewTool's default publishes an empty one.
		mcp.WithSchemaAdditionalProperties(false),
		mcp.WithOutputSchema[DescribeWorldOutput](),
	)
	srv.AddTool(describeWorldTool, strictToolHandler(describeWorldHandler))
}

// describeWorldHandler groups the running shards by world.
func describeWorldHandler(
	ctx context.Context,
	_ mcp.CallToolRequest,
	args DescribeWorldInput,
) (DescribeWorldOutput, error) {
	ctx, cancel := ensureDeadline(ctx, defaultCommandTimeout)
	defer cancel()

	shards, err := deployedShards(ctx, args.Project)
	if err != nil {
		return DescribeWorldOutput{}, err
	}

	type worldKey struct{ org, project string }
	order := make([]worldKey, 0)
	byWorld := make(map[worldKey][]string)
	for _, s := range shards {
		k := worldKey{s.Organization, s.Project}
		if _, ok := byWorld[k]; !ok {
			order = append(order, k)
		}
		byWorld[k] = append(byWorld[k], s.ShardID)
	}

	worlds := make([]DescribedWorld, 0, len(order))
	for _, k := range order {
		worlds = append(worlds, DescribedWorld{Organization: k.org, Project: k.project, Shards: byWorld[k]})
	}
	return DescribeWorldOutput{Worlds: worlds}, nil
}

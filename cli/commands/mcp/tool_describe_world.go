package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// DescribeWorldInput is the (empty) input for describe_world; it reads the
// cluster, not a local path.
type DescribeWorldInput struct{}

// DescribeWorldOutput summarizes the worlds deployed on the cluster.
type DescribeWorldOutput struct {
	Worlds []DescribedWorld `json:"worlds" jsonschema_description:"Worlds currently deployed on the local cluster"`
}

// DescribedWorld is one deployed world and its shards.
type DescribedWorld struct {
	Organization string   `json:"organization"`
	Project      string   `json:"project"`
	Shards       []string `json:"shards"`
}

// registerDescribeWorldTool registers the describe_world tool. It lists the
// worlds the cardinal-operator currently has deployed, read from ShardPool CRs.
func registerDescribeWorldTool(srv *server.MCPServer) {
	describeWorldTool := mcp.NewTool(
		"describe_world",
		mcp.WithDescription(
			"List the worlds currently deployed on the local cluster (organization, project, and shards), "+
				"read from the cardinal-operator's ShardPool resources. No project path needed. Requires a running cluster.",
		),
		mcp.WithInputSchema[DescribeWorldInput](),
		mcp.WithOutputSchema[DescribeWorldOutput](),
	)
	srv.AddTool(describeWorldTool, strictToolHandler(describeWorldHandler))
}

// describeWorldHandler groups the cluster's deployed shards by world.
func describeWorldHandler(
	ctx context.Context,
	_ mcp.CallToolRequest,
	_ DescribeWorldInput,
) (DescribeWorldOutput, error) {
	ctx, cancel := ensureDeadline(ctx, defaultCommandTimeout)
	defer cancel()

	shards, err := deployedShards(ctx)
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

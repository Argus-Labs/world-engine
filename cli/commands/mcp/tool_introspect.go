package mcp

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rotisserie/eris"

	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
)

// IntrospectInput is the structured input for the introspect tool.
type IntrospectInput struct {
	ShardID      string `json:"shard_id"                jsonschema_description:"ID of the shard to introspect"`
	InstanceName string `json:"instance_name,omitempty" jsonschema_description:"Specific pool instance; accepts variants like 'game-5', 'game 5', 'game5', or '5'. Defaults to the shard's first instance."`
	Organization string `json:"organization,omitempty"  jsonschema_description:"Ignored: since the organization left a shard's address, project alone identifies the world"`
	Project      string `json:"project,omitempty"       jsonschema_description:"Project (auto-derived from the running containers when omitted)"`
	ShardURL     string `json:"shard_url,omitempty"     jsonschema_description:"Cardinal shard API URL; auto-resolved (per instance) from the running containers when omitted. When set, Docker is not contacted: pair it with instance_name (the exact instance, e.g. 'game-2'). Also pass project to skip the container lookup entirely; otherwise it is still auto-resolved from the containers."`
}

// IntrospectOutput is the structured output for the introspect tool.
type IntrospectOutput struct {
	Commands   []NamedSchema `json:"commands"   jsonschema_description:"List of available commands with their schemas"`
	Components []NamedSchema `json:"components" jsonschema_description:"List of available components with their schemas"`
	Events     []NamedSchema `json:"events"     jsonschema_description:"List of available events with their schemas"`
}

// NamedSchema is one registered type with its reflection-derived JSON Schema.
type NamedSchema struct {
	Name        string                   `json:"name"`
	Schema      map[string]any           `json:"schema"`
	ArrayFields []*cardinalv1.ArrayField `json:"array_fields,omitempty"`
}

// registerIntrospectTool registers the introspect tool. It retrieves metadata
// about a shard's registered types (components, commands, events) by dialing the
// shard's DebugService directly through the local edge (same path as send_command).
func registerIntrospectTool(srv *server.MCPServer) {
	introspectTool := mcp.NewTool(
		"introspect",
		mcp.WithDescription(
			"Retrieve metadata about a Cardinal shard's registered types (components, commands, and events). "+
				"Returns each type's schema, useful for understanding the shard's API and generating code. "+
				"The result describes the deployed shard binary, so it stays valid for the whole session — "+
				"call it once and reuse it rather than re-calling before every command. It only goes stale "+
				"when the shard is redeployed with changed types: after a reload (or start) that "+
				"added, removed, or changed a command, component, or event, call it again. "+
				"Requires a running world.",
		),
		mcp.WithInputSchema[IntrospectInput](),
		mcp.WithOutputSchema[IntrospectOutput](),
	)
	srv.AddTool(introspectTool, strictToolHandler(introspectHandler))
}

// introspectHandler retrieves a shard's type metadata by dialing its DebugService
// directly through the local edge (mirrors the send_command tool's addressing).
func introspectHandler(
	ctx context.Context,
	_ mcp.CallToolRequest,
	args IntrospectInput,
) (IntrospectOutput, error) {
	ctx, cancel := ensureDeadline(ctx, defaultCommandTimeout)
	defer cancel()

	client, _, err := dialShardDebugService(ctx, debugTargetArgs{
		shardID:      args.ShardID,
		instanceName: args.InstanceName,
		project:      args.Project,
		shardURL:     args.ShardURL,
	})
	if err != nil {
		return IntrospectOutput{}, eris.Wrap(err, "failed to resolve introspection target")
	}

	resp, err := introspectShardTypes(ctx, client)
	if err != nil {
		return IntrospectOutput{}, eris.Wrapf(err, "failed to introspect shard %q", strings.TrimSpace(args.ShardID))
	}

	return IntrospectOutput{
		Commands:   convertTypeSchemas(resp.GetCommands()),
		Components: convertTypeSchemas(resp.GetComponents()),
		Events:     convertTypeSchemas(resp.GetEvents()),
	}, nil
}

// introspectShardTypes fetches metadata without requiring SDK-generated descriptors.
func introspectShardTypes(
	ctx context.Context,
	client cardinalv1connect.DebugServiceClient,
) (*cardinalv1.IntrospectResponse, error) {
	res, err := client.Introspect(ctx, connect.NewRequest(&cardinalv1.IntrospectRequest{}))
	if err != nil {
		return nil, eris.Wrap(err, "failed to introspect shard types")
	}
	return res.Msg, nil
}

// fetchCommandSchema retrieves the deployed command's JSON Schema.
func fetchCommandSchema(
	ctx context.Context,
	shardURL, commandName string,
) (map[string]any, error) {
	resp, err := introspectShardTypes(ctx, newDebugServiceClient(shardURL))
	if err != nil {
		return nil, eris.Wrap(err, "failed to introspect shard for command schema")
	}

	for _, ts := range resp.GetCommands() {
		if ts.GetName() == commandName {
			if ts.GetJsonSchema() == nil {
				return nil, eris.Errorf("command %q has no JSON Schema", commandName)
			}
			return ts.GetJsonSchema().AsMap(), nil
		}
	}
	return nil, eris.Errorf("command %q not found on shard", commandName)
}

// componentDescriptors maps registered component names to their JSON Schemas.
type componentDescriptors map[string]map[string]any

// fetchComponentDescriptors identifies components that can be decoded as MessagePack.
// Components without metadata stay raw rather than failing the whole read.
func fetchComponentDescriptors(
	ctx context.Context,
	client cardinalv1connect.DebugServiceClient,
) (componentDescriptors, error) {
	resp, err := introspectShardTypes(ctx, client)
	if err != nil {
		return nil, eris.Wrap(err, "failed to introspect shard for component schemas")
	}

	descriptors := make(componentDescriptors, len(resp.GetComponents()))
	for _, ts := range resp.GetComponents() {
		if ts.GetJsonSchema() == nil {
			continue
		}
		descriptors[ts.GetName()] = ts.GetJsonSchema().AsMap()
	}
	return descriptors, nil
}

// convertTypeSchemas retains the advertised JSON Schema verbatim, including $defs.
func convertTypeSchemas(schemas []*cardinalv1.TypeSchema) []NamedSchema {
	result := make([]NamedSchema, 0, len(schemas))
	for _, ts := range schemas {
		schemaMap := ts.GetJsonSchema().AsMap()
		if ts.GetJsonSchema() == nil {
			schemaMap = map[string]any{"error": "type has no JSON Schema"}
		}
		result = append(result, NamedSchema{
			Name:        ts.GetName(),
			Schema:      schemaMap,
			ArrayFields: ts.GetArrayFields(),
		})
	}
	return result
}

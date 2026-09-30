package mcp

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	"github.com/goccy/go-json"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
)

// IntrospectInput is the structured input for the introspect tool.
type IntrospectInput struct {
	ShardID      string `json:"shard_id"                jsonschema_description:"ID of the shard to introspect"`
	InstanceName string `json:"instance_name,omitempty" jsonschema_description:"Specific pool instance; accepts variants like 'game-5', 'game 5', 'game5', or '5'. Defaults to the shard's first instance."`
	Organization string `json:"organization,omitempty"  jsonschema_description:"Organization (auto-derived from the cluster's ShardPool for this shard if omitted)"`
	Project      string `json:"project,omitempty"       jsonschema_description:"Project (auto-derived from the cluster's ShardPool for this shard if omitted)"`
	ShardURL     string `json:"shard_url,omitempty"     jsonschema_description:"Cardinal shard API URL; auto-resolved (per instance) from the cluster when omitted. When set, the operator is not contacted: pair it with instance_name (the exact instance, e.g. 'game-2'). Also pass organization/project to skip the cluster lookup entirely; otherwise they're still auto-resolved via a cluster call."`
	OperatorURL  string `json:"operator_url,omitempty"  jsonschema_description:"cardinal-operator URL used to resolve instance_name (defaults to http://localhost:8090 for local dev)"`
}

// IntrospectOutput is the structured output for the introspect tool.
type IntrospectOutput struct {
	Commands   []NamedSchema `json:"commands"   jsonschema_description:"List of available commands with their schemas"`
	Components []NamedSchema `json:"components" jsonschema_description:"List of available components with their schemas"`
	Events     []NamedSchema `json:"events"     jsonschema_description:"List of available events with their schemas"`
}

// NamedSchema is one registered type (command, component, or event) with its schema — the type's
// protobuf message definition rendered as JSON.
type NamedSchema struct {
	Name   string         `json:"name"`
	Schema map[string]any `json:"schema"`
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
				"when the shard is redeployed with changed types: after a reload (or cluster start) that "+
				"added, removed, or changed a command, component, or event, call it again. "+
				"Requires a running cluster.",
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
		organization: args.Organization,
		project:      args.Project,
		shardURL:     args.ShardURL,
		operatorURL:  args.OperatorURL,
	})
	if err != nil {
		return IntrospectOutput{}, err
	}

	resp, files, err := introspectShardTypes(ctx, client)
	if err != nil {
		return IntrospectOutput{}, eris.Wrapf(err, "failed to introspect shard %q", strings.TrimSpace(args.ShardID))
	}

	return IntrospectOutput{
		Commands:   convertTypeSchemas(resp.GetCommands(), files),
		Components: convertTypeSchemas(resp.GetComponents(), files),
		Events:     convertTypeSchemas(resp.GetEvents(), files),
	}, nil
}

// introspectShardTypes fetches a shard's introspect response and resolves its proto descriptor
// set. A world with no registered types has no descriptor set; it resolves to nil files, which is
// safe because there are then no types to look up. Callers wrap the error with their own context.
func introspectShardTypes(
	ctx context.Context,
	client cardinalv1connect.DebugServiceClient,
) (*cardinalv1.IntrospectResponse, *protoregistry.Files, error) {
	res, err := client.Introspect(ctx, connect.NewRequest(&cardinalv1.IntrospectRequest{}))
	if err != nil {
		return nil, nil, err
	}
	resp := res.Msg
	if len(resp.GetCommands())+len(resp.GetComponents())+len(resp.GetEvents()) == 0 {
		return resp, nil, nil
	}
	files, err := resolveDescriptorFiles(resp.GetProtoDescriptorSet())
	if err != nil {
		return nil, nil, err
	}
	return resp, files, nil
}

// fetchCommandDescriptor introspects the shard and resolves the named command's message descriptor
// from the response's proto descriptor set, used to encode its payload as proto wire. DebugService
// is unauthenticated in dev.
func fetchCommandDescriptor(
	ctx context.Context,
	shardURL, commandName string,
) (protoreflect.MessageDescriptor, error) {
	resp, files, err := introspectShardTypes(ctx, newDebugServiceClient(shardURL))
	if err != nil {
		return nil, eris.Wrap(err, "failed to introspect shard for command schema")
	}

	for _, ts := range resp.GetCommands() {
		if ts.GetName() == commandName {
			return findMessageDescriptor(files, ts.GetProtoMessageName())
		}
	}
	return nil, eris.Errorf("command %q not found on shard", commandName)
}

// componentDescriptors maps a component's registered name to its protobuf message descriptor.
type componentDescriptors map[string]protoreflect.MessageDescriptor

// fetchComponentDescriptors introspects the shard and resolves every registered component's message
// descriptor, so get_state can decode the snapshot's component blobs. A component whose descriptor
// is unresolvable is left out — its blob stays raw rather than costing every other component.
func fetchComponentDescriptors(
	ctx context.Context,
	client cardinalv1connect.DebugServiceClient,
) (componentDescriptors, error) {
	resp, files, err := introspectShardTypes(ctx, client)
	if err != nil {
		return nil, eris.Wrap(err, "failed to introspect shard for component schemas")
	}

	descriptors := make(componentDescriptors, len(resp.GetComponents()))
	for _, ts := range resp.GetComponents() {
		md, err := findMessageDescriptor(files, ts.GetProtoMessageName())
		if err != nil {
			continue
		}
		descriptors[ts.GetName()] = md
	}
	return descriptors, nil
}

// convertTypeSchemas converts proto TypeSchema slices to the MCP output shape. Each type's schema
// is its message descriptor rendered as JSON (protodesc → protojson), i.e. the shard's generated
// protobuf definition verbatim — fields with their names, numbers, and types. A type whose
// descriptor cannot be resolved is still listed, with the failure under an "error" key instead of
// failing the whole introspect.
func convertTypeSchemas(schemas []*cardinalv1.TypeSchema, files *protoregistry.Files) []NamedSchema {
	result := make([]NamedSchema, 0, len(schemas))
	for _, ts := range schemas {
		schemaMap, err := descriptorSchema(files, ts.GetProtoMessageName())
		if err != nil {
			schemaMap = map[string]any{"error": err.Error()}
		}
		result = append(result, NamedSchema{Name: ts.GetName(), Schema: schemaMap})
	}
	return result
}

// descriptorSchema renders a message's descriptor as a JSON map, straight from the descriptor set.
func descriptorSchema(files *protoregistry.Files, protoMessageName string) (map[string]any, error) {
	md, err := findMessageDescriptor(files, protoMessageName)
	if err != nil {
		return nil, err
	}
	jsonBytes, err := protojson.Marshal(protodesc.ToDescriptorProto(md))
	if err != nil {
		return nil, eris.Wrapf(err, "failed to render descriptor for %q", protoMessageName)
	}
	var schemaMap map[string]any
	if err := json.Unmarshal(jsonBytes, &schemaMap); err != nil {
		return nil, eris.Wrapf(err, "failed to decode rendered descriptor for %q", protoMessageName)
	}
	return schemaMap, nil
}

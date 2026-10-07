package mcp

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"connectrpc.com/connect"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rotisserie/eris"

	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"

	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"

	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
)

// SendCommandInput is the structured input for the send_command tool.
type SendCommandInput struct {
	ShardID      string         `json:"shard_id"                jsonschema_description:"ID of the shard to send the command to (a shard running on local Docker)"`
	InstanceName string         `json:"instance_name,omitempty" jsonschema_description:"Specific pool instance to target (e.g., 'game-2', 'game 2', '2'); defaults to the shard's first instance. Each instance is a distinct container with its own state."`
	Organization string         `json:"organization,omitempty"  jsonschema_description:"Organization (auto-derived from the project's releases when omitted"`
	Project      string         `json:"project,omitempty"       jsonschema_description:"Project (auto-derived from the project's releases when omitted"`
	CommandName  string         `json:"command_name"            jsonschema_description:"Name of the command to execute (e.g., 'create-player', 'player-attack')"`
	Payload      map[string]any `json:"payload"                 jsonschema_description:"JSON payload for the command (the command's input data)"`
	ShardURL     string         `json:"shard_url,omitempty"     jsonschema_description:"Cardinal shard API URL; auto-resolved (per instance) from the running containers when omitted. When set, Docker is not contacted: pair it with instance_name (the exact instance, e.g. 'game-2') when targeting a non-default instance so the request address matches the shard. Also pass organization/project to skip the container lookup entirely; otherwise they're still auto-resolved from the containers."`
	PlayerID     string         `json:"player_id,omitempty"     jsonschema_description:"Player ID for dev auth (defaults to mcp-dev-player)"`
	Region       string         `json:"region,omitempty"        jsonschema_description:"Service address region (defaults to us-west1 for local dev)"`
}

// SendCommandOutput is the structured output for the send_command tool.
type SendCommandOutput struct {
	ShardID      string `json:"shard_id"`
	InstanceName string `json:"instance_name"     jsonschema_description:"The pool instance (container) this command was routed to"`
	CommandName  string `json:"command_name"`
	Message      string `json:"message,omitempty"`
}

// registerSendCommandTool registers the send_command tool.
// It sends a command directly to a specific Cardinal shard.
func registerSendCommandTool(srv *server.MCPServer) {
	sendCommandTool := mcp.NewTool(
		"send_command",
		mcp.WithDescription(
			"Send a command to a Cardinal shard. Commands are mutations that trigger game logic (e.g., "+
				"create-player, player-attack). Requires a running world. NOTE: Command names must match "+
				"the shard's registered names (e.g., 'create-player' not 'CreatePlayer'). Call the "+
				"introspect tool first to list the commands and their payload schemas — it reflects what "+
				"the running shard actually accepts, which grepping the source does not (unwired or "+
				"not-yet-deployed commands). One introspect per session is enough; its results only go "+
				"stale when a reload changes the shard's registered types.",
		),
		mcp.WithInputSchema[SendCommandInput](),
		mcp.WithOutputSchema[SendCommandOutput](),
	)
	srv.AddTool(sendCommandTool, strictToolHandler(sendCommandHandler))
}

// sendCommandHandler sends a command directly to the specified Cardinal shard.
func sendCommandHandler(
	ctx context.Context,
	_ mcp.CallToolRequest,
	args SendCommandInput,
) (SendCommandOutput, error) {
	if err := args.validate(); err != nil {
		return SendCommandOutput{}, eris.Wrap(err, "failed to validate send command input")
	}
	ctx, cancel := ensureDeadline(ctx, defaultCommandTimeout)
	defer cancel()

	// Resolve the shard's organization/project from the container labels (no world.toml).
	org, project, err := resolveShardWorld(ctx, args.ShardID, args.Organization, args.Project)
	if err != nil {
		return SendCommandOutput{}, err
	}

	// Resolve how to reach the target pool instance (a specific container) and the
	// address the shard validates against. Empty instance_name resolves to the
	// shard's first instance, preserving prior behavior; each instance has its own
	// edge route and its own ECS state.
	target, err := resolveShardTarget(
		ctx, args.ShardID, args.InstanceName, args.ShardURL, org, project, args.Region,
	)
	if err != nil {
		return SendCommandOutput{}, err
	}

	// Commands are decoded from proto wire, so encode the payload against the command's message descriptor.
	commandDesc, err := fetchCommandDescriptor(ctx, target.shardURL, args.CommandName)
	if err != nil {
		return SendCommandOutput{}, err
	}
	payloadBytes, err := encodeCommandPayload(commandDesc, args.Payload)
	if err != nil {
		return SendCommandOutput{}, eris.Wrapf(err, "failed to encode payload for command %q", args.CommandName)
	}

	// Refuse rather than silently drop the token: a caller that pinned shard_url at a
	// remote host and exported WORLD_ARGUS_TOKEN would otherwise get a puzzling 401, and
	// the token must not travel there either way.
	if os.Getenv(argusTokenEnv) != "" && !isLocalShardURL(target.shardURL) {
		return SendCommandOutput{}, eris.Errorf(
			"refusing to send %s to %q: the token is only used for worlds on this machine",
			argusTokenEnv, target.shardURL)
	}

	client := cardinalv1connect.NewCardinalServiceClient(
		&http.Client{Timeout: defaultCommandTimeout},
		target.shardURL,
		connect.WithInterceptors(&devAuthInterceptor{playerID: args.PlayerID, target: target.shardURL}),
	)

	req := connect.NewRequest(&cardinalv1.SendCommandRequest{
		Command: &iscv1.Command{
			Name:    args.CommandName,
			Address: target.address,
			Payload: payloadBytes,
		},
	})

	_, err = client.SendCommand(ctx, req)
	if err != nil {
		return SendCommandOutput{}, eris.Wrapf(
			authHint(err),
			"failed to send command %q to shard instance %q",
			args.CommandName,
			target.instanceName,
		)
	}

	return SendCommandOutput{
		ShardID:      args.ShardID,
		InstanceName: target.instanceName,
		CommandName:  args.CommandName,
		Message: fmt.Sprintf(
			"Successfully sent %s command to shard instance %s",
			args.CommandName,
			target.instanceName,
		),
	}, nil
}

func (s *SendCommandInput) validate() error {
	shardID, err := requireShardID(s.ShardID)
	if err != nil {
		return err
	}
	s.ShardID = shardID
	if s.CommandName == "" {
		return eris.New("command_name is required")
	}
	// Use defaults for optional fields (ShardURL is resolved in the handler).
	if s.PlayerID == "" {
		s.PlayerID = defaultDevPlayerID
	}
	if s.Region == "" {
		s.Region = defaultRegion
	}
	if s.Payload == nil {
		s.Payload = make(map[string]any)
	}
	return nil
}

// authHint explains an argus world's rejection: the dev header it would normally send
// is not a credential there, so say what to do instead of leaving a bare 401.
func authHint(err error) error {
	if connect.CodeOf(err) != connect.CodeUnauthenticated || os.Getenv(argusTokenEnv) != "" {
		return err
	}
	return eris.Wrapf(
		err,
		"this world authenticates players with Argus Auth; export %s=<token> or set [auth] mode = %q in world.toml and start the world again",
		argusTokenEnv,
		worldtoml.AuthModeDev,
	)
}

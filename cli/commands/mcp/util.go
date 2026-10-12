package mcp

import (
	"context"
	"log/slog"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/pkg/cluster"
	"github.com/argus-labs/world-engine/cli/pkg/docker"
	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"

	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
)

// -------------------------------------------------------------------------------------------------
// Cardinal shard client utilities (shared by every tool that dials a shard RPC)
// -------------------------------------------------------------------------------------------------

const (
	// defaultDevPlayerID is the default player ID used for dev auth.
	defaultDevPlayerID = "mcp-dev-player"
	// defaultRegion is the region local shards register with (CARDINAL_REGION);
	// it must match or a command reaches no responders.
	defaultRegion = cluster.DefaultRegion
	// defaultCommandTimeout is the default timeout for Cardinal RPC requests.
	defaultCommandTimeout = 30 * time.Second
)

// devAuthInterceptor implements connect.Interceptor to add the X-Player-Id header to all requests.
//
// Cardinal's dev auth middleware (AuthModeDev) requires this header to authenticate requests in
// development mode. The header value is the player ID for the request.
type devAuthInterceptor struct {
	playerID string
}

// WrapUnary injects the X-Player-Id header on unary RPCs (SendCommand) — the
// only interceptor path the MCP tools use.
func (i *devAuthInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		req.Header().Set("X-Player-Id", i.playerID)
		return next(ctx, req)
	}
}

// WrapStreamingClient injects the X-Player-Id header on streaming clients. Required
// by connect.Interceptor; the MCP tools make no streaming calls.
func (i *devAuthInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		conn.RequestHeader().Set("X-Player-Id", i.playerID)
		return conn
	}
}

// WrapStreamingHandler is a pass-through required by connect.Interceptor; unused
// (the MCP tools are a client, not a server).
func (i *devAuthInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return next(ctx, conn)
	}
}

// shardTarget is the resolved addressing for one shard RPC: the instance the
// call routes to, the base URL to reach it, and the ServiceAddress the shard
// validates the request against.
type shardTarget struct {
	instanceName string
	shardURL     string
	address      *microv1.ServiceAddress
}

// resolveShardTarget resolves how to reach a shard instance and the
// region.realm.org.project.serviceId address Cardinal validates the request
// against (serviceId is always the instance name, never the pool id). A
// pinned shardURL skips the operator but not org/project resolution; pair it
// with the matching instance_name or the shard rejects the address.
func resolveShardTarget(
	ctx context.Context,
	operatorURL, shardID, instanceName, shardURL, org, project, region string,
) (shardTarget, error) {
	instanceName = strings.TrimSpace(instanceName)

	if shardURL != "" {
		// Explicit URL: trust the caller's instance_name (or the first instance);
		// don't round-trip the operator to canonicalize it.
		if instanceName == "" {
			instanceName = shardID
		}
	} else {
		resolved, err := resolveInstanceName(ctx, operatorURL, shardID, instanceName)
		if err != nil {
			return shardTarget{}, err
		}
		instanceName = resolved
		shardURL = cluster.LocalShardAPIURL(org, project, instanceName)
	}

	return shardTarget{
		instanceName: instanceName,
		shardURL:     shardURL,
		address: &microv1.ServiceAddress{
			Region:       region,
			Realm:        microv1.ServiceAddress_REALM_WORLD,
			Organization: org,
			Project:      project,
			ServiceId:    instanceName,
		},
	}, nil
}

// -------------------------------------------------------------------------------------------------
// General utilities
// -------------------------------------------------------------------------------------------------

// ensureDeadline returns a context with a default timeout if no deadline is already set.
// The caller must defer the returned cancel function.
func ensureDeadline(ctx context.Context, fallback time.Duration) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, fallback)
}

// -------------------------------------------------------------------------------------------------
// DebugService utilities (shared by the introspect and get_state tools)
// -------------------------------------------------------------------------------------------------

// debugTargetArgs is the addressing subset shared by every tool that dials a
// shard's DebugService. It mirrors the identically-named fields on those tools'
// input structs, which stay flat so their generated JSON schemas keep one level.
type debugTargetArgs struct {
	shardID      string
	instanceName string
	organization string
	project      string
	shardURL     string
	operatorURL  string
}

// newDebugServiceClient builds a DebugService client for an already-resolved
// shard URL. DebugService is unauthenticated in dev, so no dev-auth sign-in.
func newDebugServiceClient(shardURL string) cardinalv1connect.DebugServiceClient {
	return cardinalv1connect.NewDebugServiceClient(
		&http.Client{Timeout: defaultCommandTimeout},
		shardURL,
	)
}

// dialShardDebugService validates the shard id, resolves the shard's
// organization/project from the cluster and then the target pool instance's
// Traefik URL, and returns a DebugService client aimed at it plus the resolved
// target, so callers can report which instance actually answered.
func dialShardDebugService(
	ctx context.Context,
	args debugTargetArgs,
) (cardinalv1connect.DebugServiceClient, shardTarget, error) {
	shardID, err := requireShardID(args.shardID)
	if err != nil {
		return nil, shardTarget{}, err
	}

	org, project, err := resolveShardWorld(ctx, shardID, args.organization, args.project)
	if err != nil {
		return nil, shardTarget{}, err
	}

	target, err := resolveShardTarget(
		ctx, args.operatorURL, shardID, args.instanceName, args.shardURL, org, project, defaultRegion,
	)
	if err != nil {
		return nil, shardTarget{}, err
	}

	return newDebugServiceClient(target.shardURL), target, nil
}

// -------------------------------------------------------------------------------------------------
// Tool handler wrapper
// -------------------------------------------------------------------------------------------------

// strictToolHandler wraps a structured handler so an argument the tool doesn't
// declare is rejected rather than dropped. encoding/json ignores unknown fields,
// which silently turns a misspelled parameter (e.g. "tail" for "tail_lines")
// into a fallback to the default — the caller gets a plausible answer to a
// question it didn't ask.
func strictToolHandler[TArgs any, TResult any](
	handler mcp.StructuredToolHandlerFunc[TArgs, TResult],
) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	known := declaredArgNames[TArgs]()
	inner := mcp.NewStructuredToolHandler(handler)

	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		for name := range req.GetArguments() {
			if _, ok := known[name]; !ok {
				// A tool-error result, not a transport error: the caller can read
				// it and retry with a corrected name, same as any other tool failure.
				return mcp.NewToolResultErrorf(
					"unknown parameter %q; this tool accepts: %s",
					name, strings.Join(slices.Sorted(maps.Keys(known)), ", "),
				), nil
			}
		}
		return inner(ctx, req)
	}
}

// declaredArgNames returns the JSON names of an argument struct's exported
// fields — the same names the tool publishes in its input schema.
func declaredArgNames[T any]() map[string]struct{} {
	names := make(map[string]struct{})

	t := reflect.TypeFor[T]()
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return names
	}

	for field := range t.Fields() {
		if !field.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "" {
			name = field.Name
		}
		if name != "-" {
			names[name] = struct{}{}
		}
	}
	return names
}

// -------------------------------------------------------------------------------------------------
// World build + (re)deploy lifecycle helpers (shared by tools that compile local
// shard source and push it to the cluster: reload and cluster).
// -------------------------------------------------------------------------------------------------

// buildWorldShards builds the world's Cardinal shard images via the local Docker
// daemon and returns the world config plus the operator Deploy payload (project +
// built image refs). An empty shardID builds every shard. It touches only Docker,
// never the cluster, so it can run BEFORE any cluster mutation — which is what
// lets reload compile before it touches the running world.
func buildWorldShards(
	ctx context.Context,
	worldPath, shardID string,
) (worldtoml.Config, cluster.DeployOpts, error) {
	var (
		cfg        worldtoml.Config
		deployOpts cluster.DeployOpts
	)
	err := docker.WithClient(worldPath, false, &docker.ClientOptions{Logger: slog.Default()},
		func(sc *service.Config, dockerClient *docker.Client) error {
			shardServices := dockerClient.ResolveServices(service.BuildCardinalShards(sc)...)
			if shardID != "" {
				shardServices = filterShardServices(shardServices, sc, shardID)
				if len(shardServices) == 0 {
					return eris.Errorf("shard %q is not defined in world.toml", shardID)
				}
			}

			// Pull build dependencies (golang + runtime base) before building, or the
			// first build on a fresh machine fails with BuildKit "no active sessions".
			if err := dockerClient.PullImages(ctx, shardServices, nil); err != nil {
				return eris.Wrap(err, "failed to pull build dependencies")
			}
			if err := dockerClient.BuildCardinalImages(ctx, shardServices, nil); err != nil {
				return eris.Wrap(err, "failed to build Cardinal images")
			}

			cfg = sc.WorldToml
			deployOpts = cluster.DeployOpts{
				Project: sc.WorldToml.Project,
				Shards:  deployShardsFromConfig(sc, shardID),
			}
			return nil
		},
	)
	if err != nil {
		return worldtoml.Config{}, cluster.DeployOpts{}, err
	}
	return cfg, deployOpts, nil
}

// deployShardsFromConfig builds the operator Deploy payload: one entry per logical
// shard ID (deduped — pools expand to multiple entries), each pointing at the
// locally-built image tag. An empty shardID includes every shard.
func deployShardsFromConfig(cfg *service.Config, shardID string) []cluster.DeployShard {
	seen := make(map[string]struct{}, len(cfg.WorldToml.Shards))
	shards := make([]cluster.DeployShard, 0, len(cfg.WorldToml.Shards))
	for _, s := range cfg.WorldToml.Shards {
		if shardID != "" && s.ID != shardID {
			continue
		}
		if _, ok := seen[s.ID]; ok {
			continue
		}
		seen[s.ID] = struct{}{}
		shards = append(shards, cluster.DeployShard{
			ID:          s.ID,
			SourceImage: service.CardinalShardImageName(cfg.Namespace, s.ID) + ":latest",
		})
	}
	return shards
}

// filterShardServices narrows the built services to one shard's image, deduped —
// a pool's replicas share a single image, so it is built once.
func filterShardServices(services []service.Service, cfg *service.Config, shardID string) []service.Service {
	image := service.CardinalShardImageName(cfg.Namespace, shardID)
	filtered := make([]service.Service, 0, 1)
	for _, s := range services {
		if s.Image != image {
			continue
		}
		if !slices.ContainsFunc(filtered, func(x service.Service) bool { return x.Image == s.Image }) {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// deployedShardIDs lists the shard IDs in a Deploy payload, for reporting what a
// reload or start actually rolled out.
func deployedShardIDs(opts cluster.DeployOpts) []string {
	ids := make([]string, len(opts.Shards))
	for i, s := range opts.Shards {
		ids[i] = s.ID
	}
	return ids
}

// requireShardID trims a shard id and rejects an empty one with the uniform error
// every shard-scoped tool shares.
func requireShardID(raw string) (string, error) {
	shardID := strings.TrimSpace(raw)
	if shardID == "" {
		return "", eris.New("shard_id is required")
	}
	return shardID, nil
}

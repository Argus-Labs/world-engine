package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/docker"
	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// deadDockerHost points the moby client at a socket that cannot exist, so any
// container listing fails on the daemon connection rather than accidentally
// reaching a world the developer has running. Mirrors the pattern in
// TestResolveInstanceName_UnknownReturnsError.
func deadDockerHost(t *testing.T) string {
	t.Helper()
	return "unix://" + filepath.Join(t.TempDir(), "nonexistent.sock")
}

// -------------------------------------------------------------------------------------------------
// shardsFromContainers: the label-reading heart of project auto-derivation
// -------------------------------------------------------------------------------------------------

// TestShardsFromContainers_DerivesProjectFromLabels is the core of the fix:
// running shard containers yield one deployedShard per pool, with the project
// (and organization) read straight from the container labels — not known ahead
// of time. Stopped shards and non-shard containers are skipped, and a pool's
// replicas are deduped by shard id.
func TestShardsFromContainers_DerivesProjectFromLabels(t *testing.T) {
	t.Parallel()
	cs := []docker.ContainerSummary{
		// running shard of project "demo" (first replica carries the org label)
		{Name: "demo-game-1", State: docker.ContainerRunning, Labels: map[string]string{
			service.RoleLabel: service.RoleShard, service.ShardIDLabel: "game",
			service.ProjectLabel: "demo", service.OrgLabel: "argus", service.InstanceLabel: "game",
		}},
		// second replica of the same pool — deduped by shard id
		{Name: "demo-game-2", State: docker.ContainerRunning, Labels: map[string]string{
			service.RoleLabel: service.RoleShard, service.ShardIDLabel: "game",
			service.ProjectLabel: "demo", service.InstanceLabel: "game-2",
		}},
		// a stopped shard is not somewhere to send an RPC, so it is skipped
		{Name: "demo-chat-1", State: "exited", Labels: map[string]string{
			service.RoleLabel: service.RoleShard, service.ShardIDLabel: "chat",
			service.ProjectLabel: "demo",
		}},
		// a non-shard container (NATS) of the project is ignored
		{Name: "demo-nats", State: docker.ContainerRunning, Labels: map[string]string{
			service.RoleLabel: service.RoleNATS, service.ProjectLabel: "demo",
		}},
		// a running shard of a *different* project is still listed
		{Name: "other-lobby-1", State: docker.ContainerRunning, Labels: map[string]string{
			service.RoleLabel: service.RoleShard, service.ShardIDLabel: "lobby",
			service.ProjectLabel: "other", service.OrgLabel: "other-org",
		}},
	}

	shards := shardsFromContainers(cs)
	require.Len(t, shards, 2)
	assert.Equal(t, deployedShard{ShardID: "game", Organization: "argus", Project: "demo"}, shards[0])
	assert.Equal(t, deployedShard{ShardID: "lobby", Organization: "other-org", Project: "other"}, shards[1])
}

func TestShardsFromContainers_Empty(t *testing.T) {
	t.Parallel()
	assert.Empty(t, shardsFromContainers(nil))
	// Nothing running → nothing to send an RPC to.
	assert.Empty(t, shardsFromContainers([]docker.ContainerSummary{{
		State:  "exited",
		Labels: map[string]string{service.RoleLabel: service.RoleShard, service.ShardIDLabel: "game"},
	}}))
}

// -------------------------------------------------------------------------------------------------
// resolveShardWorld: the gating fix
// -------------------------------------------------------------------------------------------------

// An explicit project short-circuits the lookup, so the call returns even when
// the Docker daemon is unreachable.
func TestResolveShardWorld_ExplicitProjectShortCircuits(t *testing.T) {
	t.Setenv("DOCKER_HOST", deadDockerHost(t))
	got, err := resolveShardWorld(context.Background(), "game", "rampage")
	require.NoError(t, err)
	assert.Equal(t, "rampage", got)
}

// Omitting project must not require a world.toml in the working directory. The
// schema advertises auto-derivation from the running containers, so the handler
// lists containers (Docker) and never reads world.toml. With the daemon
// unreachable the call fails on the container listing, not on a missing
// world.toml — the bug returned "project not given and no world.toml in the
// working directory".
//
// Runs from an empty temp dir (no world.toml) so a regression that re-introduces
// the world.toml gate fails here: the old path would read the cwd's (absent)
// world.toml and the error would mention it.
func TestResolveShardWorld_OmittedProjectDoesNotRequireWorldToml(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("DOCKER_HOST", deadDockerHost(t))

	_, err := resolveShardWorld(context.Background(), "game", "")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "world.toml",
		"omitted project must derive from the running containers, not the working directory's world.toml")
	assert.Contains(t, err.Error(), "list shard containers",
		"omitted project reaches the cross-project container listing (Docker), not the world.toml gate")
}

// Even when a world.toml IS present in the working directory, omitting project
// scopes the lookup across every project's containers, not just the cwd
// project's. The container listing is the cross-project one (ListShardContainers,
// "list shard containers"), never the project-scoped one (ListProjectContainers,
// "list containers for project <cwd>"), so a running shard of a different project
// is still found.
func TestResolveShardWorld_OmittedProjectNotScopedToWorkingDirProject(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, worldtoml.FileName),
		[]byte("project = \"cwdproj\"\n"), 0o644))
	t.Chdir(dir)
	t.Setenv("DOCKER_HOST", deadDockerHost(t))

	_, err := resolveShardWorld(context.Background(), "game", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list shard containers",
		"omitted project lists shard containers across every project")
	assert.NotContains(t, err.Error(), "list containers for project",
		"omitted project must not scope the listing to the working directory's project")
	assert.NotContains(t, err.Error(), "cwdproj",
		"the cwd project from world.toml must not name the listing")
}

// -------------------------------------------------------------------------------------------------
// published schemas: the auto-derive description the handler now honors
// -------------------------------------------------------------------------------------------------

// The shard-targeting tools publish that project is auto-derived from the running
// containers when omitted. The handler now honors that (cross-project container
// listing, no world.toml), so the schema must keep advertising it — this pins the
// description against a future tag-only regression that would silently hide the
// feature from the LLM driving the tool.
func TestShardTools_PublishProjectAutoDeriveDescription(t *testing.T) {
	t.Parallel()
	srv := server.NewMCPServer("test", "0")
	registerSendCommandTool(srv)
	registerGetStateTool(srv)
	registerIntrospectTool(srv)

	for _, name := range []string{"send_command", "get_state", "introspect"} {
		tool := srv.GetTool(name)
		require.NotNil(t, tool, name)
		require.NotEmpty(t, tool.Tool.RawInputSchema, "%s: input schema must be generated", name)

		var schema struct {
			Properties map[string]struct {
				Description string `json:"description"`
			} `json:"properties"`
		}
		require.NoError(t, json.Unmarshal(tool.Tool.RawInputSchema, &schema), name)
		prop, ok := schema.Properties["project"]
		require.True(t, ok, "%s: project must appear in the published schema", name)
		assert.Contains(t, prop.Description, "auto-derived from the running containers",
			"%s: project description must advertise auto-derivation", name)
	}
}

// describe_world kept the honest "defaults to the world.toml in the working
// directory" wording (its handler resolves the project through the world.toml
// gate). The fix must not change that contract, so its schema still says so.
func TestDescribeWorld_KeepsWorldTomlFallbackDescription(t *testing.T) {
	t.Parallel()
	srv := server.NewMCPServer("test", "0")
	registerDescribeWorldTool(srv)

	tool := srv.GetTool("describe_world")
	require.NotNil(t, tool)
	require.NotEmpty(t, tool.Tool.RawInputSchema)

	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(tool.Tool.RawInputSchema, &schema))
	prop, ok := schema.Properties["project"]
	require.True(t, ok)
	assert.Contains(t, prop.Description, "world.toml in the working directory",
		"describe_world still falls back to the working directory's world.toml and must document it")
}

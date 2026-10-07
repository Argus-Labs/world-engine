package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guumaster/logsymbols"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/descriptorpb"

	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
	"github.com/argus-labs/world-engine/cli/pkg/worldstatus"
)

func TestBuildMCPServer_RegistersAllTools(t *testing.T) {
	t.Parallel()
	srv := NewServer()
	require.NotNil(t, srv)
}

// An empty instance_name resolves to the shard's first instance without an
// cluster round-trip, so send_command keeps working exactly as before for
// callers that don't target a specific pod.
func TestResolveInstanceName_EmptyDefaultsToShardID(t *testing.T) {
	t.Parallel()
	got, err := resolveInstanceName(context.Background(), "", "game", "")
	require.NoError(t, err)
	assert.Equal(t, "game", got)
}

func TestPingHandler(t *testing.T) {
	t.Parallel()
	result, err := pingHandler(context.Background(), mcp.CallToolRequest{})
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Len(t, result.Content, 1)
	textContent, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Equal(t, "cardinal-mcp is running", textContent.Text)
}

// -------------------------------------------------------------------------------------------------
// SendCommandInput validation tests
// -------------------------------------------------------------------------------------------------

func TestSendCommandInput_Validate_Valid(t *testing.T) {
	t.Parallel()
	input := SendCommandInput{
		ShardID:     "game",
		CommandName: "create-player",
		Payload:     map[string]any{"name": "test"},
	}

	err := input.validate()
	require.NoError(t, err)

	assert.Empty(t, input.ShardURL) // resolved from the cluster in the handler, not validate()
	assert.Equal(t, defaultDevPlayerID, input.PlayerID)
	assert.Equal(t, defaultRegion, input.Region)
	assert.NotNil(t, input.Payload)
}

func TestSendCommandInput_Validate_MissingShardID(t *testing.T) {
	t.Parallel()
	input := SendCommandInput{CommandName: "create-player"}

	err := input.validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "shard_id is required")
}

func TestSendCommandInput_Validate_MissingCommandName(t *testing.T) {
	t.Parallel()
	input := SendCommandInput{ShardID: "game"}

	err := input.validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command_name is required")
}

func TestSendCommandInput_Validate_NilPayload(t *testing.T) {
	t.Parallel()
	input := SendCommandInput{
		ShardID:     "game",
		CommandName: "create-player",
		Payload:     nil,
	}

	err := input.validate()
	require.NoError(t, err)
	assert.NotNil(t, input.Payload)
}

func TestSendCommandInput_Validate_CustomDefaults(t *testing.T) {
	t.Parallel()
	input := SendCommandInput{
		ShardID:     "game",
		CommandName: "create-player",
		ShardURL:    "http://custom:9999",
		PlayerID:    "custom-player",
		Region:      "ap-southeast-1",
	}

	err := input.validate()
	require.NoError(t, err)

	assert.Equal(t, "http://custom:9999", input.ShardURL)
	assert.Equal(t, "custom-player", input.PlayerID)
	assert.Equal(t, "ap-southeast-1", input.Region)
}

// -------------------------------------------------------------------------------------------------
// ensureDeadline tests
// -------------------------------------------------------------------------------------------------

func TestEnsureDeadline_AppliesFallbackWhenNoDeadline(t *testing.T) {
	t.Parallel()
	fallback := 5 * time.Second

	newCtx, cancel := ensureDeadline(context.Background(), fallback)
	defer cancel()

	deadline, ok := newCtx.Deadline()
	require.True(t, ok, "expected deadline to be set")
	assert.WithinDuration(t, time.Now().Add(fallback), deadline, 1*time.Second)
}

func TestEnsureDeadline_PreservesExistingDeadline(t *testing.T) {
	t.Parallel()
	ctx, existingCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer existingCancel()

	expectedDeadline, _ := ctx.Deadline()

	newCtx, cancel := ensureDeadline(ctx, 30*time.Second)
	defer cancel()

	deadline, ok := newCtx.Deadline()
	require.True(t, ok)
	assert.Equal(t, expectedDeadline, deadline, "existing deadline should not be overwritten")
}

func TestEnsureDeadline_ReturnedCancelIsNoOpWhenDeadlineExists(t *testing.T) {
	t.Parallel()
	ctx, existingCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer existingCancel()

	newCtx, cancel := ensureDeadline(ctx, 30*time.Second)
	cancel() // should be a no-op

	select {
	case <-newCtx.Done():
		t.Fatal("cancel from ensureDeadline should not cancel context when deadline already existed")
	default:
	}
}

// -------------------------------------------------------------------------------------------------
// k8s helper tests (cluster status → MCP shape)
// -------------------------------------------------------------------------------------------------

func TestClampTail(t *testing.T) {
	t.Parallel()
	assert.Equal(t, defaultLogTailLines, clampTail(0))
	assert.Equal(t, defaultLogTailLines, clampTail(-5))
	assert.Equal(t, int32(500), clampTail(500))
	assert.Equal(t, maxLogTailLines, clampTail(999999))
}

func TestToShardPools(t *testing.T) {
	t.Parallel()
	status := []worldstatus.PoolStatus{{
		ShardID:  "gameplay",
		PoolSize: 2,
		Image:    "rampage-gameplay-shard",
		Phase:    "Running",
		Instances: []worldstatus.InstanceStatus{
			{
				Name:         "gameplay",
				Runtime:      "cardinal-gameplay-pool-0",
				Phase:        "Running",
				Ready:        true,
				RestartCount: 1,
				Age:          "5m",
			},
			{Name: "gameplay-2", Runtime: "cardinal-gameplay-pool-1", Phase: "Pending", Ready: false},
		},
	}}

	pools := toShardPools(status)
	require.Len(t, pools, 1)
	assert.Equal(t, "gameplay", pools[0].ShardID)
	assert.Equal(t, int32(2), pools[0].PoolSize)
	require.Len(t, pools[0].Instances, 2)
	assert.Equal(t, "cardinal-gameplay-pool-0", pools[0].Instances[0].Container)
	assert.True(t, pools[0].Instances[0].Ready)
	assert.Equal(t, int32(1), pools[0].Instances[0].RestartCount)
	assert.False(t, pools[0].Instances[1].Ready)
}

func TestShardContainers(t *testing.T) {
	t.Parallel()
	status := []worldstatus.PoolStatus{
		{ShardID: "gameplay", Instances: []worldstatus.InstanceStatus{
			{Name: "gameplay", Runtime: "pod-a"},
			{Name: "gameplay-2", Runtime: "pod-b"},
		}},
		{ShardID: "lobby", Instances: []worldstatus.InstanceStatus{
			{Name: "lobby", Runtime: "pod-c"},
		}},
	}

	assert.ElementsMatch(t, []string{"pod-a", "pod-b"}, shardContainers(status, "gameplay", ""))
	assert.Equal(t, []string{"pod-b"}, shardContainers(status, "gameplay", "gameplay-2"))
	assert.Equal(t, []string{"pod-c"}, shardContainers(status, "lobby", ""))
	assert.Empty(t, shardContainers(status, "nope", ""))
}

func TestShardContainers_InstanceVariants(t *testing.T) {
	t.Parallel()
	status := []worldstatus.PoolStatus{{
		ShardID: "game",
		Instances: []worldstatus.InstanceStatus{
			{Name: "game", Runtime: "pod-1"},
			{Name: "game-5", Runtime: "pod-5"},
		},
	}}

	// All of these resolve to instance "game-5".
	for _, in := range []string{"game-5", "game 5", "game5", "GAME-5", "5"} {
		assert.Equal(t, []string{"pod-5"}, shardContainers(status, "game", in), "input %q", in)
	}
	// Index-1 references resolve to the bare first instance "game".
	for _, in := range []string{"game", "game 1", "game1", "1"} {
		assert.Equal(t, []string{"pod-1"}, shardContainers(status, "game", in), "input %q", in)
	}
	// No instance → every pod of the shard; unknown instance → none.
	assert.ElementsMatch(t, []string{"pod-1", "pod-5"}, shardContainers(status, "game", ""))
	assert.Empty(t, shardContainers(status, "game", "game-9"))
}

// TestShardContainers_ScopedToShardPool guards against an instance reference resolving
// into a different shard's pool: matching must be scoped to pool.ShardId, so a
// reference naming (or index-colliding with) another shard's instance yields no
// pod for the requested shard rather than silently routing to the wrong one.
func TestShardContainers_ScopedToShardPool(t *testing.T) {
	t.Parallel()
	status := []worldstatus.PoolStatus{
		{ShardID: "game", Instances: []worldstatus.InstanceStatus{
			{Name: "game", Runtime: "pod-game"},
		}},
		{ShardID: "lobby", Instances: []worldstatus.InstanceStatus{
			{Name: "lobby", Runtime: "pod-lobby"},
		}},
		{ShardID: "game2", Instances: []worldstatus.InstanceStatus{
			{Name: "game2", Runtime: "pod-game2"},
		}},
	}

	// A sibling shard's exact name must not match through the requested shard.
	assert.Empty(t, shardContainers(status, "game", "lobby"))
	// An index that collides with another shard's name ("2" ~ "game2") must not
	// leak: shard "game" is pool size 1, so it has no instance "2".
	assert.Empty(t, shardContainers(status, "game", "2"))
	// The requested shard still resolves its own instances.
	assert.Equal(t, []string{"pod-game"}, shardContainers(status, "game", "game"))
}

// TestResolveInstanceName_UnknownReturnsError confirms a non-empty reference is
// always resolved against the running containers, so an unknown one fails instead
// of being passed through. (The empty-reference path, covered by
// TestResolveInstanceName_EmptyDefaultsToShardID, never looks anything up.)
// Docker is pointed at a dead socket so the lookup cannot accidentally succeed
// against a world the developer happens to have running.
func TestResolveInstanceName_UnknownReturnsError(t *testing.T) {
	t.Setenv("DOCKER_HOST", "unix://"+filepath.Join(t.TempDir(), "nonexistent.sock"))
	_, err := resolveInstanceName(context.Background(), "demo", "game", "lobby")
	assert.Error(t, err)
}

// -------------------------------------------------------------------------------------------------
// get_state snapshot flattening and filter tests
// -------------------------------------------------------------------------------------------------

// noFilter is the filter produced by a get_state call with no find/match/where.
func noFilter(t *testing.T) stateFilter {
	t.Helper()
	f, err := compileStateFilter(GetStateInput{})
	require.NoError(t, err)
	return f
}

// stateDescriptors builds descriptors for the Health{HP} and Position{X} components the snapshot
// fixtures use, standing in for what a shard advertises through introspect.
func stateDescriptors(t *testing.T) componentDescriptors {
	t.Helper()
	raw := testDescriptorSet(t,
		&descriptorpb.DescriptorProto{Name: new("Health"), Field: []*descriptorpb.FieldDescriptorProto{
			scalarField("HP", 1, descriptorpb.FieldDescriptorProto_TYPE_INT64),
		}},
		&descriptorpb.DescriptorProto{Name: new("Position"), Field: []*descriptorpb.FieldDescriptorProto{
			scalarField("X", 1, descriptorpb.FieldDescriptorProto_TYPE_INT64),
		}},
	)
	return componentDescriptors{
		"Health":   testMessageDescriptor(t, raw, "Health"),
		"Position": testMessageDescriptor(t, raw, "Position"),
	}
}

// componentBlob encodes a component value as the protobuf wire bytes a snapshot payload stores.
func componentBlob(t *testing.T, descriptors componentDescriptors, name string, value int) []byte {
	t.Helper()
	field := map[string]string{"Health": "HP", "Position": "X"}[name]
	blob, err := encodeCommandPayload(descriptors[name], map[string]any{field: value})
	require.NoError(t, err)
	return blob
}

// sampleWorld has Health+Position entities (7, 9) and a Health-only entity (11).
func sampleWorld(t *testing.T, descriptors componentDescriptors) *cardinalv1.WorldState {
	t.Helper()
	blob := func(name string, value int) []byte { return componentBlob(t, descriptors, name, value) }
	return &cardinalv1.WorldState{
		NextId:     12,
		Components: []string{"Health", "Position"},
		Entities: []*cardinalv1.Entity{
			{Id: 7, Components: []uint32{0, 1}, Payloads: [][]byte{blob("Health", 10), blob("Position", 1)}},
			{Id: 9, Components: []uint32{0, 1}, Payloads: [][]byte{blob("Health", 80), blob("Position", 2)}},
			{Id: 11, Components: []uint32{0}, Payloads: [][]byte{blob("Health", 55)}},
		},
	}
}

func TestFlattenWorldState_FlattensEntities(t *testing.T) {
	t.Parallel()
	descriptors := stateDescriptors(t)
	got, err := flattenWorldState(sampleWorld(t, descriptors), -1, noFilter(t), descriptors)

	require.NoError(t, err)
	require.Len(t, got.entities, 3)
	assert.Equal(t, 3, got.total)
	assert.Equal(t, 3, got.matched)
	assert.Equal(t, uint32(7), got.entities[0].ID)
	assert.Equal(t, map[string]any{"HP": int64(10)}, got.entities[0].Components["Health"])
	assert.Equal(t, map[string]any{"X": int64(1)}, got.entities[0].Components["Position"])
}

func TestFlattenWorldState_LimitTruncatesButCountsStayWhole(t *testing.T) {
	t.Parallel()
	descriptors := stateDescriptors(t)
	got, err := flattenWorldState(sampleWorld(t, descriptors), 2, noFilter(t), descriptors)

	require.NoError(t, err)
	require.Len(t, got.entities, 2)
	// Counts describe the world and the match set, not the truncated page.
	assert.Equal(t, 3, got.total)
	assert.Equal(t, 3, got.matched)
}

func TestFlattenWorldState_EmptyWorld(t *testing.T) {
	t.Parallel()
	got, err := flattenWorldState(&cardinalv1.WorldState{}, 10, noFilter(t), stateDescriptors(t))

	require.NoError(t, err)
	assert.Empty(t, got.entities)
	assert.Equal(t, 0, got.total)
	assert.Equal(t, 0, got.matched)
}

func TestFlattenWorldState_FindContainsKeepsSupersetEntities(t *testing.T) {
	t.Parallel()
	descriptors := stateDescriptors(t)
	filter, err := compileStateFilter(GetStateInput{Find: []string{"Health"}})
	require.NoError(t, err)

	got, err := flattenWorldState(sampleWorld(t, descriptors), -1, filter, descriptors)

	require.NoError(t, err)
	// Both archetypes carry Health, so contains keeps all three entities.
	assert.Equal(t, 3, got.matched)
	assert.Equal(t, 3, got.total)
}

func TestFlattenWorldState_FindExactRejectsExtraComponents(t *testing.T) {
	t.Parallel()
	descriptors := stateDescriptors(t)
	filter, err := compileStateFilter(GetStateInput{Find: []string{"Health"}, Match: matchExact})
	require.NoError(t, err)

	got, err := flattenWorldState(sampleWorld(t, descriptors), -1, filter, descriptors)

	require.NoError(t, err)
	// Only the Health-only archetype qualifies; Health+Position has an extra.
	require.Len(t, got.entities, 1)
	assert.Equal(t, uint32(11), got.entities[0].ID)
	assert.Equal(t, 1, got.matched)
	assert.Equal(t, 3, got.total)
}

func TestFlattenWorldState_WhereFiltersOnComponentField(t *testing.T) {
	t.Parallel()
	descriptors := stateDescriptors(t)
	filter, err := compileStateFilter(GetStateInput{Where: "Health.HP > 50"})
	require.NoError(t, err)

	got, err := flattenWorldState(sampleWorld(t, descriptors), -1, filter, descriptors)

	require.NoError(t, err)
	require.Len(t, got.entities, 2)
	assert.Equal(t, uint32(9), got.entities[0].ID)
	assert.Equal(t, uint32(11), got.entities[1].ID)
	assert.Equal(t, 2, got.matched)
	assert.Equal(t, 3, got.total)
}

func TestFlattenWorldState_WhereSkipsEntitiesMissingReadComponent(t *testing.T) {
	t.Parallel()
	descriptors := stateDescriptors(t)
	// Entity 11 has no Position; reading Position.X must reject it, not error on the nil lookup.
	filter, err := compileStateFilter(GetStateInput{Where: "Position.X > 0"})
	require.NoError(t, err)

	got, err := flattenWorldState(sampleWorld(t, descriptors), -1, filter, descriptors)

	require.NoError(t, err)
	require.Len(t, got.entities, 2)
	assert.Equal(t, uint32(7), got.entities[0].ID)
	assert.Equal(t, uint32(9), got.entities[1].ID)
	assert.Equal(t, 2, got.matched)
	assert.Equal(t, 3, got.total)
}

func TestFlattenWorldState_WhereBareNilCheckMatchesAbsence(t *testing.T) {
	t.Parallel()
	descriptors := stateDescriptors(t)
	// A bare reference reads no fields, so absence stays expressible.
	filter, err := compileStateFilter(GetStateInput{Where: "Position == nil"})
	require.NoError(t, err)

	got, err := flattenWorldState(sampleWorld(t, descriptors), -1, filter, descriptors)

	require.NoError(t, err)
	require.Len(t, got.entities, 1)
	assert.Equal(t, uint32(11), got.entities[0].ID)
	assert.Equal(t, 1, got.matched)
}

func TestWhereFieldReads_CollectsMemberRootsOnly(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"Health"}, whereFieldReads("Health.HP > 50"))
	assert.Equal(t, []string{"Health"}, whereFieldReads(`Health["HP"] > 50`))
	assert.Equal(t, []string{"Health", "Position"}, whereFieldReads("Health.HP > 50 && Position.X < 0"))
	assert.Empty(t, whereFieldReads("Position == nil"))
	assert.Empty(t, whereFieldReads("_id > 5"))
}

func TestFlattenWorldState_WhereCanUseEntityID(t *testing.T) {
	t.Parallel()
	descriptors := stateDescriptors(t)
	filter, err := compileStateFilter(GetStateInput{Where: "_id == 11"})
	require.NoError(t, err)

	got, err := flattenWorldState(sampleWorld(t, descriptors), -1, filter, descriptors)

	require.NoError(t, err)
	require.Len(t, got.entities, 1)
	assert.Equal(t, uint32(11), got.entities[0].ID)
}

func TestFlattenWorldState_WhereAndFindCombine(t *testing.T) {
	t.Parallel()
	descriptors := stateDescriptors(t)
	filter, err := compileStateFilter(GetStateInput{
		Find:  []string{"Health", "Position"},
		Where: "Health.HP > 50",
	})
	require.NoError(t, err)

	got, err := flattenWorldState(sampleWorld(t, descriptors), -1, filter, descriptors)

	require.NoError(t, err)
	// Entity 11 passes where but lacks Position; entity 9 passes both.
	require.Len(t, got.entities, 1)
	assert.Equal(t, uint32(9), got.entities[0].ID)
}

func TestFlattenWorldState_WhereNonBooleanIsAnError(t *testing.T) {
	t.Parallel()
	descriptors := stateDescriptors(t)
	// Compiles because the component type is unknown until an entity is in hand;
	// evaluation is where a non-boolean result gets rejected.
	filter, err := compileStateFilter(GetStateInput{Where: "Health.HP"})
	require.NoError(t, err)

	_, err = flattenWorldState(sampleWorld(t, descriptors), -1, filter, descriptors)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "where clause")
}

func TestCompileStateFilter_RejectsUnknownMatch(t *testing.T) {
	t.Parallel()
	_, err := compileStateFilter(GetStateInput{Find: []string{"Health"}, Match: "startswith"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "match must be")
}

func TestCompileStateFilter_DefaultsMatchToContains(t *testing.T) {
	t.Parallel()
	f, err := compileStateFilter(GetStateInput{Find: []string{"Health"}})

	require.NoError(t, err)
	assert.Equal(t, matchContains, f.match)
}

func TestCompileStateFilter_RejectsUnparsableWhere(t *testing.T) {
	t.Parallel()
	_, err := compileStateFilter(GetStateInput{Where: "Health.HP >"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse where clause")
}

func TestFlattenWorldState_RejectsComponentIndexOutsideNameTable(t *testing.T) {
	t.Parallel()
	ws := &cardinalv1.WorldState{
		Components: []string{"Health"},
		Entities:   []*cardinalv1.Entity{{Id: 1, Components: []uint32{1}, Payloads: [][]byte{nil}}},
	}

	_, err := flattenWorldState(ws, -1, noFilter(t), stateDescriptors(t))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "snapshot entity references component 1, but the name table has 1 entries")
}

func TestEntityComponents_DecodesAgainstDescriptors(t *testing.T) {
	t.Parallel()
	descriptors := stateDescriptors(t)

	got := entityComponents([]string{"Health", "Position"}, [][]byte{
		componentBlob(t, descriptors, "Health", 42),
		componentBlob(t, descriptors, "Position", 4),
	}, descriptors)

	assert.Equal(t, map[string]any{"HP": int64(42)}, got["Health"])
	assert.Equal(t, map[string]any{"X": int64(4)}, got["Position"])
}

func TestEntityComponents_SkipsMissingPayloadAndKeepsUndecodableRaw(t *testing.T) {
	t.Parallel()
	descriptors := stateDescriptors(t)
	// "Broken" resolves to a descriptor, so it exercises a failed decode rather than a missing type.
	descriptors["Broken"] = descriptors["Health"]
	undecodable := []byte{0xff, 0xff, 0xff} // field 31 with wire type 7, which does not exist

	got := entityComponents(
		// "Unknown" is registered by the world but absent from the descriptor set. "Missing" has no
		// payload at all.
		[]string{"Broken", "Unknown", "Missing"},
		[][]byte{undecodable, {0x01, 0x02}},
		descriptors,
	)

	assert.NotContains(t, got, "Missing")
	// No descriptor and a failed decode both fall back to the raw bytes.
	assert.Equal(t, undecodable, got["Broken"])
	assert.Equal(t, []byte{0x01, 0x02}, got["Unknown"])
}

func TestFlattenWorldState_WhereWithLimitStillCountsEveryMatch(t *testing.T) {
	t.Parallel()
	descriptors := stateDescriptors(t)
	// A where clause forces per-entity evaluation past the limit, so the count
	// must stay complete even though only one entity is returned.
	filter, err := compileStateFilter(GetStateInput{Where: "Health.HP > 5"})
	require.NoError(t, err)

	got, err := flattenWorldState(sampleWorld(t, descriptors), 1, filter, descriptors)

	require.NoError(t, err)
	require.Len(t, got.entities, 1)
	assert.Equal(t, 3, got.matched)
	assert.Equal(t, 3, got.total)
}

// -------------------------------------------------------------------------------------------------
// debug_control handler tests
// -------------------------------------------------------------------------------------------------

func TestDebugControlHandler_MissingShardID(t *testing.T) {
	t.Parallel()
	_, err := debugControlHandler(context.Background(), mcp.CallToolRequest{}, DebugControlInput{
		Operation: debugOpPause,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "shard_id is required")
}

func TestDebugControlHandler_InvalidOperation(t *testing.T) {
	t.Parallel()
	_, err := debugControlHandler(context.Background(), mcp.CallToolRequest{}, DebugControlInput{
		ShardID:   "game",
		Operation: "halt",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "operation must be one of pause, resume, step, reset")
}

// An empty operation is rejected before any cluster round-trip, alongside the
// other invalid values.
func TestDebugControlHandler_EmptyOperation(t *testing.T) {
	t.Parallel()
	_, err := debugControlHandler(context.Background(), mcp.CallToolRequest{}, DebugControlInput{
		ShardID: "game",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "operation must be one of")
}

func TestDebugStatusMessage(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "paused at tick 42", debugStatusMessage(debugOpPause, 42))
	assert.Equal(t, "stepped to tick 43", debugStatusMessage(debugOpStep, 43))
	assert.Equal(t, "resumed", debugStatusMessage(debugOpResume, 0))
	assert.Equal(t, "reset to pre-tick-0 state", debugStatusMessage(debugOpReset, 0))
}

// -------------------------------------------------------------------------------------------------
// reload handler tests
// -------------------------------------------------------------------------------------------------

// An empty world_path is rejected before any Docker or cluster work, so the tool
// can never tear a world down without a source tree to rebuild from.
func TestReloadHandler_EmptyWorldPath(t *testing.T) {
	t.Parallel()
	_, err := reloadHandler(context.Background(), mcp.CallToolRequest{}, ReloadInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "world_path is required")
}

// deployShardsFromConfig emits one Deploy entry per logical shard ID, deduping the
// pool replicas that share an ID.
func TestDeployShardsFromConfig_DedupesByID(t *testing.T) {
	t.Parallel()
	cfg := &service.Config{
		Project: "acme-demo",
		WorldToml: worldtoml.Config{
			Shards: []worldtoml.Shard{{ID: "game"}, {ID: "game"}, {ID: "lobby"}},
		},
	}
	shards := deployShardsFromConfig(cfg, "")
	require.Len(t, shards, 2)
	assert.Equal(t, "game", shards[0].ID)
	assert.Equal(t, "lobby", shards[1].ID)
}

// A shard_id narrows the Deploy payload to that pool; an empty one includes all.
func TestDeployShardsFromConfig_FiltersByShardID(t *testing.T) {
	t.Parallel()
	cfg := &service.Config{
		Project: "acme-demo",
		WorldToml: worldtoml.Config{
			Shards: []worldtoml.Shard{{ID: "game"}, {ID: "game"}, {ID: "lobby"}},
		},
	}
	shards := deployShardsFromConfig(cfg, "lobby")
	require.Len(t, shards, 1)
	assert.Equal(t, "lobby", shards[0].ID)

	assert.Empty(t, deployShardsFromConfig(cfg, "nope"))
}

// Replicas each keep their own state, so a purge wipes per instance. The first
// replica keeps the pool's base ID; later ones are suffixed.
func TestShardInstances_ListsPoolReplicas(t *testing.T) {
	t.Parallel()
	cfg := worldtoml.Config{Shards: []worldtoml.Shard{
		{ID: "game", InstanceID: "game"},
		{ID: "game", InstanceID: "game-2"},
		{ID: "lobby", InstanceID: "lobby"},
	}}

	assert.Equal(t, []string{"game", "game-2"}, shardInstances(cfg, "game"))
	assert.Equal(t, []string{"lobby"}, shardInstances(cfg, "lobby"))
	assert.Empty(t, shardInstances(cfg, "missing"))
}

// An unexpanded shard (no InstanceID) falls back to its pool ID.
func TestShardInstances_FallsBackToShardID(t *testing.T) {
	t.Parallel()
	cfg := worldtoml.Config{Shards: []worldtoml.Shard{{ID: "game"}}}
	assert.Equal(t, []string{"game"}, shardInstances(cfg, "game"))
}

// -------------------------------------------------------------------------------------------------
// cluster handler tests
// -------------------------------------------------------------------------------------------------

func TestClusterHandler_InvalidOperation(t *testing.T) {
	t.Parallel()
	_, err := clusterHandler(context.Background(), mcp.CallToolRequest{}, ClusterInput{Operation: "restart"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "operation must be one of start, stop, purge")
}

func TestClusterHandler_EmptyOperation(t *testing.T) {
	t.Parallel()
	_, err := clusterHandler(context.Background(), mcp.CallToolRequest{}, ClusterInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "operation must be one of")
}

// purge only combines with start: the purge op already deletes the cluster and
// stop deliberately preserves state, so either pairing is rejected rather than
// silently ignored.
func TestClusterHandler_PurgeOnlyValidWithStart(t *testing.T) {
	t.Parallel()
	for _, op := range []string{clusterOpStop, clusterOpPurge} {
		_, err := clusterHandler(context.Background(), mcp.CallToolRequest{}, ClusterInput{
			Operation: op,
			Purge:     true,
		})
		require.Error(t, err, op)
		assert.Contains(t, err.Error(), "purge is only valid with 'start'", op)
	}
}

// start compiles and deploys a world, so it needs a source tree; the check runs
// before any cluster work.
func TestClusterHandler_StartRequiresWorldPath(t *testing.T) {
	t.Parallel()
	_, err := clusterHandler(context.Background(), mcp.CallToolRequest{}, ClusterInput{
		Operation: clusterOpStart,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "world_path is required")
}

// -------------------------------------------------------------------------------------------------
// sdk_generate handler tests
// -------------------------------------------------------------------------------------------------

func TestSdkGenerateHandler_MissingSource(t *testing.T) {
	t.Parallel()
	_, err := sdkGenerateHandler(context.Background(), mcp.CallToolRequest{}, SdkGenerateInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source is required")
}

// The spinner redraws one line with carriage returns and colours it; only the
// final frame should survive into the tool result.
func TestCleanTerminalOutput_StripsAnsiAndSpinnerFrames(t *testing.T) {
	t.Parallel()
	raw := []byte("\x1b[32m✔ ok\x1b[0m\n" +
		"⣾ working (0s)\r⣽ working (1s)\r✔ done (2s)\n" +
		"   \n" +
		"Generated 6 commands\n")

	got := cleanTerminalOutput(raw)

	assert.Equal(t, "✔ ok\n✔ done (2s)\nGenerated 6 commands", got)
}

func TestCleanTerminalOutput_Empty(t *testing.T) {
	t.Parallel()
	assert.Empty(t, cleanTerminalOutput(nil))
}

// `world sdk generate` exits 0 even when it refuses to generate, so failure is
// detected from the printed report instead of the exit status.
func TestReportFailed(t *testing.T) {
	t.Parallel()
	marker := string(logsymbols.Error)

	assert.True(t, reportFailed(marker+" sdk generate: backend doesn't type-check — nothing generated"))
	assert.True(t, reportFailed("Discovered 6 commands\n"+marker+" resolve --go-out's Go module"))
	// Indented, as printer renders it after a newline.
	assert.True(t, reportFailed("   "+marker+" something went wrong"))

	assert.False(t, reportFailed("Generated 6 commands → Go: /tmp/gen"))
	assert.False(t, reportFailed("ORPHANS — declared with Name() but wired to no system"))
	assert.False(t, reportFailed(""))
}

// sdkGenerateArgs' defaulting matrix is the heart of the go_out/cs_out contract
// the schema advertises: go_out defaults to <source>/gen ONLY when no other
// output is named, so a {source, cs_out} call stays a client-only C# run (the
// CLI's deliberately-supported mode) and is not silently turned into a Go write
// into the backend's module.
func TestSdkGenerateArgs_DefaultsMatrix(t *testing.T) {
	t.Parallel()
	const source = "/world/shards/game"
	genDefault := filepath.Join(source, "gen")
	goDir := "/world/shards/game/go-gen"
	csDir := "/world/shards/game/csharp"
	protoDir := "/world/shards/game/proto"

	// {source} — no outputs named → go_out defaulted to <source>/gen.
	goOut, csOut, protoOut := sdkGenerateArgs(source, SdkGenerateInput{})
	assert.Equal(t, genDefault, goOut)
	assert.Empty(t, csOut)
	assert.Empty(t, protoOut)

	// {source, cs_out} — go_out omitted with cs_out set: client-only. The default
	// must NOT fire (it only applies when no other output is set), so go_out stays
	// empty and the CLI runs its C#-only branch instead of writing Go.
	goOut, csOut, protoOut = sdkGenerateArgs(source, SdkGenerateInput{CsOut: csDir})
	assert.Empty(t, goOut, "go_out must stay empty when cs_out is set and go_out omitted (client-only)")
	assert.Equal(t, csDir, csOut)
	assert.Empty(t, protoOut)

	// {source, go_out, cs_out} — both named: no defaulting, both passed through.
	goOut, csOut, protoOut = sdkGenerateArgs(source, SdkGenerateInput{GoOut: goDir, CsOut: csDir})
	assert.Equal(t, goDir, goOut)
	assert.Equal(t, csDir, csOut)
	assert.Empty(t, protoOut)

	// {source, go_out} — only Go named: passed through, no cs_out.
	goOut, csOut, protoOut = sdkGenerateArgs(source, SdkGenerateInput{GoOut: goDir})
	assert.Equal(t, goDir, goOut)
	assert.Empty(t, csOut)
	assert.Empty(t, protoOut)

	// {source, proto_out} — proto_out requires go_out, so the go_out default fires
	// (cs_out empty) and proto_out passes through; the CLI then accepts it.
	goOut, csOut, protoOut = sdkGenerateArgs(source, SdkGenerateInput{ProtoOut: protoDir})
	assert.Equal(t, genDefault, goOut)
	assert.Empty(t, csOut)
	assert.Equal(t, protoDir, protoOut)

	// {source, cs_out, proto_out} — cs_out set withholds the go_out default, so
	// go_out stays empty; the CLI rejects the run (proto-out requires go-out)
	// rather than the handler silently inventing a Go output dir.
	goOut, csOut, protoOut = sdkGenerateArgs(source, SdkGenerateInput{CsOut: csDir, ProtoOut: protoDir})
	assert.Empty(t, goOut)
	assert.Equal(t, csDir, csOut)
	assert.Equal(t, protoDir, protoOut)
}

// The flag loop omits a flag whose resolved value is empty (the CLI's
// empty==skip contract), so the presence of --go-out in cmdArgs is exactly
// what decides wire regeneration vs client-only. This pins that mapping for the
// full matrix.
func TestSdkGenerateCmdArgs_FlagPresence(t *testing.T) {
	t.Parallel()
	const source = "/world/shards/game"

	hasFlag := func(args []string, flag string) bool {
		for _, a := range args {
			if strings.HasPrefix(a, flag+"=") {
				return true
			}
		}
		return false
	}
	flagValue := func(args []string, flag string) string {
		for _, a := range args {
			if v, ok := strings.CutPrefix(a, flag+"="); ok {
				return v
			}
		}
		return ""
	}

	// {source} (go_out defaulted) → --go-out present, others absent.
	args := sdkGenerateCmdArgs(source, filepath.Join(source, "gen"), "", "")
	assert.Equal(t, []string{"sdk", "generate", source, "--go-out=" + filepath.Join(source, "gen")}, args)
	assert.True(t, hasFlag(args, "--go-out"))
	assert.False(t, hasFlag(args, "--cs-out"))
	assert.False(t, hasFlag(args, "--proto-out"))

	// {source, cs_out} (client-only) → --go-out ABSENT, --cs-out present.
	args = sdkGenerateCmdArgs(source, "", "/csharp", "")
	assert.Equal(t, []string{"sdk", "generate", source, "--cs-out=/csharp"}, args)
	assert.False(t, hasFlag(args, "--go-out"), "--go-out must be absent on a client-only run")
	assert.True(t, hasFlag(args, "--cs-out"))
	assert.False(t, hasFlag(args, "--proto-out"))

	// {source, go_out, cs_out} → both present, exact values preserved.
	args = sdkGenerateCmdArgs(source, "/go", "/csharp", "")
	assert.Equal(t, []string{"sdk", "generate", source, "--go-out=/go", "--cs-out=/csharp"}, args)
	assert.Equal(t, "/go", flagValue(args, "--go-out"))
	assert.Equal(t, "/csharp", flagValue(args, "--cs-out"))
	assert.False(t, hasFlag(args, "--proto-out"))

	// All three outputs present.
	args = sdkGenerateCmdArgs(source, "/go", "/csharp", "/proto")
	assert.Equal(t, []string{
		"sdk", "generate", source,
		"--go-out=/go", "--cs-out=/csharp", "--proto-out=/proto",
	}, args)
}

// status must reflect what was actually generated, not claim wire regeneration
// unconditionally. The bug returned "regenerated the wire layer …" even on a
// client-only C# run; these assert the branched mapping and pin the exact
// strings an agent parses.
func TestSdkGenerateStatus_ReflectsOutput(t *testing.T) {
	t.Parallel()
	const (
		wireOnly  = "regenerated the wire layer; rebuild or reload the shard to pick it up"
		wireAndCS = "regenerated the wire layer and the C# client SDK; rebuild or reload the shard to pick it up"
		csOnly    = "generated the C# client SDK"
	)

	// {source} / {source, go_out} — go_out set, no cs_out: wire regeneration.
	assert.Equal(t, wireOnly, sdkGenerateStatus("/go", ""))

	// {source, go_out, cs_out} — both: wire + client SDK.
	assert.Equal(t, wireAndCS, sdkGenerateStatus("/go", "/csharp"))

	// {source, cs_out} — client-only: NO wire claim. The regression that the
	// status field contradicted the `go_out`-absent authoritative fields lives
	// here: status must say C#-only.
	assert.Equal(t, csOnly, sdkGenerateStatus("", "/csharp"))
}

// The published input schema is generated from the struct's jsonschema tags by
// mcp.WithInputSchema (google/jsonschema-go), and is exactly what the LLM driving
// the tool sees. The go_out description must document that the <source>/gen
// default only applies when no other output is set — the contract the
// implementation actually honors — and tell the caller how to get Go+CS vs
// client-only. This pins both halves of the schema fix against a future
// tag-only regression.
func TestSdkGenerate_PublishedGoOutDefaultDoc(t *testing.T) {
	t.Parallel()
	tool := mcp.NewTool("sdk_generate", mcp.WithInputSchema[SdkGenerateInput]())
	require.NotEmpty(t, tool.RawInputSchema, "the tool must publish a generated input schema")

	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(tool.RawInputSchema, &schema))
	require.Contains(t, schema.Properties, "go_out", "go_out must appear in the published schema")

	desc := schema.Properties["go_out"].Description
	assert.Contains(t, desc, "Defaults to <source>/gen when no other output is set",
		"the conditional default must be documented, not an unconditional one")
	assert.Contains(t, desc, "When cs_out is also passed go_out is NOT defaulted",
		"the schema must warn that cs_out withholds the go_out default (the client-only entry)")
}

func TestDescribeWorld_PublishesAnEmptyObjectInputSchema(t *testing.T) {
	t.Parallel()
	srv := server.NewMCPServer("test", "0")
	registerDescribeWorldTool(srv)

	tool := srv.GetTool("describe_world")
	require.NotNil(t, tool)
	published, err := json.Marshal(tool.Tool)
	require.NoError(t, err, "tools/list must be able to serialize the tool")

	var listed struct {
		InputSchema json.RawMessage `json:"inputSchema"`
	}
	require.NoError(t, json.Unmarshal(published, &listed))
	assert.JSONEq(t, `{"type":"object","properties":{},"required":[],"additionalProperties":false}`,
		string(listed.InputSchema))
}

// shard_url is caller-supplied, so the Argus token must never follow it off this
// machine: a prompt-injected agent could otherwise name a host and be handed the
// user's credential.
func TestDevAuthInterceptor_TokenNeverLeavesLocalhost(t *testing.T) {
	t.Setenv(argusTokenEnv, "secret-token")

	local := []string{
		"http://localhost:8080/argus/demo/game",
		"http://127.0.0.1:8081",
		"http://[::1]:8080",
	}
	for _, target := range local {
		h := http.Header{}
		(&devAuthInterceptor{playerID: "mcp-dev-player", target: target}).setAuthHeader(h)
		assert.Equal(t, "Bearer secret-token", h.Get("Authorization"), target)
	}

	remote := []string{
		"https://attacker.example/argus/demo/game",
		"http://169.254.169.254/latest/meta-data",
		"http://localhost.attacker.example",
		"not a url at all",
		"",
	}
	for _, target := range remote {
		h := http.Header{}
		(&devAuthInterceptor{playerID: "mcp-dev-player", target: target}).setAuthHeader(h)
		assert.Empty(t, h.Get("Authorization"), "token must not be sent to %q", target)
		assert.Equal(t, "mcp-dev-player", h.Get(devPlayerIDHeader), target)
	}
}

func TestDevAuthInterceptor_NoTokenUsesDevHeader(t *testing.T) {
	t.Setenv(argusTokenEnv, "")

	h := http.Header{}
	(&devAuthInterceptor{playerID: "mcp-dev-player", target: "http://localhost:8080"}).setAuthHeader(h)
	assert.Empty(t, h.Get("Authorization"))
	assert.Equal(t, "mcp-dev-player", h.Get(devPlayerIDHeader))
}

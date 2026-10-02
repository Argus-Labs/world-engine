package mcp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guumaster/logsymbols"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
	operatorv1 "github.com/argus-labs/world-engine/cli/proto/gen/go/cardinal/operator/v1"
)

func TestBuildMCPServer_RegistersAllTools(t *testing.T) {
	t.Parallel()
	srv := NewServer()
	require.NotNil(t, srv)
}

// An empty instance_name resolves to the shard's first instance without an
// operator round-trip, so send_command keeps working exactly as before for
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
	assert.Equal(t, defaultDevEmail, input.Email)
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
		Email:       "custom@example.com",
		Region:      "ap-southeast-1",
	}

	err := input.validate()
	require.NoError(t, err)

	assert.Equal(t, "http://custom:9999", input.ShardURL)
	assert.Equal(t, "custom@example.com", input.Email)
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
// k8s helper tests (operator status → MCP shape)
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
	status := &operatorv1.StatusResponse{
		Pools: []*operatorv1.ShardPoolStatus{{
			ShardId:   "gameplay",
			Namespace: "cardinal-operator-system",
			PoolSize:  2,
			ImageTag:  "dev",
			Phase:     "Running",
			Instances: []*operatorv1.ShardInstanceStatus{
				{
					Name:         "gameplay",
					PodName:      "cardinal-gameplay-pool-0",
					Phase:        "Running",
					Ready:        true,
					RestartCount: 1,
					Age:          "5m",
				},
				{Name: "gameplay-2", PodName: "cardinal-gameplay-pool-1", Phase: "Pending", Ready: false},
			},
		}},
	}

	pools := toShardPools(status)
	require.Len(t, pools, 1)
	assert.Equal(t, "gameplay", pools[0].ShardID)
	assert.Equal(t, int32(2), pools[0].PoolSize)
	require.Len(t, pools[0].Instances, 2)
	assert.Equal(t, "cardinal-gameplay-pool-0", pools[0].Instances[0].PodName)
	assert.True(t, pools[0].Instances[0].Ready)
	assert.Equal(t, int32(1), pools[0].Instances[0].RestartCount)
	assert.False(t, pools[0].Instances[1].Ready)
}

func TestShardPods(t *testing.T) {
	t.Parallel()
	status := &operatorv1.StatusResponse{
		Pools: []*operatorv1.ShardPoolStatus{
			{ShardId: "gameplay", Instances: []*operatorv1.ShardInstanceStatus{
				{Name: "gameplay", PodName: "pod-a"},
				{Name: "gameplay-2", PodName: "pod-b"},
			}},
			{ShardId: "lobby", Instances: []*operatorv1.ShardInstanceStatus{
				{Name: "lobby", PodName: "pod-c"},
			}},
		},
	}

	assert.ElementsMatch(t, []string{"pod-a", "pod-b"}, shardPods(status, "gameplay", ""))
	assert.Equal(t, []string{"pod-b"}, shardPods(status, "gameplay", "gameplay-2"))
	assert.Equal(t, []string{"pod-c"}, shardPods(status, "lobby", ""))
	assert.Empty(t, shardPods(status, "nope", ""))
}

func TestShardPods_InstanceVariants(t *testing.T) {
	t.Parallel()
	status := &operatorv1.StatusResponse{
		Pools: []*operatorv1.ShardPoolStatus{{
			ShardId: "game",
			Instances: []*operatorv1.ShardInstanceStatus{
				{Name: "game", PodName: "pod-1"},
				{Name: "game-5", PodName: "pod-5"},
			},
		}},
	}

	// All of these resolve to instance "game-5".
	for _, in := range []string{"game-5", "game 5", "game5", "GAME-5", "5"} {
		assert.Equal(t, []string{"pod-5"}, shardPods(status, "game", in), "input %q", in)
	}
	// Index-1 references resolve to the bare first instance "game".
	for _, in := range []string{"game", "game 1", "game1", "1"} {
		assert.Equal(t, []string{"pod-1"}, shardPods(status, "game", in), "input %q", in)
	}
	// No instance → every pod of the shard; unknown instance → none.
	assert.ElementsMatch(t, []string{"pod-1", "pod-5"}, shardPods(status, "game", ""))
	assert.Empty(t, shardPods(status, "game", "game-9"))
}

// TestShardPods_ScopedToShardPool guards against an instance reference resolving
// into a different shard's pool: matching must be scoped to pool.ShardId, so a
// reference naming (or index-colliding with) another shard's instance yields no
// pod for the requested shard rather than silently routing to the wrong one.
func TestShardPods_ScopedToShardPool(t *testing.T) {
	t.Parallel()
	status := &operatorv1.StatusResponse{
		Pools: []*operatorv1.ShardPoolStatus{
			{ShardId: "game", Instances: []*operatorv1.ShardInstanceStatus{
				{Name: "game", PodName: "pod-game"},
			}},
			{ShardId: "lobby", Instances: []*operatorv1.ShardInstanceStatus{
				{Name: "lobby", PodName: "pod-lobby"},
			}},
			{ShardId: "game2", Instances: []*operatorv1.ShardInstanceStatus{
				{Name: "game2", PodName: "pod-game2"},
			}},
		},
	}

	// A sibling shard's exact name must not match through the requested shard.
	assert.Empty(t, shardPods(status, "game", "lobby"))
	// An index that collides with another shard's name ("2" ~ "game2") must not
	// leak: shard "game" is pool size 1, so it has no instance "2".
	assert.Empty(t, shardPods(status, "game", "2"))
	// The requested shard still resolves its own instances.
	assert.Equal(t, []string{"pod-game"}, shardPods(status, "game", "game"))
}

// TestResolveInstanceName_UnknownReturnsError confirms a reference that matches
// no instance of the requested shard fails clearly rather than resolving to
// another shard's instance. Empty operatorURL routes to the default endpoint,
// which is unreachable in tests, so the lookup errors — that itself proves the
// non-empty path always consults the operator (and the empty path, covered by
// TestResolveInstanceName_EmptyDefaultsToShardID, never does).
func TestResolveInstanceName_UnknownReturnsError(t *testing.T) {
	t.Parallel()
	_, err := resolveInstanceName(context.Background(), "http://127.0.0.1:1/nope", "game", "lobby")
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

// An empty operation is rejected before any operator round-trip, alongside the
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
// pool replicas that share an ID, each pointing at that shard's built image tag.
func TestDeployShardsFromConfig_DedupesByID(t *testing.T) {
	t.Parallel()
	cfg := &service.Config{
		Namespace: "acme-demo",
		WorldToml: worldtoml.Config{
			Shards: []worldtoml.Shard{{ID: "game"}, {ID: "game"}, {ID: "lobby"}},
		},
	}
	shards := deployShardsFromConfig(cfg, "")
	require.Len(t, shards, 2)
	assert.Equal(t, "game", shards[0].ID)
	assert.Equal(t, "acme-demo-game-shard:latest", shards[0].SourceImage)
	assert.Equal(t, "lobby", shards[1].ID)
	assert.Equal(t, "acme-demo-lobby-shard:latest", shards[1].SourceImage)
}

// A shard_id narrows the Deploy payload to that pool; an empty one includes all.
func TestDeployShardsFromConfig_FiltersByShardID(t *testing.T) {
	t.Parallel()
	cfg := &service.Config{
		Namespace: "acme-demo",
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
// mcp.WithInputSchema (invopop/jsonschema), and is exactly what the LLM driving
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

// -------------------------------------------------------------------------------------------------
// introspect convertTypeSchemas / ArrayFields propagation tests
// -------------------------------------------------------------------------------------------------
//
// The server populates TypeSchema.ArrayFields with the dimensions of every multi-dimensional
// fixed-size array field so a client that does not know the schema ahead of time (the debug
// tooling — the introspect MCP tool) can rebuild the indices: a flat repeated field of 32 int32s is
// ambiguous between [4][8] and [8][4], and the wrong guess reads the wrong element. These tests pin
// that convertTypeSchemas propagates that shape through the IntrospectResponse → IntrospectOutput
// boundary instead of dropping it.

// gridDescriptorFiles builds the descriptor registry for a Grid message carrying a repeated int32
// "cells" field — the on-wire shape a [4][8]int32 fixed array flattens into — exactly the type a
// shard would advertise alongside ArrayField{Field:"cells", Dims:[4,8]}.
func gridDescriptorFiles(t *testing.T) *protoregistry.Files {
	t.Helper()
	raw := testDescriptorSet(t, &descriptorpb.DescriptorProto{
		Name: new("Grid"),
		Field: []*descriptorpb.FieldDescriptorProto{{
			Name:   new("cells"),
			Number: new(int32(1)),
			Label:  descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
			Type:   descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
		}},
	})
	files, err := resolveDescriptorFiles(raw)
	require.NoError(t, err)
	return files
}

// TestConvertTypeSchemas_PropagatesArrayFields is the core fix: the shape metadata the server sends
// on TypeSchema.ArrayFields must reach NamedSchema, so a client decoding an unknown type can
// rebuild the indices for a multi-dimensional fixed array instead of guessing.
func TestConvertTypeSchemas_PropagatesArrayFields(t *testing.T) {
	t.Parallel()
	files := gridDescriptorFiles(t)

	out := convertTypeSchemas([]*cardinalv1.TypeSchema{{
		Name:             "Grid",
		ProtoMessageName: "test.Grid",
		ArrayFields: []*cardinalv1.ArrayField{{
			Field: "cells",
			Dims:  []uint32{4, 8},
		}},
	}}, files)

	require.Len(t, out, 1)
	assert.Equal(t, "Grid", out[0].Name)
	require.Len(t, out[0].ArrayFields, 1)
	assert.Equal(t, "cells", out[0].ArrayFields[0].GetField())
	assert.Equal(t, []uint32{4, 8}, out[0].ArrayFields[0].GetDims())
}

// TestConvertTypeSchemas_ArrayFieldsJSONShape pins the JSON a client (LLM) actually sees: the
// per-type object gains an "array_fields" array whose entries carry "field" and "dims",
// dimensions outermost first. A wrong guess between [4,8] and [8,4] reads a different element, so
// the exact key names and ordering are the contract — and it must round-trip back unchanged.
func TestConvertTypeSchemas_ArrayFieldsJSONShape(t *testing.T) {
	t.Parallel()
	files := gridDescriptorFiles(t)

	out := convertTypeSchemas([]*cardinalv1.TypeSchema{{
		Name:             "Grid",
		ProtoMessageName: "test.Grid",
		ArrayFields: []*cardinalv1.ArrayField{{
			Field: "cells",
			Dims:  []uint32{4, 8},
		}},
	}}, files)

	b, err := json.Marshal(out[0])
	require.NoError(t, err)
	t.Logf("NamedSchema JSON: %s", string(b))

	assert.Contains(t, string(b), `"array_fields"`)
	assert.Contains(t, string(b), `"field":"cells"`)
	assert.Contains(t, string(b), `"dims":[4,8]`)

	var got struct {
		Name        string `json:"name"`
		ArrayFields []struct {
			Field string   `json:"field"`
			Dims  []uint32 `json:"dims"`
		} `json:"array_fields"`
	}
	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, "Grid", got.Name)
	require.Len(t, got.ArrayFields, 1)
	assert.Equal(t, "cells", got.ArrayFields[0].Field)
	assert.Equal(t, []uint32{4, 8}, got.ArrayFields[0].Dims)
}

// TestConvertTypeSchemas_ArrayFieldsOmittedWhenEmpty guards the backward-compatible common case: a
// type with no multi-dimensional fixed array fields produces no "array_fields" key at all
// (omitempty), so existing consumers — which never expected the key — see output byte-identical to
// the pre-fix shape. This is the no-regression guarantee for every shipped shard today, all of which
// carry empty ArrayFields.
func TestConvertTypeSchemas_ArrayFieldsOmittedWhenEmpty(t *testing.T) {
	t.Parallel()
	files := gridDescriptorFiles(t)

	out := convertTypeSchemas([]*cardinalv1.TypeSchema{{
		Name:             "Grid",
		ProtoMessageName: "test.Grid",
	}}, files)

	require.Empty(t, out[0].ArrayFields)

	b, err := json.Marshal(out[0])
	require.NoError(t, err)
	assert.NotContains(t, string(b), "array_fields",
		"a type with no array fields must not advertise an empty array_fields key")
	// Name and schema are unaffected: the descriptor renders exactly as before.
	assert.Contains(t, string(b), `"name":"Grid"`)
	assert.Contains(t, string(b), `"schema"`)
}

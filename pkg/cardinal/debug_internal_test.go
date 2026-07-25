package cardinal

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/invopop/jsonschema"
	"github.com/shamaton/msgpack/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
)

// schemaSample mixes the tag cases that decide a field's wire name: a msgpack
// tag, a json-only tag whose value differs from the field name, an untagged
// field, and an explicitly excluded field.
type schemaSample struct {
	Tagged   string `json:"tagged"   msgpack:"nickname"` // msgpack tag wins
	JSONOnly string `json:"jsonOnly"`                    // json tag ignored -> field name
	Plain    int    // no tags -> field name
	Skipped  string `msgpack:"-"` // excluded from the wire
}

func (schemaSample) Name() string { return "schema-sample" }

// TestIntrospectSchemaNamesMatchWireFormat guards the introspect↔serialize
// contract: the field names advertised by the introspection schema must equal
// the keys shamaton/msgpack actually reads and writes, so a client that fills a
// command/component from the schema isn't silently dropped on the wire.
// Regression for the create-player "nickname" mismatch.
func TestIntrospectSchemaNamesMatchWireFormat(t *testing.T) {
	t.Parallel()

	// Names the wire format actually uses.
	encoded, err := msgpack.Marshal(schemaSample{Tagged: "a", JSONOnly: "b", Plain: 1, Skipped: "x"})
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, msgpack.Unmarshal(encoded, &wire))

	// Names introspection advertises, via the real register() path.
	d := &debugModule{
		commands: make(map[string]*structpb.Struct),
		reflector: &jsonschema.Reflector{
			Anonymous:      true, // Don't add $id based on package path
			ExpandedStruct: true, // Inline the struct fields directly
			FieldNameTag:   "msgpack",
		},
	}
	require.NoError(t, d.register("command", schemaSample{}))
	schemaMap := d.commands["schema-sample"].AsMap()
	props, ok := schemaMap["properties"].(map[string]any)
	require.True(t, ok, "schema should have properties")

	assert.ElementsMatch(t, mapKeys(wire), mapKeys(props),
		"introspect schema field names must match the msgpack wire keys")

	// Spot-check the specifics the fix turns on.
	assert.Contains(t, props, "nickname")   // msgpack tag wins over json
	assert.Contains(t, props, "JSONOnly")   // json tag ignored; Go field name used
	assert.Contains(t, props, "Plain")      // untagged -> field name
	assert.NotContains(t, props, "Skipped") // msgpack:"-" excluded
	assert.NotContains(t, props, "tagged")  // the json tag value must not leak through
}

func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// -------------------------------------------------------------------------------------------------
// GetState & the published state view
// -------------------------------------------------------------------------------------------------

func newTestDebugModule() *debugModule {
	return &debugModule{control: newTickControl()}
}

func getStateRequest() *connect.Request[cardinalv1.GetStateRequest] {
	return connect.NewRequest(&cardinalv1.GetStateRequest{})
}

// TestGetStateUnavailableBeforeFirstPublish: before the first tick publishes a view, GetState must
// return Unavailable instead of touching live ECS state.
func TestGetStateUnavailableBeforeFirstPublish(t *testing.T) {
	t.Parallel()
	d := newTestDebugModule()

	_, err := d.GetState(context.Background(), getStateRequest())
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
}

// TestPublishStateRoundtrip: GetState serves exactly what publishState stored — same object, same
// height, same timestamp, and the paused flag captured at publish time.
func TestPublishStateRoundtrip(t *testing.T) {
	t.Parallel()
	d := newTestDebugModule()
	ts := time.Unix(42, 0)
	ws := &cardinalv1.WorldState{NextId: 7}

	d.publishState(ws, 41, ts)

	resp, err := d.GetState(context.Background(), getStateRequest())
	require.NoError(t, err)
	assert.False(t, resp.Msg.GetIsPaused())
	assert.Equal(t, uint64(41), resp.Msg.GetSnapshot().GetTickHeight())
	assert.Equal(t, ts.Unix(), resp.Msg.GetSnapshot().GetTimestamp().AsTime().Unix())
	assert.Same(t, ws, resp.Msg.GetSnapshot().GetWorldState())

	// The paused flag is captured at publish time, not read live at request time.
	d.control.isPaused.Store(true)
	d.publishState(&cardinalv1.WorldState{}, 42, ts)
	resp, err = d.GetState(context.Background(), getStateRequest())
	require.NoError(t, err)
	assert.True(t, resp.Msg.GetIsPaused())
}

// TestPublishStateClonesArchetypeBitmaps: ComponentsBitmap aliases live bitmap memory when it
// arrives from archetype.toProto (bitmap.ToBytes is zero-copy). The view outlives the tick, so
// publishState must clone it — a later mutation of the source buffer must not be visible.
func TestPublishStateClonesArchetypeBitmaps(t *testing.T) {
	t.Parallel()
	d := newTestDebugModule()
	live := []byte{1, 2, 3, 4, 5, 6, 7, 8} // stands in for the live bitmap's backing array
	ws := &cardinalv1.WorldState{
		Archetypes: []*cardinalv1.Archetype{{ComponentsBitmap: live}},
	}

	d.publishState(ws, 1, time.Time{})
	live[0] = 0xFF // the live world mutating after the tick

	got := d.view.Load().ws.GetArchetypes()[0].GetComponentsBitmap()
	assert.Equal(t, byte(1), got[0], "published view must not alias live bitmap memory")
}

// TestSetPausedUpdatesView: setPaused flips both the control flag and the published view in one
// call, republishing the same state with only the flag changed; before the first publish it only
// sets the control flag.
func TestSetPausedUpdatesView(t *testing.T) {
	t.Parallel()
	d := newTestDebugModule()

	d.setPaused(true) // no view yet: control flag set, no view created
	assert.True(t, d.control.isPaused.Load())
	assert.Nil(t, d.view.Load())
	d.setPaused(false)

	d.publishState(&cardinalv1.WorldState{NextId: 3}, 5, time.Unix(1, 0))
	d.setPaused(true)
	assert.True(t, d.control.isPaused.Load())
	v := d.view.Load()
	assert.True(t, v.paused)
	assert.Equal(t, uint64(5), v.height, "flag flip must preserve the rest of the view")
	assert.Equal(t, uint32(3), v.ws.GetNextId())

	d.setPaused(false)
	assert.False(t, d.control.isPaused.Load())
	assert.False(t, d.view.Load().paused)
}

// TestDebugStateNilSafety: the prod path (debug disabled) calls these on a nil receiver.
func TestDebugStateNilSafety(t *testing.T) {
	t.Parallel()
	var d *debugModule
	assert.NotPanics(t, func() {
		d.publishState(&cardinalv1.WorldState{}, 1, time.Time{})
		d.setPaused(true)
	})
}

// debugStateCounter is a minimal tag component so the spawner changes world state every tick.
type debugStateCounter struct{}

func (debugStateCounter) Name() string { return "debug_state_counter" }

type debugStateSpawnerState struct {
	BaseSystemState
	Counters Exact[struct {
		Counter Ref[debugStateCounter]
	}]
}

// debugStateSpawnerSystem creates exactly one entity per tick, so entity count == executed ticks.
func debugStateSpawnerSystem(s *debugStateSpawnerState) {
	_, entity := s.Counters.Create()
	entity.Counter.Set(debugStateCounter{})
}

func countEntities(ws *cardinalv1.WorldState) int {
	n := 0
	for _, arch := range ws.GetArchetypes() {
		n += len(arch.GetEntities())
	}
	return n
}

// TestGetStateLifecycle drives the real run() loop and checks GetState against every debugger
// transition. The critical assertion is the step one: a paused world that steps once must show the
// stepped state in GetState immediately — this is what breaks if a demand gate or lazy publish is
// ever reintroduced.
func TestGetStateLifecycle(t *testing.T) {
	t.Setenv("LOG_LEVEL", "disabled")

	debug := true
	w, err := NewWorld(WorldOptions{
		Region:              "test",
		Organization:        "test",
		Project:             "test",
		ShardID:             "0",
		TickRate:            100, // 10ms ticks keep the Eventually waits short
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        1_000_000,
		Debug:               &debug,
	})
	require.NoError(t, err)
	RegisterSystem(w, debugStateSpawnerSystem)

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- w.run(ctx) }()
	t.Cleanup(func() {
		cancel()
		require.ErrorIs(t, <-runErr, context.Canceled)
	})

	d := w.debug

	// getState fetches the current view, failing the test on error. Scoped here because ctx is fixed
	// and, once the world is running, GetState is never expected to error.
	getState := func() *cardinalv1.GetStateResponse {
		resp, getErr := d.GetState(ctx, getStateRequest())
		require.NoError(t, getErr)
		return resp.Msg
	}

	// Wait until the world has ticked past the initial (height 0) view, so entities
	// exist before we pause. run() publishes a height-0 view at boot, so we must
	// wait for a real tick, not just any view.
	require.Eventually(t, func() bool {
		resp, getErr := d.GetState(ctx, getStateRequest())
		return getErr == nil && !resp.Msg.GetIsPaused() && resp.Msg.GetSnapshot().GetTickHeight() > 0
	}, 5*time.Second, 5*time.Millisecond)

	// Pause: the view flips to paused immediately (no tick needed), and its height is the last
	// executed tick — one less than the pause reply, which reports the next tick to run.
	pauseResp, err := d.Pause(ctx, connect.NewRequest(&cardinalv1.PauseRequest{}))
	require.NoError(t, err)
	state := getState()
	assert.True(t, state.GetIsPaused())
	assert.Equal(t, pauseResp.Msg.GetTickHeight()-1, state.GetSnapshot().GetTickHeight())

	before := countEntities(state.GetSnapshot().GetWorldState())
	require.Positive(t, before, "spawner must have created entities while running")

	// Step: exactly one tick runs, and its result is visible in GetState immediately.
	stepResp, err := d.Step(ctx, connect.NewRequest(&cardinalv1.StepRequest{}))
	require.NoError(t, err)
	state = getState()
	assert.True(t, state.GetIsPaused(), "stepping must not unpause the view")
	assert.Equal(t, stepResp.Msg.GetTickHeight()-1, state.GetSnapshot().GetTickHeight())
	assert.Equal(t, before+1, countEntities(state.GetSnapshot().GetWorldState()),
		"the stepped tick's state must be visible in GetState immediately")

	// Reset: no tick runs, but the view must be refreshed to the fresh world.
	_, err = d.Reset(ctx, connect.NewRequest(&cardinalv1.ResetRequest{}))
	require.NoError(t, err)
	state = getState()
	assert.True(t, state.GetIsPaused())
	assert.Equal(t, uint64(0), state.GetSnapshot().GetTickHeight())
	assert.Zero(t, countEntities(state.GetSnapshot().GetWorldState()),
		"the view must reflect the reset world, not the pre-reset one")

	// Resume: the paused flag clears without waiting for the next tick's publish.
	_, err = d.Resume(ctx, connect.NewRequest(&cardinalv1.ResumeRequest{}))
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		resp, getErr := d.GetState(ctx, getStateRequest())
		return getErr == nil && !resp.Msg.GetIsPaused()
	}, 5*time.Second, 5*time.Millisecond)
}

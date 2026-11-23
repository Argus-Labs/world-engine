package ecs

import (
	"sync"
	"testing"

	"github.com/argus-labs/world-engine/pkg/assert"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// World represents the root ECS state.
type World struct {
	state               *worldState
	initialized         bool                  // True once Init has completed; reset to false by Reset
	systems             [4][]systemMetadata   // Systems for each hook (PreUpdate, Update, PostUpdate, Init)
	systemEvents        systemEventManager    // Manages system events
	onComponentRegister func(Component) error // Callback called when a component is registered

	// searchSnapshots reuses IterEntities buffers so a search doesn't allocate once warm. A pool
	// rather than one buffer keeps nested, interleaved, and concurrent read-only searches safe.
	searchSnapshots sync.Pool
}

// NewWorld creates a new World instance.
func NewWorld() *World {
	return &World{
		state:        newWorldState(),
		systemEvents: newSystemEventManager(),
	}
}

// Init runs init systems once.
func (w *World) Init() {
	assert.That(!w.initialized, "Init called when world is already initialized")

	for _, system := range w.systems[Init] {
		system.fn()
	}

	w.initialized = true
}

// Tick executes the registered systems in order: PreUpdate, Update, PostUpdate.
// System events are cleared after each tick.
func (w *World) Tick() {
	assert.That(w.initialized, "Tick called before initialization")

	// Clear system events after each tick.
	defer w.systemEvents.clear()

	// Run the systems sequentially.
	for _, phase := range []Phase{PreUpdate, Update, PostUpdate} {
		for _, system := range w.systems[phase] {
			system.fn()
		}
	}
}

// ClearSystemEvents drops every pending system event, as Tick does when it returns.
func (w *World) ClearSystemEvents() {
	w.systemEvents.clear()
}

// Reset clears the world state back to its initial empty state.
// Components remain registered but all entities and archetypes are cleared.
func (w *World) Reset() {
	w.state.reset()
	w.systemEvents.clear()
	w.initialized = false
}

// Schedules returns the systems for all execution phases.
func (w *World) Schedules() []ScheduleInfo {
	schedules := make([]ScheduleInfo, 4)
	for phase := range 4 {
		systems := make([]SystemInfo, len(w.systems[phase]))
		for i, sys := range w.systems[phase] {
			systems[i] = SystemInfo{
				ID:   i,
				Name: sys.name,
			}
		}

		schedules[phase] = ScheduleInfo{Phase: Phase(phase), Systems: systems}
	}

	return schedules
}

// LiveEntityIDs returns the IDs that are currently present in the world.
// This exists only for testing helpers such as DST and e2e fuzzing.
func (w *World) LiveEntityIDs() []EntityID {
	w.state.mu.Lock()
	defer w.state.mu.Unlock()

	ids := make([]EntityID, 0)

	for index, slot := range w.state.slots {
		if slot.archetype == noArchetype {
			continue
		}

		ids = append(ids, newEntityID(uint32(index), slot.generation))
	}

	return ids
}

func (w *World) OnComponentRegister(callback func(zero Component) error) {
	w.onComponentRegister = callback
}

// -------------------------------------------------------------------------------------------------
// Serialization methods
// -------------------------------------------------------------------------------------------------

// ToProto captures the world with MessagePack component payloads. Call only while the tick owns it.
func (w *World) ToProto() (*cardinalv1.WorldState, error) {
	return w.state.toProto()
}

// FromProto populates the World's state from a proto message.
// This should only be called after the World has been properly initialized with components registered.
func (w *World) FromProto(pb *cardinalv1.WorldState) error {
	if err := w.state.fromProto(pb); err != nil {
		return err
	}

	return nil
}

// EncodeState appends a serialized WorldState for test harnesses and serializability checks.
// Like snapshot production, it fails fast if a component cannot be encoded.
func (w *World) EncodeState(buf []byte) []byte {
	state, err := w.ToProto()
	if err != nil {
		panic(err)
	}

	data, err := proto.MarshalOptions{}.MarshalAppend(buf, state)
	if err != nil {
		panic(err)
	}

	return data
}

// -------------------------------------------------------------------------------------------------
// Test helpers
// -------------------------------------------------------------------------------------------------

// CheckWorld checks structural ECS invariants that must always hold regardless of game logic.
// It fails the test with a descriptive message on the first violation found.
func (w *World) CheckWorld(t *testing.T) {
	t.Helper()

	ws := w.state

	// Invariant: void archetype (index 0) always exists, has no components, and no columns.
	require.NotEmpty(t, ws.archetypes, "archetypes array is empty (missing void archetype)")
	require.Equal(t, 0, ws.archetypes[voidArchetypeID].components.Count(),
		"void archetype has non-empty components bitmap")
	require.Empty(t, ws.archetypes[voidArchetypeID].columns,
		"void archetype has columns")

	// Collect all live entities from archetypes (ground truth).
	liveEntities := make(map[EntityID]int)

	for aid, arch := range ws.archetypes {
		// Invariant: archetype ID matches its index in the array.
		require.Equal(t, aid, arch.id, "archetype at index %d has id %d", aid, arch.id)

		// Invariant: compCount matches components.Count() and len(columns).
		require.Equal(t, arch.components.Count(), arch.compCount,
			"archetype %d: compCount %d != components.Count() %d",
			aid, arch.compCount, arch.components.Count())
		require.Equal(t, len(arch.columns), arch.compCount,
			"archetype %d: len(columns) %d != compCount %d",
			aid, len(arch.columns), arch.compCount)

		// Invariant: every column length matches entity count.
		for _, col := range arch.columns {
			require.Equal(t, len(arch.entities), col.len(),
				"archetype %d: column %s length %d != entity count %d",
				aid, col.name(), col.len(), len(arch.entities))
		}

		// Invariant: every bit in the components bitmap corresponds to a registered component.
		arch.components.Range(func(cid uint32) {
			require.Less(t, cid, ws.components.nextID,
				"archetype %d: component ID %d not registered (nextID=%d)",
				aid, cid, ws.components.nextID)
		})

		// Invariant: rows sparseSet is a bijection between entities and row indices [0, len).
		rowsSeen := make(map[int]EntityID, len(arch.entities))
		for _, eid := range arch.entities {
			row, exists := arch.rows.get(eid.Index())
			require.True(t, exists,
				"archetype %d: entity %d has no row entry", aid, eid)
			require.Less(t, row, len(arch.entities),
				"archetype %d: entity %d row %d out of bounds (len=%d)",
				aid, eid, row, len(arch.entities))
			otherEid, dup := rowsSeen[row]
			require.False(t, dup,
				"archetype %d: entities %d and %d share row %d", aid, otherEid, eid, row)
			rowsSeen[row] = eid
		}

		// Invariant: no entity appears in multiple archetypes.
		for _, eid := range arch.entities {
			existingAid, exists := liveEntities[eid]
			require.False(t, exists, "entity %d in archetype %d and %d", eid, existingAid, aid)
			liveEntities[eid] = aid
		}
	}

	// Invariant: the entity table matches archetype membership, including the generation.
	for eid, expectedAid := range liveEntities {
		require.Less(t, int(eid.Index()), len(ws.slots), "entity %d index beyond the entity table", eid)
		slot := ws.slots[eid.Index()]
		require.Equal(t, expectedAid, int(slot.archetype),
			"entity %d: entity table archetype=%d but in archetype %d", eid, slot.archetype, expectedAid)
		require.Equal(t, slot.generation, eid.Generation(),
			"entity %d: entity table generation=%d", eid, slot.generation)
		require.NotEqual(t, uint32(retiredGeneration), eid.Generation(), "entity %d has the retired generation", eid)
	}

	// Invariant: every occupied slot in the entity table corresponds to a live entity.
	retired := 0

	for index, slot := range ws.slots {
		if slot.archetype == noArchetype {
			if slot.generation == retiredGeneration {
				retired++
			}

			continue
		}

		eid := newEntityID(uint32(index), slot.generation)
		_, exists := liveEntities[eid]
		require.True(t, exists,
			"entity table has entity %d -> archetype %d but entity not in any archetype", eid, slot.archetype)
	}

	// Invariant: free list has no duplicates, and every free index is unoccupied and not retired.
	freeSeen := make(map[uint32]struct{}, len(ws.free))
	for _, index := range ws.free {
		_, dup := freeSeen[index]
		require.False(t, dup, "duplicate free index %d", index)
		freeSeen[index] = struct{}{}

		require.Less(t, int(index), len(ws.slots), "free index %d beyond the entity table", index)
		slot := ws.slots[index]
		require.Equal(t, int32(noArchetype), slot.archetype, "index %d is both free and live", index)
		require.NotEqual(t, uint32(retiredGeneration), slot.generation, "index %d is both free and retired", index)
	}

	// Invariant: every index is live, free, or retired (no gaps).
	require.Len(t, ws.slots, len(liveEntities)+len(ws.free)+retired,
		"entity table has %d slots but live=%d + free=%d + retired=%d",
		len(ws.slots), len(liveEntities), len(ws.free), retired)
}

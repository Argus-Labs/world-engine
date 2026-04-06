package ecs

import (
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/assert"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/stretchr/testify/require"
)

// World represents the root ECS state.
type World struct {
	state               *worldState
	initialized         bool                  // True once Init has completed; reset to false by Reset
	initSystems         []initSystem          // Initialization systems, run once per world initialization
	scheduler           [3]systemScheduler    // Systems schedulers (PreTick, Update, PostTick)
	systemEvents        systemEventManager    // Manages system events
	onComponentRegister func(Component) error // Callback called when a component is registered
	disk                *diskStore            // Disk-backed component storage (nil if no disk components)
	diskStoragePath     string                // Path for disk storage (from config)
}

// NewWorld creates a new World instance.
func NewWorld() *World {
	world := &World{
		state:        newWorldState(),
		initSystems:  make([]initSystem, 0),
		scheduler:    [3]systemScheduler{},
		systemEvents: newSystemEventManager(),
	}

	systemHookNames := [3]SystemHook{PreUpdate, Update, PostUpdate}
	for i := range world.scheduler {
		world.scheduler[i] = newSystemScheduler()
		world.scheduler[i].systemHook = systemHookNames[i]
	}

	return world
}

// Init initializes system schedulers by creating their schedules and runs init systems.
func (w *World) Init() {
	assert.That(!w.initialized, "Init called when world is already initialized")

	for i := range w.scheduler {
		w.scheduler[i].createSchedule()
	}

	for _, system := range w.initSystems {
		system.fn()
	}

	w.initialized = true
}

// Tick passes external events into the event manager and executes the
// registered systems in order. If any system returns an error, the entire tick is considered
// failed, changes are discarded, and the error is returned. If the tick succeeds, the events
// emmitted during the tick is returned.
func (w *World) Tick() {
	assert.That(w.initialized, "Tick called before initialization")

	// Clear system events after each tick.
	defer w.systemEvents.clear()

	// Run the systems.
	for i := range w.scheduler {
		w.scheduler[i].Run()
	}
}

// Reset clears the world state back to its initial empty state.
// Components remain registered but all entities and archetypes are cleared.
// Disk storage is also cleared to prevent stale data from reused entity IDs.
func (w *World) Reset() {
	w.state.reset()
	w.initialized = false
	if w.disk != nil {
		w.disk.reset()
	}
}

// SetOnSystemRun sets a callback invoked after each system execution.
// Must be called before Init.
func (w *World) OnSystemRun(fn func(name string, systemHook SystemHook, startTime, endTime time.Time)) {
	for i := range w.scheduler {
		w.scheduler[i].onSystemRun = fn
	}
}

// Schedules returns the dependency graphs for all execution phases.
func (w *World) Schedules() []ScheduleInfo {
	schedules := make([]ScheduleInfo, len(w.scheduler))
	for i := range w.scheduler {
		schedules[i] = w.scheduler[i].scheduleInfo()
	}
	return schedules
}

// LiveEntityIDs returns the IDs that are currently present in the world.
// This exists only for testing helpers such as DST and e2e fuzzing.
func (w *World) LiveEntityIDs() []EntityID {
	w.state.mu.Lock()
	defer w.state.mu.Unlock()

	ids := make([]EntityID, 0)
	for i, aid := range w.state.entityArch {
		if aid == sparseTombstone {
			continue
		}
		ids = append(ids, EntityID(i)) //nolint:gosec // entityArch indices are entity IDs
	}
	return ids
}

func (w *World) OnComponentRegister(callback func(zero Component) error) {
	w.onComponentRegister = callback
}

// SetDiskStoragePath configures the path for disk-backed components.
// The disk store itself is created lazily when the first disk component is registered.
func (w *World) SetDiskStoragePath(path string) {
	w.diskStoragePath = path
}

// compactableDiskColumn is implemented by diskColumn[T] to support compaction.
type compactableDiskColumn interface {
	collectLiveRecords() ([]compactRecord, error)
	applyCompactedOffsets(records []compactRecord)
}

// columnRecords tracks a column and its record range in the allRecords slice.
type columnRecords struct {
	col        compactableDiskColumn
	start, end int
}

// CompactDisk rewrites the data file with only live entries from all disk columns.
func (w *World) CompactDisk() error {
	if w.disk == nil {
		return nil
	}

	// Collect all live records from all disk columns, tracking per-column ranges.
	var allRecords []compactRecord
	var columns []columnRecords
	for _, arch := range w.state.archetypes {
		for _, col := range arch.columns {
			if dc, ok := col.(compactableDiskColumn); ok {
				start := len(allRecords)
				records, err := dc.collectLiveRecords()
				if err != nil {
					return err
				}
				allRecords = append(allRecords, records...)
				columns = append(columns, columnRecords{col: dc, start: start, end: len(allRecords)})
			}
		}
	}

	// Rewrite the file. compact() updates offset/size in each record.
	if err := w.disk.compact(allRecords); err != nil {
		return err
	}

	// Apply the new offsets back to each column's entries.
	for _, cr := range columns {
		cr.col.applyCompactedOffsets(allRecords[cr.start:cr.end])
	}
	return nil
}

// CloseDisk closes the disk store's data file.
func (w *World) CloseDisk() error {
	if w.disk == nil {
		return nil
	}
	return w.disk.close()
}

// DiskStore returns the disk store, or nil if disk storage is not enabled.
func (w *World) DiskStore() *diskStore {
	return w.disk
}

// flushableDiskColumn is implemented by diskColumn[T] to support flushing pending writes.
type flushableDiskColumn interface {
	flush() error
}

// FlushDisk writes all modified disk components back to disk and clears pending buffers.
// Called after all systems finish in a tick.
func (w *World) FlushDisk() error {
	if w.disk == nil {
		return nil
	}
	// Walk all archetypes and flush any disk columns.
	for _, arch := range w.state.archetypes {
		for _, col := range arch.columns {
			if dc, ok := col.(flushableDiskColumn); ok {
				if err := dc.flush(); err != nil {
					return err
				}
			}
		}
	}
	return nil
}


// ExportDiskFile returns the raw Bitcask file bytes for inclusion in the snapshot.
// Must flush before calling. Returns nil if disk storage is not enabled.
func (w *World) ExportDiskFile() ([]byte, error) {
	if w.disk == nil {
		return nil, nil
	}
	return w.disk.readAll()
}

// ImportDiskFile writes the raw Bitcask file bytes from a snapshot.
// Must be called BEFORE FromProto so that diskColumn offsets point to valid data.
func (w *World) ImportDiskFile(data []byte) error {
	if w.disk == nil {
		return nil
	}
	// nil/empty data means snapshot had no disk state; reset the file to avoid stale offsets.
	if len(data) == 0 {
		w.disk.reset()
		return nil
	}
	return w.disk.writeAll(data)
}

// -------------------------------------------------------------------------------------------------
// Serialization methods
// -------------------------------------------------------------------------------------------------

// ToProto converts the World's state to a proto message.
// Only serializes the WorldState as components, systems, and managers are recreated on startup.
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

// -------------------------------------------------------------------------------------------------
// Test helpers
// -------------------------------------------------------------------------------------------------

// CheckWorld checks structural ECS invariants that must always hold regardless of game logic.
// It fails the test with a descriptive message on the first violation found.
func CheckWorld(t *testing.T, w *World) {
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
			row, exists := arch.rows.get(eid)
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

	// Invariant: entityArch mapping matches archetype membership.
	for eid, expectedAid := range liveEntities {
		aid, exists := ws.entityArch.get(eid)
		require.True(t, exists, "entity %d in archetype %d but not in entityArch", eid, expectedAid)
		require.Equal(t, expectedAid, aid,
			"entity %d: entityArch=%d but in archetype %d", eid, aid, expectedAid)
	}

	// Invariant: every non-tombstone entry in entityArch corresponds to a live entity.
	for i, val := range ws.entityArch {
		if val == sparseTombstone {
			continue
		}
		eid := EntityID(i) //nolint:gosec // sparset max length is entity id max
		_, exists := liveEntities[eid]
		require.True(t, exists,
			"entityArch has entity %d -> archetype %d but entity not in any archetype", eid, val)
	}

	// Invariant: free list has no duplicates.
	freeSeen := make(map[EntityID]struct{}, len(ws.free))
	for _, freeID := range ws.free {
		_, dup := freeSeen[freeID]
		require.False(t, dup, "duplicate free ID %d", freeID)
		freeSeen[freeID] = struct{}{}
	}

	// Invariant: no overlap between free IDs and live entity IDs.
	for _, freeID := range ws.free {
		aid, exists := liveEntities[freeID]
		require.False(t, exists, "entity %d is both free and live (archetype %d)", freeID, aid)
		// Invariant: all free IDs < nextID.
		require.Less(t, freeID, ws.nextID, "free ID %d >= nextID %d", freeID, ws.nextID)
	}

	// Invariant: all live IDs < nextID.
	for eid := range liveEntities {
		require.Less(t, eid, ws.nextID, "live entity %d >= nextID %d", eid, ws.nextID)
	}

	// Invariant: every ID below nextID is either live or free (no gaps).
	require.Equal(t, int(ws.nextID), len(liveEntities)+len(ws.free),
		"nextID=%d but live=%d + free=%d = %d",
		ws.nextID, len(liveEntities), len(ws.free), len(liveEntities)+len(ws.free))
}

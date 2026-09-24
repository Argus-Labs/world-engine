// Package ecs_test drives snapshot migration from outside the package, through the same exported
// API cardinal uses.
//
// It is an external test package on purpose. The fixture's migration package declares migrations,
// which means it imports ecs; a test inside package ecs could not import it back without a cycle.
//
// # Coverage
//
// Every case in the tree from ADR-065 has a test here. The ones that need more than one entity
// live at the bottom of the file, since they run a second kind of migration.
//
//	1a  add a field                  TestMigrateAddedField
//	1b  remove a field               TestMigrateRemovedField
//	1c  rename a field               TestMigrateRenamedField
//	1d  reorder fields               TestMigrateReorderedFields
//	1e  retype a field               TestMigrateRetypedField
//	1f  change of meaning            by design, none: the struct is identical, so no hash can see it
//	2a  split one into many          TestMigrateSplitComponent
//	2b  merge many into new          TestMigrateMergesIntoNewComponent
//	2c  merge many into existing     TestMigrateAbsorbsIntoExisting
//	2d  move between components      TestMigrateMovesBetweenComponents
//	2e  drop a component             TestMigrateDroppedComponent
//	3a  compute from other entities  TestMigrateRanksAcrossEntities
//	3b  create or destroy entities   TestMigrateCreatesEntities
//	3c  fix references               TestMigrateResolvesReferences
//
// 1f is the one case with nothing to run. Metres becoming centimetres leaves every byte and every
// hash identical, so no engine can detect it; the ADR assigns it to the developer and so does this.
//
// Ordering is a second axis in the same ADR, about how migrations fit together rather than what
// any one of them does:
//
//	chain across releases            TestMigrateRunsChain
//	route loops                      TestMigrateRefusesCycle
//	no route registered              TestMigrateRefusesUnknownShape
//	name no longer registered        TestMigrateRefusesUnregisteredComponent
//	shape unchanged                  TestMigrateSkipsMatchingShapes
//	restart runs nothing whole-world TestMigrateSkipsWorldMigrationOnRestart
//	declared by a plugin             TestMigrateDeclaredByPlugin, in pkg/cardinal
//
// # Known deviation from the ADR
//
// The ADR interleaves per-entity and whole-world migrations in passes, so a whole-world migration
// could feed a per-entity one. This build runs every per-entity migration first and every
// whole-world one afterwards, as a single pass. All fifteen cases are reachable that way, and no
// case in the tree needs the other direction.
package ecs_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	// Both fixture builds name their component package component, the way a shard does, so the
	// import has to say which build each name comes from.
	early "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/earlyaccess/component"
	full "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/fullrelease/component"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/fullrelease/migration"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// The values the early-access world holds. They are written out here and asserted against by hand
// rather than derived, because a test that computes what it expects from the same source the code
// used can pass while the code is wrong.
const (
	renamedStored       uint32 = 7
	unchangedStored     uint32 = 3
	loneUnchangedStored uint32 = 11
)

// The stored value the chain starts from and what its two steps make of it. Written out rather
// than derived: a test computing its expectation from the code under test can agree with a bug.
const (
	chainedStored uint32 = 5
	chainedWantB  uint32 = 10
	chainedWantC  uint32 = 15
)

// storedSave builds a world from the early-access component shapes and encodes it, returning the
// snapshot a shard of that build would have written.
//
// Two entities, carrying different component sets: one with both components, one with only
// unchanged. Distinct sets mean restore has to decide what to migrate per entity rather than once
// for the whole world.
func storedSave(t *testing.T) (*cardinalv1.WorldState, ecs.EntityID, ecs.EntityID) {
	t.Helper()

	world := ecs.NewWorld()
	_, err := world.RegisterComponent[early.Unchanged]()
	require.NoError(t, err)
	_, err = world.RegisterComponent[early.Renamed]()
	require.NoError(t, err)

	both := world.Create()
	require.NoError(t, world.Set(both, early.Unchanged{A: unchangedStored}))
	require.NoError(t, world.Set(both, early.Renamed{Before: renamedStored}))

	lone := world.Create()
	require.NoError(t, world.Set(lone, early.Unchanged{A: loneUnchangedStored}))

	var state cardinalv1.WorldState
	require.NoError(t, proto.Unmarshal(world.EncodeState(nil), &state))
	return &state, both, lone
}

// TestMigrateRenamedField restores an early-access save into a full-release world.
//
// A rename is the case where a wrong answer looks right: the field keeps its number and wire type,
// so the stored bytes decode into the new struct and land the correct number in After whether or
// not a migration runs. The values alone therefore prove nothing, which is why this test asserts
// that the migration ran for renamed and did NOT run for unchanged.
func TestMigrateRenamedField(t *testing.T) {
	t.Parallel()

	save, both, lone := storedSave(t)

	world := ecs.NewWorld()
	_, err := world.RegisterComponent[full.Unchanged]()
	require.NoError(t, err)
	_, err = world.RegisterComponent[full.Renamed]()
	require.NoError(t, err)

	rename := &migration.RenamedV1ToCurrent{}
	require.NoError(t, world.RegisterMigration(rename))

	require.NoError(t, world.FromProto(save))

	// The migration ran once: for the one entity carrying renamed, and not for the one without it.
	assert.Equal(t, 1, rename.Calls)

	renamed, err := world.Get[full.Renamed](both)
	require.NoError(t, err)
	assert.Equal(t, full.Renamed{After: renamedStored}, renamed)

	// The control passed through untouched. A runtime that converted everything it found would
	// fail here, which is the only reason the rename assertion above means anything.
	unchanged, err := world.Get[full.Unchanged](both)
	require.NoError(t, err)
	assert.Equal(t, full.Unchanged{A: unchangedStored}, unchanged)

	// The entity that never carried renamed still has its own value, and did not acquire one.
	loneUnchanged, err := world.Get[full.Unchanged](lone)
	require.NoError(t, err)
	assert.Equal(t, full.Unchanged{A: loneUnchangedStored}, loneUnchanged)
	assert.False(t, world.Has[full.Renamed](lone),
		"restoring must not give an entity a component its save never held")
}

// TestMigrateRefusesUnknownShape: the same save, restored by a build that declares the new shape
// but registers no migration for the old one. There is nothing that can convert the stored bytes,
// so the restore stops rather than decoding them as the current struct.
//
// Without this, the previous test could pass on a build that ignored shapes entirely.
func TestMigrateRefusesUnknownShape(t *testing.T) {
	t.Parallel()

	save, _, _ := storedSave(t)

	world := ecs.NewWorld()
	_, err := world.RegisterComponent[full.Unchanged]()
	require.NoError(t, err)
	_, err = world.RegisterComponent[full.Renamed]()
	require.NoError(t, err)

	err = world.FromProto(save)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "renamed")
}

// TestMigrateSkipsMatchingShapes: a save written and read by the same build. Every stored shape
// matches, so no migration runs and the values arrive unchanged. This is what an ordinary restart
// is, and it must not become a migration just because one is registered.
func TestMigrateSkipsMatchingShapes(t *testing.T) {
	t.Parallel()

	source := ecs.NewWorld()
	_, err := source.RegisterComponent[full.Unchanged]()
	require.NoError(t, err)
	_, err = source.RegisterComponent[full.Renamed]()
	require.NoError(t, err)

	entity := source.Create()
	require.NoError(t, source.Set(entity, full.Renamed{After: renamedStored}))

	var save cardinalv1.WorldState
	require.NoError(t, proto.Unmarshal(source.EncodeState(nil), &save))

	world := ecs.NewWorld()
	_, err = world.RegisterComponent[full.Unchanged]()
	require.NoError(t, err)
	_, err = world.RegisterComponent[full.Renamed]()
	require.NoError(t, err)

	rename := &migration.RenamedV1ToCurrent{}
	require.NoError(t, world.RegisterMigration(rename))
	require.NoError(t, world.FromProto(&save))

	assert.Equal(t, 0, rename.Calls, "a restart must not migrate an already-current save")

	renamed, err := world.Get[full.Renamed](entity)
	require.NoError(t, err)
	assert.Equal(t, full.Renamed{After: renamedStored}, renamed)
}

// TestMigrateRunsChain restores a save holding the oldest shape of chained into a build
// that has moved two versions past it.
//
// Neither registered migration converts the stored shape into the current one on its own. The first
// produces a shape no save holds and no entity can carry; the second is the only thing that can
// consume it. Getting a correct entity out the other end means the engine found the route and ran
// both steps in order.
func TestMigrateRunsChain(t *testing.T) {
	t.Parallel()

	source := ecs.NewWorld()
	_, err := source.RegisterComponent[early.Chained]()
	require.NoError(t, err)

	entity := source.Create()
	require.NoError(t, source.Set(entity, early.Chained{A: chainedStored}))

	var save cardinalv1.WorldState
	require.NoError(t, proto.Unmarshal(source.EncodeState(nil), &save))

	world := ecs.NewWorld()
	_, err = world.RegisterComponent[full.Chained]()
	require.NoError(t, err)

	// Registered last step first, on purpose. The order the two have to run in is fixed by the
	// shapes they declare, and must not depend on the order a developer happened to register them.
	second := &migration.ChainedV2ToCurrent{}
	first := &migration.ChainedV1ToV2{}
	require.NoError(t, world.RegisterMigration(second))
	require.NoError(t, world.RegisterMigration(first))

	require.NoError(t, world.FromProto(&save))

	chained, err := world.Get[full.Chained](entity)
	require.NoError(t, err)
	assert.Equal(t, full.Chained{B: chainedWantB, C: chainedWantC}, chained)

	assert.Equal(t, 1, first.Calls, "the first step must run once")
	assert.Equal(t, 1, second.Calls, "the second step must run once")
}

// TestMigrateRefusesCycle registers a third migration that converts the current shape
// back to the oldest one, closing the loop.
//
// Left unchecked this is not a wrong answer but a walk with no end. The engine works a route out
// once per stored shape, and that walk is where the loop shows up, so the cost of finding it is
// nothing on top of the walk already being done.
func TestMigrateRefusesCycle(t *testing.T) {
	t.Parallel()

	source := ecs.NewWorld()
	_, err := source.RegisterComponent[early.Chained]()
	require.NoError(t, err)
	require.NoError(t, source.Set(source.Create(), early.Chained{A: chainedStored}))

	var save cardinalv1.WorldState
	require.NoError(t, proto.Unmarshal(source.EncodeState(nil), &save))

	world := ecs.NewWorld()
	_, err = world.RegisterComponent[full.Chained]()
	require.NoError(t, err)

	require.NoError(t, world.RegisterMigration(&migration.ChainedV1ToV2{}))
	require.NoError(t, world.RegisterMigration(&migration.ChainedV2ToCurrent{}))
	require.NoError(t, world.RegisterMigration(&migration.ChainedCurrentToV1{}))

	err = world.FromProto(&save)
	require.Error(t, err)

	// The message names the loop, so the migration to delete is in the error rather than something
	// to work out afterwards. Which migration it starts from is fixed by the same type-name order
	// the planner uses, not by which save happened to trigger it, so the text is stable.
	assert.Contains(t, err.Error(), "migrations form a loop")
	assert.Contains(t, err.Error(), "ChainedCurrentToV1 then migration.ChainedV1ToV2 "+
		"then migration.ChainedV2ToCurrent then migration.ChainedCurrentToV1")
}

// The stored values for the single-component cases below, and what each conversion makes of them.
// Distinct per case so a test cannot pass on a value that belongs to another.
const (
	addedStored      uint32 = 21
	removedStoredA   uint32 = 31
	removedStoredB   uint32 = 32
	reorderedStoredA uint32 = 41
	reorderedStoredB uint32 = 42
	retypedStored    uint32 = 51
	splitStoredA     uint32 = 61
	splitStoredB     uint32 = 62
)

// storedOne encodes a world holding one entity with one component, which is all the single-case
// tests below need.
func storedOne[T ecs.Component](t *testing.T, value T) (*cardinalv1.WorldState, ecs.EntityID) {
	t.Helper()

	world := ecs.NewWorld()
	_, err := world.RegisterComponent[T]()
	require.NoError(t, err)

	entity := world.Create()
	require.NoError(t, world.Set(entity, value))

	var state cardinalv1.WorldState
	require.NoError(t, proto.Unmarshal(world.EncodeState(nil), &state))
	return &state, entity
}

// TestMigrateAddedField covers case 1a. The stored value has no B, and the migration decides what
// B becomes rather than leaving it at the zero a decoder would produce.
func TestMigrateAddedField(t *testing.T) {
	t.Parallel()

	save, entity := storedOne(t, early.Added{A: addedStored})

	world := ecs.NewWorld()
	_, err := world.RegisterComponent[full.Added]()
	require.NoError(t, err)
	m := &migration.AddedV1ToCurrent{}
	require.NoError(t, world.RegisterMigration(m))
	require.NoError(t, world.FromProto(save))

	got, err := world.Get[full.Added](entity)
	require.NoError(t, err)
	assert.Equal(t, full.Added{A: addedStored, B: migration.AddedDefault}, got)
	assert.Equal(t, 1, m.Calls)
}

// TestMigrateRemovedField covers case 1b. B is still in the stored bytes and has nowhere to go in
// the current struct; the retired shape is what lets the migration see it before it is dropped.
func TestMigrateRemovedField(t *testing.T) {
	t.Parallel()

	save, entity := storedOne(t, early.Removed{A: removedStoredA, B: removedStoredB})

	world := ecs.NewWorld()
	_, err := world.RegisterComponent[full.Removed]()
	require.NoError(t, err)
	m := &migration.RemovedV1ToCurrent{}
	require.NoError(t, world.RegisterMigration(m))
	require.NoError(t, world.FromProto(save))

	got, err := world.Get[full.Removed](entity)
	require.NoError(t, err)
	assert.Equal(t, full.Removed{A: removedStoredA}, got)
	assert.Equal(t, 1, m.Calls)
}

// TestMigrateReorderedFields covers case 1d, the case that fails worst when it goes unnoticed.
//
// Both structs hold the same two names with the same two wire types, so the stored bytes decode
// into the current struct with no error at all — putting A's value in B and B's in A. The
// assertion below is what a silent swap would fail, and the shape hash covering declaration order
// is what makes the migration run in the first place.
func TestMigrateReorderedFields(t *testing.T) {
	t.Parallel()

	save, entity := storedOne(t, early.Reordered{A: reorderedStoredA, B: reorderedStoredB})

	world := ecs.NewWorld()
	_, err := world.RegisterComponent[full.Reordered]()
	require.NoError(t, err)
	m := &migration.ReorderedV1ToCurrent{}
	require.NoError(t, world.RegisterMigration(m))
	require.NoError(t, world.FromProto(save))

	got, err := world.Get[full.Reordered](entity)
	require.NoError(t, err)
	assert.Equal(t, full.Reordered{A: reorderedStoredA, B: reorderedStoredB}, got)
	assert.Equal(t, 1, m.Calls)
}

// TestMigrateRetypedField covers case 1e. The field goes from varint to fixed32, so the current
// struct cannot decode the stored bytes at all and the retired shape is the only reader.
func TestMigrateRetypedField(t *testing.T) {
	t.Parallel()

	save, entity := storedOne(t, early.Retyped{A: retypedStored})

	world := ecs.NewWorld()
	_, err := world.RegisterComponent[full.Retyped]()
	require.NoError(t, err)
	m := &migration.RetypedV1ToCurrent{}
	require.NoError(t, world.RegisterMigration(m))
	require.NoError(t, world.FromProto(save))

	got, err := world.Get[full.Retyped](entity)
	require.NoError(t, err)
	assert.Equal(t, full.Retyped{A: float32(retypedStored)}, got)
	assert.Equal(t, 1, m.Calls)
}

// TestMigrateSplitComponent covers case 2a: one stored component becomes two, so the entity ends
// up in an archetype its save never described.
func TestMigrateSplitComponent(t *testing.T) {
	t.Parallel()

	save, entity := storedOne(t, early.Split{A: splitStoredA, B: splitStoredB})

	world := ecs.NewWorld()
	_, err := world.RegisterComponent[full.Split]()
	require.NoError(t, err)
	_, err = world.RegisterComponent[full.SplitOff]()
	require.NoError(t, err)
	m := &migration.SplitV1ToCurrent{}
	require.NoError(t, world.RegisterMigration(m))
	require.NoError(t, world.FromProto(save))

	kept, err := world.Get[full.Split](entity)
	require.NoError(t, err)
	assert.Equal(t, full.Split{A: splitStoredA}, kept)

	// The component the save never held, produced by the migration alone.
	off, err := world.Get[full.SplitOff](entity)
	require.NoError(t, err)
	assert.Equal(t, full.SplitOff{B: splitStoredB}, off)

	assert.Equal(t, 1, m.Calls)
}

// droppedStored is the value of the component that goes away, and droppedKept the value of the one
// beside it that must survive.
const (
	droppedStored uint32 = 71
	droppedKept   uint32 = 72
)

// storedDropped encodes a world whose entity holds both a component the current build still
// declares and one it does not.
func storedDropped(t *testing.T) (*cardinalv1.WorldState, ecs.EntityID) {
	t.Helper()

	world := ecs.NewWorld()
	_, err := world.RegisterComponent[early.Unchanged]()
	require.NoError(t, err)
	_, err = world.RegisterComponent[early.Dropped]()
	require.NoError(t, err)

	entity := world.Create()
	require.NoError(t, world.Set(entity, early.Unchanged{A: droppedKept}))
	require.NoError(t, world.Set(entity, early.Dropped{A: droppedStored}))

	var state cardinalv1.WorldState
	require.NoError(t, proto.Unmarshal(world.EncodeState(nil), &state))
	return &state, entity
}

// TestMigrateDroppedComponent covers case 2e. The current build does not declare dropped at all,
// so there is no shape to compare the stored bytes against and the value can only leave through a
// migration that declares no output.
func TestMigrateDroppedComponent(t *testing.T) {
	t.Parallel()

	save, entity := storedDropped(t)

	world := ecs.NewWorld()
	_, err := world.RegisterComponent[full.Unchanged]()
	require.NoError(t, err)

	m := &migration.DroppedV1ToNothing{}
	require.NoError(t, world.RegisterMigration(m))
	require.NoError(t, world.FromProto(save))

	assert.Equal(t, 1, m.Calls)

	// The entity survives the loss of one component with the other intact, which is what proves the
	// stored column was consumed rather than the whole restore being skipped.
	kept, err := world.Get[full.Unchanged](entity)
	require.NoError(t, err)
	assert.Equal(t, full.Unchanged{A: droppedKept}, kept)
}

// TestMigrateRefusesUnregisteredComponent is the 2e counterpart to TestMigrateRefusesUnknownShape:
// same save, no migration declared for the component this build dropped.
//
// It takes a different path through the engine and gets a different message, because there is no
// stored shape to compare here — the name itself is unknown.
func TestMigrateRefusesUnregisteredComponent(t *testing.T) {
	t.Parallel()

	save, _ := storedDropped(t)

	world := ecs.NewWorld()
	_, err := world.RegisterComponent[full.Unchanged]()
	require.NoError(t, err)

	err = world.FromProto(save)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not register")
	assert.Contains(t, err.Error(), "dropped")
}

// Stored values for the multi-input cases. Distinct per case and per field so a value landing in
// the wrong slot shows up as a wrong number rather than a plausible one.
const (
	mergeLeftStored  uint32 = 81
	mergeRightStored uint32 = 82
	absorberStored   uint32 = 91
	absorbedStored   uint32 = 92
	absorberLone     uint32 = 93
	movedFromAStored uint32 = 101
	movedFromBStored uint32 = 102
	movedToCStored   uint32 = 103
)

// TestMigrateMergesIntoNewComponent covers case 2b: two components the current build no longer
// declares become one that it does.
//
// This is the first case that needs an order. The migration has two required inputs, so it cannot
// run until both stored values have been read, and neither value can be carried over alone.
func TestMigrateMergesIntoNewComponent(t *testing.T) {
	t.Parallel()

	source := ecs.NewWorld()
	_, err := source.RegisterComponent[early.MergeLeft]()
	require.NoError(t, err)
	_, err = source.RegisterComponent[early.MergeRight]()
	require.NoError(t, err)

	entity := source.Create()
	require.NoError(t, source.Set(entity, early.MergeLeft{A: mergeLeftStored}))
	require.NoError(t, source.Set(entity, early.MergeRight{B: mergeRightStored}))

	var save cardinalv1.WorldState
	require.NoError(t, proto.Unmarshal(source.EncodeState(nil), &save))

	world := ecs.NewWorld()
	_, err = world.RegisterComponent[full.Merged]()
	require.NoError(t, err)

	m := &migration.MergedFromV1{}
	require.NoError(t, world.RegisterMigration(m))
	require.NoError(t, world.FromProto(&save))

	merged, err := world.Get[full.Merged](entity)
	require.NoError(t, err)
	assert.Equal(t, full.Merged{A: mergeLeftStored, B: mergeRightStored}, merged)

	// One call for the entity, not one per stored value it consumed.
	assert.Equal(t, 1, m.Calls)
}

// TestMigrateAbsorbsIntoExisting covers case 2c, and with it the optional input.
//
// Two entities: one holding both components, one holding only the survivor. Both have to migrate.
// A required second input would skip the second entity and leave its stored absorber with nothing
// to consume it, which the restore would then refuse.
func TestMigrateAbsorbsIntoExisting(t *testing.T) {
	t.Parallel()

	source := ecs.NewWorld()
	_, err := source.RegisterComponent[early.Absorber]()
	require.NoError(t, err)
	_, err = source.RegisterComponent[early.Absorbed]()
	require.NoError(t, err)

	both := source.Create()
	require.NoError(t, source.Set(both, early.Absorber{A: absorberStored}))
	require.NoError(t, source.Set(both, early.Absorbed{B: absorbedStored}))

	lone := source.Create()
	require.NoError(t, source.Set(lone, early.Absorber{A: absorberLone}))

	var save cardinalv1.WorldState
	require.NoError(t, proto.Unmarshal(source.EncodeState(nil), &save))

	world := ecs.NewWorld()
	_, err = world.RegisterComponent[full.Absorber]()
	require.NoError(t, err)

	m := &migration.AbsorberV1ToCurrent{}
	require.NoError(t, world.RegisterMigration(m))
	require.NoError(t, world.FromProto(&save))

	// The entity that had both keeps the absorbed value.
	got, err := world.Get[full.Absorber](both)
	require.NoError(t, err)
	assert.Equal(t, full.Absorber{A: absorberStored, B: absorbedStored}, got)

	// The entity that had only the survivor migrated too, with the optional input absent.
	loneGot, err := world.Get[full.Absorber](lone)
	require.NoError(t, err)
	assert.Equal(t, full.Absorber{A: absorberLone, B: migration.AbsorbedDefault}, loneGot)

	assert.Equal(t, 2, m.Calls, "both entities migrate")
	assert.Equal(t, 1, m.WithAbsorbed, "only one of them had the optional component")
}

// TestMigrateMovesBetweenComponents covers case 2d: a field moves from one component to another
// and both survive. Two inputs, two outputs, one migration.
func TestMigrateMovesBetweenComponents(t *testing.T) {
	t.Parallel()

	source := ecs.NewWorld()
	_, err := source.RegisterComponent[early.MovedFrom]()
	require.NoError(t, err)
	_, err = source.RegisterComponent[early.MovedTo]()
	require.NoError(t, err)

	entity := source.Create()
	require.NoError(t, source.Set(entity, early.MovedFrom{A: movedFromAStored, B: movedFromBStored}))
	require.NoError(t, source.Set(entity, early.MovedTo{C: movedToCStored}))

	var save cardinalv1.WorldState
	require.NoError(t, proto.Unmarshal(source.EncodeState(nil), &save))

	world := ecs.NewWorld()
	_, err = world.RegisterComponent[full.MovedFrom]()
	require.NoError(t, err)
	_, err = world.RegisterComponent[full.MovedTo]()
	require.NoError(t, err)

	m := &migration.MovedV1ToCurrent{}
	require.NoError(t, world.RegisterMigration(m))
	require.NoError(t, world.FromProto(&save))

	from, err := world.Get[full.MovedFrom](entity)
	require.NoError(t, err)
	assert.Equal(t, full.MovedFrom{A: movedFromAStored}, from)

	to, err := world.Get[full.MovedTo](entity)
	require.NoError(t, err)
	assert.Equal(t, full.MovedTo{C: movedToCStored, B: movedFromBStored}, to)

	assert.Equal(t, 1, m.Calls)
}

// Stored values for the whole-world cases.
const (
	rankedLowStored   uint32 = 110
	rankedHighStored  uint32 = 120
	rankedMidStored   uint32 = 115
	spawnerCount      uint32 = 3
	pointerSlotFound  uint32 = 5
	pointerSlotAbsent uint32 = 9
)

// TestMigrateRanksAcrossEntities covers case 3a: a value that cannot be worked out from inside one
// entity, because it is a position among all of them.
//
// Two migrations run. The per-entity one carries each score across and leaves the rank at zero;
// the whole-world one runs once afterwards, sees every entity, and fills the ranks in. The order
// between them comes from the shape they share, not from registration order.
func TestMigrateRanksAcrossEntities(t *testing.T) {
	t.Parallel()

	source := ecs.NewWorld()
	_, err := source.RegisterComponent[early.Ranked]()
	require.NoError(t, err)

	low := source.Create()
	require.NoError(t, source.Set(low, early.Ranked{Score: rankedLowStored}))
	high := source.Create()
	require.NoError(t, source.Set(high, early.Ranked{Score: rankedHighStored}))
	mid := source.Create()
	require.NoError(t, source.Set(mid, early.Ranked{Score: rankedMidStored}))

	var save cardinalv1.WorldState
	require.NoError(t, proto.Unmarshal(source.EncodeState(nil), &save))

	world := ecs.NewWorld()
	_, err = world.RegisterComponent[full.Ranked]()
	require.NoError(t, err)

	// Registered whole-world first, to show the order is decided by the shapes they declare.
	rank := &migration.RankAll{}
	carry := &migration.RankedV1ToCurrent{}
	require.NoError(t, world.RegisterMigration(rank))
	require.NoError(t, world.RegisterMigration(carry))
	require.NoError(t, world.FromProto(&save))

	assert.Equal(t, 3, carry.Calls, "the per-entity migration runs once per entity")
	assert.Equal(t, 1, rank.Calls, "the whole-world migration runs once for the whole restore")

	// Ranks are by score, and each entity kept its own.
	for entity, want := range map[ecs.EntityID]full.Ranked{
		high: {Score: rankedHighStored, Rank: 1},
		mid:  {Score: rankedMidStored, Rank: 2},
		low:  {Score: rankedLowStored, Rank: 3},
	} {
		got, err := world.Get[full.Ranked](entity)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
}

// TestMigrateCreatesEntities covers case 3b: a migration that brings entities into existence.
func TestMigrateCreatesEntities(t *testing.T) {
	t.Parallel()

	source := ecs.NewWorld()
	_, err := source.RegisterComponent[early.Spawner]()
	require.NoError(t, err)

	spawner := source.Create()
	require.NoError(t, source.Set(spawner, early.Spawner{Count: spawnerCount}))

	var save cardinalv1.WorldState
	require.NoError(t, proto.Unmarshal(source.EncodeState(nil), &save))

	world := ecs.NewWorld()
	_, err = world.RegisterComponent[full.Spawner]()
	require.NoError(t, err)
	_, err = world.RegisterComponent[full.Minion]()
	require.NoError(t, err)

	spawn := &migration.SpawnMinions{}
	require.NoError(t, world.RegisterMigration(&migration.SpawnerV1ToCurrent{}))
	require.NoError(t, world.RegisterMigration(spawn))
	require.NoError(t, world.FromProto(&save))

	assert.Equal(t, 1, spawn.Calls)
	assert.Equal(t, int(spawnerCount), spawn.Created)

	// The spawner records what was made for it, which only the whole-world pass could fill in.
	got, err := world.Get[full.Spawner](spawner)
	require.NoError(t, err)
	assert.Equal(t, full.Spawner{Count: spawnerCount, Spawned: spawnerCount}, got)
}

// TestMigrateSkipsWorldMigrationOnRestart is what keeps case 3b honest.
//
// A whole-world migration is not triggered by a stored shape, so without a gate it would run at
// every boot and create the entities again. Here the save is already current, nothing is
// converted, and the whole-world migration must not run at all.
func TestMigrateSkipsWorldMigrationOnRestart(t *testing.T) {
	t.Parallel()

	source := ecs.NewWorld()
	_, err := source.RegisterComponent[full.Spawner]()
	require.NoError(t, err)
	_, err = source.RegisterComponent[full.Minion]()
	require.NoError(t, err)
	require.NoError(t, source.Set(source.Create(), full.Spawner{Count: spawnerCount, Spawned: spawnerCount}))

	var save cardinalv1.WorldState
	require.NoError(t, proto.Unmarshal(source.EncodeState(nil), &save))

	world := ecs.NewWorld()
	_, err = world.RegisterComponent[full.Spawner]()
	require.NoError(t, err)
	_, err = world.RegisterComponent[full.Minion]()
	require.NoError(t, err)

	spawn := &migration.SpawnMinions{}
	require.NoError(t, world.RegisterMigration(&migration.SpawnerV1ToCurrent{}))
	require.NoError(t, world.RegisterMigration(spawn))
	require.NoError(t, world.FromProto(&save))

	assert.Equal(t, 0, spawn.Calls, "a restart converts nothing, so nothing whole-world may run")
	assert.Equal(t, 0, spawn.Created, "restarting must not spawn the minions a second time")
}

// TestMigrateResolvesReferences covers case 3c: a reference by slot number becomes a reference by
// entity ID, which needs two whole-world reads and cannot be answered from one entity.
func TestMigrateResolvesReferences(t *testing.T) {
	t.Parallel()

	source := ecs.NewWorld()
	_, err := source.RegisterComponent[early.Pointer]()
	require.NoError(t, err)
	_, err = source.RegisterComponent[early.Slotted]()
	require.NoError(t, err)

	target := source.Create()
	require.NoError(t, source.Set(target, early.Slotted{Number: pointerSlotFound}))

	good := source.Create()
	require.NoError(t, source.Set(good, early.Pointer{Slot: pointerSlotFound}))
	dangling := source.Create()
	require.NoError(t, source.Set(dangling, early.Pointer{Slot: pointerSlotAbsent}))

	var save cardinalv1.WorldState
	require.NoError(t, proto.Unmarshal(source.EncodeState(nil), &save))

	world := ecs.NewWorld()
	_, err = world.RegisterComponent[full.Pointer]()
	require.NoError(t, err)
	_, err = world.RegisterComponent[full.Slotted]()
	require.NoError(t, err)

	resolve := &migration.ResolvePointers{}
	require.NoError(t, world.RegisterMigration(&migration.PointerV1ToCurrent{}))
	require.NoError(t, world.RegisterMigration(resolve))
	require.NoError(t, world.FromProto(&save))

	assert.Equal(t, 1, resolve.Calls)

	// The pointer whose slot exists now names that entity by ID.
	resolved, err := world.Get[full.Pointer](good)
	require.NoError(t, err)
	assert.Equal(t, full.Pointer{Slot: pointerSlotFound, Target: uint32(target)}, resolved)

	// The one whose slot matches nothing is marked rather than pointed somewhere arbitrary.
	broken, err := world.Get[full.Pointer](dangling)
	require.NoError(t, err)
	assert.Equal(t, full.Pointer{Slot: pointerSlotAbsent, Target: migration.UnresolvedSlot}, broken)
	assert.Equal(t, 1, resolve.Unresolved)
}

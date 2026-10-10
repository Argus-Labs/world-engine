package internal

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/event"
)

// These tests pin the two suppressed-flush branches in FlushBufferedContacts that the
// NoPersistedActiveContactsBaseline flag selects. They drive the real Runtime, FullRebuildFromECS,
// Step and FlushBufferedContacts directly (not PhysicsPipelineSystem.Run), so they isolate the
// contact-flush contract from the pipeline's singleton wiring — which is covered by the
// pipeline_singleton_baseline_internal_test.go in the system package.

type recordingEmitter struct {
	begins     int
	ends       int
	trigBegins int
	trigEnds   int
}

func (r *recordingEmitter) EmitContactBegin(event.ContactBeginEvent) { r.begins++ }
func (r *recordingEmitter) EmitContactEnd(event.ContactEndEvent)     { r.ends++ }
func (r *recordingEmitter) EmitTriggerBegin(event.TriggerBeginEvent) { r.trigBegins++ }
func (r *recordingEmitter) EmitTriggerEnd(event.TriggerEndEvent)     { r.trigEnds++ }

// overlappingPairEntries is a static ground plane plus a dynamic crate that overlaps it, so the
// first suppressed step has exactly one live contact. Box2D's narrow phase reports it after Step
// even though the suppressed listener wrote nothing into the buffer.
func overlappingPairEntries() []PhysicsRebuildEntry {
	box := func(hw, hh float64) immutable.Slice[component.ColliderShape] {
		return immutable.SliceOf(component.ColliderShape{
			ShapeType:    component.ShapeTypeBox,
			Density:      1,
			Friction:     0.6,
			HalfExtents:  component.Vec2{X: hw, Y: hh},
			CategoryBits: 1,
			MaskBits:     ^uint64(0),
		})
	}
	return []PhysicsRebuildEntry{
		{
			EntityID:  1,
			Transform: component.Transform2D{Position: component.Vec2{Y: -1}},
			PhysicsBody: component.PhysicsBody2D{
				BodyType:        component.BodyTypeStatic,
				Active:          true,
				Awake:           true,
				SleepingAllowed: true,
				GravityScale:    1,
				Shapes:          box(40, 1),
			},
		},
		{
			EntityID:  2,
			Transform: component.Transform2D{Position: component.Vec2{Y: -0.45}},
			PhysicsBody: component.PhysicsBody2D{
				BodyType:        component.BodyTypeDynamic,
				Active:          true,
				Awake:           true,
				SleepingAllowed: true,
				GravityScale:    1,
				Shapes:          box(0.5, 0.5),
			},
		},
	}
}

// runSuppressedFlush rebuilds the world from entries, loads the given persisted baseline, optionally
// arms NoPersistedActiveContactsBaseline, then steps and flushes once. The runtime starts with
// SuppressContactsStep=true (NewRuntime and FullRebuildFromECS both set it), so the flush takes the
// suppressed branch that the flag selects. It returns the runtime so callers can inspect post-flush
// state (the flag, the in-memory contact map).
func runSuppressedFlush(
	t *testing.T, entries []PhysicsRebuildEntry, baseline component.ActiveContacts, armNoBaseline bool,
) (*Runtime, recordingEmitter) {
	t.Helper()
	g := component.Vec2{Y: -10.0}
	rt := NewRuntime(g, 1.0/60.0, 1, 0)
	if err := rt.FullRebuildFromECS(g, entries); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	rt.LoadActiveContactsFromComponent(baseline)
	if armNoBaseline {
		rt.NoPersistedActiveContactsBaseline = true
	}
	em := &recordingEmitter{}
	rt.SetStepEmitter(em)
	rt.Step()
	rt.FlushBufferedContacts()
	return rt, *em
}

// TestSuppressedStepSpuriousBegins is the behavioural contract for the two suppressed-flush branches:
// with an empty persisted baseline and NoPersistedActiveContactsBaseline unset (a fresh world whose
// singleton exists but carries no contacts), diffActiveContactsAfterRebuild emits a Begin for every
// live overlap; with NoPersistedActiveContactsBaseline set (a cross-plugin restore whose singleton
// was just created), adoptLiveContactsWithoutEmit adopts the same overlaps silently. The flag is the
// only thing that distinguishes "the game has never been told, so tell it" from "the game already
// knows, so do not tell it again".
func TestSuppressedStepSpuriousBegins(t *testing.T) {
	t.Parallel()
	entries := overlappingPairEntries()

	_, sanity := runSuppressedFlush(t, entries, component.ActiveContacts{}, false)
	if sanity.begins == 0 && sanity.trigBegins == 0 {
		t.Fatalf("sanity: pair never touches (begins=%d trigBegins=%d) -- fixture invalid",
			sanity.begins, sanity.trigBegins)
	}

	_, current := runSuppressedFlush(t, entries, component.ActiveContacts{}, false)
	if current.begins+current.trigBegins == 0 {
		t.Fatalf("expected begins for the touching pair under the empty-baseline path, got 0")
	}

	intendedRT, intended := runSuppressedFlush(t, entries, component.ActiveContacts{}, true)
	if intended.begins+intended.trigBegins != 0 {
		t.Fatalf("expected ZERO begins under NoPersistedActiveContactsBaseline, got begins=%d trigBegins=%d",
			intended.begins, intended.trigBegins)
	}
	if intendedRT.NoPersistedActiveContactsBaseline {
		t.Fatalf("NoPersistedActiveContactsBaseline must be cleared after the suppressed flush")
	}
	if len(intendedRT.ActiveContacts) == 0 {
		t.Fatalf("adoptLiveContactsWithoutEmit must seed ActiveContacts from the live contact list")
	}
}

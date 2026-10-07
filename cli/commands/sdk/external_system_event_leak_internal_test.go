package sdk

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/internal/sdkgen"
)

// cardinalShimSysEvent is the cardinal registration stand-in used by the external-system-event fixture.
// It mirrors the shim in cli/internal/sdkgen's tests but is duplicated here because that helper is not
// exported across the _test package boundary. RegisterSystemEvent is the roster the physics2d plugin's
// contact/trigger events are wired through, so it must be present for discovery to record the
// dependency's system event for the external pass.
const cardinalShimSysEvent = `package cardinal

type World struct{}

func (*World) RegisterCommand[T any]()     {}
func (*World) RegisterEvent[T any]()       {}
func (*World) RegisterComponent[T any]()   {}
func (*World) RegisterSystemEvent[T any]() {}

func (*World) SendToShard(to string, cmd any) {}
`

// writeMultiModule writes a map of module-relative path -> content under a fresh temp dir, returning the
// root. Mirrors the unexported multiModule helper in cli/internal/sdkgen/sdkgen_test.go so this package
// can build multi-module fixtures (a backend module plus its cardinal shim and a dependency) without
// crossing the _test package boundary.
func writeMultiModule(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	return root
}

// externalSystemEventFixture builds a multi-module layout that mirrors the in-tree physics2d plugin's
// wire ownership path: a `dep` module whose `event` package declares a system event (ContactBeginEvent)
// with an embedded nested payload (ContactEventPayload), a `//go:build !sdkgen` wire.gen.go that imports
// the owner's gen package (dep/gen/event), and a `game` module that registers the dependency's system
// event alongside a local command.
//
// Discovery against `game` yields the local command in res.Messages and the dependency's system event
// (plus its nested payload) in res.ExternalMessages — the exact shape the C# pass merges in emitProtos.
// The wire.gen.go import is what stamps m.GenImport="dep/gen/event" and m.Own=OwnLocal (the second pass
// treats the dependency's packages as local, while the original discoverer sees the gen owner as
// imported) — replicating the real physics2d/event/wire.gen.go shape, not a synthetic stand-in.
func externalSystemEventFixture(t *testing.T) string {
	t.Helper()
	return writeMultiModule(t, map[string]string{
		"cardinalshim/go.mod":               "module cardinalshim\n\ngo 1.27\n",
		"cardinalshim/cardinal/cardinal.go": cardinalShimSysEvent,

		"dep/go.mod": "module dep\n\ngo 1.27\n",
		// The dependency's system event + nested payload, mirroring physics2d/event/contact.go's shape:
		// an embedded payload so the proto field carries the payload's type name (exactly as the real
		// plugin's wire.gen.go renders it: `ContactEventPayload ContactEventPayload = 1`).
		"dep/event/event.go": `package event

type ContactEventPayload struct {
	EntityA int32
	EntityB int32
}

type ContactBeginEvent struct {
	ContactEventPayload
}

func (ContactBeginEvent) Name() string { return "physics2d_contact_begin" }
`,
		// The owner generated: their wire.gen.go records the --go-out choice (dep/gen/event), the only
		// place the gen import exists. The //go:build !sdkgen tag keeps this file out of the sdkgen-tagged
		// load, so discovery tolerates the absent wire methods and treats ContactBeginEvent as a
		// generatable type the dependency owns. ownerGenPackage reads this import back out of the file.
		"dep/event/wire.gen.go": "//go:build !sdkgen\n\npackage event\n\n" +
			"import pbevent \"dep/gen/event\"\n\n" +
			"func (c ContactBeginEvent) ToProto() *pbevent.ContactBeginEvent { return nil }\n\n" +
			"func (c ContactBeginEvent) SizeWire() int { return 0 }\n",
		// Stub gen package so the dep module resolves; it is not loaded under the sdkgen tag (the
		// wire.gen.go that imports it is excluded), but is kept for parity with the real plugin layout.
		"dep/gen/event/event.go": "package event\n\ntype ContactBeginEvent struct{}\n\ntype ContactEventPayload struct{}\n",

		"game/go.mod": "module game\n\ngo 1.27\n\nrequire (\n\tcardinalshim v0.0.0\n\tdep v0.0.0\n)\n\n" +
			"replace cardinalshim => ../cardinalshim\n\nreplace dep => ../dep\n",
		"game/game.go": `package game

import (
	"cardinalshim/cardinal"
	"dep/event"
)

// A local command: in the C# set by design (commands carry TargetCSharp). Pins the happy path — the
// client-facing subset must keep flowing through the C# pass after the fix.
type MoveCommand struct{ Target int32 }

func (MoveCommand) Name() string { return "move" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[MoveCommand]()
	// Register the dependency's system event: the external pass records ContactBeginEvent (and its
	// nested payload) for res.ExternalMessages — the C#-only 2nd pass that emitProtos merges in.
	w.RegisterSystemEvent[event.ContactBeginEvent]()
}
`,
	})
}

// TestExternalSystemEventExcludedAfterFix guards the C# pass's target filter for external (plugin/
// dependency) messages. A dependency-defined system event carries only TargetGo by policy
// (kindTargets[KindSystemEvent] = {TargetGo}), so it must never reach the C# client SDK — the same
// contract the local pass already enforces via withTarget(TargetCSharp). Before the fix the C# branch
// merged res.ExternalMessages unfiltered, so the system event leaked into the C# pass as a generated
// .proto that runBufWith handed to buf generate, and protoc-gen-csharp compiled it into a client class.
//
// The fixture mirrors the in-tree physics2d plugin's wire ownership path (a `dep` module whose `event`
// package declares a system event with a nested payload, a //go:build !sdkgen wire.gen.go importing the
// owner's gen package, registered from `game`) so discovery produces the external system event in the
// leaking shape: Own=OwnLocal, GenImport stamped from wire.gen.go, Targets=[TargetGo]. The first
// block asserts that shape, so the test fails loudly if the fixture ever stops exercising the leak path
// (and stops guarding anything); the rest assert the fix drops it from the C# pass while keeping the
// local command (the client-facing subset) flowing through.
func TestExternalSystemEventExcludedAfterFix(t *testing.T) {
	t.Parallel()

	root := externalSystemEventFixture(t)
	res, err := sdkgen.Discover(filepath.Join(root, "game"))
	require.NoError(t, err)
	res.ResolveGen("game/gen")

	// The fixture must yield the external system event in the leaking shape — otherwise the assertions
	// below are vacuous and the test is no longer guarding the contract.
	var externalSysEvent *sdkgen.Message
	for i := range res.ExternalMessages {
		m := &res.ExternalMessages[i]
		if m.Name == "ContactBeginEvent" {
			externalSysEvent = m
			break
		}
	}
	require.NotNil(t, externalSysEvent, "fixture did not yield an external ContactBeginEvent")
	assert.Equal(t, sdkgen.KindSystemEvent, externalSysEvent.Kind,
		"ContactBeginEvent must be discovered as a system event")
	assert.True(t, slices.Contains(externalSysEvent.Targets, sdkgen.TargetGo),
		"the external system event carries the Go target (engine-internal)")
	assert.False(t, slices.Contains(externalSysEvent.Targets, sdkgen.TargetCSharp),
		"the external system event does NOT carry the C# target — it must be filtered out of the C# pass")
	assert.Equal(t, sdkgen.OwnLocal, externalSysEvent.Own,
		"the external pass marks the dependency's package local, so without the filter the proto would be "+
			"emitted (not import-only) and reach buf generate")

	// The fix: filter external messages by TargetCSharp, symmetric with the local pass. The system event
	// (Targets=[TargetGo]) is dropped; client-facing external types (commands/events/components, which
	// carry TargetCSharp) continue to flow through unchanged.
	fixedMerged := sdkgen.MergeMessages(
		withTarget(res.Messages, sdkgen.TargetCSharp),
		withTarget(res.ExternalMessages, sdkgen.TargetCSharp),
	)
	for _, m := range fixedMerged {
		if m.Name == "ContactBeginEvent" {
			t.Errorf("the fix must drop the external system event from the C# pass; found %s "+
				"(Kind=%q Targets=%v)", m.Name, m.Kind, m.Targets)
		}
	}

	fixedFiles, err := sdkgen.EmitProtos(res.Module, "game/gen", fixedMerged)
	require.NoError(t, err)
	for _, f := range fixedFiles {
		assert.NotContains(t, f.Content, "message ContactBeginEvent",
			"no emitted .proto may contain the plugin's engine-internal system event (file %s)", f.Path)
	}

	// End-to-end through the real emitProtos (now fixed): no C# proto for the external system event, while
	// the local command still reaches the C# pass. emitProtos returns (goProtos, csProtos, error); with
	// only CsOut set, goProtos is nil and csProtos is the C# pass.
	_, csProtos, err := (&GenerateCmd{CsOut: "out"}).emitProtos(res, "game/gen")
	require.NoError(t, err)
	require.NotEmpty(t, csProtos, "the C# pass must still emit the client-facing subset")

	var sawMoveCommand bool
	for _, f := range csProtos {
		assert.NotContains(t, f.Content, "message ContactBeginEvent",
			"emitProtos must not emit the plugin's system event into the C# client SDK (file %s)", f.Path)
		assert.False(t, f.ImportOnly,
			"generateAll clears ImportOnly so every C#-pass file reaches buf generate (file %s)", f.Path)
		if strings.Contains(f.Content, "message MoveCommand") {
			sawMoveCommand = true
		}
	}
	assert.True(t, sawMoveCommand,
		"the local command (TargetCSharp) must still reach the C# pass — no regression on the happy path")
}

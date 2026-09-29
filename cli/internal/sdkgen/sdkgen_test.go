package sdkgen_test

import (
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/cli/internal/sdkgen"
)

// discover writes body as a one-file command package into a temp module (with a
// minimal cardinal shim so World.RegisterCommand resolves), runs Discover against
// it, and returns the result. No committed fixtures, no golden files — each test
// states its own input and expectation inline.
func discover(t *testing.T, body string) sdkgen.Result {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", sdkgentestGoMod)
	write(
		"cardinal/cardinal.go",
		cardinalShim,
	)
	writeImmutableShim(write)
	write("commands.go", "package commands\n\n"+body+"\n")

	res, err := sdkgen.Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	return res
}

// cardinalShim stands in for cardinal's registration API: one generic method on *World per wire role.
// Discovery reads which of them a type is instantiated with, so the bodies never matter.
const cardinalShim = `package cardinal

type World struct{}

func (*World) RegisterCommand[T any]()     {}
func (*World) RegisterEvent[T any]()       {}
func (*World) RegisterComponent[T any]()   {}
func (*World) RegisterSystemEvent[T any]() {}

func (*World) SendToShard(to string, cmd any) {}
`

const sdkgentestGoMod = `module sdkgentest

go 1.27

require github.com/argus-labs/world-engine v0.0.0

replace github.com/argus-labs/world-engine => ./worldengine
`

// writeImmutableShim lays down a stand-in for world-engine's pkg/immutable.
//
// It has to sit at the REAL import path, because discovery matches that path exactly and nothing else
// — see engineSliceElem. The nested module above plus the replace in sdkgentestGoMod get it there and
// resolve offline, so tests still need no copy of world-engine.
//
// Methods are left out: discovery type-checks the source and only parses what it emits, so none is
// ever called.
func writeImmutableShim(write func(rel, content string)) {
	write("worldengine/go.mod", "module github.com/argus-labs/world-engine\n\ngo 1.27\n")
	write("worldengine/pkg/immutable/slice.go", `package immutable

type Slice[T any] struct{ items []T }

func SliceOf[T any](items ...T) Slice[T] { return Slice[T]{items: items} }
`)
}

// proto returns the generated .proto text for body, for substring assertions.
// notDataFindings returns the findings that mean the field was refused outright, whatever its shape.
//
// Every finding blocks, so "no violations" no longer says a field was emitted — a slice reports one
// whether it classified into a repeated field or was refused for its element. These two categories only
// ever come from a refusal, so they are what a test asking for a proto type wants to hear about. That a
// field WAS emitted is pinned by each test's own assertion on the line it expects.
func notDataFindings(res sdkgen.Result) []sdkgen.Violation {
	var out []sdkgen.Violation
	for _, v := range res.Violations {
		if v.Category == sdkgen.CatUnserializable || v.Category == sdkgen.CatUnsupportedType {
			out = append(out, v)
		}
	}
	return out
}

func proto(t *testing.T, body string) string {
	t.Helper()
	res := discover(t, body)
	if bad := notDataFindings(res); len(bad) != 0 {
		t.Fatalf("unexpected violations: %v", bad)
	}
	files, err := sdkgen.EmitProtos(res.Module, "", res.Messages)
	if err != nil {
		t.Fatalf("emit proto: %v", err)
	}
	var b strings.Builder
	for _, f := range files {
		b.WriteString(f.Content)
	}
	return b.String()
}

// cmd wraps a single command struct body + a Setup that registers it.
func cmd(name string, fields string) string {
	return `import "sdkgentest/cardinal"

type C struct {
` + fields + `
}

func (C) Name() string { return "` + name + `" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[C]()
}`
}

func TestProtoTypeMapping(t *testing.T) {
	cases := []struct {
		name   string
		fields string
		want   string // proto line that must appear
	}{
		{"bool", "B bool", "bool B = 1;"},
		{"int widens to int64", "N int", "int64 N = 1;"},
		{"int8 widens to int32", "N int8", "int32 N = 1;"},
		{"int16 widens to int32", "N int16", "int32 N = 1;"},
		{"int32", "N int32", "int32 N = 1;"},
		{"int64", "N int64", "int64 N = 1;"},
		{"uint widens to uint64", "N uint", "uint64 N = 1;"},
		{"uint8 widens to uint32", "N uint8", "uint32 N = 1;"},
		{"uint32", "N uint32", "uint32 N = 1;"},
		{"uint64", "N uint64", "uint64 N = 1;"},
		{"float32 to float", "F float32", "float F = 1;"},
		{"float64 to double", "F float64", "double F = 1;"},
		{"string", "S string", "string S = 1;"},
		{"bytes", "B []byte", "bytes B = 1;"},
		{"repeated scalar", "Xs []int32", "repeated int32 Xs = 1;"},
		{"repeated string", "Ss []string", "repeated string Ss = 1;"},
		{"repeated bytes", "Bs [][]byte", "repeated bytes Bs = 1;"},
		{"map scalar to scalar", "M map[string]int32", "map<string, int32> M = 1;"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := proto(t, cmd("c", "\t"+c.fields))
			if !strings.Contains(got, c.want) {
				t.Fatalf("want proto to contain %q, got:\n%s", c.want, got)
			}
		})
	}
}

func TestProtoNamedByteSliceIsRepeatedUint32(t *testing.T) {
	got := proto(t, `import "sdkgentest/cardinal"

type MyByte byte

type C struct {
	Values []MyByte
}

func (C) Name() string { return "c" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[C]()
}`)

	if !strings.Contains(got, "repeated uint32 Values = 1;") {
		t.Fatalf("want named byte slice to generate repeated uint32, got:\n%s", got)
	}
	if strings.Contains(got, "bytes Values = 1;") {
		t.Fatalf("named byte slice must not generate bytes, got:\n%s", got)
	}
}

// TestProtoEmbeddedRecursive ensures a cyclic embed (valid Go via pointers) nests as recursive proto
// messages — proto supports message recursion, and enqueue's dedup stops discovery from recursing
// forever — rather than being rejected. The test completing at all proves there's no infinite recursion.
func TestProtoEmbeddedRecursive(t *testing.T) {
	res := discover(t, `import "sdkgentest/cardinal"

type A struct {
	*B
	X int32
}

type B struct {
	*A
	Y int32
}

func (A) Name() string { return "a" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[A]()
}`)
	// The *B / *A fields report as pointers — that is the shape rule, not a failure to nest. What this
	// pins is that the recursion itself produces nothing else.
	if bad := notDataFindings(res); len(bad) != 0 {
		t.Fatalf("recursive embed should nest, not violate, got: %+v", bad)
	}
}

// TestProtoEmbedded checks that an embedded struct nests as a sub-message (following the Go structure),
// not flattened — its fields live in the embedded type's own message, and the outer type gets one field
// for the embed itself.
func TestProtoEmbedded(t *testing.T) {
	got := proto(t, `import "sdkgentest/cardinal"

type Base struct {
	RequestID string
	Origin    string
}

type C struct {
	Base
	X int32
}

func (C) Name() string { return "c" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[C]()
}`)
	for _, want := range []string{
		"message C {",
		"int32 X = 2;",          // X is field 2 — Base nests as field 1, so X is NOT flattened (would be 3)
		"message Base {",        // Base is its own message, not promoted into C
		"string RequestID = 1;", // inside Base's message
		"string Origin = 2;",    // inside Base's message
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want nested field %q, got:\n%s", want, got)
		}
	}
}

func TestProtoNested(t *testing.T) {
	got := proto(t, `import "sdkgentest/cardinal"

type Vec2 struct {
	X float64
	Y float64
}

type C struct {
	Pos    Vec2
	Origin *Vec2
	Items  []Vec2
}

func (C) Name() string { return "c" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[C]()
}`)
	for _, want := range []string{
		"message Vec2 {",
		"Vec2 Pos = 1;",
		"Vec2 Origin = 2;",
		"repeated Vec2 Items = 3;",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q, got:\n%s", want, got)
		}
	}
}

func TestCommandWireName(t *testing.T) {
	res := discover(t, cmd("move.v2", "\tX int32"))
	if res.CommandCount() != 1 {
		t.Fatalf("CommandCount = %d, want 1", res.CommandCount())
	}
	var found bool
	for _, m := range res.Messages {
		if m.Name == "C" && m.Wire == "move.v2" {
			found = true
		}
	}
	if !found {
		t.Fatalf("command C should carry wire name move.v2, got %+v", res.Messages)
	}
}

// evt wraps a single event struct + a Setup that registers it via World.RegisterEvent.
func evt(name string, fields string) string {
	return `import "sdkgentest/cardinal"

type E struct {
` + fields + `
}

func (E) Name() string { return "` + name + `" }

func Setup(w *cardinal.World) {
	w.RegisterEvent[E]()
}`
}

// TestEventProto verifies an event discovered via World.RegisterEvent[T] is emitted into the .proto as a
// message (with its wire-name comment), exactly like a command, and is classified as an event.
func TestEventProto(t *testing.T) {
	body := evt("player_died", "\tPlayerID uint64\n\tCause string")
	got := proto(t, body)
	for _, want := range []string{
		"// wire name: \"player_died\"",
		"message E {",
		"uint64 PlayerID = 1;",
		"string Cause = 2;",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want proto to contain %q, got:\n%s", want, got)
		}
	}

	res := discover(t, body)
	if res.EventCount() != 1 || res.CommandCount() != 0 {
		t.Fatalf("EventCount=%d CommandCount=%d, want 1 and 0", res.EventCount(), res.CommandCount())
	}
	for _, m := range res.Messages {
		if m.Name == "E" && m.Kind != sdkgen.KindEvent {
			t.Fatalf("event E should have Kind=event, got %q", m.Kind)
		}
	}
}

// TestCorrelatedWireName covers an event whose Name() is dynamic — `receiver.Field + "literal"`, the
// request/reply correlation pattern (e.g. GetMapResult -> "{LobbyID}_get_map_result"). Such a type owns
// only the suffix, so it must be captured as WireSuffix+WireField (not the exact Wire), and emitted as
// IWireCorrelated rather than IWireNamed.
func TestCorrelatedWireName(t *testing.T) {
	body := `import "sdkgentest/cardinal"

type E struct {
	LobbyID string
	MapID   string
}

func (e E) Name() string { return e.LobbyID + "_get_map_result" }

func Setup(w *cardinal.World) {
	w.RegisterEvent[E]()
}`

	res := discover(t, body)

	var m sdkgen.Message
	for _, x := range res.Messages {
		if x.Name == "E" {
			m = x
		}
	}
	if m.Name != "E" {
		t.Fatalf("event E not discovered; messages: %+v", res.Messages)
	}
	if m.Wire != "" {
		t.Fatalf("dynamic Name() must not populate exact Wire, got %q", m.Wire)
	}
	if m.WireSuffix != "_get_map_result" {
		t.Fatalf("WireSuffix = %q, want %q", m.WireSuffix, "_get_map_result")
	}
	if m.WireField != "LobbyID" {
		t.Fatalf("WireField = %q, want %q", m.WireField, "LobbyID")
	}

	cs := sdkgen.EmitWireNamesCS(res)
	for _, want := range []string{
		"public sealed partial class E : global::WorldEngine.SDK.IWireCorrelated",
		`public string WireSuffix => "_get_map_result";`,
		`public string CorrelationField => "LobbyID";`,
	} {
		if !strings.Contains(cs, want) {
			t.Fatalf("EmitWireNamesCS missing %q, got:\n%s", want, cs)
		}
	}
	// A correlated type must NOT also be emitted as an exact IWireNamed.
	if strings.Contains(cs, "class E : global::WorldEngine.SDK.IWireNamed") {
		t.Fatalf("correlated type E wrongly emitted as IWireNamed:\n%s", cs)
	}
}

// TestEmitProtos_CrossPackage verifies package-per-directory emission: a type in one Go package that
// references a nested type in another gets its own proto package, qualifies the cross-package field, and
// imports the other file — so same-named types across packages can no longer collide.
func TestEmitProtos_CrossPackage(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module sdkgentest\n\ngo 1.27\n")
	write("cardinal/cardinal.go", cardinalShim)
	write("types/types.go", "package types\n\ntype Reward struct {\n\tGold uint32\n}\n")
	write("events/events.go", `package events

import (
	"sdkgentest/cardinal"
	"sdkgentest/types"
)

type LootDropped struct {
	Loot types.Reward
}

func (LootDropped) Name() string { return "loot_dropped" }

func Setup(w *cardinal.World) {
	w.RegisterEvent[LootDropped]()
}`)

	res, err := sdkgen.Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(res.Violations) != 0 {
		t.Fatalf("unexpected violations: %v", res.Violations)
	}
	files, err := sdkgen.EmitProtos(res.Module, "sdkgentest/gen", res.Messages)
	if err != nil {
		t.Fatalf("EmitProtos: %v", err)
	}
	got := map[string]string{}
	for _, f := range files {
		got[f.Path] = f.Content
	}
	ev, ok := got["events/events.proto"]
	if !ok {
		t.Fatalf("missing events proto; got files: %v", keysOf(got))
	}
	ty, ok := got["types/types.proto"]
	if !ok {
		t.Fatalf("missing types proto; got files: %v", keysOf(got))
	}
	for _, want := range []string{
		"package sdkgentest.events;",
		`import "types/types.proto";`,
		"sdkgentest.types.Reward Loot = 1;", // cross-package field is fully qualified
	} {
		if !strings.Contains(ev, want) {
			t.Errorf("events proto missing %q:\n%s", want, ev)
		}
	}
	for _, want := range []string{"package sdkgentest.types;", "message Reward {", "uint32 Gold = 1;"} {
		if !strings.Contains(ty, want) {
			t.Errorf("types proto missing %q:\n%s", want, ty)
		}
	}
}

func keysOf(m map[string]string) []string {
	var k []string
	for key := range m {
		k = append(k, key)
	}
	return k
}

// TestProtoPackageOf verifies the Go-dir → proto-package/file derivation (one package per directory,
// product-prefixed, version-less).
func TestProtoPackageOf(t *testing.T) {
	const mod = "github.com/argus-labs/rampage"
	cases := []struct {
		pkgPath  string
		wantPkg  string
		wantFile string
	}{
		// The package is the import path dotted; the file stays module-relative and short.
		{
			mod + "/shards/gameplay/event",
			"github_com.argus_labs.rampage.shards.gameplay.event",
			"shards/gameplay/event/event.proto",
		},
		{
			mod + "/shards/meta/component",
			"github_com.argus_labs.rampage.shards.meta.component",
			"shards/meta/component/component.proto",
		},
		{mod, "github_com.argus_labs.rampage", "rampage.proto"},
		// A sibling module whose path merely shares a prefix must not be treated as inside this one.
		// A bare TrimPrefix made "…/gamelib/component" look like "…/game" + "lib/component".
		{
			"github.com/argus-labs/rampagelib/component",
			"github_com.argus_labs.rampagelib.component",
			"github.com/argus-labs/rampagelib/component/component.proto",
		},
	}
	for _, c := range cases {
		gotPkg, gotFile := sdkgen.ProtoPackageOf(mod, c.pkgPath)
		if gotPkg != c.wantPkg || gotFile != c.wantFile {
			t.Errorf(
				"ProtoPackageOf(%q)\n  got  pkg=%q file=%q\n  want pkg=%q file=%q",
				c.pkgPath,
				gotPkg,
				gotFile,
				c.wantPkg,
				c.wantFile,
			)
		}
	}
}

// TestProtoPackageIsGeneratorIndependent pins the property the import-path derivation exists for: one Go
// type gets one proto identity, whoever generates it. A consumer reaching into world-engine must derive
// the same package and file that world-engine derives for itself — otherwise each backend would define a
// separate message for the same type and nothing downstream could tell they were the same thing.
func TestProtoPackageIsGeneratorIndependent(t *testing.T) {
	t.Parallel()

	root := multiModule(t, map[string]string{
		"cardinalshim/go.mod":               "module cardinalshim\n\ngo 1.27\n",
		"cardinalshim/cardinal/cardinal.go": cardinalShim,
		"dep/go.mod":                        "module dep\n\ngo 1.27\n",
		"dep/component/component.go":        "package component\n\ntype Vec2 struct{ X, Y int32 }\n",
		"game/go.mod": "module game\n\ngo 1.27\n\nrequire (\n\tcardinalshim v0.0.0\n\tdep v0.0.0\n)\n\n" +
			"replace cardinalshim => ../cardinalshim\n\nreplace dep => ../dep\n",
		"game/game.go": `package game

import (
	"cardinalshim/cardinal"
	"dep/component"
)

type MoveCommand struct{ Target component.Vec2 }

func (MoveCommand) Name() string { return "move" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[MoveCommand]()
}`,
	})

	// What "dep" derives for its own package, generating itself.
	wantPkg, wantFile := sdkgen.ProtoPackageOf("dep", "dep/component")

	// What "game" derives for the same package, reached as a field. It must agree — the whole reason the
	// proto package comes from the import path is that one Go type gets one identity whoever generates it.
	res, err := sdkgen.Discover(filepath.Join(root, "game"))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	var found bool
	for _, m := range res.Messages {
		if m.PkgPath != "dep/component" {
			continue
		}
		found = true
		if m.ProtoPkg != wantPkg {
			t.Errorf("proto package differs by generator: owner=%q consumer=%q", wantPkg, m.ProtoPkg)
		}
		if m.ProtoFile != wantFile {
			t.Errorf("proto file differs by generator: owner=%q consumer=%q", wantFile, m.ProtoFile)
		}
		// The C# namespace is deliberately the opposite — scoped to whoever generates, so a game
		// developer reads a short name rather than the module path spelled out.
		if m.CSNamespace == "" || !strings.HasPrefix(m.CSNamespace, "Game.") {
			t.Errorf("C# namespace should be consumer-scoped, got %q", m.CSNamespace)
		}
	}
	if !found {
		t.Fatal("the foreign package was not discovered; the fixture no longer exercises this")
	}
}

// TestOptionalScalar verifies a pointer-to-scalar field becomes a proto3 `optional` scalar (nil/presence
// preserved) rather than being rejected as an unsupported pointer.
func TestOptionalScalar(t *testing.T) {
	cases := []struct {
		field string
		want  string
	}{
		{"X *float64", "optional double X = 1;"},
		{"N *uint32", "optional uint32 N = 1;"},
		{"S *string", "optional string S = 1;"},
		{"B *bool", "optional bool B = 1;"},
	}
	for _, c := range cases {
		t.Run(c.field, func(t *testing.T) {
			got := proto(t, cmd("c", "\t"+c.field))
			if !strings.Contains(got, c.want) {
				t.Fatalf("want proto to contain %q, got:\n%s", c.want, got)
			}
		})
	}
}

// TestEventNestedType verifies an event's nested struct field is pulled into the .proto as its own
// message, the same transitive-collection path commands use.
func TestEventNestedType(t *testing.T) {
	body := `import "sdkgentest/cardinal"

type Reward struct {
	Gold uint32
}

type E struct {
	Loot Reward
}

func (E) Name() string { return "loot_dropped" }

func Setup(w *cardinal.World) {
	w.RegisterEvent[E]()
}`
	got := proto(t, body)
	for _, want := range []string{"message E {", "Reward Loot = 1;", "message Reward {", "uint32 Gold = 1;"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want proto to contain %q, got:\n%s", want, got)
		}
	}
}

// TestDiscover_SkipsImportedModuleCommands sets up a game module that imports a separate plugin
// module and registers both its own command and the plugin's. The generator must emit
// wire code only for the local command — the imported plugin ships its own committed wire code, so
// regenerating it would clobber a read-only dependency.
// multiModule writes a set of files under a temp root and returns it. Shared by the cross-module tests,
// which all need a real second module on disk — locality is decided from import paths, so a fixture in
// one package cannot exercise it.
func multiModule(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// splitForRender does what generate.go does before emitting: resolve every generated-package path once,
// now that --go-out is known, then split the way emitWireBridges splits. Messages the run owns a
// directory for render into that directory's wire file; mirrored types own none and are looked up by the
// packages that reference them.
func splitForRender(res sdkgen.Result, goOutImport string) ([]sdkgen.Message, map[string]sdkgen.Message) {
	res.ResolveGen(goOutImport)
	var local []sdkgen.Message
	mirrored := map[string]sdkgen.Message{}
	for _, m := range res.Messages {
		if m.Own.Mirrored() {
			mirrored[m.PkgPath+"."+m.Name] = m
			continue
		}
		if m.Dir != "" {
			local = append(local, m)
		}
	}
	return local, mirrored
}

// TestEveryCategoryHasGuidance pins that no category reaches the report without a fix to offer.
//
// Guidance is the only registration a category has left: every finding blocks, so there is no tier to
// get wrong, and a category added without a guidance entry would be printed as a bare name.
func TestEveryCategoryHasGuidance(t *testing.T) {
	t.Parallel()

	for _, cat := range sdkgen.Categories {
		if g := sdkgen.GuidanceFor(cat); g.Proper == "" {
			t.Errorf("category %q has no guidance — the report would name it with no fix", cat)
		}
	}
}

// TestTwoMultiDimensionalByteArrays pins that a message can hold more than one of them.
//
// The encoder accumulates into a local before assigning, and that local used to have a fixed name — so a
// second such field redeclared it and the emitted package failed to build with "no new variables on left
// side of :=". format.Source only parses, so generation reported success and the error surfaced in the
// user's build instead.
func TestTwoMultiDimensionalByteArrays(t *testing.T) {
	t.Parallel()

	out := wire(t, cmd("c", "\tA [2][4]byte\n\tB [3][5]byte"))
	decls := regexp.MustCompile(`(\w+) := make\(\[\]byte`).FindAllStringSubmatch(out, -1)
	if len(decls) != 2 {
		t.Fatalf("want one accumulator per field, got %d:\n%s", len(decls), out)
	}
	if decls[0][1] == decls[1][1] {
		t.Errorf("both fields declared %q — the second is a redeclaration:\n%s", decls[0][1], out)
	}
}

// TestRefusedFieldReportsOneCategory pins the routing: one finding per field, filed under the fix it
// needs. The category comes from the TYPE, never from which encoder happened to refuse it.
//
// The last two cases are the ones that used to be misfiled. Categories were chosen by whether the opaque
// JSON fallback could carry the field, so a float map key was reported as "unserializable" — it is a map
// and wants a map's fix — and complex128 was reported the same way, when it is ordinary inline data this
// generator declines to map. Nothing about them is unserializable in the sense chan is.
func TestRefusedFieldReportsOneCategory(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name, field, wantCat string
	}{
		{"map proto can express", "M map[string]int32", sdkgen.CatMap},
		{"map proto cannot express", "M map[string][]int32", sdkgen.CatMap},
		{"slice of slices", "S [][]int32", sdkgen.CatSlice},
		{"plain slice", "S []int32", sdkgen.CatSlice},
		{"no shape to name", "V [2]uintptr", sdkgen.CatUnsupportedType},
		{"chan is not data", "Ch chan int", sdkgen.CatUnserializable},
		{"chan below the surface", "S []chan int", sdkgen.CatUnserializable},
		{"float map key", "M map[float64]int32", sdkgen.CatMap},
		{"complex is data we decline", "X complex128", sdkgen.CatUnsupportedType},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			res := discover(t, cmd("c", "\t"+c.field))
			if len(res.Violations) != 1 {
				t.Fatalf("want exactly one finding, got %+v", res.Violations)
			}
			if got := res.Violations[0].Category; got != c.wantCat {
				t.Errorf("category: want %s, got %s (reason: %s)", c.wantCat, got, res.Violations[0].Reason)
			}
		})
	}
}

// TestEveryBadFieldIsReported pins that discovery does not stop at the first refusal. Every other case
// uses a single-field command, so a regression that bailed after one finding would pass the suite while
// making the report useless: a developer fixes one field, re-runs, and meets the next one instead of
// seeing the whole of the work in a single pass.
//
// It is also the only case that puts all five field-level categories in one message, so a routing change
// that crosses two of them fails here.
func TestEveryBadFieldIsReported(t *testing.T) {
	t.Parallel()

	res := discover(t, cmd("c", "\tA []string\n\tB chan int\n\tC map[float64]int32\n\tD complex128\n\tE *int32"))
	want := map[string]string{
		"C.A": sdkgen.CatSlice, "C.B": sdkgen.CatUnserializable, "C.C": sdkgen.CatMap,
		"C.D": sdkgen.CatUnsupportedType, "C.E": sdkgen.CatPointer,
	}
	got := map[string]string{}
	for _, v := range res.Violations {
		got[v.Where] = v.Category
	}
	if !maps.Equal(got, want) {
		t.Errorf("want %v, got %v", want, got)
	}
}

// TestUnregisteredCategorySaysItIsAGeneratorBug pins that a category with no guidance explains itself. A
// finding printed with no fix tells a developer their field is a violation when the fault is ours and
// their code is probably fine — so it has to say that.
func TestUnregisteredCategorySaysItIsAGeneratorBug(t *testing.T) {
	t.Parallel()

	body := `import "sdkgentest/cardinal"

type C struct {
	Ch chan int
}

func (C) Name() string { return "c" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[C]()
}`
	// chan is a registered category (unserializable), so nothing should be annotated here — this pins
	// that the note is not attached to ordinary findings.
	res := discover(t, body)
	if len(res.Violations) == 0 {
		t.Fatal("expected a violation for a chan field")
	}
	if strings.Contains(res.Violations[0].Reason, "generator bug") {
		t.Errorf("a registered category must not be annotated as a generator bug: %+v", res.Violations[0])
	}
}

// TestImportOnlyMarksOwnerGeneratedFiles pins the Go half of the import path: a foreign package whose
// owner already generated it is emitted so the schema resolves, but marked ImportOnly so buf does not
// generate a second Go type for a message that already exists.
//
// The whole import path had no test, which is how a bug in it reached review: the same flag was being
// honoured on the C# run, where nothing ships a generated class, so the client SDK referenced types that
// were never emitted. That half is asserted in the command package; this pins the flag's meaning here.
func TestImportOnlyMarksOwnerGeneratedFiles(t *testing.T) {
	t.Parallel()

	root := multiModule(t, map[string]string{
		"cardinalshim/go.mod":               "module cardinalshim\n\ngo 1.27\n",
		"cardinalshim/cardinal/cardinal.go": cardinalShim,
		"dep/go.mod":                        "module dep\n\ngo 1.27\n",
		"dep/component/component.go":        "package component\n\ntype Vec2 struct{ X, Y int32 }\n",
		// The marker of an owner that generated: a wire file importing its own gen package.
		"dep/component/wire.gen.go": "//go:build !sdkgen\n\npackage component\n\n" +
			"import pbcomponent \"dep/gen/component\"\n\n" +
			"func (c Vec2) ToProto() *pbcomponent.Vec2 { return nil }\n\nfunc (c Vec2) SizeWire() int { return 0 }\n",
		"dep/gen/component/component.go": "package component\n\ntype Vec2 struct{}\n",
		"game/go.mod": "module game\n\ngo 1.27\n\nrequire (\n\tcardinalshim v0.0.0\n\tdep v0.0.0\n)\n\n" +
			"replace cardinalshim => ../cardinalshim\n\nreplace dep => ../dep\n",
		"game/game.go": `package game

import (
	"cardinalshim/cardinal"
	"dep/component"
)

type MoveCommand struct{ Target component.Vec2 }

func (MoveCommand) Name() string { return "move" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[MoveCommand]()
}`,
	})

	res, err := sdkgen.Discover(filepath.Join(root, "game"))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	files, err := sdkgen.EmitProtos(res.Module, "game/gen", res.Messages)
	if err != nil {
		t.Fatalf("EmitProtos: %v", err)
	}

	var foreign, local int
	for _, f := range files {
		if strings.Contains(f.Content, "message Vec2") {
			foreign++
			if !f.ImportOnly {
				t.Errorf("the owner generated Vec2, so its file must be import-only: %s", f.Path)
			}
			if !strings.Contains(f.Content, `option go_package = "dep/gen/component`) {
				t.Errorf("want the OWNER's go_package so protoc-gen-go imports their type, got:\n%s", f.Content)
			}
			continue
		}
		local++
		if f.ImportOnly {
			t.Errorf("this module's own file must be generated, not import-only: %s", f.Path)
		}
	}
	if foreign != 1 || local == 0 {
		t.Fatalf("want one foreign file and at least one local, got foreign=%d local=%d", foreign, local)
	}
}

// TestOwnerGenPackageMatchesThisPackage pins that the gen import chosen for a foreign type is the one for
// THAT package. A dependency's wire file imports a gen package per package it references, so matching on
// "some gen import under the same module" returned whichever gofmt sorted first — stamping z's schema
// with a's go_package, which makes protoc-gen-go emit an import of a package that has no such type.
func TestOwnerGenPackageMatchesThisPackage(t *testing.T) {
	t.Parallel()

	root := multiModule(t, map[string]string{
		"cardinalshim/go.mod":               "module cardinalshim\n\ngo 1.27\n",
		"cardinalshim/cardinal/cardinal.go": cardinalShim,
		"dep/go.mod":                        "module dep\n\ngo 1.27\n",
		"dep/a/a.go":                        "package a\n\ntype Other struct{ N int32 }\n",
		"dep/z/z.go":                        "package z\n\nimport \"dep/a\"\n\ntype Holder struct {\n\tN int32\n\tOther a.Other\n}\n",
		"dep/gen/a/a.go":                    "package a\n\ntype Other struct{}\n",
		"dep/gen/z/z.go":                    "package z\n\ntype Holder struct{}\n",
		// Two gen imports, "a" first — the trap.
		"dep/z/wire.gen.go": "//go:build !sdkgen\n\npackage z\n\nimport (\n\tpba \"dep/gen/a\"\n" +
			"\tpbz \"dep/gen/z\"\n)\n\nfunc (c Holder) ToProto() *pbz.Holder { _ = pba.Other{}; return nil }\n\nfunc (c Holder) SizeWire() int { return 0 }\n",
		"game/go.mod": "module game\n\ngo 1.27\n\nrequire (\n\tcardinalshim v0.0.0\n\tdep v0.0.0\n)\n\n" +
			"replace cardinalshim => ../cardinalshim\n\nreplace dep => ../dep\n",
		"game/game.go": `package game

import (
	"cardinalshim/cardinal"
	"dep/z"
)

type UseCommand struct{ H z.Holder }

func (UseCommand) Name() string { return "use" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[UseCommand]()
}`,
	})

	res, err := sdkgen.Discover(filepath.Join(root, "game"))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	for _, m := range res.Messages {
		if m.Name != "Holder" {
			continue
		}
		if m.GenImport != "dep/gen/z" {
			t.Fatalf("want Holder stamped with dep/gen/z, got %q", m.GenImport)
		}
		return
	}
	t.Fatal("Holder was not discovered; the fixture no longer exercises this")
}

// TestProtoFileCollisionIsRefused pins that two packages sharing a module-relative path are rejected
// rather than merged. The proto package is the full import path and is unique, but the file is
// module-relative and is not — "game/component" and "dep/component" both give component/component.proto,
// and grouping by file would concatenate them under whichever package sorted first.
func TestProtoFileCollisionIsRefused(t *testing.T) {
	t.Parallel()

	msgs := []sdkgen.Message{
		{Name: "Local", PkgPath: "game/component", ProtoPkg: "game.component", ProtoFile: "component/component.proto"},
		{Name: "Foreign", PkgPath: "dep/component", ProtoPkg: "dep.component", ProtoFile: "component/component.proto"},
	}
	_, err := sdkgen.EmitProtos("game", "game/gen", msgs)
	if err == nil {
		t.Fatal("two packages resolving to one .proto path must be refused, not merged")
	}
	if !strings.Contains(err.Error(), "component/component.proto") {
		t.Errorf("the error should name the colliding file, got: %v", err)
	}
}

// TestMirrorReachesEveryDepth pins that a type another module owns is rebuilt along with everything it
// reaches, however deep, and through fixed arrays as well as direct fields.
//
// Emission used to collect converters in one pass over the LOCAL messages' fields. A mirrored type's own
// fields are not local fields, so a chain of foreign types emitted a body calling a converter that was
// never written: generation reported success and the package failed to compile on an undefined symbol.
func TestMirrorReachesEveryDepth(t *testing.T) {
	t.Parallel()

	root := multiModule(t, map[string]string{
		"cardinalshim/go.mod":               "module cardinalshim\n\ngo 1.27\n",
		"cardinalshim/cardinal/cardinal.go": cardinalShim,
		// A dependency that never runs the generator, so nothing exists to import and every level mirrors.
		"dep/go.mod":   "module dep\n\ngo 1.27\n",
		"dep/l3/l3.go": "package l3\n\ntype Tax struct{ Rate float64 }\n",
		"dep/l2/l2.go": "package l2\n\nimport \"dep/l3\"\n\ntype Line struct {\n\tSKU string\n\tTax l3.Tax\n}\n",
		"dep/l1/l1.go": "package l1\n\nimport \"dep/l2\"\n\n" +
			"type Invoice struct {\n\tLines [2]l2.Line\n}\n",
		"game/go.mod": "module game\n\ngo 1.27\n\nrequire (\n\tcardinalshim v0.0.0\n\tdep v0.0.0\n)\n\n" +
			"replace cardinalshim => ../cardinalshim\n\nreplace dep => ../dep\n",
		"game/game.go": `package game

import (
	"cardinalshim/cardinal"
	"dep/l1"
)

type BillCommand struct{ Invoice l1.Invoice }

func (BillCommand) Name() string { return "bill" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[BillCommand]()
}`,
	})

	res, err := sdkgen.Discover(filepath.Join(root, "game"))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	// Split the way emitWireBridges does: a wire file is rendered per source directory, so it receives
	// only that package's messages. A mirrored type owns no directory and is supplied separately —
	// passing everything as msgs would hide the bug this test exists for.
	local, mirrored := splitForRender(res, "game/gen")
	out, err := sdkgen.RenderGoWire(local, mirrored)
	if err != nil {
		t.Fatalf("RenderGoWire: %v", err)
	}

	// Every converter the emitted bodies call must also be declared in the same file. Tax is reachable
	// only as Invoice -> Line -> Tax, and Line only through an array.
	called := regexp.MustCompile(`(mirror\w+(?:To|From)Proto)\(`).FindAllStringSubmatch(out, -1)
	if len(called) == 0 {
		t.Fatal("no mirror converters were called; the fixture no longer exercises mirroring")
	}
	for _, c := range called {
		if !strings.Contains(out, "func "+c[1]+"(") {
			t.Errorf("wire code calls %s but never declares it:\n%s", c[1], out)
		}
	}
	// Asserted by the type each converter is for, not by the exact identifier: how converters are named
	// is an implementation detail, but every level of the chain having one is the property under test.
	for _, typ := range []string{"Invoice", "Line", "Tax"} {
		if !regexp.MustCompile(`func mirror\w*` + typ + `ToProto\(`).MatchString(out) {
			t.Errorf("want a converter for every level of the chain, none found for %s:\n%s", typ, out)
		}
	}
}

// TestMirrorSameLeafPackages pins that two foreign packages sharing a package leaf can be mirrored into
// one file. "component" names a package in every plugin, so naming converters after the leaf alone made
// both resolve to mirrorComponentVec2ToProto — a redeclaration, which format.Source accepts because it
// only parses, so it reached disk and failed at go build.
func TestMirrorSameLeafPackages(t *testing.T) {
	t.Parallel()

	root := multiModule(t, map[string]string{
		"cardinalshim/go.mod":                "module cardinalshim\n\ngo 1.27\n",
		"cardinalshim/cardinal/cardinal.go":  cardinalShim,
		"dep/go.mod":                         "module dep\n\ngo 1.27\n",
		"dep/lobby/component/component.go":   "package component\n\ntype Vec2 struct{ X, Y int32 }\n",
		"dep/physics/component/component.go": "package component\n\ntype Vec2 struct{ X, Y float64 }\n",
		"game/go.mod": "module game\n\ngo 1.27\n\nrequire (\n\tcardinalshim v0.0.0\n\tdep v0.0.0\n)\n\n" +
			"replace cardinalshim => ../cardinalshim\n\nreplace dep => ../dep\n",
		"game/game.go": `package game

import (
	"cardinalshim/cardinal"
	lobby "dep/lobby/component"
	physics "dep/physics/component"
)

type PlaceCommand struct {
	Grid  lobby.Vec2
	World physics.Vec2
}

func (PlaceCommand) Name() string { return "place" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[PlaceCommand]()
}`,
	})

	res, err := sdkgen.Discover(filepath.Join(root, "game"))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	local, mirrored := splitForRender(res, "game/gen")
	out, err := sdkgen.RenderGoWire(local, mirrored)
	if err != nil {
		t.Fatalf("two same-leaf foreign packages must render: %v", err)
	}

	// Every declared converter must be declared exactly once.
	declared := map[string]int{}
	for _, m := range regexp.MustCompile(`func (mirror\w+)\(`).FindAllStringSubmatch(out, -1) {
		declared[m[1]]++
	}
	if len(declared) == 0 {
		t.Fatal("no converters declared; the fixture no longer exercises mirroring")
	}
	for name, n := range declared {
		if n > 1 {
			t.Errorf("%s declared %d times — the two Vec2 types collapsed onto one name:\n%s", name, n, out)
		}
	}
}

// TestMirrorSingleSegmentPackage pins that a foreign package whose import path is one segment can be
// mirrored. The wire file imports the source package and its generated counterpart together, and both
// derive the same leaf ("net" and "<gen>/net"), so an unprefixed alias scheme claimed one name twice and
// refused to emit. Most of the standard library has single-segment paths.
func TestMirrorSingleSegmentPackage(t *testing.T) {
	t.Parallel()

	root := multiModule(t, map[string]string{
		"cardinalshim/go.mod":               "module cardinalshim\n\ngo 1.27\n",
		"cardinalshim/cardinal/cardinal.go": cardinalShim,
		"dep/go.mod":                        "module dep\n\ngo 1.27\n",
		"dep/dep.go":                        "package dep\n\ntype Addr struct{ Host string }\n",
		"game/go.mod": "module game\n\ngo 1.27\n\nrequire (\n\tcardinalshim v0.0.0\n\tdep v0.0.0\n)\n\n" +
			"replace cardinalshim => ../cardinalshim\n\nreplace dep => ../dep\n",
		"game/game.go": `package game

import (
	"cardinalshim/cardinal"
	"dep"
)

type PingCommand struct{ Addr dep.Addr }

func (PingCommand) Name() string { return "ping" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[PingCommand]()
}`,
	})

	res, err := sdkgen.Discover(filepath.Join(root, "game"))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	local, mirrored := splitForRender(res, "game/gen")
	if _, err := sdkgen.RenderGoWire(local, mirrored); err != nil {
		t.Fatalf("a single-segment foreign package must still render: %v", err)
	}
}

func TestDiscover_SkipsImportedModuleCommands(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Shared cardinal shim so World resolves to the same type in both modules.
	write("cardinalshim/go.mod", "module cardinalshim\n\ngo 1.27\n")
	write("cardinalshim/cardinal/cardinal.go", cardinalShim)

	// Plugin module: defines and uses its own command.
	write(
		"extplugin/go.mod",
		"module extplugin\n\ngo 1.27\n\nrequire cardinalshim v0.0.0\n\nreplace cardinalshim => ../cardinalshim\n",
	)
	write("extplugin/extplugin.go", `package extplugin

import "cardinalshim/cardinal"

type PluginCommand struct{ X int32 }

func (PluginCommand) Name() string { return "plugin_cmd" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[PluginCommand]()
}`)

	// Game module: imports the plugin, registers both its own command and the plugin's.
	write(
		"gamemod/go.mod",
		"module gamemod\n\ngo 1.27\n\nrequire (\n\tcardinalshim v0.0.0\n\textplugin v0.0.0\n)\n\nreplace cardinalshim => ../cardinalshim\n\nreplace extplugin => ../extplugin\n",
	)
	write("gamemod/game.go", `package game

import (
	"cardinalshim/cardinal"
	"extplugin"
)

type GameCommand struct{ Y int32 }

func (GameCommand) Name() string { return "game_cmd" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[GameCommand]()
	w.RegisterCommand[extplugin.PluginCommand]()
}`)

	res, err := sdkgen.Discover(filepath.Join(root, "gamemod"))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	var gotLocal, gotImported bool
	for _, m := range res.Messages {
		switch m.Name {
		case "GameCommand":
			gotLocal = true
		case "PluginCommand":
			gotImported = true
		}
	}
	if !gotLocal {
		t.Errorf("local GameCommand should be emitted, got messages %+v", res.Messages)
	}
	if gotImported {
		t.Errorf("imported PluginCommand must NOT be emitted (it ships its own wire code), got %+v", res.Messages)
	}
}

// TestOrphan verifies a type carrying a Name() badge but wired to no system is flagged as an orphan
// (declared-but-never-wired) rather than silently omitted, while a properly-wired type is not.
func TestOrphan(t *testing.T) {
	res := discover(t, `import "sdkgentest/cardinal"

type Wired struct { X int32 }

func (Wired) Name() string { return "wired" }

type Forgot struct { Y int32 }

func (Forgot) Name() string { return "forgot" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[Wired]()
}`)
	if len(res.Orphans) != 1 || !strings.Contains(res.Orphans[0], "Forgot") {
		t.Fatalf("expected Forgot flagged as the one orphan, got %v", res.Orphans)
	}
	for _, o := range res.Orphans {
		if strings.Contains(o, "Wired") {
			t.Errorf("Wired is registered via RegisterCommand, should not be an orphan: %v", res.Orphans)
		}
	}
}

// TestViolations covers the cases that stay HARD violations — genuinely-unserializable types (chan/func).
// Generically-encodable-but-unclean types (any, nested slices, exotic maps) are NOT here; they warn + fall
// back (see TestFallbackToBytes). time.Time is NOT here either; it maps to Timestamp (see TestTimestamp).
func TestViolations(t *testing.T) {
	cases := []struct {
		name    string
		imports string
		fields  string
		want    string // substring of the violation reason
		wantCat string // expected issue category
	}{
		{"chan", "", "Ch chan int", "not representable", sdkgen.CatUnserializable},
		{"func", "", "F func()", "not representable", sdkgen.CatUnserializable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := `import (
	"sdkgentest/cardinal"` + c.imports + `
)

type C struct {
	` + c.fields + `
}

func (C) Name() string { return "c" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[C]()
}`
			res := discover(t, body)
			if len(res.Violations) == 0 {
				t.Fatalf("expected a violation for %s, got none", c.name)
			}
			var reasons, cats []string
			for _, v := range res.Violations {
				reasons = append(reasons, v.Reason)
				cats = append(cats, v.Category)
			}
			if joined := strings.Join(reasons, " | "); !strings.Contains(joined, c.want) {
				t.Fatalf("want violation containing %q, got: %s", c.want, joined)
			}
			if !slices.Contains(cats, c.wantCat) {
				t.Fatalf("want a violation with category %q, got: %v", c.wantCat, cats)
			}
		})
	}
}

// TestDiscover_SendOnlyCommand pins discovery of a command a module sends through World.SendToShard but
// never registers: the receiving shard registers it, and only this module can generate its wire code.
func TestDiscover_SendOnlyCommand(t *testing.T) {
	t.Parallel()
	res := discover(t, `import "sdkgentest/cardinal"

type Ping struct{ X int32 }

func (Ping) Name() string { return "ping" }

func Run(w *cardinal.World) { w.SendToShard("other", Ping{X: 1}) }`)
	got := map[string]string{}
	for _, m := range res.Messages {
		got[m.Name] = m.Kind
	}
	if want := map[string]string{"Ping": sdkgen.KindCommand}; !maps.Equal(got, want) {
		t.Errorf("discovered kinds\n  got  %v\n  want %v", got, want)
	}
}

// TestComponentDiscovery verifies World.RegisterComponent[T] discovers T as a component (KindComponent)
// and emits its proto message.
func TestComponentDiscovery(t *testing.T) {
	t.Parallel()
	body := `import "sdkgentest/cardinal"

type Position struct {
	X int32
	Y int32
}

func (Position) Name() string { return "position" }

func Setup(w *cardinal.World) {
	w.RegisterComponent[Position]()
}`
	res := discover(t, body)
	if len(res.Violations) != 0 {
		t.Fatalf("unexpected violations: %v", res.Violations)
	}
	var found *sdkgen.Message
	for i := range res.Messages {
		if res.Messages[i].Name == "Position" {
			found = &res.Messages[i]
		}
	}
	if found == nil {
		t.Fatalf("Position not discovered as a message")
	}
	if found.Kind != sdkgen.KindComponent {
		t.Errorf("Position should be KindComponent, got %q", found.Kind)
	}
}

// registerAPIModule lays out a backend on the world-engine API that declares wire types with generic
// methods on *World (Go 1.27) instead of system fields. The shim's constraint requires MarshalWire the way
// the real Serializable one does, so every instantiation fails it during discovery exactly as it does
// against a real backend, whose wire.gen.go discovery never reads.
func registerAPIModule(t *testing.T, game string) string {
	t.Helper()
	return multiModule(t, map[string]string{
		"cardinalshim/go.mod": "module cardinalshim\n\ngo 1.27\n",
		"cardinalshim/cardinal/cardinal.go": `package cardinal

type Wire interface{ MarshalWire() ([]byte, error) }

type World struct{}

func (w *World) RegisterCommand[T Wire]()     {}
func (w *World) RegisterEvent[T Wire]()       {}
func (w *World) RegisterComponent[T Wire]()   {}
func (w *World) RegisterSystemEvent[T Wire]() {}
func (w *World) Commands[T Wire]() []T        { return nil }
`,
		"game/go.mod":  "module game\n\ngo 1.27\n\nrequire cardinalshim v0.0.0\n\nreplace cardinalshim => ../cardinalshim\n",
		"game/game.go": "package game\n\n" + game,
	})
}

// TestDiscover_RegisterMethods pins discovery through World.RegisterX[T](), which replaced the WithX
// system fields as the one place a world-engine backend declares its wire types.
func TestDiscover_RegisterMethods(t *testing.T) {
	t.Parallel()
	root := registerAPIModule(t, `import "cardinalshim/cardinal"

type Move struct{ X int32 }

func (Move) Name() string { return "move" }

type Moved struct{ X int32 }

func (Moved) Name() string { return "moved" }

type Health struct{ HP int32 }

func (Health) Name() string { return "health" }

type Died struct{ ID uint32 }

func (Died) Name() string { return "died" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[Move]()
	w.RegisterEvent[Moved]()
	w.RegisterComponent[Health]()
	w.RegisterSystemEvent[Died]()
}`)
	res, err := sdkgen.Discover(filepath.Join(root, "game"))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	got := map[string]string{}
	for _, m := range res.Messages {
		got[m.Name] = m.Kind
	}
	want := map[string]string{
		"Move":   sdkgen.KindCommand,
		"Moved":  sdkgen.KindEvent,
		"Health": sdkgen.KindComponent,
		"Died":   sdkgen.KindSystemEvent,
	}
	if !maps.Equal(got, want) {
		t.Errorf("discovered kinds\n  got  %v\n  want %v", got, want)
	}
	if len(res.Violations) != 0 || len(res.Orphans) != 0 {
		t.Errorf("want no findings, got violations %v, orphans %v", res.Violations, res.Orphans)
	}
}

// TestDiscover_UndeclaredCardinalUseBlocks pins the guard for an API the rosters do not know. A wire type
// the source hands to cardinal but no roster declared would otherwise be dropped from generation, which
// deletes its generated code and breaks the build. A Name()-badged type nothing uses stays an orphan.
func TestDiscover_UndeclaredCardinalUseBlocks(t *testing.T) {
	t.Parallel()
	root := registerAPIModule(t, `import "cardinalshim/cardinal"

type Move struct{ X int32 }

func (Move) Name() string { return "move" }

type Unused struct{ X int32 }

func (Unused) Name() string { return "unused" }

func Run(w *cardinal.World) {
	for range w.Commands[Move]() {
	}
}`)
	res, err := sdkgen.Discover(filepath.Join(root, "game"))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	var got []string
	for _, v := range res.Violations {
		got = append(got, v.Category+" "+v.Where+" "+v.GoType)
	}
	if want := []string{"undeclared game.Move cardinal.Commands"}; !slices.Equal(got, want) {
		t.Errorf("violations\n  got  %q\n  want %q", got, want)
	}
	if want := []string{`Unused (Name()="unused")`}; !slices.Equal(res.Orphans, want) {
		t.Errorf("orphans\n  got  %q\n  want %q", res.Orphans, want)
	}
}

// TestTimestamp verifies time.Time (value, pointer, slice, map-value) maps to the google.protobuf.Timestamp
// well-known type — a clean typed field, not a violation and not a bytes fallback — and pulls its import.
func TestTimestamp(t *testing.T) {
	t.Parallel()
	body := `import (
	"time"

	"sdkgentest/cardinal"
)

type C struct {
	CreatedAt  time.Time
	DeletedAt  *time.Time
	Seen       []time.Time
	LastByTeam map[string]time.Time
}

func (C) Name() string { return "c" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[C]()
}`
	res := discover(t, body)
	// A shape finding is expected here: the fields are a pointer, a slice and a map, which hold
	// references whatever their element type. What this test pins is that time.Time itself maps cleanly,
	// so nothing is reported about its representation.
	shapes := map[string]bool{sdkgen.CatPointer: true, sdkgen.CatSlice: true, sdkgen.CatMap: true}
	for _, v := range res.Violations {
		if !shapes[v.Category] {
			t.Fatalf("time.Time is a clean mapping, should not be reported: %+v", v)
		}
	}
	out := proto(t, body)
	for _, want := range []string{
		`import "google/protobuf/timestamp.proto";`,
		"google.protobuf.Timestamp CreatedAt = 1;",
		"google.protobuf.Timestamp DeletedAt = 2;",
		"repeated google.protobuf.Timestamp Seen = 3;",
		"map<string, google.protobuf.Timestamp> LastByTeam = 4;",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("proto missing %q:\n%s", want, out)
		}
	}
}

// TestForeignStructIsMirrored verifies a struct another module owns is rebuilt into this schema as a real
// message rather than an opaque JSON blob — the generator can read its Go, so there is nothing to guess.
// A foreign struct with no exported fields is the exception: nothing crosses the wire, so it violates.
func TestForeignStructIsMirrored(t *testing.T) {
	t.Parallel()

	t.Run("readable struct is mirrored", func(t *testing.T) {
		t.Parallel()
		body := `import (
	"sdkgentest/cardinal"
	"net"
)

type C struct {
	Addr net.TCPAddr
	X    int32
}

func (C) Name() string { return "c" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[C]()
}`
		res := discover(t, body)
		if bad := notDataFindings(res); len(bad) != 0 {
			t.Fatalf("a readable foreign struct should mirror, not violate: %+v", bad)
		}
	})

	t.Run("struct with no exported fields violates", func(t *testing.T) {
		t.Parallel()
		body := `import (
	"sdkgentest/cardinal"
	"sync"
)

type C struct {
	Mu sync.Mutex
	X  int32
}

func (C) Name() string { return "c" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[C]()
}`
		res := discover(t, body)
		if len(res.Violations) == 0 {
			t.Fatalf("sync.Mutex exposes nothing; generating it would emit an empty message: %+v", res.Violations)
		}
		if res.Violations[0].Category != sdkgen.CatUnserializable {
			t.Fatalf("want %s, got %+v", sdkgen.CatUnserializable, res.Violations[0])
		}
	})
}

// TestSanitizerKeepsIdentifiersValid pins the two rules the sanitizer exists to enforce: an existing
// underscore survives unescaped (protobuf forbids "__", and the C# namespace built from this is read by
// game developers), and a segment may not start with a digit, which a Go path element may.
func TestSanitizerKeepsIdentifiersValid(t *testing.T) {
	t.Parallel()

	const mod = "github.com/x/mod"
	under, _ := sdkgen.ProtoPackageOf(mod, "github.com/x/my_game")
	if strings.Contains(under, "__") {
		t.Errorf("protobuf forbids a doubled underscore: %q", under)
	}
	if !strings.Contains(under, ".my_game") {
		t.Errorf("an existing underscore should pass through unchanged: %q", under)
	}
	if digit, _ := sdkgen.ProtoPackageOf(mod, mod+"/2d/component"); strings.Contains(digit, ".2d.") {
		t.Errorf("a segment starting with a digit is not a valid identifier: %q", digit)
	}
}

// TestDuplicateName verifies the duplicate proto-name check fires on a sanitizer collapse.
//
// The sanitizer replaces every non-identifier rune with "_" and leaves an existing "_" alone, so
// "abc-x" and "abc_x" both become "abc_x". Go import path elements may contain either. This check is
// what keeps them apart; the alternative is protoc reporting the clash cryptically, far from the two
// directories that caused it.
func TestDuplicateName(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module sdkgentest\n\ngo 1.27\n")
	write("cardinal/cardinal.go", cardinalShim)
	pkg := func(pkgName string) string {
		return `package ` + pkgName + `

import "sdkgentest/cardinal"

type C struct {
	X int32
}

func (C) Name() string { return "` + pkgName + `_c" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[C]()
}`
	}
	write("abc-x/a.go", pkg("abcx"))  // import path sdkgentest/abc-x -> proto pkg ...abc_x
	write("abc_x/b.go", pkg("abcx2")) // import path sdkgentest/abc_x -> proto pkg ...abc_x (collides)

	res, err := sdkgen.Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	var found *sdkgen.Violation
	for i := range res.Violations {
		if res.Violations[i].Category == sdkgen.CatDuplicateName {
			found = &res.Violations[i]
		}
	}
	if found == nil {
		t.Fatalf("expected a %s violation, got: %v", sdkgen.CatDuplicateName, res.Violations)
	}
	if !strings.Contains(found.Reason, "same name") {
		t.Errorf("duplicate-name reason unexpected: %s", found.Reason)
	}
}

// TestMultiRole verifies a type wired under two top-level roles (command + component) is generated once
// with the union of its roles' recorded — not reported as a clash. All wire kinds emit the same methods;
// a role only tunes the output target, so multiple roles merge rather than conflict.
func TestMultiRole(t *testing.T) {
	t.Parallel()
	body := `import "sdkgentest/cardinal"

type Thing struct {
	X int32
}

func (Thing) Name() string { return "thing" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[Thing]()
	w.RegisterComponent[Thing]()
}`
	res := discover(t, body)
	if len(res.Violations) != 0 {
		t.Fatalf("multi-role type should not clash, got violations: %v", res.Violations)
	}
	var thing *sdkgen.Message
	n := 0
	for i := range res.Messages {
		if res.Messages[i].Name == "Thing" {
			thing = &res.Messages[i]
			n++
		}
	}
	if n != 1 {
		t.Fatalf("expected Thing generated exactly once, got %d", n)
	}
	hasCmd, hasComp := false, false
	for _, k := range thing.Kinds {
		if k == sdkgen.KindCommand {
			hasCmd = true
		}
		if k == sdkgen.KindComponent {
			hasComp = true
		}
	}
	if !hasCmd || !hasComp {
		t.Errorf("expected Thing.Kinds to include command and component, got %v", thing.Kinds)
	}
}

// wire renders the generated Go wire code for a discovered body, the counterpart to proto above.
func wire(t *testing.T, body string) string {
	t.Helper()
	res := discover(t, body)
	if bad := notDataFindings(res); len(bad) != 0 {
		t.Fatalf("unexpected violations: %v", bad)
	}
	// A real --go-out, matching proto() above: an empty one used to be tolerated only because the gen
	// alias was re-derived per call site, and the renderer now reads the path off the message.
	res.ResolveGen("sdkgentest/gen")
	out, err := sdkgen.RenderGoWire(res.Messages, nil)
	if err != nil {
		t.Fatalf("render wire: %v", err)
	}
	return out
}

// -------------------------------------------------------------------------------------------------
// Fixed arrays
// -------------------------------------------------------------------------------------------------
// A fixed array carries its length in the schema rather than the payload, so it flattens to one
// repeated field of its base element in row-major order, however many dimensions it has.

func TestFixedArray_FlattensToRepeated(t *testing.T) {
	t.Parallel()

	for _, c := range []struct{ name, field, wantType, wantBound string }{
		{"scalar", "V [8]int32", "repeated int32 V", "// Go: [8]int32, 8 elements"},
		{"two dimensions", "V [4][8]int32", "repeated int32 V", "// Go: [4][8]int32 (row-major), 32 elements"},
		{"three dimensions", "V [2][3][4]float64", "repeated double V", "24 elements"},
		{"byte array is a blob", "ID [16]byte", "bytes ID", "// Go: [16]byte, 16 elements"},
	} {
		t.Run(c.name, func(t *testing.T) {
			res := discover(t, cmd("c", "\t"+c.field))
			if len(res.Violations) != 0 {
				t.Fatalf("a fixed array should generate cleanly, got: %+v", res.Violations)
			}
			got := proto(t, cmd("c", "\t"+c.field))
			if !strings.Contains(got, c.wantType) {
				t.Errorf("want %q in:\n%s", c.wantType, got)
			}
			if !strings.Contains(got, c.wantBound) {
				t.Errorf("want shape comment %q in:\n%s", c.wantBound, got)
			}
		})
	}
}

// TestFixedArray_RecordsShape covers the part a flat repeated field cannot express on its own: the
// dimensions are only recoverable from the schema, so the generator writes them down.
func TestFixedArray_RecordsShape(t *testing.T) {
	t.Parallel()

	got := proto(t, cmd("c", "\tV [4][8]int32"))
	if !strings.Contains(got, "// Go: [4][8]") {
		t.Errorf("want the Go shape recorded as a comment, got:\n%s", got)
	}
	// The codegen toolchain resolves no BSR dependencies, so the shape must not pull one in.
	if strings.Contains(got, "buf/validate") {
		t.Errorf("the emitted proto must not depend on protovalidate, got:\n%s", got)
	}
}

// TestFixedArray_WireRoundTrip pins the generated encode and decode: encode walks the dimensions,
// decode walks the flat field once and rebuilds the indices from the strides, bounded by the array's
// own length so an over-long message truncates instead of panicking.
func TestFixedArray_WireRoundTrip(t *testing.T) {
	t.Parallel()

	got := wire(t, cmd("c", "\tV [4][8]int32"))
	for _, want := range []string{
		"for i0 := range c.V {",
		"for i1 := range c.V[i0] {",
		"p.V = append(p.V, int32(c.V[i0][i1]))",
		"if i >= 32 {",
		"c.V[i/8][i%8] = int32(e)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in:\n%s", want, got)
		}
	}
}

func TestFixedArray_ThreeDimensionalIndexing(t *testing.T) {
	t.Parallel()

	got := wire(t, cmd("c", "\tV [2][3][4]float64"))
	if want := "c.V[i/12][(i/4)%3][i%4] = float64(e)"; !strings.Contains(got, want) {
		t.Errorf("want %q in:\n%s", want, got)
	}
}

func TestFixedArray_ByteArrayCopies(t *testing.T) {
	t.Parallel()

	got := wire(t, cmd("c", "\tID [16]byte"))
	for _, want := range []string{"p.ID = c.ID[:]", "copy(c.ID[:], p.ID)"} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in:\n%s", want, got)
		}
	}
}

// TestFixedArray_ZeroNonLeadingDimRefused pins that a fixed array with a zero-length NON-LEADING
// dimension is refused rather than emitted as uncompilable code.
//
// The decode emitter rebuilds each index as a quotient/remainder of the flat position by the
// per-dimension stride, and the stride is the product of the dimensions to its right — which is 0
// when any inner dim is 0 — so without this refusal the generator emits `c.V[i/0][i%0]`, a
// compile-time division by zero. A refused field is not appended to the message (see addFields), so
// the emitted wire file contains no broken index expression for it. The category is
// CatUnsupportedType because the array recurses to a basic (representable) element: shapeCategory
// walks the array to its leaf and returns "", and refusedCategory falls through to unsupported.
func TestFixedArray_ZeroNonLeadingDimRefused(t *testing.T) {
	t.Parallel()

	for _, c := range []struct{ name, field string }{
		{"2D int32, zero second dim", "V [4][0]int32"},
		{"2D byte, zero second dim", "ID [4][0]byte"},
		{"3D int32, zero middle dim", "V [2][0][3]int32"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			res := discover(t, cmd("c", "\t"+c.field))
			if len(res.Violations) != 1 {
				t.Fatalf("want exactly one finding, got %+v", res.Violations)
			}
			v := res.Violations[0]
			if v.Category != sdkgen.CatUnsupportedType {
				t.Errorf("category: want %s, got %s (reason: %s)", sdkgen.CatUnsupportedType, v.Category, v.Reason)
			}
			if !strings.Contains(v.Reason, "zero") || !strings.Contains(v.Reason, "row-major") {
				t.Errorf("reason should explain the zero-dim row-major problem, got: %q", v.Reason)
			}
			// The refused field is not appended to the message, so the rendered wire file has no
			// assignment to it — broken (`i/0`, `%0`, `/0`) or otherwise.
			res.ResolveGen("sdkgentest/gen")
			out, err := sdkgen.RenderGoWire(res.Messages, nil)
			if err != nil {
				t.Fatalf("render wire: %v", err)
			}
			for _, broken := range []string{"i/0", "%0", "/0"} {
				if strings.Contains(out, broken) {
					t.Errorf("emitted wire must not contain %q, got:\n%s", broken, out)
				}
			}
		})
	}
}

// TestFixedArray_ZeroLeadingDimStillCompiles pins the asymmetry that justifies targeting only the
// non-leading dimension: a zero LEADING dimension carries no data, but no stride or modulus becomes
// 0, so the (unreachable) loop body still compiles. The generator must keep accepting it.
func TestFixedArray_ZeroLeadingDimStillCompiles(t *testing.T) {
	t.Parallel()

	for _, c := range []struct{ name, field, wantIndex string }{
		{"zero leading, multi-dim", "V [0][8]int32", "c.V[i/8][i%8] = int32(e)"},
		{"zero single dim", "V [0]int32", "c.V[i] = int32(e)"},
		{"zero leading byte, multi-dim", "ID [0][16]byte", "c.ID[i/16][i%16] = v"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			res := discover(t, cmd("c", "\t"+c.field))
			if len(res.Violations) != 0 {
				t.Fatalf("a zero LEADING dimension should generate cleanly, got: %+v", res.Violations)
			}
			got := wire(t, cmd("c", "\t"+c.field))
			if !strings.Contains(got, c.wantIndex) {
				t.Errorf("want %q in:\n%s", c.wantIndex, got)
			}
			for _, broken := range []string{"i/0", "%0", "/0"} {
				if strings.Contains(got, broken) {
					t.Errorf("emitted wire must not contain %q, got:\n%s", broken, got)
				}
			}
		})
	}
}

// -------------------------------------------------------------------------------------------------
// Shape findings
// -------------------------------------------------------------------------------------------------
// A component is copied by value to freeze a snapshot, so a field holding a reference leaves the copy
// sharing memory with the live world. Findings are named after the Go type that causes them — slice,
// map, pointer, interface — and applied to every wire kind alike, so "is this allowed?" is answerable
// by reading the field rather than by knowing how the type is wired.

// shapeFindings returns the findings raised for a field's shape, as "Type.Field" strings.
func shapeFindings(t *testing.T, body string) []string {
	t.Helper()
	res := discover(t, body)
	var out []string
	shapes := map[string]bool{
		sdkgen.CatSlice: true, sdkgen.CatMap: true, sdkgen.CatPointer: true, sdkgen.CatInterface: true,
	}
	for _, v := range res.Violations {
		if shapes[v.Category] {
			out = append(out, v.Where)
		}
	}
	return out
}

func component(fields string) string {
	return `import "sdkgentest/cardinal"

type Pos struct {
` + fields + `
}

func (Pos) Name() string { return "pos" }

func Setup(w *cardinal.World) {
	w.RegisterComponent[Pos]()
}`
}

// TestShapeFindingsReachNestedTypes covers a field that is not on the wire type itself but on a struct
// it holds. Nesting is not a way out of the rule, and the finding names the field's own message rather
// than the outer one, so the report points at the line that has to change.
func TestShapeFindingsReachNestedTypes(t *testing.T) {
	t.Parallel()

	body := `import "sdkgentest/cardinal"

type Inner struct {
	Tags []string
}

type Pos struct {
	Inner Inner
}

func (Pos) Name() string { return "pos" }

func Setup(w *cardinal.World) {
	w.RegisterComponent[Pos]()
}`
	got := shapeFindings(t, body)
	if len(got) != 1 || got[0] != "Inner.Tags" {
		t.Errorf("want the nested field reported as Inner.Tags, got %v", got)
	}
}

// TestUnsupportedTypeIsReported covers a type this generator has no mapping for that holds no reference:
// safe to store, and with no offending shape, so the shape categories cannot catch it. Its own category
// is what keeps it from being refused with nothing said about why.
func TestUnsupportedTypeIsReported(t *testing.T) {
	t.Parallel()

	body := `import "sdkgentest/cardinal"

type Pos struct {
	Grid [2][3][4]uintptr
}

func (Pos) Name() string { return "pos" }

func Setup(w *cardinal.World) {
	w.RegisterComponent[Pos]()
}`
	res := discover(t, body)
	if len(res.Violations) != 1 {
		t.Fatalf("want exactly one finding, got %+v", res.Violations)
	}
	if got := res.Violations[0].Category; got != sdkgen.CatUnsupportedType {
		t.Fatalf("want %s, got %s (%+v)", sdkgen.CatUnsupportedType, got, res.Violations[0])
	}
}

// -------------------------------------------------------------------------------------------------
// The shape matrix
// -------------------------------------------------------------------------------------------------
// One rule decides everything here: a fixed array flattens when its base element is a scalar, a
// string, a named struct, or a byte; anything else is refused, and no field is emitted for it. This
// table pins that rule for every shape at once, so a special case added for one of them cannot
// quietly make it inconsistent with the rest.
//
// It exists because that happened: [N]byte was handled only at one dimension, so [4][16]byte fell to
// JSON while [4][8]int32 flattened — an exception nothing tested and nothing justified.

// protoFieldType returns the proto type rendered for a message's field V.
func protoFieldType(t *testing.T, body string) string {
	t.Helper()
	for line := range strings.SplitSeq(proto(t, body), "\n") {
		if strings.Contains(line, " V = ") {
			return strings.TrimSpace(strings.Split(line, " V = ")[0])
		}
	}
	return ""
}

// findingCategories returns the categories reported for body, sorted for a stable comparison.
func findingCategories(t *testing.T, body string) []string {
	t.Helper()
	res := discover(t, body)
	var cats []string
	for _, v := range res.Violations {
		cats = append(cats, v.Category)
	}
	slices.Sort(cats)
	return cats
}

func TestShapeMatrix(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		field       string
		wantProto   string   // "" when the field is refused, so no field is emitted for it
		wantFinding []string // nil means it must generate cleanly
	}{
		// Leaves and single-level containers.
		{"scalar", "V int32", "int32", nil},
		{"string", "V string", "string", nil},
		{"named struct", "V Vec2", "Vec2", nil},
		{"slice of scalar", "V []int32", "repeated int32", []string{sdkgen.CatSlice}},
		{"slice of struct", "V []Vec2", "repeated Vec2", []string{sdkgen.CatSlice}},
		{"byte slice", "V []byte", "bytes", []string{sdkgen.CatSlice}},
		{"map", "V map[string]int32", "map<string, int32>", []string{sdkgen.CatMap}},
		{"pointer to scalar", "V *int32", "optional int32", []string{sdkgen.CatPointer}},
		{"pointer to struct", "V *Vec2", "Vec2", []string{sdkgen.CatPointer}},

		// Fixed arrays flatten, however deep, whatever the base — scalar, struct or byte.
		{"array of scalar", "V [8]int32", "repeated int32", nil},
		{"array of array of scalar", "V [4][8]int32", "repeated int32", nil},
		{"three dimensions", "V [2][3][4]float64", "repeated double", nil},
		{"array of struct", "V [2]Vec2", "repeated Vec2", nil},
		{"array of array of struct", "V [2][2]Vec2", "repeated Vec2", nil},
		{"byte array", "V [16]byte", "bytes", nil},
		{"byte array, two dimensions", "V [4][16]byte", "bytes", nil},
		{"byte array, three dimensions", "V [2][3][4]byte", "bytes", nil},

		// A collection under a fixed array has no flat layout: the inner length is data, not schema.
		{"array of slice", "V [2][]int32", "", []string{sdkgen.CatSlice}},
		{"slice of array", "V [][3]int32", "", []string{sdkgen.CatSlice}},
		{"array of array of slice", "V [2][3][]int32", "", []string{sdkgen.CatSlice}},
		{"array of map", "V [4]map[string]int32", "", []string{sdkgen.CatMap}},

		// Shapes proto cannot type for their own reasons, unrelated to arrays.
		{"map with slice value", "V map[string][]int32", "", []string{sdkgen.CatMap}},
		{"nested slice", "V [][]int32", "", []string{sdkgen.CatSlice}},
		{"interface", "V any", "", []string{sdkgen.CatInterface}},
		{"interface literal", "V interface{}", "", []string{sdkgen.CatInterface}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			body := `import "sdkgentest/cardinal"

type Vec2 struct{ X, Y float64 }

type C struct {
	` + tc.field + `
}

func (C) Name() string { return "c" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[C]()
}`
			if got := protoFieldType(t, body); got != tc.wantProto {
				t.Errorf("proto type: want %q, got %q", tc.wantProto, got)
			}
			got := findingCategories(t, body)
			if len(got) != len(tc.wantFinding) {
				t.Fatalf("findings: want %v, got %v", tc.wantFinding, got)
			}
			for i := range got {
				if got[i] != tc.wantFinding[i] {
					t.Errorf("findings: want %v, got %v", tc.wantFinding, got)
				}
			}
		})
	}
}

// TestShapeRulesAreKindIndependent pins the property the whole design rests on: nothing the generator
// decides depends on whether a type is a command, an event or a component.
//
// Both questions are answered from the Go type alone — what it becomes on the wire, and whether its
// bytes are the whole value. A change that starts branching on kind fails here, which is the point:
// the moment one kind is treated differently, "is this allowed?" stops being answerable by reading
// the field.
func TestShapeRulesAreKindIndependent(t *testing.T) {
	t.Parallel()

	kinds := map[string]string{
		"command":   "\tw.RegisterCommand[C]()",
		"event":     "\tw.RegisterEvent[C]()",
		"component": "\tw.RegisterComponent[C]()",
	}

	for _, tc := range []struct {
		field       string
		wantProto   string
		wantFinding []string
	}{
		{"V [4][8]int32", "repeated int32", nil},
		{"V []int32", "repeated int32", []string{sdkgen.CatSlice}},
		{"V [2][]int32", "", []string{sdkgen.CatSlice}},
		{"V map[string][]int32", "", []string{sdkgen.CatMap}},
	} {
		t.Run(tc.field, func(t *testing.T) {
			t.Parallel()

			for kind, decl := range kinds {
				body := `import "sdkgentest/cardinal"

type C struct {
	` + tc.field + `
}

func (C) Name() string { return "c" }

func Setup(w *cardinal.World) {
` + decl + `
}`
				if got := protoFieldType(t, body); got != tc.wantProto {
					t.Errorf("%s: proto type: want %q, got %q", kind, tc.wantProto, got)
				}
				if got := findingCategories(t, body); !slices.Equal(got, tc.wantFinding) {
					t.Errorf("%s: findings: want %v, got %v", kind, tc.wantFinding, got)
				}
			}
		})
	}
}

// TestMirroredFieldsDriveImports pins that a mirrored type's fields reach the import list.
//
// A mirrored converter is written into this package's wire file, so its fields need exactly the imports
// a local message's would. The feature scan and the import collection used to walk only the local
// messages, so a foreign type carrying a time.Time emitted timestamppb.New with no timestamppb import:
// generation reported success and the package failed to compile.
func TestMirroredFieldsDriveImports(t *testing.T) {
	t.Parallel()

	root := multiModule(t, map[string]string{
		"cardinalshim/go.mod":               "module cardinalshim\n\ngo 1.27\n",
		"cardinalshim/cardinal/cardinal.go": cardinalShim,
		// Never generates, so Stamp mirrors. Its fields are the ONLY source of time and json in this run.
		"dep/go.mod": "module dep\n\ngo 1.27\n",
		"dep/stamp/stamp.go": "package stamp\n\nimport \"time\"\n\n" +
			"type Stamp struct {\n\tAt time.Time\n}\n",
		"game/go.mod": "module game\n\ngo 1.27\n\nrequire (\n\tcardinalshim v0.0.0\n\tdep v0.0.0\n)\n\n" +
			"replace cardinalshim => ../cardinalshim\n\nreplace dep => ../dep\n",
		// MarkCommand itself has no time field, so the timestamppb import can only come from Stamp.
		"game/game.go": `package game

import (
	"cardinalshim/cardinal"
	"dep/stamp"
)

type MarkCommand struct {
	Label string
	S     stamp.Stamp
}

func (MarkCommand) Name() string { return "mark" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[MarkCommand]()
}`,
	})

	res, err := sdkgen.Discover(filepath.Join(root, "game"))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	local, mirrored := splitForRender(res, "game/gen")
	if _, ok := mirrored["dep/stamp.Stamp"]; !ok {
		t.Fatalf("Stamp should have mirrored; got mirrored=%v", slices.Sorted(maps.Keys(mirrored)))
	}
	out, err := sdkgen.RenderGoWire(local, mirrored)
	if err != nil {
		t.Fatalf("RenderGoWire: %v", err)
	}

	// The body/header pair: use the symbol, import the package. Either half alone does not compile.
	for _, want := range []struct{ symbol, imp string }{
		{"timestamppb.", "google.golang.org/protobuf/types/known/timestamppb"},
	} {
		if !strings.Contains(out, want.symbol) {
			t.Errorf("expected the mirrored converter to use %s; it is what makes the import load-bearing", want.symbol)
			continue
		}
		if !strings.Contains(out, strconv.Quote(want.imp)) {
			t.Errorf("body uses %s but %q is not imported:\n%s", want.symbol, want.imp, out)
		}
	}
}

// TestOwnerGenPackageAtModuleRoot pins the choice among a root package's gen imports.
//
// The owner's gen import is found by matching a wire file's imports against the package's module-relative
// path — which is empty for a package AT the module root, leaving every gen import in the file a match
// and the first one read winning. Here that first one is a different package's, and taking it would point
// protoc-gen-go at the wrong Go type.
func TestOwnerGenPackageAtModuleRoot(t *testing.T) {
	t.Parallel()

	root := multiModule(t, map[string]string{
		"cardinalshim/go.mod":               "module cardinalshim\n\ngo 1.27\n",
		"cardinalshim/cardinal/cardinal.go": cardinalShim,
		"dep/go.mod":                        "module dep\n\ngo 1.27\n",
		// The wire type lives at the module root, so its own gen package is the gen root.
		"dep/dep.go":             "package dep\n\ntype Vec2 struct{ X, Y int32 }\n",
		"dep/inner/inner.go":     "package inner\n\ntype Tag struct{ S string }\n",
		"dep/gen/gen.go":         "package gen\n\ntype Vec2 struct{}\n",
		"dep/gen/inner/inner.go": "package inner\n\ntype Tag struct{}\n",
		// A real wire file imports every gen package it references, and gofmt sorts them, so the deeper
		// package's import legitimately comes first. Only the root one describes THIS package.
		"dep/wire.gen.go": "//go:build !sdkgen\n\npackage dep\n\nimport (\n" +
			"\tpbinner \"dep/gen/inner\"\n" +
			"\tpbgen \"dep/gen\"\n" +
			")\n\n" +
			"var _ = pbinner.Tag{}\n\nfunc (c Vec2) ToProto() *pbgen.Vec2 { return nil }\n\nfunc (c Vec2) SizeWire() int { return 0 }\n",
		"game/go.mod": "module game\n\ngo 1.27\n\nrequire (\n\tcardinalshim v0.0.0\n\tdep v0.0.0\n)\n\n" +
			"replace cardinalshim => ../cardinalshim\n\nreplace dep => ../dep\n",
		"game/game.go": `package game

import (
	"cardinalshim/cardinal"
	"dep"
)

type MoveCommand struct{ Target dep.Vec2 }

func (MoveCommand) Name() string { return "move" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[MoveCommand]()
}`,
	})

	res, err := sdkgen.Discover(filepath.Join(root, "game"))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	files, err := sdkgen.EmitProtos(res.Module, "game/gen", res.Messages)
	if err != nil {
		t.Fatalf("EmitProtos: %v", err)
	}

	var found bool
	for _, f := range files {
		if !strings.Contains(f.Content, "message Vec2") {
			continue
		}
		found = true
		if !strings.Contains(f.Content, `option go_package = "dep/gen;`) {
			t.Errorf("want the root gen package dep/gen, got:\n%s", f.Content)
		}
		if strings.Contains(f.Content, `option go_package = "dep/gen/inner`) {
			t.Errorf("took another package's gen import for a root package:\n%s", f.Content)
		}
	}
	if !found {
		t.Fatal("no proto emitted for Vec2")
	}
}

// TestForeignTypeWiredAndUsedAsField pins that a second sighting under a new role still gets built.
//
// A dependency's top-level type is handed to the C#-only pass and put on no build queue. Reaching that
// same type again as a FIELD makes it data this schema carries, but the walk short-circuited on "already
// seen" and queued it for neither — so the wire file called a converter nothing declared.
func TestForeignTypeWiredAndUsedAsField(t *testing.T) {
	t.Parallel()

	root := multiModule(t, map[string]string{
		"cardinalshim/go.mod":               "module cardinalshim\n\ngo 1.27\n",
		"cardinalshim/cardinal/cardinal.go": cardinalShim,
		"dep/go.mod":                        "module dep\n\ngo 1.27\n",
		"dep/ping/ping.go": "package ping\n\ntype Ping struct{ N int32 }\n\n" +
			"func (Ping) Name() string { return \"ping\" }\n",
		"game/go.mod": "module game\n\ngo 1.27\n\nrequire (\n\tcardinalshim v0.0.0\n\tdep v0.0.0\n)\n\n" +
			"replace cardinalshim => ../cardinalshim\n\nreplace dep => ../dep\n",
		// Field order is the walk order: Ping is wired top-level FIRST, then reached as a field of Wrap.
		"game/game.go": `package game

import (
	"cardinalshim/cardinal"
	"dep/ping"
)

type WrapCommand struct{ P ping.Ping }

func (WrapCommand) Name() string { return "wrap" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[ping.Ping]()
	w.RegisterCommand[WrapCommand]()
}`,
	})

	res, err := sdkgen.Discover(filepath.Join(root, "game"))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	local, mirrored := splitForRender(res, "game/gen")
	out, err := sdkgen.RenderGoWire(local, mirrored)
	if err != nil {
		t.Fatalf("RenderGoWire: %v", err)
	}

	// Every mirror converter the body calls has to be declared in the same file.
	called := regexp.MustCompile(`(mirror\w+(?:To|From)Proto)\(`).FindAllStringSubmatch(out, -1)
	if len(called) == 0 {
		t.Fatalf("Ping is a foreign field, so its converter should be called:\n%s", out)
	}
	for _, c := range called {
		if !strings.Contains(out, "func "+c[1]+"(") {
			t.Errorf("body calls %s but nothing declares it:\n%s", c[1], out)
		}
	}
}

// TestMergeMessagesKeepsOneCopy pins that a package reachable both ways is emitted once.
//
// A dependency's type can be reached as a local field AND wired as an external top-level type, so it
// lands in both message lists. Emitting each list separately gave two .proto files claiming one path,
// and the work dir keeps only the last: every message the loser held disappeared from the client SDK
// with nothing reported. Local is passed first because it is the copy the rest of the schema points at.
func TestMergeMessagesKeepsOneCopy(t *testing.T) {
	t.Parallel()

	got := sdkgen.MergeMessages(
		[]sdkgen.Message{
			{ProtoPkg: "dep.component", Name: "Vec2", Dir: "local"},
			{ProtoPkg: "game.command", Name: "MoveCommand"},
		},
		[]sdkgen.Message{
			{ProtoPkg: "dep.component", Name: "Vec2", Dir: "external"}, // same message, reached the other way
			{ProtoPkg: "dep.command", Name: "PingCommand"},
		},
	)

	seen := map[string]int{}
	for _, m := range got {
		seen[m.ProtoPkg+"."+m.Name]++
	}
	if n := seen["dep.component.Vec2"]; n != 1 {
		t.Errorf("Vec2 is in both lists and must survive exactly once, got %d", n)
	}
	// Deduping must not cost the messages that are only in one list.
	for _, want := range []string{"game.command.MoveCommand", "dep.command.PingCommand"} {
		if seen[want] != 1 {
			t.Errorf("%s was dropped by the merge", want)
		}
	}
	for _, m := range got {
		if m.Name == "Vec2" && m.Dir != "local" {
			t.Errorf("the first group must win a tie; got the copy from %q", m.Dir)
		}
	}
}

// TestOwnerGenPackageAcceptsUnprefixedAlias pins that an older wire file still reads as "generated".
//
// The current generator aliases every gen import with a "pb" prefix, which is what tells a generated
// package from the hand-written ones beside it. Backends generated before that prefix existed carry bare
// aliases, and treating the prefix as a requirement would read their wire file as importing nothing —
// reporting the owner as having generated nothing, and mirroring a type they already ship.
func TestOwnerGenPackageAcceptsUnprefixedAlias(t *testing.T) {
	t.Parallel()

	root := multiModule(t, map[string]string{
		"cardinalshim/go.mod":               "module cardinalshim\n\ngo 1.27\n",
		"cardinalshim/cardinal/cardinal.go": cardinalShim,
		"dep/go.mod":                        "module dep\n\ngo 1.27\n",
		"dep/component/component.go":        "package component\n\ntype Vec2 struct{ X, Y int32 }\n",
		// The pre-"pb" alias scheme: the gen import is named after the package itself.
		"dep/component/wire.gen.go": "//go:build !sdkgen\n\npackage component\n\n" +
			"import component \"dep/gen/component\"\n\n" +
			"func (c Vec2) ToProto() *component.Vec2 { return nil }\n\nfunc (c Vec2) SizeWire() int { return 0 }\n",
		"dep/gen/component/component.go": "package component\n\ntype Vec2 struct{}\n",
		"game/go.mod": "module game\n\ngo 1.27\n\nrequire (\n\tcardinalshim v0.0.0\n\tdep v0.0.0\n)\n\n" +
			"replace cardinalshim => ../cardinalshim\n\nreplace dep => ../dep\n",
		"game/game.go": `package game

import (
	"cardinalshim/cardinal"
	"dep/component"
)

type MoveCommand struct{ Target component.Vec2 }

func (MoveCommand) Name() string { return "move" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[MoveCommand]()
}`,
	})

	res, err := sdkgen.Discover(filepath.Join(root, "game"))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	// Ownership is the whole answer: OwnImported means the owner generated and we point at their Go.
	var found bool
	for _, m := range res.Messages {
		if m.Name != "Vec2" {
			continue
		}
		found = true
		if m.Own != sdkgen.OwnImported || m.GenImport != "dep/gen/component" {
			t.Errorf("the owner generated Vec2, so it must be imported from dep/gen/component, got own=%s %q",
				m.Own, m.GenImport)
		}
	}
	if !found {
		t.Fatal("no Vec2 message was discovered")
	}
}

// TestLocalPackagesAreNotImportOnly pins that a re-run generates its own output.
//
// "The owner already generated this" is read out of a package's wire.gen.go, and a LOCAL package has one
// too — naming the gen import this very run writes. Consulting it for local packages made every file
// import-only from the second run onward. Nothing failed: the schema still resolved, and buf given no
// --path generates everything rather than nothing, so the output looked right until one foreign file
// appeared and the filter finally had something to select. Then a re-run emitted that file alone and
// wiped the rest of the tree.
func TestLocalPackagesAreNotImportOnly(t *testing.T) {
	t.Parallel()

	root := multiModule(t, map[string]string{
		"cardinalshim/go.mod":               "module cardinalshim\n\ngo 1.27\n",
		"cardinalshim/cardinal/cardinal.go": cardinalShim,
		"game/go.mod":                       "module game\n\ngo 1.27\n\nrequire cardinalshim v0.0.0\n\nreplace cardinalshim => ../cardinalshim\n",
		"game/component/component.go":       "package component\n\ntype Vec2 struct{ X, Y int32 }\n",
		// The state a second run starts from: this package generated last time, so its wire file names
		// the gen package THIS run writes. That is not another module having generated it.
		"game/component/wire.gen.go": "//go:build !sdkgen\n\npackage component\n\n" +
			"import pbcomponent \"game/gen/component\"\n\n" +
			"func (c Vec2) ToProto() *pbcomponent.Vec2 { return nil }\n\nfunc (c Vec2) SizeWire() int { return 0 }\n",
		"game/gen/component/component.go": "package component\n\ntype Vec2 struct{}\n",
		"game/game.go": `package game

import (
	"cardinalshim/cardinal"
	"game/component"
)

type MoveCommand struct{ Target component.Vec2 }

func (MoveCommand) Name() string { return "move" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[MoveCommand]()
}`,
	})

	res, err := sdkgen.Discover(filepath.Join(root, "game"))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	for _, m := range res.Messages {
		if m.Own != sdkgen.OwnLocal {
			t.Errorf("%s is this run's own package, so nothing else owns its generated code; got own=%s",
				m.PkgPath, m.Own)
		}
	}

	files, err := sdkgen.EmitProtos(res.Module, "game/gen", res.Messages)
	if err != nil {
		t.Fatalf("EmitProtos: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no proto files emitted")
	}
	for _, f := range files {
		if f.ImportOnly {
			t.Errorf("%s is this run's own file and must be generated, not imported", f.Path)
		}
	}
}

// TestNamedScalarFromAnotherPackageImportsIt covers a field whose type is a NAMED scalar declared in a
// different package of the same module (the shape of physics2d's cardinal.EntityID fields). The decode
// side must cast to the declared type, so it writes the name qualified — and the import has to travel
// with it. It did not: discovery recorded the import on the Scalar, the emitter wrote the qualified
// name, and nothing carried the import across, so every wire file for that package failed to render.
func TestNamedScalarFromAnotherPackageImportsIt(t *testing.T) {
	t.Parallel()

	root := multiModule(t, map[string]string{
		"cardinalshim/go.mod":               "module cardinalshim\n\ngo 1.27\n",
		"cardinalshim/cardinal/cardinal.go": cardinalShim,
		"game/go.mod": "module game\n\ngo 1.27\n\nrequire cardinalshim v0.0.0\n\n" +
			"replace cardinalshim => ../cardinalshim\n",
		// EntityID is a named scalar in a sibling package, so it is spelled ids.EntityID on decode.
		"game/ids/ids.go": "package ids\n\ntype EntityID uint32\n",
		"game/game.go": `package game

import (
	"cardinalshim/cardinal"
	"game/ids"
)

type MoveCommand struct {
	Entity ids.EntityID
	Owners []ids.EntityID
}

func (MoveCommand) Name() string { return "move" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[MoveCommand]()
}`,
	})

	res, err := sdkgen.Discover(filepath.Join(root, "game"))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	local, mirrored := splitForRender(res, "game/gen")
	// RenderGoWire runs checkSymbolsResolve, which is what rejects a qualified name with no import.
	out, err := sdkgen.RenderGoWire(local, mirrored)
	if err != nil {
		t.Fatalf("RenderGoWire: %v", err)
	}
	if !strings.Contains(out, `"game/ids"`) {
		t.Errorf("the named scalar's package should be imported:\n%s", out)
	}
	// Singular and repeated positions each write the cast independently.
	for _, want := range []string{"ids.EntityID(p.Entity)", "ids.EntityID(x)"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing qualified cast %q:\n%s", want, out)
		}
	}
}

// TestWireNamesCSEmitsOnePartialPerType pins that a type present in BOTH res.Messages and
// res.ExternalMessages yields a single partial. C# merges partial declarations of one class, so two
// blocks each defining WireName is a duplicate-member compile error, not an override. A same-named type
// in a different namespace is a different class and must still get its own partial.
func TestWireNamesCSEmitsOnePartialPerType(t *testing.T) {
	t.Parallel()

	dup := sdkgen.Message{
		Name: "Move", Wire: "move", ProtoPkg: "dep.command",
		CSNamespace: "Game.Dep.Command", Targets: []sdkgen.Target{sdkgen.TargetCSharp},
	}
	sameNameOtherNS := sdkgen.Message{
		Name: "Move", Wire: "move_v2", ProtoPkg: "local.command",
		CSNamespace: "Game.Local.Command", Targets: []sdkgen.Target{sdkgen.TargetCSharp},
	}
	out := sdkgen.EmitWireNamesCS(sdkgen.Result{
		Messages:         []sdkgen.Message{dup, sameNameOtherNS},
		ExternalMessages: []sdkgen.Message{dup},
	})

	if got := strings.Count(out, `WireName => "move"`); got != 1 {
		t.Errorf("the duplicated type should declare WireName once, got %d\n%s", got, out)
	}
	if got := strings.Count(out, `WireName => "move_v2"`); got != 1 {
		t.Errorf("a same-named type in another namespace must keep its own partial, got %d\n%s", got, out)
	}
}

// TestNotDataIgnoresUnexportedFields covers how deep the "is this data at all" walk looks.
//
// Only exported fields cross the wire, so an unexported chan is not a reason to call the struct holding
// it machinery — the field simply never reaches an encoder. An EXPORTED chan is, and so is a struct
// whose fields are all unexported: it would generate an empty message and drop the value in silence.
//
// Every case here is refused, because the field is a nested slice either way. What the test pins is
// WHICH finding: the shape, or the fact that something inside is not data. Getting that wrong offers
// "give it a bound" as the fix for a chan.
//
// A MarshalJSON/UnmarshalJSON pair no longer excuses an all-unexported struct. It did while the field
// could ride as opaque JSON and json would defer to those methods; with no fallback, nothing calls them
// and the struct has nothing a proto message could carry.
func TestNotDataIgnoresUnexportedFields(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name, inner, wantCat string
	}{
		{
			name:    "unexported chan alongside exported data",
			inner:   "type Inner struct {\n\tData int32\n\thidden chan int\n}",
			wantCat: sdkgen.CatSlice,
		},
		{
			name: "unexported-only, JSON methods no longer speak for it",
			inner: "type Inner struct{ v int32 }\n\n" +
				"func (Inner) MarshalJSON() ([]byte, error) { return nil, nil }\n" +
				"func (*Inner) UnmarshalJSON([]byte) error { return nil }",
			wantCat: sdkgen.CatUnserializable,
		},
		{
			name:    "unexported-only",
			inner:   "type Inner struct{ v int32 }",
			wantCat: sdkgen.CatUnserializable,
		},
		{
			name:    "exported chan",
			inner:   "type Inner struct {\n\tData int32\n\tCh chan int\n}",
			wantCat: sdkgen.CatUnserializable,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			body := "import \"sdkgentest/cardinal\"\n\n" + c.inner + `

type C struct {
	V [][]Inner
}

func (C) Name() string { return "c" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[C]()
}`
			res := discover(t, body)
			if len(res.Violations) != 1 {
				t.Fatalf("want exactly one finding, got %+v", res.Violations)
			}
			if got := res.Violations[0].Category; got != c.wantCat {
				t.Fatalf("category: want %s, got %s (%+v)", c.wantCat, got, res.Violations[0])
			}
		})
	}
}

// TestOwnerGenPackageRejectsLookalikeAlias pins that the gen import is matched by the whole alias, not
// by its "pb" prefix.
//
// The alias a hand-written import gets is <parent>_<leaf>, so a package whose parent directory is named
// "pb..." produces one that starts with "pb" too. The suffix filter cannot separate them: it pins the
// last two path segments, which both imports share by construction. Only reconstructing the alias the
// gen import would have been given ("pb" + leaf) tells them apart. The decoy is listed first so a
// first-candidate-wins read of a prefix match picks it.
func TestOwnerGenPackageRejectsLookalikeAlias(t *testing.T) {
	t.Parallel()

	root := multiModule(t, map[string]string{
		"cardinalshim/go.mod":               "module cardinalshim\n\ngo 1.27\n",
		"cardinalshim/cardinal/cardinal.go": cardinalShim,
		"dep/go.mod":                        "module dep\n\ngo 1.27\n",
		"dep/pbfoo/component/component.go":  "package component\n\ntype Vec2 struct{ X, Y int32 }\n",
		"dep/pbfoo/component/wire.gen.go": "//go:build !sdkgen\n\npackage component\n\n" +
			"import (\n" +
			"\tpbfoo_component \"dep/aaa/pbfoo/component\"\n" +
			"\tpbcomponent \"dep/gen/pbfoo/component\"\n" +
			")\n\n" +
			"func (c Vec2) ToProto() *pbcomponent.Vec2 { return nil }\n\nfunc (c Vec2) SizeWire() int { return 0 }\n",
		"dep/aaa/pbfoo/component/component.go": "package component\n\ntype Vec2 struct{}\n",
		"dep/gen/pbfoo/component/component.go": "package component\n\ntype Vec2 struct{}\n",
		"game/go.mod": "module game\n\ngo 1.27\n\nrequire (\n\tcardinalshim v0.0.0\n\tdep v0.0.0\n)\n\n" +
			"replace cardinalshim => ../cardinalshim\n\nreplace dep => ../dep\n",
		"game/game.go": `package game

import (
	"cardinalshim/cardinal"
	"dep/pbfoo/component"
)

type MoveCommand struct{ Target component.Vec2 }

func (MoveCommand) Name() string { return "move" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[MoveCommand]()
}`,
	})

	res, err := sdkgen.Discover(filepath.Join(root, "game"))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	var found bool
	for _, m := range res.Messages {
		if m.Name != "Vec2" {
			continue
		}
		found = true
		if m.GenImport != "dep/gen/pbfoo/component" {
			t.Errorf("Vec2 must be stamped with the owner's gen package, got %q", m.GenImport)
		}
	}
	if !found {
		t.Fatal("no Vec2 message was discovered")
	}
}

// TestExternalTopLevelKeepsOwnerGenImport pins that a dependency type reached BOTH as a local field and as
// an external top-level type keeps its owner's gen package.
//
// The two routes produce two Message copies: the field lands in res.Messages via the normal discoverer
// (OwnImported, stamped with the owner's path), and the top-level role lands in res.ExternalMessages via a
// second pass that deliberately treats the dependency's packages as local. That second pass cannot set
// GenImport, and ResolveGen fills an empty one with this run's --go-out — from ExternalMessages last, so
// the empty copy would win. The emitted wire file would then import the consumer's own gen tree for a type
// the consumer never generates.
func TestExternalTopLevelKeepsOwnerGenImport(t *testing.T) {
	t.Parallel()

	const ownerGen = "dep/gen/component"

	root := multiModule(t, map[string]string{
		"cardinalshim/go.mod":               "module cardinalshim\n\ngo 1.27\n",
		"cardinalshim/cardinal/cardinal.go": cardinalShim,
		"dep/go.mod":                        "module dep\n\ngo 1.27\n",
		"dep/component/component.go": "package component\n\n" +
			"type Health struct{ HP int32 }\n\nfunc (Health) Name() string { return \"health\" }\n",
		// The owner generated: their wire file records the --go-out choice, which is the only place it exists.
		"dep/component/wire.gen.go": "//go:build !sdkgen\n\npackage component\n\n" +
			"import pbcomponent \"dep/gen/component\"\n\n" +
			"func (c Health) ToProto() *pbcomponent.Health { return nil }\n\nfunc (c Health) SizeWire() int { return 0 }\n",
		"dep/gen/component/component.go": "package component\n\ntype Health struct{}\n",
		"game/go.mod": "module game\n\ngo 1.27\n\nrequire (\n\tcardinalshim v0.0.0\n\tdep v0.0.0\n)\n\n" +
			"replace cardinalshim => ../cardinalshim\n\nreplace dep => ../dep\n",
		"game/game.go": `package game

import (
	"cardinalshim/cardinal"
	"dep/component"
)

// Reached as a FIELD: reruns through the normal discoverer as OwnImported.
type ReportCommand struct{ H component.Health }

func (ReportCommand) Name() string { return "report" }

// Reached as a TOP-LEVEL component the dependency owns: recorded for the external pass.
func Setup(w *cardinal.World) {
	w.RegisterComponent[component.Health]()
	w.RegisterCommand[ReportCommand]()
}`,
	})

	res, err := sdkgen.Discover(filepath.Join(root, "game"))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	res.ResolveGen("game/gen")

	var sawLocal, sawExternal, sawRef bool
	for _, m := range res.Messages {
		if m.Name == "Health" {
			sawLocal = true
			if m.GenImport != ownerGen {
				t.Errorf("res.Messages Health: GenImport = %q, want %q", m.GenImport, ownerGen)
			}
		}
		for _, f := range m.Fields {
			if f.Msg.Name == "Health" {
				sawRef = true
				if f.Msg.GenImport != ownerGen {
					t.Errorf("field ref to Health: GenImport = %q, want %q", f.Msg.GenImport, ownerGen)
				}
			}
		}
	}
	for _, m := range res.ExternalMessages {
		if m.Name != "Health" {
			continue
		}
		sawExternal = true
		if m.GenImport != ownerGen {
			t.Errorf("res.ExternalMessages Health: GenImport = %q, want %q", m.GenImport, ownerGen)
		}
	}
	if !sawExternal {
		t.Fatal("Health was not reached as an external top-level type; the fixture no longer exercises this")
	}
	if !sawLocal && !sawRef {
		t.Fatal("Health was not reached as a field; the fixture no longer exercises this")
	}
}

// TestNoExportedFieldsRefusedInEveryContainer pins that a struct with nothing to serialize is refused
// however it is reached, not only when a field is declared as it directly.
//
// Such a type generates an empty message: encoding writes nothing, decoding returns a zero value, and
// neither reports an error — so the data is lost in silence. Wrapping it in a slice, map, or pointer does
// not change that, and for a while only the direct spelling was caught. An empty struct{} has no data to
// lose and stays legal in every position.
func TestNoExportedFieldsRefusedInEveryContainer(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		field string
	}{
		{"direct", "G Guard"},
		{"slice", "G []Guard"},
		{"map value", "G map[string]Guard"},
		{"pointer", "G *Guard"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res := discover(t, `import "sdkgentest/cardinal"

type Guard struct{ mu int }

type C struct { `+tc.field+` }

func (C) Name() string { return "c" }

func Setup(w *cardinal.World) { w.RegisterCommand[C]() }`)
			if len(res.Violations) == 0 {
				t.Fatalf("%s: generated with no violation; Guard has no exported fields and would emit an "+
					"empty message", tc.field)
			}
			if !strings.Contains(res.Violations[0].Reason, "no exported fields") {
				t.Errorf("violation reason = %q, want it to name the missing exported fields",
					res.Violations[0].Reason)
			}
		})
	}

	t.Run("empty struct stays legal", func(t *testing.T) {
		t.Parallel()
		res := discover(t, `import "sdkgentest/cardinal"

type Marker struct{}

type C struct {
	A Marker
	B []Marker
	D map[string]Marker
	E *Marker
}

func (C) Name() string { return "c" }

func Setup(w *cardinal.World) { w.RegisterCommand[C]() }`)
		// The containers themselves report as shapes; what this pins is that the empty struct inside
		// them is not refused for having nothing to export.
		if bad := notDataFindings(res); len(bad) != 0 {
			t.Fatalf("empty struct carries no data to lose and must stay legal: %v", bad)
		}
	})
}

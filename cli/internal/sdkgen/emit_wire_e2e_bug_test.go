package sdkgen_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/cli/internal/sdkgen"
)

// TestTimestamppbMirrorAfterTimeField pins the bug report's exact scenario: a component whose [time.Time]
// field drifts the timestamppb import in bare (need), followed by a raw timestamppb.Timestamp field whose
// mirror drifts it under the hand alias known_timestamppb (spec). Before the fix the two spellings
// disagreed and RenderGoWire refused with "qualifies names with known_timestamppb"; now they agree on the
// bare leaf (need won, the mirror is re-qualified to bare) and the file compiles.
func TestTimestamppbMirrorAfterTimeField(t *testing.T) {
	// The temp module needs -mod=mod to resolve google.golang.org/protobuf from the module cache (it has
	// no go.sum of its own). t.Setenv precludes t.Parallel, so these run serially.
	t.Setenv("GOFLAGS", "-mod=mod")

	const src = `package game

import (
	"cardinalshim/cardinal"
	"time"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type MoveCommand struct {
	When  time.Time
	Stamp timestamppb.Timestamp
}

func (MoveCommand) Name() string { return "move" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[MoveCommand]()
}`
	out := renderTimestamppbFixture(t, src)

	// need won, so the import is bare and every reference — the time.Time branch's New and the mirror's
	// parameter/return — spells it timestamppb. The hand alias known_timestamppb must not survive into
	// the file at all; any leftover would be an unresolved qualifier.
	if !strings.Contains(out, `"google.golang.org/protobuf/types/known/timestamppb"`) {
		t.Errorf("expected a BARE timestamppb import (need won); the import block:\n%s", out)
	}
	if strings.Contains(out, "known_timestamppb") {
		t.Errorf("the hand alias known_timestamppb should not appear once need wins; got:\n%s", out)
	}
	if !strings.Contains(out, "timestamppb.New(c.When)") {
		t.Errorf("the time.Time field should convert via the bare timestamppb.New; got:\n%s", out)
	}
	if !strings.Contains(out, "c timestamppb.Timestamp") {
		t.Errorf("the mirror converter should take a bare timestamppb.Timestamp; got:\n%s", out)
	}
	assertQualifiersImported(t, out)
}

// TestTimestamppbMirrorBeforeTimeField pins the same schema with the fields reversed. The mirror's spec
// now claims the timestamppb path first, so the hand alias wins; the [time.Time] branch adapts (qualify)
// to known_timestamppb.New instead of hardcoding the bare leaf. This is the ordering the recommended
// "upgrade spec" fix in the report could not handle — it would leave the bare timestamppb.New write the
// [time.Time] branch already emitted stranded under an aliased import.
func TestTimestamppbMirrorBeforeTimeField(t *testing.T) {
	t.Setenv("GOFLAGS", "-mod=mod")

	const src = `package game

import (
	"cardinalshim/cardinal"
	"time"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type MoveCommand struct {
	Stamp timestamppb.Timestamp
	When  time.Time
}

func (MoveCommand) Name() string { return "move" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[MoveCommand]()
}`
	out := renderTimestamppbFixture(t, src)

	// spec won, so the import is the hand alias and every reference follows it.
	if !strings.Contains(out, `known_timestamppb "google.golang.org/protobuf/types/known/timestamppb"`) {
		t.Errorf("expected an ALIASED timestamppb import (spec won); the import block:\n%s", out)
	}
	if !strings.Contains(out, "known_timestamppb.New(c.When)") {
		t.Errorf("the time.Time field should adapt to the winning alias known_timestamppb.New; got:\n%s", out)
	}
	if !strings.Contains(out, "c known_timestamppb.Timestamp") {
		t.Errorf("the mirror converter should keep its hand alias known_timestamppb.Timestamp; got:\n%s", out)
	}
	// The bare leaf must NOT be used as a qualifier: with the import aliased, a bare timestamppb.X would be
	// exactly the unresolved-qualifier refusal this bug is about. (A bare "timestamppb" can still appear
	// inside the longer alias known_timestamppb, so check the qualifier position, not the substring.)
	if strings.Contains(out, " timestamppb.") || strings.Contains(out, "= timestamppb.") {
		t.Errorf("the bare timestamppb qualifier must not appear once the alias won; got:\n%s", out)
	}
	assertQualifiersImported(t, out)
}

// TestTimestamppbMirrorWithRepeatedTimeField pins []time.Time + timestamppb.Timestamp — elemToProto's
// Timestamp branch is the second reachable bare-qualifier write site (value and repeated [time.Time] are
// the only shapes the direct encoder accepts; *[time.Time] and map[K][time.Time] are refused earlier as a
// pointer-to-message / map, so they never reach the collision).
func TestTimestamppbMirrorWithRepeatedTimeField(t *testing.T) {
	t.Setenv("GOFLAGS", "-mod=mod")

	const src = `package game

import (
	"cardinalshim/cardinal"
	"time"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type MoveCommand struct {
	Whens []time.Time
	Stamp timestamppb.Timestamp
}

func (MoveCommand) Name() string { return "move" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[MoveCommand]()
}`
	out := renderTimestamppbFixture(t, src)
	if !strings.Contains(out, "timestamppb.New(") {
		t.Errorf("repeated time.Time elems should convert via the bare timestamppb.New(...); got:\n%s", out)
	}
	if strings.Contains(out, "known_timestamppb") {
		t.Errorf("need won; alias must not leak; got:\n%s", out)
	}
	assertQualifiersImported(t, out)
}

// renderTimestamppbFixture writes the two-module fixture (cardinal shim + game), discovers it, splits the
// way generate.go does, renders the wire layer, and returns the generated source. It fails the test if
// discovery or rendering fails — the bug was a generation-time refusal, so a clean render is the fix.
func renderTimestamppbFixture(t *testing.T, gameSrc string) string {
	t.Helper()
	root := multiModule(t, map[string]string{
		"cardinalshim/go.mod":               "module cardinalshim\n\ngo 1.27\n",
		"cardinalshim/cardinal/cardinal.go": cardinalShim,
		"game/go.mod": "module game\n\ngo 1.27\n\n" +
			"require (\n\tcardinalshim v0.0.0\n\tgoogle.golang.org/protobuf v1.36.12\n)\n\n" +
			"replace cardinalshim => ../cardinalshim\n",
		"game/game.go": gameSrc,
	})

	res, err := sdkgen.Discover(filepath.Join(root, "game"))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	local, mirrored := splitForRender(res, "game/gen")

	tsKey := "google.golang.org/protobuf/types/known/timestamppb.Timestamp"
	tsMsg, ok := mirrored[tsKey]
	if !ok {
		t.Fatalf("timestamppb.Timestamp should be mirrored; got mirrored=%v", slices.Sorted(maps.Keys(mirrored)))
	}
	if !strings.Contains(tsMsg.Self.GoType, "known_timestamppb") {
		t.Fatalf("discovery should stamp the hand alias known_timestamppb on Self.GoType, got %q",
			tsMsg.Self.GoType)
	}

	out, err := sdkgen.RenderGoWire(local, mirrored)
	if err != nil {
		t.Fatalf("RenderGoWire failed (the bug was a generation-time refusal): %v", err)
	}
	return out
}

// assertQualifiersImported re-derives the import/qualifier agreement independently of the generator's own
// checkSymbolsResolve, so the test pins the property directly: every package used as a selector qualifier
// in the generated source is imported (by alias, or by leaf for an unaliased import). The bug was a
// qualifier the import block did not name; this asserts no such qualifier survives the fix.
func assertQualifiersImported(t *testing.T, src string) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "wire_gen.go", src, 0)
	if err != nil {
		t.Fatalf("generated wire Go did not parse: %v", err)
	}
	imported := map[string]bool{}
	for _, spec := range f.Imports {
		p, uerr := strconv.Unquote(spec.Path.Value)
		if uerr != nil {
			continue
		}
		if spec.Name != nil {
			imported[spec.Name.Name] = true
			continue
		}
		imported[path.Base(p)] = true
	}
	qualifiers := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, idOK := sel.X.(*ast.Ident); idOK {
				qualifiers[id.Name] = true
			}
		}
		return true
	})
	for _, id := range f.Unresolved {
		if !qualifiers[id.Name] || goBuiltins[id.Name] || imported[id.Name] {
			continue
		}
		t.Errorf("generated wire qualifies names with %q but imports no such package:\n%s", id.Name, src)
	}
}

// goBuiltins is the predeclared identifier set, copied from the generator's own guard so the test's
// independent check agrees with it about what is not a missing import.
var goBuiltins = map[string]bool{
	"append": true, "cap": true, "clear": true, "close": true, "complex": true, "copy": true,
	"delete": true, "imag": true, "len": true, "make": true, "max": true, "min": true, "new": true,
	"panic": true, "print": true, "println": true, "real": true, "recover": true,
	"any": true, "bool": true, "byte": true, "comparable": true, "complex64": true, "complex128": true,
	"error": true, "float32": true, "float64": true, "int": true, "int8": true, "int16": true,
	"int32": true, "int64": true, "rune": true, "string": true, "uint": true, "uint8": true,
	"uint16": true, "uint32": true, "uint64": true, "uintptr": true,
	"true": true, "false": true, "iota": true, "nil": true,
}

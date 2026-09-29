package sdkgen_test

import (
	"iter"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/internal/sdkgen"
)

// sliceCmd wraps one command holding immutable.Slice fields of every wire shape a Slice can take: a
// scalar element, a message element, and the byte element that becomes bytes.
const sliceCmd = `import (
	"sdkgentest/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
)

type Vec2 struct{ X, Y float32 }

type C struct {
	Ids    immutable.Slice[int32]
	Points immutable.Slice[Vec2]
	Blob   immutable.Slice[byte]
}

func (C) Name() string { return "c" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[C]()
}`

// TestEngineSliceIsNotAFinding pins the one difference between immutable.Slice[T] and []T: the raw
// slice is refused for being reached by address, the Slice is carried, on the wire shape []T would have.
func TestEngineSliceIsNotAFinding(t *testing.T) {
	t.Parallel()

	res := discover(t, sliceCmd)
	if len(res.Violations) != 0 {
		t.Fatalf("a Slice must not be a finding, got %+v", res.Violations)
	}

	got := proto(t, sliceCmd)
	for _, want := range []string{
		"repeated int32 Ids = 1;",
		"repeated Vec2 Points = 2;",
		"bytes Blob = 3;",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("proto missing %q:\n%s", want, got)
		}
	}
}

// TestEngineSliceWire pins the Go side: ToProto reads every Slice, bytes included, through Values;
// FromProto collects into a per-field slice and rebuilds with SliceOf; and the file imports the
// declaring package.
func TestEngineSliceWire(t *testing.T) {
	t.Parallel()

	out := wire(t, sliceCmd)
	for _, want := range []string{
		`"github.com/argus-labs/world-engine/pkg/immutable"`,
		"for v := range c.Ids.Values() {",
		"p.Ids = append(p.Ids, int32(v))",
		"for v := range c.Points.Values() {",
		"p.Points = append(p.Points, v.ToProto())",
		"for v := range c.Blob.Values() {",
		"p.Blob = append(p.Blob, v)",
		"if len(p.Ids) > 0 {", // an empty repeated field must leave the zero value
		"itemsIds := make([]int32, 0, len(p.Ids))",
		"itemsIds = append(itemsIds, int32(x))",
		"itemsPoints := make([]Vec2, 0, len(p.Points))",
		"itemsPoints = append(itemsPoints, v)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("wire missing %q:\n%s", want, out)
		}
	}
	for _, want := range []string{
		`\.SliceOf\(itemsIds\.\.\.\)`,
		`\.SliceOf\(itemsPoints\.\.\.\)`,
		`c\.Blob = \w+\.SliceOf\(p\.Blob\.\.\.\)`,
	} {
		if !regexp.MustCompile(want).MatchString(out) {
			t.Errorf("wire missing /%s/:\n%s", want, out)
		}
	}
}

// TestEngineSliceElementIsChecked pins that the Slice hides nothing: its element is held to every
// rule a field is, and the finding names the element rather than the Slice.
//
// Location and Go type are asserted, not the reason, because those two are what the report prints —
// a reason nobody sees cannot be the thing a test guards. A finding that said "SubmitBatch.Payloads
// (immutable.Slice[[]byte])" would send a developer to change the field they already fixed; it has to
// say "Payloads[] ([]byte)".
func TestEngineSliceElementIsChecked(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name, field, wantCat, wantWhere, wantGoType string
	}{
		{"pointer element", "Ps immutable.Slice[*int32]", sdkgen.CatPointer, "C.Ps[]", "*int32"},
		{"map element", "Ms immutable.Slice[map[string]int32]", sdkgen.CatMap, "C.Ms[]", "map[string]int32"},
		{"raw slice element", "Ss immutable.Slice[[]int32]", sdkgen.CatSlice, "C.Ss[]", "[]int32"},
		{"raw byte slice element", "Bs immutable.Slice[[]byte]", sdkgen.CatSlice, "C.Bs[]", "[]byte"},
		{"interface element", "As immutable.Slice[any]", sdkgen.CatInterface, "C.As[]", "any"},
		{"chan element is not data", "Cs immutable.Slice[chan int]", sdkgen.CatUnserializable, "C.Cs[]", "chan int"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			res := discover(t, sliceCmdWith(c.field))
			if len(res.Violations) != 1 {
				t.Fatalf("want exactly one finding, got %+v", res.Violations)
			}
			v := res.Violations[0]
			if v.Category != c.wantCat {
				t.Errorf("category: want %s, got %s", c.wantCat, v.Category)
			}
			if v.Where != c.wantWhere {
				t.Errorf("location: want %s, got %s — the report must point at the element", c.wantWhere, v.Where)
			}
			if v.GoType != c.wantGoType {
				t.Errorf("go type: want %s, got %s — the report must name the element type", c.wantGoType, v.GoType)
			}
		})
	}
}

// TestSliceOfSliceIsRefused covers the shapes where the Slice itself cannot be carried, rather than
// its element being wrong: protobuf has no repeated-inside-repeated.
func TestSliceOfSliceIsRefused(t *testing.T) {
	t.Parallel()

	for _, c := range []struct{ name, field string }{
		{"Slice in a Slice", "Ns immutable.Slice[immutable.Slice[int32]]"},
		{"Slice in a fixed array", "Arr [2]immutable.Slice[int32]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			res := discover(t, sliceCmdWith(c.field))
			if len(res.Violations) != 1 {
				t.Fatalf("want exactly one finding, got %+v", res.Violations)
			}
			v := res.Violations[0]
			if v.Category != sdkgen.CatUnsupportedType {
				t.Errorf("category: want %s, got %s", sdkgen.CatUnsupportedType, v.Category)
			}
			if !strings.Contains(v.Reason, "wrap") {
				t.Errorf("reason should say to wrap the inner Slice, got %q", v.Reason)
			}
		})
	}
}

// sliceCmdWith wraps a single field that needs the immutable import.
func sliceCmdWith(field string) string {
	return `import (
	"sdkgentest/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
)

type C struct {
	` + field + `
}

func (C) Name() string { return "c" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[C]()
}`
}

// TestEngineSlicePointerInsideElementStruct pins that the walk continues into the element: a pointer two
// levels down is reported once, on the nested message, and the Slice field itself is clean.
func TestEngineSlicePointerInsideElementStruct(t *testing.T) {
	t.Parallel()

	res := discover(t, `import (
	"sdkgentest/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
)

type Inner struct{ P *int32 }

type C struct {
	Is immutable.Slice[Inner]
}

func (C) Name() string { return "c" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[C]()
}`)
	if len(res.Violations) != 1 {
		t.Fatalf("want exactly one finding, got %+v", res.Violations)
	}
	if v := res.Violations[0]; v.Where != "Inner.P" || v.Category != sdkgen.CatPointer {
		t.Errorf("want Inner.P filed as %s, got %+v", sdkgen.CatPointer, v)
	}
}

// TestEngineSliceOfTime pins the well-known-type path: a Slice[time.Time] is repeated Timestamp on the
// wire, and the collecting slice names time.Time, which needs the time import.
func TestEngineSliceOfTime(t *testing.T) {
	t.Parallel()

	body := `import (
	"time"

	"sdkgentest/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
)

type C struct {
	Ts immutable.Slice[time.Time]
}

func (C) Name() string { return "c" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[C]()
}`
	if got := proto(t, body); !strings.Contains(got, "repeated google.protobuf.Timestamp Ts = 1;") {
		t.Errorf("proto:\n%s", got)
	}
	out := wire(t, body)
	for _, want := range []string{
		`"time"`,
		"itemsTs := make([]time.Time, 0, len(p.Ts))",
		"itemsTs = append(itemsTs, e.AsTime())",
		"p.Ts = append(p.Ts, timestamppb.New(v))",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("wire missing %q:\n%s", want, out)
		}
	}
}

// TestEngineSliceLookalikeIsRefused pins that the exemption is granted on identity alone: a package
// merely NAMED immutable does not get it, however complete its surface looks.
//
// This is the only enforcement point for the value-only rule — immutable.Slice's own doc says "the
// wire generator refuses such an element" — so matching on the package name or the method set would
// hand the exemption to a type that aliases freely. The look-alike here is the worst case: right name,
// right symbols, and a Clone that returns the backing array plus a Set that writes through it, which
// is the exact sharing a raw []int32 field is refused for.
func TestEngineSliceLookalikeIsRefused(t *testing.T) {
	t.Parallel()

	root := multiModule(t, map[string]string{
		"go.mod":                      "module game\n\ngo 1.27\n",
		"cardinal/cardinal.go":        cardinalShim,
		"internal/immutable/slice.go": lookalikeSlice,
		"commands.go":                 lookalikeCommand,
	})
	res, err := sdkgen.Discover(filepath.Clean(root))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(res.Violations) != 1 {
		t.Fatalf("a look-alike must not buy the exemption, got %+v", res.Violations)
	}
	// Refused for its own shape: a struct whose only field is unexported carries nothing on the wire.
	if got := res.Violations[0].Category; got != sdkgen.CatUnserializable {
		t.Errorf("category: want %s, got %s (reason: %s)", sdkgen.CatUnserializable, got, res.Violations[0].Reason)
	}
}

// TestEngineSliceMisuseIsExplained pins the two refusals that used to blame the Slice itself ("type
// Slice has no exported fields"). The finding is filed under the OUTER shape, and the reason names
// the fix for that shape rather than suggesting the Slice is broken.
func TestEngineSliceMisuseIsExplained(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name, field, wantCat, wantReason string
	}{
		{"pointer to Slice", "P *immutable.Slice[int32]", sdkgen.CatPointer, "hold it by value"},
		{"map of Slice", "M map[string]immutable.Slice[int32]", sdkgen.CatMap, "wrap it in a named struct"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			res := discover(t, sliceCmdWith(c.field))
			if len(res.Violations) != 1 {
				t.Fatalf("want exactly one finding, got %+v", res.Violations)
			}
			v := res.Violations[0]
			if v.Category != c.wantCat {
				t.Errorf("category: want %s, got %s (reason: %s)", c.wantCat, v.Category, v.Reason)
			}
			if !strings.Contains(v.Reason, c.wantReason) {
				t.Errorf("reason should say %q, got %q", c.wantReason, v.Reason)
			}
			if strings.Contains(v.Reason, "no exported fields") {
				t.Errorf("reason blames the Slice itself: %q", v.Reason)
			}
		})
	}
}

// lookalikeSlice has world-engine's surface and none of its guarantees.
const lookalikeSlice = `package immutable

type Slice[T any] struct{ items []T }

func SliceOf[T any](items ...T) Slice[T] { return Slice[T]{items: items} }

// Hands the backing array straight out, and lets callers write through it.
func (s Slice[T]) Clone() []T     { return s.items }
func (s Slice[T]) Set(i int, v T) { s.items[i] = v }
`

const lookalikeCommand = `package commands

import (
	"game/cardinal"
	"game/internal/immutable"
)

type C struct {
	Ids immutable.Slice[int32]
}

func (C) Name() string { return "c" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[C]()
}
`

// TestEmittedSliceSurfaceMatchesEngine binds the two immutable.Slice symbols the emitter writes as
// text (sliceCtor and sliceIterMethod in emit_wire.go) to the real ones.
//
// Nothing else links this package's compile path to world-engine: the generator type-checks the
// caller's module, and it emits these two as format-string text. So a rename or a signature change
// in world-engine would leave the monorepo green, world-cli building, and the breakage waiting in
// every generated backend until a game next builds. This is the one place that turns that into a
// failure here.
//
// The typed vars pin the signatures, which usage alone does not: swapping Values back to All fails
// with "cannot use ... iter.Seq2[int, int32] as func(...) iter.Seq[int32]". The loop below pins the
// shape the emitter actually writes, so a failure reads the way the game's build error would.
//
// It does NOT prove a whole generated file compiles — that needs protoc and a real build. This covers
// the engine surface only.
func TestEmittedSliceSurfaceMatchesEngine(t *testing.T) {
	t.Parallel()

	// The explicit types ARE the check: inferred ones would accept any signature.
	//nolint:staticcheck // QF1011: omitting the type would defeat the purpose of this test
	var (
		_ func(...int32) immutable.Slice[int32]        = immutable.SliceOf[int32]
		_ func(immutable.Slice[int32]) iter.Seq[int32] = immutable.Slice[int32].Values
	)

	var ids []int32
	for v := range immutable.SliceOf[int32](1, 2, 3).Values() {
		ids = append(ids, v)
	}
	require.Equal(t, []int32{1, 2, 3}, ids)

	// The bytes path is the same loop over a byte element; pin that it appends as-is.
	var blob []byte
	for v := range immutable.SliceOf[byte](1, 2).Values() {
		blob = append(blob, v)
	}
	require.Equal(t, []byte{1, 2}, blob)
}

// TestSliceGuidanceIsNotCircular pins that a CatSlice finding on a Slice element does not tell the
// developer to do the thing they have already done. The report groups findings by category and prints
// one Fix per group, so this text is the whole of what they are given.
func TestSliceGuidanceIsNotCircular(t *testing.T) {
	t.Parallel()

	res := discover(t, sliceCmdWith("Bs immutable.Slice[[]byte]"))
	if len(res.Violations) != 1 {
		t.Fatalf("want exactly one finding, got %+v", res.Violations)
	}
	fix := sdkgen.GuidanceFor(res.Violations[0].Category).Proper
	if !strings.Contains(fix, "[]") || !strings.Contains(fix, "ELEMENT") {
		t.Errorf("slice guidance must explain the Field[] case, got %q", fix)
	}
}

// TestEmittedDecodeLeavesEmptyAsZero pins that FromProto does not build an allocated Slice for a
// repeated field that arrived empty.
//
// A component restored from a snapshot has to compare equal to the same component built fresh. Go's
// zero value has a nil backing array and cannot be changed, so the decode side is what has to match
// it: constructing unconditionally would hand back an empty-but-allocated Slice, and reflect.DeepEqual
// — so require.Equal — would call the two different.
func TestEmittedDecodeLeavesEmptyAsZero(t *testing.T) {
	t.Parallel()

	out := wire(t, sliceCmd)
	for _, want := range []string{
		"if len(p.Ids) > 0 {",
		"if len(p.Points) > 0 {",
		"if len(p.Blob) > 0 {",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("decode must be guarded by %q:\n%s", want, out)
		}
	}
}

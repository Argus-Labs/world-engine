package sdkgen_test

import (
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/cli/internal/sdkgen"
)

// The direct-encoder tests check the emitted Go as text: nothing here compiles it (the proto package
// it references does not exist), so byte-exactness against proto.Marshal is not checkable at this
// level. What is pinned is the shape of every field encoder — the presence test, the wire type, the
// packing rule, the helpers and imports — which is where a divergence from protobuf-go would start.

const (
	directPkg = "example.com/g/component"
	directGen = "example.com/g/gen/component"
	sliceImp  = `pkg_immutable "github.com/argus-labs/world-engine/pkg/immutable"`
)

func scalarOf(family, goType, proto string) *sdkgen.Scalar {
	return &sdkgen.Scalar{Family: family, GoType: goType, Proto: proto}
}

var (
	scI32 = scalarOf("varint", "int32", "int32")
	scU64 = scalarOf("varint", "uint64", "uint64")
	scF64 = scalarOf("i64", "float64", "double")
	scF32 = scalarOf("i32", "float32", "float")
	scBl  = scalarOf("bool", "bool", "bool")
	scStr = scalarOf("string", "string", "string")
	scBy  = scalarOf("bytes", "[]byte", "bytes")
)

// vec2 is a local nested message every test can carry.
var vec2 = sdkgen.TypeRef{Name: "Vec2", GoType: "Vec2", PkgPath: directPkg, GenImport: directGen}

func vec2Message() sdkgen.Message {
	return sdkgen.Message{Name: "Vec2", PkgName: "component", PkgPath: directPkg, GenImport: directGen,
		Fields: []sdkgen.Field{
			{Name: "X", Number: 1, Kind: "scalar", Scalar: scF64},
			{Name: "Y", Number: 2, Kind: "scalar", Scalar: scF64},
		}}
}

func directComponent(name string, fields ...sdkgen.Field) sdkgen.Message {
	return sdkgen.Message{Name: name, Kind: sdkgen.KindComponent, PkgName: "component", PkgPath: directPkg,
		GenImport: directGen, Fields: fields}
}

func renderDirect(t *testing.T, mirrored map[string]sdkgen.Message, msgs ...sdkgen.Message) string {
	t.Helper()
	out, err := sdkgen.RenderGoWire(msgs, mirrored)
	if err != nil {
		t.Fatalf("RenderGoWire failed: %v", err)
	}
	return out
}

func wantAll(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("direct encoder missing %q:\n%s", w, out)
		}
	}
}

func wantNone(t *testing.T, out string, unwanted ...string) {
	t.Helper()
	for _, u := range unwanted {
		if strings.Contains(out, u) {
			t.Errorf("direct encoder must not contain %q:\n%s", u, out)
		}
	}
}

// TestRenderGoWireDirect_Scalars pins the singular scalar rules: proto3 implicit presence (absent at
// zero; a float compares its bit pattern so -0.0 is present), the wire type per family, and the
// encode-side casts to basic types so a named scalar (EntityID) needs no import of its own.
func TestRenderGoWireDirect_Scalars(t *testing.T) {
	t.Parallel()
	eid := &sdkgen.Scalar{Family: "varint", GoType: "pkg_cardinal.EntityID", Proto: "uint32",
		Imports: []string{`pkg_cardinal "github.com/argus-labs/world-engine/pkg/cardinal"`}}
	out := renderDirect(t, nil, directComponent("Stats",
		sdkgen.Field{Name: "Count", Number: 1, Kind: "scalar", Scalar: scI32},
		sdkgen.Field{Name: "Ratio", Number: 2, Kind: "scalar", Scalar: scF32},
		sdkgen.Field{Name: "Mass", Number: 3, Kind: "scalar", Scalar: scF64},
		sdkgen.Field{Name: "On", Number: 4, Kind: "scalar", Scalar: scBl},
		sdkgen.Field{Name: "Label", Number: 5, Kind: "scalar", Scalar: scStr},
		sdkgen.Field{Name: "Blob", Number: 6, Kind: "scalar", Scalar: scBy},
		sdkgen.Field{Name: "Owner", Number: 7, Kind: "scalar", Scalar: eid},
	))
	wantAll(
		t,
		out,
		`"google.golang.org/protobuf/encoding/protowire"`,
		`"math"`,
		"func (c Stats) SizeWire() int {",
		"func (c Stats) AppendWire(b []byte) []byte {",
		"if c.Count != 0 {\n\t\tn += protowire.SizeTag(1) + protowire.SizeVarint(uint64(c.Count))",
		"b = protowire.AppendTag(b, 1, protowire.VarintType)\n\t\tb = protowire.AppendVarint(b, uint64(c.Count))",
		"if math.Float32bits(float32(c.Ratio)) != 0 {\n\t\tn += protowire.SizeTag(2) + protowire.SizeFixed32()",
		"if math.Float32bits(float32(c.Ratio)) != 0 {\n\t\tb = protowire.AppendTag(b, 2, protowire.Fixed32Type)\n\t\tb = protowire.AppendFixed32(b, math.Float32bits(float32(c.Ratio)))",
		"if math.Float64bits(float64(c.Mass)) != 0 {\n\t\tb = protowire.AppendTag(b, 3, protowire.Fixed64Type)\n\t\tb = protowire.AppendFixed64(b, math.Float64bits(float64(c.Mass)))",
		"if c.On {\n\t\tn += protowire.SizeTag(4) + 1",
		"if c.On {\n\t\tb = protowire.AppendTag(b, 4, protowire.VarintType)\n\t\tb = protowire.AppendVarint(b, 1)",
		"if len(c.Label) > 0 {\n\t\tn += protowire.SizeTag(5) + wireStringSize(\"Stats.Label\", string(c.Label))",
		"b = protowire.AppendTag(b, 5, protowire.BytesType)\n\t\tb = protowire.AppendString(b, string(c.Label))",
		"b = protowire.AppendTag(b, 6, protowire.BytesType)\n\t\tb = protowire.AppendBytes(b, []byte(c.Blob))",
		"if c.Owner != 0 {\n\t\tn += protowire.SizeTag(7) + protowire.SizeVarint(uint64(c.Owner))",
		// The bytes layer is still emitted for a top-level type.
		"func (c Stats) MarshalWire() []byte {",
	)
	// An encode-side cast never targets the declared type: the EntityID import the decode side needs
	// (FromProto's cast) is not what the direct encoders reach for.
	_, direct, _ := strings.Cut(out, "func (c Stats) SizeWire()")
	wantNone(t, direct, "pkg_cardinal", "wireBool(")
}

// TestRenderGoWireDirect_IntOnlyNeedsNoMath pins that math is imported only when a float is written:
// an unused import would fail the build of every backend without one.
func TestRenderGoWireDirect_IntOnlyNeedsNoMath(t *testing.T) {
	t.Parallel()
	out := renderDirect(t, nil, directComponent("Counter",
		sdkgen.Field{Name: "N", Number: 1, Kind: "scalar", Scalar: scI32},
	))
	wantAll(t, out, `"google.golang.org/protobuf/encoding/protowire"`)
	wantNone(t, out, `"math"`, `"time"`)
}

// TestRenderGoWireDirect_Messages pins nested messages: always present (ToProto always sets it), sized
// and written through the nested type's own pair — which it gets even though it is not a top-level kind.
// AppendWire never sizes: the length prefix is a placeholder patched in after the body.
func TestRenderGoWireDirect_Messages(t *testing.T) {
	t.Parallel()
	out := renderDirect(t, nil, vec2Message(), directComponent("Body",
		sdkgen.Field{Name: "Pos", Number: 1, Kind: "message", Msg: vec2},
	))
	wantAll(
		t,
		out,
		"func (c Vec2) SizeWire() int {",
		"func (c Vec2) AppendWire(b []byte) []byte {",
		"\tn += protowire.SizeTag(1) + protowire.SizeBytes(c.Pos.SizeWire())\n",
		"b = protowire.AppendTag(b, 1, protowire.BytesType)\n\tatPos := len(b)\n\tb = append(b, 0)\n\tb = c.Pos.AppendWire(b)\n\tb = wireLenPrefix(b, atPos)",
		"func wireLenPrefix(b []byte, at int) []byte {",
	)
	// Nested types never cross the wire on their own, so the bytes layer stays top-level only.
	wantNone(t, out, "func (c Vec2) MarshalWire()")
	_, app, _ := strings.Cut(out, "func (c Body) AppendWire(")
	app, _, _ = strings.Cut(app, "\n}\n")
	wantNone(t, app, "SizeWire(")
}

// TestRenderGoWireDirect_Timestamp pins the well-known type: the Timestamp body is written by a pair
// of helpers declared once per file, and always present.
func TestRenderGoWireDirect_Timestamp(t *testing.T) {
	t.Parallel()
	out := renderDirect(t, nil, directComponent("Audit",
		sdkgen.Field{Name: "At", Number: 1, Kind: "message", Timestamp: true},
		sdkgen.Field{Name: "Seen", Number: 3, Kind: "repeated", Timestamp: true},
	))
	wantAll(
		t,
		out,
		`"time"`,
		"n += protowire.SizeTag(1) + protowire.SizeBytes(sizeWireTimestamp(c.At))",
		"b = protowire.AppendTag(b, 1, protowire.BytesType)\n\tatAt := len(b)\n\tb = append(b, 0)\n\tb = appendWireTimestamp(b, c.At)\n\tb = wireLenPrefix(b, atAt)",
		"for _, x := range c.Seen {\n\t\tn += protowire.SizeTag(3) + protowire.SizeBytes(sizeWireTimestamp(x))",
		"func sizeWireTimestamp(t time.Time) int {",
		"if s := t.Unix(); s != 0 {\n\t\tn += protowire.SizeTag(1) + protowire.SizeVarint(uint64(s))",
		"if ns := t.Nanosecond(); ns != 0 {\n\t\tn += protowire.SizeTag(2) + protowire.SizeVarint(uint64(ns))",
		"func appendWireTimestamp(b []byte, t time.Time) []byte {",
	)
	if n := strings.Count(out, "func sizeWireTimestamp("); n != 1 {
		t.Errorf("timestamp helper declared %d times, want once", n)
	}
}

// TestRenderGoWireDirect_Repeated pins the packing rule: numerics and bools pack (one tag, one length,
// payloads back to back — a varint payload needs a sizing pass, a fixed-width one is arithmetic),
// strings and messages are length-delimited per element, and an empty repeated writes nothing.
func TestRenderGoWireDirect_Repeated(t *testing.T) {
	t.Parallel()
	out := renderDirect(t, nil, vec2Message(), directComponent("Lists",
		sdkgen.Field{Name: "Ids", Number: 1, Kind: "repeated", Scalar: scU64},
		sdkgen.Field{Name: "Weights", Number: 2, Kind: "repeated", Scalar: scF64},
		sdkgen.Field{Name: "Flags", Number: 3, Kind: "repeated", Scalar: scBl},
		sdkgen.Field{Name: "Names", Number: 4, Kind: "repeated", Scalar: scStr},
		sdkgen.Field{Name: "Path", Number: 5, Kind: "repeated", Msg: vec2},
	))
	wantAll(
		t,
		out,
		"if len(c.Ids) > 0 {\n\t\tpackedIds := 0\n\t\tfor _, x := range c.Ids {\n\t\t\tpackedIds += protowire.SizeVarint(uint64(x))\n\t\t}\n\t\tn += protowire.SizeTag(1) + protowire.SizeBytes(packedIds)",
		"b = protowire.AppendTag(b, 1, protowire.BytesType)\n\t\tb = protowire.AppendVarint(b, uint64(packedIds))\n\t\tfor _, x := range c.Ids {\n\t\t\tb = protowire.AppendVarint(b, uint64(x))",
		"if len(c.Weights) > 0 {\n\t\tn += protowire.SizeTag(2) + protowire.SizeBytes(len(c.Weights)*protowire.SizeFixed64())",
		"b = protowire.AppendFixed64(b, math.Float64bits(float64(x)))",
		"if len(c.Flags) > 0 {\n\t\tn += protowire.SizeTag(3) + protowire.SizeBytes(len(c.Flags))",
		"b = protowire.AppendVarint(b, wireBool(bool(x)))",
		"for _, x := range c.Names {\n\t\tn += protowire.SizeTag(4) + wireStringSize(\"Lists.Names[]\", string(x))",
		"for _, x := range c.Names {\n\t\tb = protowire.AppendTag(b, 4, protowire.BytesType)\n\t\tb = protowire.AppendString(b, string(x))",
		"for _, x := range c.Path {\n\t\tn += protowire.SizeTag(5) + protowire.SizeBytes(x.SizeWire())",
		"for _, x := range c.Path {\n\t\tb = protowire.AppendTag(b, 5, protowire.BytesType)\n\t\tatPath := len(b)\n\t\tb = append(b, 0)\n\t\tb = x.AppendWire(b)\n\t\tb = wireLenPrefix(b, atPath)",
	)
	if n := strings.Count(out, "func wireBool("); n != 1 {
		t.Errorf("wireBool declared %d times, want once", n)
	}
}

// TestRenderGoWireDirect_EngineSlice pins immutable.Slice: the same wire shape as a Go slice, read
// through Len and Values (both allocation-free); Slice[byte] is a bytes field written a byte at a time
// since a Slice exposes no backing array. No SliceOf and no immutable import on the encode side.
func TestRenderGoWireDirect_EngineSlice(t *testing.T) {
	t.Parallel()
	out := renderDirect(t, nil, vec2Message(), directComponent("Held",
		sdkgen.Field{Name: "Shapes", Number: 1, Kind: "repeated", Msg: vec2, Slice: true, SliceImport: sliceImp},
		sdkgen.Field{Name: "Scores", Number: 2, Kind: "repeated", Scalar: scI32, Slice: true, SliceImport: sliceImp},
		sdkgen.Field{Name: "Raw", Number: 3, Kind: "scalar", Scalar: scBy, Slice: true, SliceImport: sliceImp},
	))
	wantAll(
		t,
		out,
		"for x := range c.Shapes.Values() {\n\t\tn += protowire.SizeTag(1) + protowire.SizeBytes(x.SizeWire())",
		"if c.Scores.Len() > 0 {\n\t\tpackedScores := 0\n\t\tfor x := range c.Scores.Values() {\n\t\t\tpackedScores += protowire.SizeVarint(uint64(x))",
		"if c.Raw.Len() > 0 {\n\t\tn += protowire.SizeTag(3) + protowire.SizeBytes(c.Raw.Len())",
		"b = protowire.AppendVarint(b, uint64(c.Raw.Len()))\n\t\tfor x := range c.Raw.Values() {\n\t\t\tb = append(b, x)",
	)
}

// TestRenderGoWireDirect_Arrays pins fixed arrays, which ToProto flattens with every element present:
// [N]byte is one bytes field of a constant length at any depth, a numeric array packs (constant length
// for fixed-width, a walk for varints), a message array is one length-delimited element each.
func TestRenderGoWireDirect_Arrays(t *testing.T) {
	t.Parallel()
	out := renderDirect(t, nil, vec2Message(), directComponent("Fixed",
		sdkgen.Field{Name: "Hash", Number: 1, Kind: "scalar", Scalar: scBy, ArrayDims: []int64{32}},
		sdkgen.Field{Name: "Grid", Number: 2, Kind: "scalar", Scalar: scBy, ArrayDims: []int64{2, 4}},
		sdkgen.Field{Name: "Verts", Number: 3, Kind: "repeated", Msg: vec2, ArrayDims: []int64{8}},
		sdkgen.Field{Name: "Cells", Number: 4, Kind: "repeated", Scalar: scI32, ArrayDims: []int64{3, 3}},
		sdkgen.Field{Name: "Levels", Number: 5, Kind: "repeated", Scalar: scF64, ArrayDims: []int64{4}},
		sdkgen.Field{Name: "Never", Number: 6, Kind: "repeated", Scalar: scI32, ArrayDims: []int64{0}},
	))
	wantAll(
		t,
		out,
		"\tn += protowire.SizeTag(1) + protowire.SizeBytes(32)\n",
		"b = protowire.AppendTag(b, 1, protowire.BytesType)\n\tb = protowire.AppendVarint(b, 32)\n\tb = append(b, c.Hash[:]...)",
		"\tn += protowire.SizeTag(2) + protowire.SizeBytes(8)\n",
		"b = protowire.AppendVarint(b, 8)\n\tfor i0 := range c.Grid {\n\t\tb = append(b, c.Grid[i0][:]...)",
		"for i0 := range c.Verts {\n\t\tn += protowire.SizeTag(3) + protowire.SizeBytes(c.Verts[i0].SizeWire())",
		"for i0 := range c.Verts {\n\t\tb = protowire.AppendTag(b, 3, protowire.BytesType)\n\t\tatVerts := len(b)\n\t\tb = append(b, 0)\n\t\tb = c.Verts[i0].AppendWire(b)\n\t\tb = wireLenPrefix(b, atVerts)",
		"packedCells := 0\n\tfor i0 := range c.Cells {\n\t\tfor i1 := range c.Cells[i0] {\n\t\t\tpackedCells += protowire.SizeVarint(uint64(c.Cells[i0][i1]))",
		"\tn += protowire.SizeTag(4) + protowire.SizeBytes(packedCells)\n",
		"b = protowire.AppendVarint(b, uint64(packedCells))\n\tfor i0 := range c.Cells {\n\t\tfor i1 := range c.Cells[i0] {\n\t\t\tb = protowire.AppendVarint(b, uint64(c.Cells[i0][i1]))",
		"\tn += protowire.SizeTag(5) + protowire.SizeBytes(4*protowire.SizeFixed64())\n",
		"for i0 := range c.Levels {\n\t\tb = protowire.AppendFixed64(b, math.Float64bits(float64(c.Levels[i0])))",
	)
	// [0]T is an empty repeated: nothing on the wire, and no field 6 in either method.
	wantNone(t, out, "SizeTag(6)", "AppendTag(b, 6,")
}

// TestRenderGoWireDirect_Mirrored pins a type another module owns: no methods can be declared on it,
// so its pair are free functions in this file, called wherever the type appears.
func TestRenderGoWireDirect_Mirrored(t *testing.T) {
	t.Parallel()
	money := sdkgen.TypeRef{Name: "Money", GoType: "pkg_dep.Money", PkgPath: "example.com/dep",
		GenImport: "example.com/g/gen/dep", Import: `pkg_dep "example.com/dep"`, Own: sdkgen.OwnMirrored}
	mirrored := map[string]sdkgen.Message{
		money.Key(): {
			Own:       sdkgen.OwnMirrored,
			Self:      money,
			Name:      "Money",
			PkgName:   "dep",
			PkgPath:   "example.com/dep",
			GenImport: "example.com/g/gen/dep",
			Fields: []sdkgen.Field{
				{Name: "Cents", Number: 1, Kind: "scalar", Scalar: scalarOf("varint", "int64", "int64")},
			},
		},
	}
	out := renderDirect(t, mirrored, directComponent("Wallet",
		sdkgen.Field{Name: "Balance", Number: 1, Kind: "message", Msg: money},
		sdkgen.Field{Name: "History", Number: 3, Kind: "repeated", Msg: money},
	))
	wantAll(
		t,
		out,
		"func mirrorExampleComDepMoneySizeWire(c pkg_dep.Money) int {",
		"func mirrorExampleComDepMoneyAppendWire(b []byte, c pkg_dep.Money) []byte {",
		"if c.Cents != 0 {\n\t\tn += protowire.SizeTag(1) + protowire.SizeVarint(uint64(c.Cents))",
		"n += protowire.SizeTag(1) + protowire.SizeBytes(mirrorExampleComDepMoneySizeWire(c.Balance))",
		"b = mirrorExampleComDepMoneyAppendWire(b, c.Balance)",
		"for _, x := range c.History {\n\t\tn += protowire.SizeTag(3) + protowire.SizeBytes(mirrorExampleComDepMoneySizeWire(x))",
	)
}

// TestRenderGoWireDirect_NamedScalars pins that every encode-side cast goes to the basic type, so a
// NAMED scalar (type Flag bool, type Angle float64, type Label string) compiles wherever it appears:
// a named bool is a valid condition but not a valid argument to wireBool's bool parameter, and a named
// float or string is not assignable to [math.Float64bits] or AppendString either.
func TestRenderGoWireDirect_NamedScalars(t *testing.T) {
	t.Parallel()
	flag := scalarOf("bool", "Flag", "bool")
	angle := scalarOf("i64", "Angle", "double")
	label := scalarOf("string", "Label", "string")
	out := renderDirect(t, nil, directComponent("Named",
		sdkgen.Field{Name: "On", Number: 1, Kind: "scalar", Scalar: flag},
		sdkgen.Field{Name: "Flags", Number: 3, Kind: "repeated", Scalar: flag},
		sdkgen.Field{Name: "Grid", Number: 4, Kind: "repeated", Scalar: flag, ArrayDims: []int64{2}},
		sdkgen.Field{Name: "Heading", Number: 6, Kind: "scalar", Scalar: angle},
		sdkgen.Field{Name: "Name", Number: 7, Kind: "scalar", Scalar: label},
	))
	wantAll(
		t,
		out,
		"if c.On {\n\t\tb = protowire.AppendTag(b, 1, protowire.VarintType)\n\t\tb = protowire.AppendVarint(b, 1)",
		"for _, x := range c.Flags {\n\t\t\tb = protowire.AppendVarint(b, wireBool(bool(x)))",
		"for i0 := range c.Grid {\n\t\tb = protowire.AppendVarint(b, wireBool(bool(c.Grid[i0])))",
		"if math.Float64bits(float64(c.Heading)) != 0 {\n\t\tb = protowire.AppendTag(b, 6, protowire.Fixed64Type)\n\t\tb = protowire.AppendFixed64(b, math.Float64bits(float64(c.Heading)))",
		"b = protowire.AppendString(b, string(c.Name))",
	)
	// The helper's own parameter is the basic type, and the bare (uncast) call never appears.
	wantAll(t, out, "func wireBool(v bool) uint64 {")
	wantNone(t, out, "wireBool(x)")
}

// TestRenderGoWireDirect_StringsAreUTF8Checked pins proto.Marshal's UTF-8 check, kept in the size
// pass so a bad string fails the write before a byte is appended.
func TestRenderGoWireDirect_StringsAreUTF8Checked(t *testing.T) {
	t.Parallel()
	out := renderDirect(t, nil, directComponent("Player",
		sdkgen.Field{Name: "ID", Number: 1, Kind: "scalar", Scalar: scStr},
		sdkgen.Field{Name: "Blob", Number: 2, Kind: "scalar", Scalar: scBy},
		sdkgen.Field{Name: "Tags", Number: 3, Kind: "repeated", Scalar: scStr},
		sdkgen.Field{Name: "Slots", Number: 4, Kind: "repeated", Scalar: scStr, ArrayDims: []int64{2}},
	))
	wantAll(t, out,
		`"unicode/utf8"`,
		"func wireStringSize(field, s string) int {",
		"if !utf8.ValidString(s) {",
		`panic("failed to encode " + field + ": string field contains invalid UTF-8")`,
		// Every string position is checked, and the panic names the field it came from.
		`wireStringSize("Player.ID", string(c.ID))`,
		`wireStringSize("Player.Tags[]", string(x))`,
		`wireStringSize("Player.Slots[]", string(c.Slots[i0]))`,
	)
	// A bytes field carries arbitrary octets by definition, so it must NOT be checked.
	wantAll(t, out, "protowire.SizeBytes(len(c.Blob))")
	wantNone(t, out, `wireStringSize("Player.Blob"`)
	if n := strings.Count(out, "func wireStringSize("); n != 1 {
		t.Errorf("wireStringSize declared %d times, want once", n)
	}
}

// TestRenderGoWireDirect_NoStringNoHelper pins that the helper and its import appear only when a
// string is actually written — an unused import fails the build of every backend without one.
func TestRenderGoWireDirect_NoStringNoHelper(t *testing.T) {
	t.Parallel()
	out := renderDirect(t, nil, directComponent("Numeric",
		sdkgen.Field{Name: "N", Number: 1, Kind: "scalar", Scalar: scI32},
		sdkgen.Field{Name: "Blob", Number: 2, Kind: "scalar", Scalar: scBy},
	))
	wantNone(t, out, "wireStringSize", `"unicode/utf8"`)
}

// TestRenderGoWireDirect_MarshalWireIsDerived pins that MarshalWire is the direct encoders, not a
// second encoder that has to agree with them.
func TestRenderGoWireDirect_MarshalWireIsDerived(t *testing.T) {
	t.Parallel()
	out := renderDirect(t, nil, directComponent("Position",
		sdkgen.Field{Name: "X", Number: 1, Kind: "scalar", Scalar: scI32},
	))
	wantAll(t, out,
		"func (c Position) MarshalWire() []byte {",
		"return c.AppendWire(make([]byte, 0, c.SizeWire()))",
		// Decode is untouched and still goes through proto.
		"func (c Position) UnmarshalWire(data []byte) (any, error) {",
		"proto.Unmarshal(data, &p)",
	)
	wantNone(t, out,
		"proto.Marshal(c.ToProto())",
		`panic("failed to marshal Position: " + err.Error())`,
	)
}

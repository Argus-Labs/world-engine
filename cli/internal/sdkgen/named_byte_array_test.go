package sdkgen_test

import (
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/cli/internal/sdkgen"
)

// namedByteCmd wraps a command C whose struct body is fields, with a leading
// `type MyByte byte` declaration so the field types reference a NAMED byte type.
func namedByteCmd(name, fields string) string {
	return `import "sdkgentest/cardinal"

type MyByte byte

type C struct {
` + fields + `
}

func (C) Name() string { return "` + name + `" }

func Setup(w *cardinal.World) {
	w.RegisterCommand[C]()
}`
}

// -------------------------------------------------------------------------------------------------
// Named byte arrays: classification
// -------------------------------------------------------------------------------------------------
// A NAMED byte type (type X byte) must not be mis-classified as a proto `bytes` blob the way an
// unnamed [N]byte is. Go rejects a []X <-> []byte slice cast, so the bytes-blob wire emitter would
// write uncompilable conversions. A named-byte array therefore flattens to the existing
// repeated-scalar path (repeated uint32) exactly as a named-byte SLICE already does, so the array
// and the slice of the same named type agree on the proto type.

// TestProtoNamedByteArrayIsRepeatedUint32 pins the array case against the bug that routed [N]MyByte
// to a `bytes` blob via isByte's Underlying() peephole: it must land on `repeated uint32`, not `bytes`,
// and the array must agree with the slice form of the same named type.
func TestProtoNamedByteArrayIsRepeatedUint32(t *testing.T) {
	t.Parallel()

	got := proto(t, namedByteCmd("c", "\tValues [4]MyByte"))
	if !strings.Contains(got, "repeated uint32 Values = 1;") {
		t.Fatalf("named byte array must generate repeated uint32, got:\n%s", got)
	}
	if strings.Contains(got, "bytes Values = 1;") {
		t.Fatalf("named byte array must not generate a bytes blob, got:\n%s", got)
	}
	// Shape is schema, so it is recorded as a comment naming the declared (named) element type.
	if !strings.Contains(got, "// Go: [4]MyByte, 4 elements") {
		t.Fatalf("want the array shape recorded as [4]MyByte, got:\n%s", got)
	}

	// The slice form of the same named type already takes the repeated-scalar path; the array form
	// must agree so the same named type does not pick a different proto type by collection shape.
	slice := proto(t, namedByteCmd("c", "\tValues []MyByte"))
	if !strings.Contains(slice, "repeated uint32 Values = 1;") {
		t.Fatalf("named byte slice must still generate repeated uint32, got:\n%s", slice)
	}
	if strings.Contains(slice, "bytes Values = 1;") {
		t.Fatalf("named byte slice must not generate a bytes blob, got:\n%s", slice)
	}
}

// TestProtoNamedByteArrayMultiDimIsRepeatedUint32 pins the multi-dimensional array case: [4][16]MyByte
// peels to the named byte element and must flatten to `repeated uint32` (64 elements, row-major),
// not a `bytes` blob. Before the fix this was emitted as a single `bytes` field with a flattened
// byte wire body that did not compile.
func TestProtoNamedByteArrayMultiDimIsRepeatedUint32(t *testing.T) {
	t.Parallel()

	got := proto(t, namedByteCmd("c", "\tValues [4][16]MyByte"))
	if !strings.Contains(got, "repeated uint32 Values = 1;") {
		t.Fatalf("named byte multi-dim array must generate repeated uint32, got:\n%s", got)
	}
	if strings.Contains(got, "bytes Values = 1;") {
		t.Fatalf("named byte multi-dim array must not generate a bytes blob, got:\n%s", got)
	}
	if !strings.Contains(got, "// Go: [4][16]MyByte (row-major), 64 elements") {
		t.Fatalf("want the multi-dim array shape recorded as [4][16]MyByte, got:\n%s", got)
	}
}

// -------------------------------------------------------------------------------------------------
// Named byte arrays: wire is compilable
// -------------------------------------------------------------------------------------------------
// The bytes-blob wire emitter writes slice-level conversions (p.Values = c.Values[:], copy(...), and
// for the multi-dim case a flat []byte buffer filled from []MyByte slices) that Go rejects across a
// defined-type boundary. The repeated-scalar path instead converts element-wise: uint32(...MyByte)
// on encode and MyByte(...uint32) on decode, both of which compile. These tests pin the element-wise
// form and assert the broken slice-level conversions are absent — the generator's pre-write guards
// only parse the body, so a regression to the bytes-blob path would land uncompilable code on disk.

func TestWireNamedByteArrayCompiles(t *testing.T) {
	t.Parallel()

	got := wire(t, namedByteCmd("c", "\tValues [4]MyByte"))
	// Element-wise encode/decode across the named-byte <-> uint32 boundary — both conversions compile.
	for _, want := range []string{
		"uint32(c.Values[i0])",
		"c.Values[i] = MyByte(e)",
		"if i >= 4 {",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in:\n%s", want, got)
		}
	}
	// The bytes-blob single-dim body must NOT be present: those lines do not compile for a named byte.
	for _, broken := range []string{
		"p.Values = c.Values[:]",
		"copy(c.Values[:], p.Values)",
	} {
		if strings.Contains(got, broken) {
			t.Errorf("named byte array must NOT emit the bytes-blob %q, got:\n%s", broken, got)
		}
	}
}

func TestWireNamedByteArrayMultiDimCompiles(t *testing.T) {
	t.Parallel()

	got := wire(t, namedByteCmd("c", "\tValues [4][16]MyByte"))
	// Element-wise encode walks both dims; decode rebuilds the 2-D index and casts back to the named type.
	for _, want := range []string{
		"uint32(c.Values[i0][i1])",
		"c.Values[i/16][i%16] = MyByte(e)",
		"if i >= 64 {",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in:\n%s", want, got)
		}
	}
	// The bytes-blob multi-dim body must NOT be present: it allocates a flat []byte buffer, slices the
	// named-byte array element into it (an invalid []MyByte -> []byte conversion), and assigns a raw
	// `byte` back into a MyByte slot on decode. None of those compile across the defined-type boundary.
	for _, broken := range []string{
		"flatValues",          // the byte-buffer local (byteBufName)
		"make([]byte, 0, 64)", // the buffer allocation
		"c.Values[i0][:]",     // the slice cast of a []MyByte element to []byte
		"= v\n\t}",            // assigning a raw byte (v) into a MyByte slot
	} {
		if strings.Contains(got, broken) {
			t.Errorf("named byte multi-dim array must NOT emit the bytes-blob form near %q, got:\n%s", broken, got)
		}
	}
}

// -------------------------------------------------------------------------------------------------
// Nested slice of a named byte: refused (no broken wire)
// -------------------------------------------------------------------------------------------------
// A nested slice [][]MyByte is not representable in protobuf regardless of the element being named
// or not. The strict isByte fix closes the gap the lenient version left open: [][]MyByte used to be
// mis-classified as `repeated bytes` and emit []byte([]MyByte) conversions that do not compile, with
// no Violation raised. It now falls through to the existing nested-slice refusal, so the field is
// DROPPED and a Violation is reported, and no broken wire is written.

func TestNestedNamedByteSliceIsRefused(t *testing.T) {
	t.Parallel()

	res := discover(t, namedByteCmd("c", "\tValues [][]MyByte"))
	// The field must be refused (a blocking Violation), not silently emitted.
	var found *sdkgen.Violation
	for i := range res.Violations {
		if strings.HasSuffix(res.Violations[i].Where, ".Values") {
			found = &res.Violations[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("want a blocking Violation for C.Values; got none. Violations: %+v", res.Violations)
	}
	// The field's own shape (a slice) is the reported category — not an opaque unsupported-type.
	if found.Category != sdkgen.CatSlice {
		t.Errorf("want category %s, got %s (%+v)", sdkgen.CatSlice, found.Category, found)
	}
	// The refusal names the nested-slice reason, offering the wrap-in-a-message fix.
	if !strings.Contains(found.Reason, "nested slice") {
		t.Errorf("want the nested-slice refusal reason, got %q", found.Reason)
	}
	// No wire for the field is emitted: the field was dropped from the message, so the rendered
	// wire carries no Values reference at all.
	files, err := sdkgen.EmitProtos(res.Module, "", res.Messages)
	if err != nil {
		t.Fatalf("emit proto: %v", err)
	}
	var protoBody strings.Builder
	for _, f := range files {
		protoBody.WriteString(f.Content)
	}
	if strings.Contains(protoBody.String(), "Values") {
		t.Errorf("a refused field must not appear in the proto, got:\n%s", protoBody.String())
	}
}

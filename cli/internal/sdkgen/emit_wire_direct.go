package sdkgen

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// The direct encoders: SizeWire and AppendWire.
//
// The pair encodes a value straight into the caller's buffer as protobuf wire bytes, with no proto
// message in between. SizeWire is the exact byte count the encoding takes; AppendWire writes exactly
// that many. Neither allocates, which is the point: the engine's snapshot sizes every component of
// every entity in one pass and writes them all into one buffer in a second, so the per-row proto graph
// MarshalWire builds (ToProto, then proto.Marshal) would be the dominant allocation of a tick.
//
// The bytes are the ones proto.Marshal(c.ToProto()) produces, field for field, so UnmarshalWire
// (proto.Unmarshal, then FromProto) decodes either and decode stays as it is. That pins every rule
// below to protobuf-go's proto3 behaviour rather than to a choice of ours:
//   - implicit presence: a singular scalar is absent at its zero value — and -0.0 is not zero, since
//     protobuf-go tests the sign bit, so floats compare their bit pattern to zero, never their value;
//   - a nested message is always present, as ToProto always sets it: an empty message still writes
//     its tag and a zero length;
//   - a repeated numeric or bool is packed (one tag, one length, the payloads back to back); a
//     repeated string, bytes or message is length-delimited one element at a time; an empty repeated
//     writes nothing;
//   - fields go out in field-number order, which is proto.Marshal's order as well.
//
// Every message gets the pair, top-level and nested alike, because a parent sizes and writes a nested
// field through the field's own methods. AppendWire never sizes: a nested message's length prefix is
// written as a one-byte placeholder and patched once its body is down (see wireLenPrefix), so the
// append pass is one walk. A mirrored type gets free functions, as it does for ToProto/FromProto.
// Package-level helpers are written on demand (see renderHelpers).

const (
	protowirePkg = "google.golang.org/protobuf/encoding/protowire"
	mathPkg      = "math" // Float32bits / Float64bits: the fixed32 / fixed64 payload of a float
	utf8Pkg      = "unicode/utf8"
)

// Scalar.Family values (see scalarOf): each is a protobuf wire type, which is what the encoders
// switch on. bytes is protoBytes.
const (
	famBool   = "bool"
	famI32    = "i32"
	famI64    = "i64"
	famString = "string"
)

// Helpers the direct encoders may call. Each is written once per wire file and only when a field asks
// for it — recorded the way an import is (wireGen.helper), by the call that writes the name.
const (
	helperBool            = "wireBool"            // bool -> the varint it encodes as
	helperString          = "wireStringSize"      // a string's encoded size, with proto's UTF-8 check
	helperTimestamp       = "timestamp"           // key for the pair below
	helperTimestampSize   = "sizeWireTimestamp"   // the encoded size of the Timestamp timestamppb.New(t) builds
	helperTimestampAppend = "appendWireTimestamp" // its bytes
	helperLenPrefix       = "wireLenPrefix"       // patches a message's length prefix in after its body
)

// helper records that the file must declare the named helper, and returns the name for the call site.
func (g *wireGen) helper(name string) string {
	g.helpers[name] = true
	return name
}

// renderHelpers emits every helper the body asked for, in name order so the file's contents never
// depend on discovery order, registering the imports the bodies need. Call it after the body and the
// mirrors are rendered — both may ask for helpers — and before importSpecs.
// Helper source. %[1]s is the helper's own name, so a rename of the constant cannot drift from the
// declaration it emits.
const (
	tmplString = `// %[1]s is protowire.SizeBytes(len(s)) plus the UTF-8 check proto.Marshal
// performs: a proto3 string holding invalid UTF-8 cannot be decoded, so the size pass
// fails.
func %[1]s(field, s string) int {
	if !utf8.ValidString(s) {
		panic("failed to encode " + field + ": string field contains invalid UTF-8")
	}
	return protowire.SizeBytes(len(s))
}

`
	tmplBool = `// %[1]s is the varint a bool encodes as.
func %[1]s(v bool) uint64 {
	if v {
		return 1
	}
	return 0
}

`
	tmplLenPrefix = `// %[1]s writes the length of the bytes appended after the placeholder at b[at].
// The body is moved up only when the length needs more than the one byte reserved.
func %[1]s(b []byte, at int) []byte {
	n := len(b) - at - 1
	if n < 0x80 {
		b[at] = byte(n)
		return b
	}
	k := protowire.SizeVarint(uint64(n)) - 1
	b = append(b, make([]byte, k)...)
	copy(b[at+1+k:], b[at+1:at+1+n])
	protowire.AppendVarint(b[at:at], uint64(n)) // in place: cap reaches the body
	return b
}

`
	// The body timestamppb.New(t) marshals to: int64 seconds = 1, int32 nanos = 2, each absent at
	// zero. Nanosecond() is 0..999999999, so its varint is the int32 one.
	tmplTimestamp = `// %[1]s is the encoded size of the google.protobuf.Timestamp holding t.
func %[1]s(t time.Time) int {
	n := 0
	if s := t.Unix(); s != 0 {
		n += protowire.SizeTag(1) + protowire.SizeVarint(uint64(s))
	}
	if ns := t.Nanosecond(); ns != 0 {
		n += protowire.SizeTag(2) + protowire.SizeVarint(uint64(ns))
	}
	return n
}

// %[2]s writes the google.protobuf.Timestamp holding t.
func %[2]s(b []byte, t time.Time) []byte {
	if s := t.Unix(); s != 0 {
		b = protowire.AppendTag(b, 1, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(s))
	}
	if ns := t.Nanosecond(); ns != 0 {
		b = protowire.AppendTag(b, 2, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(ns))
	}
	return b
}

`
)

func renderHelpers(g *wireGen) string {
	var b strings.Builder
	for _, name := range slices.Sorted(maps.Keys(g.helpers)) {
		switch name {
		case helperString:
			g.need(protowirePkg)
			g.need(utf8Pkg)
			fmt.Fprintf(&b, tmplString, helperString)
		case helperBool:
			fmt.Fprintf(&b, tmplBool, helperBool)
		case helperLenPrefix:
			g.need(protowirePkg)
			fmt.Fprintf(&b, tmplLenPrefix, helperLenPrefix)
		case helperTimestamp:
			g.need(protowirePkg)
			g.need(timePkg)
			fmt.Fprintf(&b, tmplTimestamp, helperTimestampSize, helperTimestampAppend)
		}
	}
	return b.String()
}

// -------------------------------------------------------------------------------------------------
// Shapes these encoders decline
// -------------------------------------------------------------------------------------------------

// unsupportedDirectShape names the shape f has that SizeWire/AppendWire cannot write, or "" for one
// they can. ToProto and FromProto carry three that they do not, and each is a blocking violation at
// discovery (CatMap, CatPointer), so no `world sdk generate` run reaches here holding one. RenderGoWire
// is exported, though, and the field emitters have no default arm: an unhandled shape would be dropped
// from both passes in silence, and MarshalWire is now these encoders rather than proto.Marshal, so the
// field would go missing from the bytes instead of being caught by the proto layer.
func unsupportedDirectShape(f Field) string {
	switch {
	case f.Kind == kindMap:
		return "a map"
	case f.Optional:
		return "an optional scalar (*T)"
	case f.Pointer:
		return "a pointer to a message (*T)"
	}
	return ""
}

// checkDirectEncodable refuses a group holding a shape the direct encoders decline, mirrors included.
func checkDirectEncodable(msgs []Message, mirrored map[string]Message) error {
	check := func(m Message) error {
		for _, f := range m.Fields {
			if shape := unsupportedDirectShape(f); shape != "" {
				return fmt.Errorf("%s: SizeWire/AppendWire cannot encode %s", fieldLabel(m.Name, f), shape)
			}
		}
		return nil
	}
	for _, m := range msgs {
		if err := check(m); err != nil {
			return err
		}
	}
	// Sorted, so a group with two bad mirrors always names the same one.
	for _, k := range slices.Sorted(maps.Keys(mirrored)) {
		if err := check(mirrored[k]); err != nil {
			return err
		}
	}
	return nil
}

// wireDirect emits SizeWire and AppendWire as methods on m.
func wireDirect(b *strings.Builder, g *wireGen, m Message) {
	fmt.Fprintf(b, "func (c %s) SizeWire() int {\n", m.Name)
	wireSizeBody(b, g, m.Name, m.Fields)
	fmt.Fprintf(b, "func (c %s) AppendWire(b []byte) []byte {\n", m.Name)
	wireAppendBody(b, g, m.Fields)
}

// wireDirectMirror emits the pair for a mirrored message as free functions, the parameter named c so
// the field emitters apply unchanged.
func wireDirectMirror(b *strings.Builder, g *wireGen, m Message, self string) {
	fmt.Fprintf(b, "func %s(c %s) int {\n", m.Self.ConvFunc("SizeWire"), self)
	wireSizeBody(b, g, m.Name, m.Fields)
	fmt.Fprintf(b, "func %s(b []byte, c %s) []byte {\n", m.Self.ConvFunc("AppendWire"), self)
	wireAppendBody(b, g, m.Fields)
}

func wireSizeBody(b *strings.Builder, g *wireGen, owner string, fields []Field) {
	b.WriteString("\tn := 0\n")
	for _, f := range fields {
		wireSizeField(b, g, owner, f)
	}
	b.WriteString("\treturn n\n}\n\n")
}

func wireAppendBody(b *strings.Builder, g *wireGen, fields []Field) {
	for _, f := range fields {
		wireAppendField(b, g, f)
	}
	b.WriteString("\treturn b\n}\n\n")
}

// -------------------------------------------------------------------------------------------------
// Scalars
// -------------------------------------------------------------------------------------------------
// A scalar is written through its Scalar.Family, which is its wire type: varint and bool are varints,
// i32 and i64 are the fixed-width floats, string and bytes are length-delimited. The encode-side casts
// go to Go's basic types (uint64 for every varint, which sign-extends a negative int32 exactly as
// protobuf does; float64/string/[]byte for the rest) so a named scalar such as cardinal.EntityID
// needs no import here — the decode side is where the declared type matters.

// wireTypeOf is the protowire.Type constant a scalar's own tag carries.
func wireTypeOf(s *Scalar) string {
	switch s.Family {
	case famI32:
		return "protowire.Fixed32Type"
	case famI64:
		return "protowire.Fixed64Type"
	case famString, protoBytes:
		return "protowire.BytesType"
	default: // varint, bool
		return "protowire.VarintType"
	}
}

// packable reports whether a repeated scalar is packed on the wire: every numeric and bool is; a
// string or bytes is length-delimited one element at a time.
func packable(s *Scalar) bool { return s.Family != famString && s.Family != protoBytes }

// scalarPresent is the proto3 implicit-presence test for the singular scalar at x.
func scalarPresent(g *wireGen, s *Scalar, x string) string {
	switch s.Family {
	case famBool:
		return x
	case famI32:
		g.need(mathPkg)
		return fmt.Sprintf("math.Float32bits(float32(%s)) != 0", x)
	case famI64:
		g.need(mathPkg)
		return fmt.Sprintf("math.Float64bits(float64(%s)) != 0", x)
	case famString, protoBytes:
		return fmt.Sprintf("len(%s) > 0", x)
	default:
		return x + " != 0"
	}
}

// scalarSize is the encoded size of the scalar payload at x, tag excluded. label names the field in
// the panic a bad string raises, so the failure points at the value rather than at the encoder.
func scalarSize(g *wireGen, s *Scalar, label, x string) string {
	switch s.Family {
	case famBool:
		return "1"
	case famI32:
		return "protowire.SizeFixed32()"
	case famI64:
		return "protowire.SizeFixed64()"
	case famString:
		// Only a string is checked. A bytes field carries arbitrary octets by definition.
		return fmt.Sprintf("%s(%q, string(%s))", g.helper(helperString), label, x)
	case protoBytes:
		return fmt.Sprintf("protowire.SizeBytes(len(%s))", x)
	default:
		return fmt.Sprintf("protowire.SizeVarint(uint64(%s))", x)
	}
}

// scalarAppend is the statement appending the scalar payload at x, tag excluded. A string or bytes
// payload carries its own length prefix, as AppendString/AppendBytes write it.
func scalarAppend(g *wireGen, s *Scalar, x string) string {
	switch s.Family {
	case famBool:
		// Cast like every other family: a named bool (type Flag bool) is not assignable to the helper's
		// bool parameter, though it is usable as a condition, which is why the singular path needs none.
		return fmt.Sprintf("b = protowire.AppendVarint(b, %s(bool(%s)))", g.helper(helperBool), x)
	case famI32:
		g.need(mathPkg)
		return fmt.Sprintf("b = protowire.AppendFixed32(b, math.Float32bits(float32(%s)))", x)
	case famI64:
		g.need(mathPkg)
		return fmt.Sprintf("b = protowire.AppendFixed64(b, math.Float64bits(float64(%s)))", x)
	case famString:
		return fmt.Sprintf("b = protowire.AppendString(b, string(%s))", x)
	case protoBytes:
		return fmt.Sprintf("b = protowire.AppendBytes(b, []byte(%s))", x)
	default:
		return fmt.Sprintf("b = protowire.AppendVarint(b, uint64(%s))", x)
	}
}

// appendTag is the statement writing field num's tag with the given protowire.Type.
func appendTag(num int, typ string) string {
	return fmt.Sprintf("b = protowire.AppendTag(b, %d, %s)", num, typ)
}

// -------------------------------------------------------------------------------------------------
// Messages
// -------------------------------------------------------------------------------------------------
// A nested message is sized and written through its own pair: a generated method for a local or
// imported type, a free function for a mirrored one, a helper for a Timestamp. x is the value expression.

// msgSize is the encoded size of the message body at x, tag and length prefix excluded.
func msgSize(g *wireGen, ref TypeRef, timestamp bool, x string) string {
	switch {
	case timestamp:
		g.helper(helperTimestamp)
		return fmt.Sprintf("%s(%s)", helperTimestampSize, x)
	case ref.Own.Mirrored():
		return fmt.Sprintf("%s(%s)", g.mirrorCall(ref, "SizeWire"), x)
	default:
		return x + ".SizeWire()"
	}
}

// msgAppend is the statement appending the message body at x, tag and length prefix excluded.
func msgAppend(g *wireGen, ref TypeRef, timestamp bool, x string) string {
	switch {
	case timestamp:
		g.helper(helperTimestamp)
		return fmt.Sprintf("b = %s(b, %s)", helperTimestampAppend, x)
	case ref.Own.Mirrored():
		return fmt.Sprintf("b = %s(b, %s)", g.mirrorCall(ref, "AppendWire"), x)
	default:
		return "b = " + x + ".AppendWire(b)"
	}
}

// prefixName is the local holding a message field's length-placeholder offset, one per field so two
// such fields in one method never redeclare it (compare packedName).
func prefixName(field string) string { return "at" + field }

// appendDelimited writes the message body at x with its length prefix: a one-byte placeholder, the
// body, then the length patched in. The body is never sized, so the append pass stays one walk.
func appendDelimited(b *strings.Builder, g *wireGen, f Field, x string) {
	at := prefixName(f.Name)
	fmt.Fprintf(b, "\t%s := len(b)\n\tb = append(b, 0)\n", at)
	fmt.Fprintf(b, "\t%s\n", msgAppend(g, f.Msg, f.Timestamp, x))
	fmt.Fprintf(b, "\tb = %s(b, %s)\n", g.helper(helperLenPrefix), at)
}

// appendMsg writes one length-delimited message under f's number: its tag, its length, its body.
func appendMsg(b *strings.Builder, g *wireGen, f Field, x string) {
	fmt.Fprintf(b, "\t%s\n", appendTag(f.Number, "protowire.BytesType"))
	appendDelimited(b, g, f, x)
}

// elemSize is the encoded size, tag excluded, of one element of a non-packed repeated or array field:
// a string or bytes scalar, or a message with its length prefix.
func elemSize(g *wireGen, owner string, f Field, x string) string {
	if f.Scalar != nil {
		return scalarSize(g, f.Scalar, fieldLabel(owner, f)+"[]", x)
	}
	return "protowire.SizeBytes(" + msgSize(g, f.Msg, f.Timestamp, x) + ")"
}

// elemAppend writes one element of a non-packed repeated or array field after its tag: a string or
// bytes scalar (AppendString/AppendBytes carry the length), or a message's length and body.
func elemAppend(b *strings.Builder, g *wireGen, f Field, x string) {
	if f.Scalar != nil {
		fmt.Fprintf(b, "\t%s\n", scalarAppend(g, f.Scalar, x))
		return
	}
	appendDelimited(b, g, f, x)
}

// -------------------------------------------------------------------------------------------------
// Fields
// -------------------------------------------------------------------------------------------------

// fieldLabel names one field for a panic message, e.g. "PlayerComponent.PlayerID".
func fieldLabel(owner string, f Field) string { return owner + "." + f.Name }

// wireSizeField adds field f's encoded size to n.
func wireSizeField(b *strings.Builder, g *wireGen, owner string, f Field) {
	g.need(protowirePkg)
	tag := fmt.Sprintf("protowire.SizeTag(%d)", f.Number)
	x := "c." + f.Name
	label := fieldLabel(owner, f)
	if len(f.ArrayDims) > 0 {
		wireSizeArray(b, g, owner, f, tag)
		return
	}
	switch f.Kind {
	case kindScalar:
		switch {
		case f.Slice: // Slice[byte] -> bytes
			fmt.Fprintf(b, "\tif %s.Len() > 0 {\n\t\tn += %s + protowire.SizeBytes(%s.Len())\n\t}\n", x, tag, x)
		default:
			fmt.Fprintf(b, "\tif %s {\n\t\tn += %s + %s\n\t}\n",
				scalarPresent(g, f.Scalar, x), tag, scalarSize(g, f.Scalar, label, x))
		}
	case kindMessage:
		fmt.Fprintf(b, "\tn += %s + protowire.SizeBytes(%s)\n", tag, msgSize(g, f.Msg, f.Timestamp, x))
	case kindRepeated:
		wireSizeRepeated(b, g, owner, f, tag)
	}
}

// wireAppendField appends field f's bytes to b.
func wireAppendField(b *strings.Builder, g *wireGen, f Field) {
	g.need(protowirePkg)
	x := "c." + f.Name
	if len(f.ArrayDims) > 0 {
		wireAppendArray(b, g, f)
		return
	}
	switch f.Kind {
	case kindScalar:
		switch {
		case f.Slice: // Slice[byte] -> bytes, read a byte at a time: a Slice exposes no backing array
			fmt.Fprintf(b, "\tif %s.Len() > 0 {\n\t\t%s\n\t\tb = protowire.AppendVarint(b, uint64(%s.Len()))\n",
				x, appendTag(f.Number, "protowire.BytesType"), x)
			fmt.Fprintf(b, "\t\tfor x := range %s.%s() {\n\t\t\tb = append(b, x)\n\t\t}\n\t}\n", x, sliceIterMethod)
		default:
			wireAppendScalar(b, g, f, x)
		}
	case kindMessage:
		appendMsg(b, g, f, x)
	case kindRepeated:
		wireAppendRepeated(b, g, f)
	}
}

// wireAppendScalar writes a singular scalar under implicit presence. A bool is only ever written as 1.
func wireAppendScalar(b *strings.Builder, g *wireGen, f Field, x string) {
	tag := appendTag(f.Number, wireTypeOf(f.Scalar))
	if f.Scalar.Family == famBool {
		fmt.Fprintf(b, "\tif %s {\n\t\t%s\n\t\tb = protowire.AppendVarint(b, 1)\n\t}\n", x, tag)
		return
	}
	fmt.Fprintf(
		b,
		"\tif %s {\n\t\t%s\n\t\t%s\n\t}\n",
		scalarPresent(g, f.Scalar, x),
		tag,
		scalarAppend(g, f.Scalar, x),
	)
}

// -------------------------------------------------------------------------------------------------
// Repeated fields: a Go slice, or an immutable.Slice read through Len and Values
// -------------------------------------------------------------------------------------------------

func repeatedLen(f Field) string {
	if f.Slice {
		return fmt.Sprintf("c.%s.Len()", f.Name)
	}
	return fmt.Sprintf("len(c.%s)", f.Name)
}

// repeatedRange opens the loop over the field's elements, each bound to x.
func repeatedRange(f Field) string {
	if f.Slice {
		return fmt.Sprintf("for x := range c.%s.%s() {", f.Name, sliceIterMethod)
	}
	return fmt.Sprintf("for _, x := range c.%s {", f.Name)
}

// packedName is the local a varint-packed field's payload size is summed into, one per field so two
// such fields in one message never redeclare it (compare byteBufName).
func packedName(field string) string { return "packed" + field }

// packedPayloadSize returns the expression for a packed field's payload size, given the expression
// for its element count. A fixed-width or bool payload is arithmetic on that count; a varint payload
// needs a pass over the elements, which sumVarints writes, summing into the local it is handed.
func packedPayloadSize(b *strings.Builder, f Field, count string, sumVarints func(packed string)) string {
	switch f.Scalar.Family {
	case famBool:
		return count
	case famI32:
		return count + " * protowire.SizeFixed32()"
	case famI64:
		return count + " * protowire.SizeFixed64()"
	default:
		packed := packedName(f.Name)
		fmt.Fprintf(b, "\t%s := 0\n", packed)
		sumVarints(packed)
		return packed
	}
}

// packedSize is the payload size of a packed repeated field, counted at run time.
func packedSize(b *strings.Builder, f Field) string {
	return packedPayloadSize(b, f, repeatedLen(f), func(packed string) {
		fmt.Fprintf(b, "\t%s\n\t%s += protowire.SizeVarint(uint64(x))\n\t}\n", repeatedRange(f), packed)
	})
}

func wireSizeRepeated(b *strings.Builder, g *wireGen, owner string, f Field, tag string) {
	if f.Scalar != nil && packable(f.Scalar) {
		fmt.Fprintf(b, "\tif %s > 0 {\n", repeatedLen(f))
		size := packedSize(b, f)
		fmt.Fprintf(b, "\tn += %s + protowire.SizeBytes(%s)\n\t}\n", tag, size)
		return
	}
	fmt.Fprintf(b, "\t%s\n\tn += %s + %s\n\t}\n", repeatedRange(f), tag, elemSize(g, owner, f, "x"))
}

func wireAppendRepeated(b *strings.Builder, g *wireGen, f Field) {
	if f.Scalar != nil && packable(f.Scalar) {
		fmt.Fprintf(b, "\tif %s > 0 {\n", repeatedLen(f))
		size := packedSize(b, f)
		fmt.Fprintf(
			b,
			"\t%s\n\tb = protowire.AppendVarint(b, uint64(%s))\n",
			appendTag(f.Number, "protowire.BytesType"),
			size,
		)
		fmt.Fprintf(b, "\t%s\n\t%s\n\t}\n\t}\n", repeatedRange(f), scalarAppend(g, f.Scalar, "x"))
		return
	}
	fmt.Fprintf(b, "\t%s\n\t%s\n", repeatedRange(f), appendTag(f.Number, "protowire.BytesType"))
	elemAppend(b, g, f, "x")
	b.WriteString("\t}\n")
}

// -------------------------------------------------------------------------------------------------
// Fixed arrays: the flat repeated (or bytes) field ToProto builds, every element present
// -------------------------------------------------------------------------------------------------
// ToProto appends all N elements, zero or not, so the field is present for any N > 0 and the packed
// or bytes length is a compile-time constant wherever the payload is fixed-width. A [0]T array is the
// one exception: it is an empty repeated, which writes nothing.

// arrayPackedSize is the packed payload size of a numeric or bool array. Every element is present, so
// the count is a compile-time constant.
func arrayPackedSize(b *strings.Builder, f Field) string {
	count := strconv.FormatInt(f.ArrayCount(), 10)
	return packedPayloadSize(b, f, count, func(packed string) {
		access := arrayLoopHeader(b, f)
		fmt.Fprintf(b, "\t%s += protowire.SizeVarint(uint64(%s))\n", packed, access)
		arrayLoopFooter(b, f)
	})
}

func wireSizeArray(b *strings.Builder, g *wireGen, owner string, f Field, tag string) {
	if f.ArrayCount() == 0 {
		return
	}
	if f.Kind == kindScalar { // [N]byte, any depth: one bytes field of every byte
		fmt.Fprintf(b, "\tn += %s + protowire.SizeBytes(%d)\n", tag, f.ArrayCount())
		return
	}
	if f.Scalar != nil && packable(f.Scalar) {
		size := arrayPackedSize(b, f)
		fmt.Fprintf(b, "\tn += %s + protowire.SizeBytes(%s)\n", tag, size)
		return
	}
	access := arrayLoopHeader(b, f)
	fmt.Fprintf(b, "\tn += %s + %s\n", tag, elemSize(g, owner, f, access))
	arrayLoopFooter(b, f)
}

func wireAppendArray(b *strings.Builder, g *wireGen, f Field) {
	if f.ArrayCount() == 0 {
		return
	}
	if f.Kind == kindScalar { // [N]byte, any depth
		fmt.Fprintf(
			b,
			"\t%s\n\tb = protowire.AppendVarint(b, %d)\n",
			appendTag(f.Number, "protowire.BytesType"),
			f.ArrayCount(),
		)
		if len(f.ArrayDims) == 1 {
			fmt.Fprintf(b, "\tb = append(b, c.%s[:]...)\n", f.Name)
			return
		}
		// The innermost array is what slices to []byte; the loops walk everything above it in the
		// row-major order the decoder rebuilds.
		access := "c." + f.Name
		outer := f.ArrayDims[:len(f.ArrayDims)-1]
		for j := range outer {
			fmt.Fprintf(b, "\tfor i%d := range %s {\n", j, access)
			access += fmt.Sprintf("[i%d]", j)
		}
		fmt.Fprintf(b, "\tb = append(b, %s[:]...)\n", access)
		for range outer {
			b.WriteString("\t}\n")
		}
		return
	}
	if f.Scalar != nil && packable(f.Scalar) {
		size := arrayPackedSize(b, f)
		fmt.Fprintf(
			b,
			"\t%s\n\tb = protowire.AppendVarint(b, uint64(%s))\n",
			appendTag(f.Number, "protowire.BytesType"),
			size,
		)
		access := arrayLoopHeader(b, f)
		fmt.Fprintf(b, "\t%s\n", scalarAppend(g, f.Scalar, access))
		arrayLoopFooter(b, f)
		return
	}
	access := arrayLoopHeader(b, f)
	fmt.Fprintf(b, "\t%s\n", appendTag(f.Number, "protowire.BytesType"))
	elemAppend(b, g, f, access)
	arrayLoopFooter(b, f)
}

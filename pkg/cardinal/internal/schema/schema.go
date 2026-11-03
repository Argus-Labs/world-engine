package schema

import (
	"bytes"
	"encoding"
	"fmt"
	"math"
	"reflect"

	"github.com/rotisserie/eris"
	// vmihailenco/msgpack rather than shamaton/msgpack or ugorji/go/codec:
	//   - immutable.Slice[T] hides its items, so it must encode itself. vmihailenco calls its
	//     EncodeMsgpack/DecodeMsgpack for every T; shamaton's hook registers one exact type, and its
	//     decoder never learns the target type.
	//   - msgpack.Register can replace the decoder of a built-in type such as float32.
	//   - omitempty respects IsZero, so an empty Slice is left out.
	//   - ugorji/go/codec can encode Slice[T] too, but it ships every other format (~5x binary size).
	//   - No release since 2023 is acceptable: the MessagePack spec has not changed since 2017, and
	//     at ~4.5k lines the library is small enough to fork if it ever needs a fix.
	"github.com/vmihailenco/msgpack/v5"
	"github.com/vmihailenco/msgpack/v5/msgpcode"
)

// maxNestingDepth bounds how deeply arrays and maps may nest in decoded input. msgpack recurses once
// per level, including when it skips an unknown field, so a small payload of nested arrays can
// overflow the stack, a fatal error that recover cannot catch. 10,000 matches the limit in
// protobuf-go (protowire.DefaultRecursionLimit) and Go's encoding/json, encoding/xml and
// encoding/gob, the last two added after stack exhaustion CVEs (CVE-2022-30633, CVE-2022-30635).
const maxNestingDepth = 10000

// Serializable is the interface all user-defined payload types must implement.
type Serializable interface {
	Name() string
}

// Serialize encodes a payload as MessagePack.
func Serialize(s Serializable) ([]byte, error) {
	data, err := msgpack.Marshal(s)
	if err != nil {
		return nil, eris.Wrap(err, "failed to serialize")
	}
	return data, nil
}

// Deserialize replaces the target with a freshly decoded value. The target must be a non-nil
// pointer. Missing fields start at zero; failed decoding leaves the target unchanged.
func Deserialize(data []byte, v any) (err error) {
	defer func() {
		// Keep malformed input and custom decoder panics at the serialization boundary.
		if r := recover(); r != nil {
			err = eris.Wrap(fmt.Errorf("panic: %v", r), "failed to deserialize")
		}
	}()

	target := reflect.ValueOf(v)
	if target.Kind() != reflect.Pointer || target.IsNil() {
		return eris.New("failed to deserialize: target must be a non-nil pointer")
	}
	if err := checkNesting(data); err != nil {
		return eris.Wrap(err, "failed to deserialize")
	}
	fresh := reflect.New(target.Elem().Type())
	if err := msgpack.Unmarshal(data, fresh.Interface()); err != nil {
		return eris.Wrap(err, "failed to deserialize")
	}
	target.Elem().Set(fresh.Elem())
	return nil
}

// checkNesting returns an error if data nests arrays and maps deeper than maxNestingDepth. It keeps
// its own stack instead of recursing, so the check itself cannot overflow.
func checkNesting(data []byte) error {
	d := msgpack.NewDecoder(bytes.NewReader(data))
	left := []int{1} // values still to read in each open array or map, outermost first
	for len(left) > 0 {
		top := len(left) - 1
		if left[top] == 0 {
			left = left[:top]
			continue
		}
		left[top]--

		c, err := d.PeekCode()
		if err != nil {
			return err
		}
		var n int
		switch {
		case msgpcode.IsFixedArray(c) || c == msgpcode.Array16 || c == msgpcode.Array32:
			n, err = d.DecodeArrayLen()
		case msgpcode.IsFixedMap(c) || c == msgpcode.Map16 || c == msgpcode.Map32:
			n, err = d.DecodeMapLen()
			n *= 2 // a key and a value per entry
		default:
			// Anything else holds no nested values, so skipping it does not recurse.
			if err := d.Skip(); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if len(left) > maxNestingDepth {
			return eris.Errorf("nesting exceeds %d levels", maxNestingDepth)
		}
		left = append(left, n)
	}
	return nil
}

// Register prepares T for Deserialize. Call it when T is registered, before T is first encoded or
// decoded: msgpack caches a struct's field decoders on first use.
func Register[T any]() {
	registerFloat32Types(reflect.TypeFor[T](), make(map[reflect.Type]bool))
}

// registerFloat32Types gives every float32 type in t, including named ones such as
// `type Meters float32`, the decoder below. msgpack matches registered decoders by exact type, so
// each named type needs its own registration. It walks unexported fields too, to reach the element
// type of an immutable.Slice. A type that decodes itself keeps its own decoder, since a registered
// one would take precedence over it.
func registerFloat32Types(t reflect.Type, seen map[reflect.Type]bool) {
	if seen[t] {
		return
	}
	seen[t] = true
	switch t.Kind() { //nolint:exhaustive // Other kinds cannot contain a float32.
	case reflect.Float32:
		if !decodesItself(t) {
			msgpack.Register(reflect.Zero(t).Interface(), nil, decodeFloat32)
		}
	case reflect.Pointer, reflect.Slice, reflect.Array:
		registerFloat32Types(t.Elem(), seen)
	case reflect.Map:
		registerFloat32Types(t.Key(), seen)
		registerFloat32Types(t.Elem(), seen)
	case reflect.Struct:
		for f := range t.Fields() {
			registerFloat32Types(f.Type, seen)
		}
	}
}

// decodesItself reports whether msgpack would decode t through one of its own methods.
func decodesItself(t reflect.Type) bool {
	p := reflect.PointerTo(t)
	return p.Implements(reflect.TypeFor[msgpack.CustomDecoder]()) ||
		p.Implements(reflect.TypeFor[msgpack.Unmarshaler]()) ||
		p.Implements(reflect.TypeFor[encoding.BinaryUnmarshaler]()) ||
		p.Implements(reflect.TypeFor[encoding.TextUnmarshaler]())
}

// decodeFloat32 accepts any MessagePack number into a float32. Clients without a 32-bit float, such
// as the MCP tools feeding JSON numbers or C# models using double, send 64-bit floats, which msgpack
// rejects for float32 on its own. Values outside float32 range are rejected rather than becoming Inf.
func decodeFloat32(d *msgpack.Decoder, v reflect.Value) error {
	f, err := d.DecodeFloat64()
	if err != nil {
		return err
	}
	if math.Abs(f) > math.MaxFloat32 && !math.IsInf(f, 0) {
		return eris.Errorf("%v overflows float32", f)
	}
	v.SetFloat(f)
	return nil
}

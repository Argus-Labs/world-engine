package schema

import (
	"bytes"
	"encoding"
	"fmt"
	"math"
	"reflect"
	"slices"
	"sync"

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

// Serialize encodes a payload as MessagePack. Equal payloads encode to equal bytes, maps included.
func Serialize(s Serializable) ([]byte, error) {
	prepare(reflect.TypeOf(s))
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
	prepare(reflect.TypeFor[T]())
}

//nolint:gochecknoglobals // msgpack's codec registry is global too, so what is registered must be.
var (
	preparedMu sync.Mutex
	prepared   sync.Map // reflect.Type to struct{}, stored once its codecs are registered
)

// prepare registers t's codecs once. A type counts as prepared only after registration finishes, so
// a concurrent first Serialize cannot encode t, and cache its field encoders, before then.
func prepare(t reflect.Type) {
	if _, ok := prepared.Load(t); ok {
		return
	}
	preparedMu.Lock()
	defer preparedMu.Unlock()
	if _, ok := prepared.Load(t); !ok {
		registerTypes(t, make(map[reflect.Type]bool))
		prepared.Store(t, struct{}{})
	}
}

// registerTypes gives every float32 type in t, including named ones such as `type Meters float32`,
// the decoder decodeFloat32, and every map type the encoder encodeSortedMap. msgpack matches
// registered codecs by exact type, so each named type needs its own registration. It walks
// unexported fields too, to reach the element type of an immutable.Slice. A type with its own codec
// keeps it, since a registered one would take precedence over it.
func registerTypes(t reflect.Type, seen map[reflect.Type]bool) {
	if t == nil || seen[t] {
		return
	}
	seen[t] = true
	switch t.Kind() { //nolint:exhaustive // Other kinds hold no float32 or map.
	case reflect.Float32:
		if !DecodesItself(t) {
			msgpack.Register(reflect.Zero(t).Interface(), nil, decodeFloat32)
		}
	case reflect.Pointer, reflect.Slice, reflect.Array:
		registerTypes(t.Elem(), seen)
	case reflect.Map:
		// A map type with its own encoder keeps it, even one with a pointer receiver.
		if !EncodesItself(t) && !EncodesItself(reflect.PointerTo(t)) {
			msgpack.Register(reflect.Zero(t).Interface(), encodeSortedMap, nil)
		}
		registerTypes(t.Key(), seen)
		registerTypes(t.Elem(), seen)
	case reflect.Struct:
		for f := range t.Fields() {
			registerTypes(f.Type, seen)
		}
	}
}

// encodeSortedMap encodes a map with its entries ordered by their encoded keys, so equal maps encode
// to equal bytes. msgpack otherwise writes entries in Go's random map order, and its own
// SetSortMapKeys covers only map[string]string, map[string]bool and map[string]any.
func encodeSortedMap(e *msgpack.Encoder, v reflect.Value) error {
	if v.IsNil() {
		return e.EncodeNil()
	}
	type entry struct {
		key   msgpack.RawMessage
		value reflect.Value
	}
	entries := make([]entry, 0, v.Len())
	var buf bytes.Buffer
	keys := msgpack.NewEncoder(&buf)
	for iter := v.MapRange(); iter.Next(); {
		buf.Reset()
		if err := keys.EncodeValue(iter.Key()); err != nil {
			return err
		}
		entries = append(entries, entry{key: bytes.Clone(buf.Bytes()), value: iter.Value()})
	}
	slices.SortFunc(entries, func(a, b entry) int { return bytes.Compare(a.key, b.key) })
	if err := e.EncodeMapLen(len(entries)); err != nil {
		return err
	}
	for _, en := range entries {
		if err := e.Encode(en.key); err != nil {
			return err
		}
		if err := e.EncodeValue(en.value); err != nil {
			return err
		}
	}
	return nil
}

// DecodesItself reports whether msgpack would decode t through one of its own methods.
func DecodesItself(t reflect.Type) bool {
	p := reflect.PointerTo(t)
	return p.Implements(reflect.TypeFor[msgpack.CustomDecoder]()) ||
		p.Implements(reflect.TypeFor[msgpack.Unmarshaler]()) ||
		p.Implements(reflect.TypeFor[encoding.BinaryUnmarshaler]()) ||
		p.Implements(reflect.TypeFor[encoding.TextUnmarshaler]())
}

// EncodesItself reports whether msgpack would encode a t value through one of t's own methods.
func EncodesItself(t reflect.Type) bool {
	return t.Implements(reflect.TypeFor[msgpack.CustomEncoder]()) ||
		t.Implements(reflect.TypeFor[msgpack.Marshaler]()) ||
		t.Implements(reflect.TypeFor[encoding.BinaryMarshaler]()) ||
		t.Implements(reflect.TypeFor[encoding.TextMarshaler]())
}

// EncodesOnlyByPointer reports whether t's encoding method has a pointer receiver. msgpack can call
// it only on an addressable value, so encoding a copy of t fails.
func EncodesOnlyByPointer(t reflect.Type) bool {
	return t.Kind() != reflect.Pointer && !EncodesItself(t) && EncodesItself(reflect.PointerTo(t))
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

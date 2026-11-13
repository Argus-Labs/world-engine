package sync

import (
	"crypto/sha256"
	"encoding"
	"encoding/hex"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/goccy/go-json"
	"github.com/vmihailenco/msgpack/v5"
)

// Contract is the offline, exact wire schema consumed by Unity's contract generator.
// Version includes the receive-only, per-entity commit protocol and session audience policy.
type Contract struct {
	Version    int         `json:"version"`
	Components []Component `json:"components"`
}

type Component struct {
	Name   string  `json:"name"`
	Fields []Field `json:"fields"`
}

type Field struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Describe returns the sync contract of one registered component, or false for private types.
// The MVP intentionally accepts only exact scalar shapes; custom codecs require a new contract.
func Describe(t reflect.Type, name string) (Component, bool, error) {
	result := Component{Name: name, Fields: []Field{}}
	if !marked(t) {
		return result, false, nil
	}
	if name == "" || name == "sync" {
		return result, true, fmt.Errorf("invalid sync component name %q", name)
	}
	if customCodec(t) {
		return result, true, fmt.Errorf("sync component %s has a custom codec", name)
	}
	names := map[string]bool{}
	for f := range t.Fields() {
		fieldName, encoded, err := wireKey(name, f)
		if err != nil {
			return result, true, err
		}
		if !encoded {
			continue
		}
		if names[fieldName] {
			return result, true, fmt.Errorf("sync %s repeats field %s", name, fieldName)
		}
		names[fieldName] = true
		wireType := scalar(f.Type)
		if wireType == "" || customCodec(f.Type) {
			return result, true, fmt.Errorf("sync %s.%s: unsupported wire type %v", name, f.Name, f.Type)
		}
		result.Fields = append(result.Fields, Field{Name: fieldName, Type: wireType})
	}
	if len(result.Fields) == 0 {
		return result, true, fmt.Errorf("sync component %s has no visible scalar fields", name)
	}
	slices.SortFunc(result.Fields, func(a, b Field) int { return strings.Compare(a.Name, b.Name) })
	return result, true, nil
}

// CanonicalJSON is stable across registration and Go field order. MessagePack maps use field names.
func (c Contract) CanonicalJSON() ([]byte, error) {
	sorted := Contract{Version: c.Version, Components: slices.Clone(c.Components)}
	slices.SortFunc(sorted.Components, func(a, b Component) int { return strings.Compare(a.Name, b.Name) })
	for i := range sorted.Components {
		sorted.Components[i].Fields = slices.Clone(sorted.Components[i].Fields)
		slices.SortFunc(sorted.Components[i].Fields, func(a, b Field) int { return strings.Compare(a.Name, b.Name) })
	}
	return json.Marshal(sorted)
}

func (c Contract) Fingerprint() (string, error) {
	data, err := c.CanonicalJSON()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

// marked reports whether t is a struct that embeds the State marker.
func marked(t reflect.Type) bool {
	if t.Kind() != reflect.Struct {
		return false
	}
	for f := range t.Fields() {
		if f.Anonymous && f.Type == reflect.TypeFor[State]() {
			return true
		}
	}
	return false
}

// wireKey returns the MessagePack map key of one field of a sync component, or false when the
// field is not encoded: the State marker, an unexported field, or a field tagged "-".
func wireKey(component string, f reflect.StructField) (string, bool, error) {
	if f.Type == reflect.TypeFor[State]() && f.Anonymous {
		return "", false, nil
	}
	if !f.IsExported() {
		return "", false, nil
	}
	tag := strings.Split(f.Tag.Get("msgpack"), ",")
	if tag[0] == "-" {
		return "", false, nil
	}
	if f.Anonymous || len(tag) > 1 {
		return "", false, fmt.Errorf("sync %s.%s: embedded fields and MessagePack options are not supported",
			component, f.Name)
	}
	if tag[0] != "" {
		return tag[0], true, nil
	}
	return f.Name, true, nil
}

func scalar(t reflect.Type) string {
	switch t.Kind() { //nolint:exhaustive // Every other kind has no sync wire type.
	case reflect.Bool:
		return "bool"
	case reflect.String:
		return "string"
	case reflect.Int8:
		return "int8"
	case reflect.Int16:
		return "int16"
	case reflect.Int32:
		return "int32"
	case reflect.Int, reflect.Int64:
		return "int64"
	case reflect.Uint8:
		return "uint8"
	case reflect.Uint16:
		return "uint16"
	case reflect.Uint32:
		return "uint32"
	case reflect.Uint, reflect.Uint64:
		return "uint64"
	case reflect.Float32:
		return "float32"
	case reflect.Float64:
		return "float64"
	default:
		return ""
	}
}

func customCodec(t reflect.Type) bool {
	codecs := []reflect.Type{
		reflect.TypeFor[msgpack.CustomEncoder](), reflect.TypeFor[msgpack.Marshaler](),
		reflect.TypeFor[msgpack.CustomDecoder](), reflect.TypeFor[msgpack.Unmarshaler](),
		reflect.TypeFor[encoding.BinaryMarshaler](), reflect.TypeFor[encoding.BinaryUnmarshaler](),
		reflect.TypeFor[encoding.TextMarshaler](), reflect.TypeFor[encoding.TextUnmarshaler](),
	}
	for _, target := range []reflect.Type{t, reflect.PointerTo(t)} {
		if slices.ContainsFunc(codecs, target.Implements) {
			return true
		}
	}
	return false
}

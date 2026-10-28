package mcp

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"strings"

	"github.com/goccy/go-json"
	"github.com/rotisserie/eris"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/vmihailenco/msgpack/v5"
)

// encodeCommandPayload keeps JSON numbers as text until conversion to native integers.
// In particular, neither command integers nor their range checks pass through float64.
func encodeCommandPayload(schema map[string]any, payload string) ([]byte, error) {
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		return nil, eris.Wrap(err, "failed to parse command object")
	}
	if object == nil {
		return nil, eris.New("command payload must be a JSON object")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, eris.New("command payload must contain exactly one JSON object")
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("https://cardinal.local/command.json", schema); err != nil {
		return nil, eris.Wrap(err, "failed to load command schema")
	}
	compiled, err := compiler.Compile("https://cardinal.local/command.json")
	if err != nil {
		return nil, eris.Wrap(err, "failed to compile command schema")
	}
	if err := compiled.Validate(object); err != nil {
		return nil, eris.Wrap(err, "invalid command payload")
	}
	value, err := commandValue(object, schema, schema)
	if err != nil {
		return nil, eris.Wrap(err, "failed to build command payload")
	}
	blob, err := msgpack.Marshal(value)
	return blob, eris.Wrap(err, "failed to encode command MessagePack")
}

// commandValue also rejects unknown struct fields if an older schema omitted
// additionalProperties. Map schemas may explicitly advertise additional values.
func commandValue(value any, schema, root map[string]any) (any, error) {
	if branches, ok := schema["anyOf"].([]any); ok && value != nil {
		// Reflection uses anyOf for nullable pointer fields.
		for _, branch := range branches {
			candidate, _ := branch.(map[string]any)
			if candidate["type"] != "null" {
				return commandValue(value, candidate, root)
			}
		}
	}
	if ref, ok := schema["$ref"].(string); ok {
		var target any = root
		if !strings.HasPrefix(ref, "#/") {
			return nil, eris.Errorf("unsupported schema reference %q", ref)
		}
		for part := range strings.SplitSeq(strings.TrimPrefix(ref, "#/"), "/") {
			object, _ := target.(map[string]any)
			target = object[strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")]
		}
		resolved, ok := target.(map[string]any)
		if !ok {
			return nil, eris.Errorf("unknown schema reference %q", ref)
		}
		return commandValue(value, resolved, root)
	}
	switch v := value.(type) {
	case map[string]any:
		return commandObject(v, schema, root)
	case []any:
		items, _ := schema["items"].(map[string]any)
		for i, item := range v {
			converted, err := commandValue(item, items, root)
			if err != nil {
				return nil, eris.Wrapf(err, "array element %d", i)
			}
			v[i] = converted
		}
	case json.Number:
		return commandNumber(v, schema)
	case string:
		if schema["contentEncoding"] == "base64" {
			decoded, err := base64.StdEncoding.DecodeString(v)
			return decoded, eris.Wrap(err, "invalid base64 bytes")
		}
	}
	return value, nil
}

func commandObject(v, schema, root map[string]any) (any, error) {
	properties, _ := schema["properties"].(map[string]any)
	keyFormat, _ := schema["x-msgpack-key-format"].(string)
	keyed := make(map[any]any, len(v))
	for name, field := range v {
		child, ok := properties[name].(map[string]any)
		if !ok {
			child, ok = schema["additionalProperties"].(map[string]any)
			if !ok && schema["additionalProperties"] != true && len(schema) != 0 {
				return nil, eris.Errorf("unknown field %q", name)
			}
		}
		converted, err := commandValue(field, child, root)
		if err != nil {
			return nil, eris.Wrapf(err, "field %q", name)
		}
		v[name] = converted
		if keyFormat != "" {
			key, err := commandNumber(json.Number(name), map[string]any{"format": keyFormat})
			if err != nil {
				return nil, eris.Wrapf(err, "map key %q", name)
			}
			if _, exists := keyed[key]; exists {
				return nil, eris.Errorf("duplicate numeric map key %q", name)
			}
			keyed[key] = converted
		}
	}
	if keyFormat != "" {
		return keyed, nil
	}
	return v, nil
}

func commandNumber(v json.Number, schema map[string]any) (any, error) {
	if schema["type"] == "number" || (len(schema) == 0 && strings.ContainsAny(v.String(), ".eE")) {
		n, err := strconv.ParseFloat(v.String(), 64)
		return n, eris.Wrap(err, "invalid floating point number")
	}
	text, err := integerText(v.String())
	if err != nil {
		return nil, err
	}
	if format, ok := schema["format"].(string); ok {
		if bits, unsigned := integerFormat(format); bits != 0 {
			if unsigned {
				n, err := strconv.ParseUint(text, 10, bits)
				return n, eris.Wrapf(err, "invalid %s integer", format)
			}
			n, err := strconv.ParseInt(text, 10, bits)
			return n, eris.Wrapf(err, "invalid %s integer", format)
		}
	}
	if strings.HasPrefix(text, "-") {
		n, err := strconv.ParseInt(text, 10, 64)
		return n, eris.Wrap(err, "integer outside signed 64-bit range")
	}
	if n, err := strconv.ParseInt(text, 10, 64); err == nil {
		return n, nil
	}
	n, err := strconv.ParseUint(text, 10, 64)
	return n, eris.Wrap(err, "integer outside unsigned 64-bit range")
}

// integerText rewrites a whole number written with a fraction or exponent, such as 1.0 or 1e3, as
// plain digits. JSON Schema counts these as integers, so they pass validation and must encode. The
// parse is exact: anything that would round, such as 1.0000000000000000001, is rejected.
func integerText(s string) (string, error) {
	if !strings.ContainsAny(s, ".eE") {
		return s, nil
	}
	f, _, err := big.ParseFloat(s, 10, 256, big.ToZero)
	if err != nil || f.Acc() != big.Exact || !f.IsInt() {
		return "", eris.Errorf("%s is not an integer", s)
	}
	return f.Text('f', 0), nil
}

// integerFormat enforces native ranges without rounded JSON Schema bounds.
// structpb stores numeric schema bounds as doubles, so format is authoritative
// for limits such as MaxInt64 and MaxUint64.
func integerFormat(format string) (int, bool) {
	switch format {
	case "int8", "uint8":
		return 8, strings.HasPrefix(format, "u")
	case "int16", "uint16":
		return 16, strings.HasPrefix(format, "u")
	case "int32", "uint32":
		return 32, strings.HasPrefix(format, "u")
	case "int64", "uint64":
		return 64, strings.HasPrefix(format, "u")
	case "int", "uint":
		return strconv.IntSize, strings.HasPrefix(format, "u")
	default:
		return 0, false
	}
}

// decodeMessage preserves MessagePack integer types for state filter comparisons.
//
//nolint:nonamedreturns // Recovery must return an error and no partially decoded state.
func decodeMessage(blob []byte) (result map[string]any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = nil
			err = eris.Wrap(fmt.Errorf("panic: %v", recovered), "failed to decode component MessagePack")
		}
	}()
	reader := bytes.NewReader(blob)
	decoder := msgpack.NewDecoder(reader)
	// Nested MessagePack maps may have integer keys; render those as JSON object keys.
	decoder.SetMapDecoder(func(d *msgpack.Decoder) (any, error) {
		entries, err := d.DecodeUntypedMap()
		if err != nil {
			return nil, eris.Wrap(err, "failed to decode component map")
		}
		out := make(map[string]any, len(entries))
		for key, value := range entries {
			out[fmt.Sprint(key)] = value
		}
		return out, nil
	})
	var out map[string]any
	if err := decoder.Decode(&out); err != nil {
		return nil, eris.Wrap(err, "failed to decode component MessagePack")
	}
	if out == nil || reader.Len() != 0 {
		return nil, eris.New("component payload must contain one MessagePack object")
	}
	return out, nil
}

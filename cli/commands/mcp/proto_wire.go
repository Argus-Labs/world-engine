package mcp

import (
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// Commands are decoded by the shard as protobuf wire, but the MCP has no compiled descriptor for an
// arbitrary shard's commands. The shard's introspect response carries a serialized FileDescriptorSet
// (proto_descriptor_set) and each type's proto_message_name; this file resolves message descriptors
// from that set and encodes a JSON command payload against them via protojson — so the MCP never
// needs generated command code.

// resolveDescriptorFiles parses an introspect response's proto_descriptor_set into a resolvable
// descriptor registry.
func resolveDescriptorFiles(raw []byte) (*protoregistry.Files, error) {
	if len(raw) == 0 {
		return nil, eris.New(
			"shard returned no proto descriptor set — regenerate the shard SDK with 'world sdk generate' and reload",
		)
	}
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(raw, &set); err != nil {
		return nil, eris.Wrap(err, "failed to parse shard proto descriptor set")
	}
	files, err := protodesc.NewFiles(&set)
	if err != nil {
		return nil, eris.Wrap(err, "failed to build descriptors from shard proto descriptor set")
	}
	return files, nil
}

// findMessageDescriptor resolves a TypeSchema's proto_message_name inside the shard's descriptor set.
func findMessageDescriptor(files *protoregistry.Files, fullName string) (protoreflect.MessageDescriptor, error) {
	if fullName == "" {
		return nil, eris.New("type has no proto_message_name — regenerate the shard SDK with 'world sdk generate'")
	}
	desc, err := files.FindDescriptorByName(protoreflect.FullName(fullName))
	if err != nil {
		return nil, eris.Wrapf(err, "message %q not found in shard descriptor set", fullName)
	}
	md, ok := desc.(protoreflect.MessageDescriptor)
	if !ok {
		return nil, eris.Errorf("descriptor %q is not a message", fullName)
	}
	return md, nil
}

// encodeCommandPayload encodes a JSON command payload as protobuf wire bytes against the command's
// message descriptor. It populates a dynamic message from the payload via protojson (which rejects
// unknown fields, surfacing name mismatches) and marshals it.
//
// payload is a JSON-stringified object (a string holding a serialized JSON object), fed to protojson
// verbatim. Taking the raw JSON string instead of a map[string]any avoids the float64 coercion that
// mcp-go's transport applies when decoding Arguments: encoding/json turns every number in a
// map[string]any into float64, so any int64/uint64 above 2^53 would be silently rounded before
// reaching this function and could never be recovered. protojson itself decodes large integers
// exactly when handed the original JSON text.
func encodeCommandPayload(md protoreflect.MessageDescriptor, payload string) ([]byte, error) {
	msg := dynamicpb.NewMessage(md)
	if err := protojson.Unmarshal([]byte(payload), msg); err != nil {
		return nil, eris.Wrap(err, "failed to build command message from payload")
	}
	// Deterministic: a dynamicpb message holds its fields in a map, so a plain
	// proto.Marshal emits them in random order. Decoders don't care, but stable
	// bytes make the encoding testable and its logs diffable.
	return proto.MarshalOptions{Deterministic: true}.Marshal(msg)
}

// decodeMessage decodes protobuf wire bytes against a message descriptor into a plain Go map.
func decodeMessage(md protoreflect.MessageDescriptor, blob []byte) (map[string]any, error) {
	msg := dynamicpb.NewMessage(md)
	if err := proto.Unmarshal(blob, msg); err != nil {
		return nil, eris.Wrapf(err, "failed to decode %q", md.FullName())
	}
	return messageToMap(msg), nil
}

// messageToMap renders a decoded message as a map keyed by field name. Every field is present,
// zero values included, so a where clause never trips over a missing key. Values keep their native
// Go types — protojson would render 64-bit ints as strings, breaking numeric comparisons.
func messageToMap(msg protoreflect.Message) map[string]any {
	fields := msg.Descriptor().Fields()
	out := make(map[string]any, fields.Len())
	for i := range fields.Len() {
		fd := fields.Get(i)
		// An absent singular message field has nothing to report, and recursing into the empty
		// message a Get would hand back never terminates for a self-referential type.
		if fd.Message() != nil && !fd.IsList() && !fd.IsMap() && !msg.Has(fd) {
			out[string(fd.Name())] = nil
			continue
		}
		out[string(fd.Name())] = fieldValue(fd, msg.Get(fd))
	}
	return out
}

// fieldValue converts one field's value, spreading lists and maps over their elements.
func fieldValue(fd protoreflect.FieldDescriptor, v protoreflect.Value) any {
	switch {
	case fd.IsMap():
		entries := v.Map()
		out := make(map[string]any, entries.Len())
		entries.Range(func(k protoreflect.MapKey, val protoreflect.Value) bool {
			// JSON object keys are strings, so non-string proto keys render as their literal.
			out[k.String()] = scalarValue(fd.MapValue(), val)
			return true
		})
		return out
	case fd.IsList():
		list := v.List()
		out := make([]any, 0, list.Len())
		for i := range list.Len() {
			out = append(out, scalarValue(fd, list.Get(i)))
		}
		return out
	default:
		return scalarValue(fd, v)
	}
}

// scalarValue converts a single (non-repeated) protobuf value to its Go equivalent. Enums render as
// their name, which is what a where clause can usefully compare against.
func scalarValue(fd protoreflect.FieldDescriptor, v protoreflect.Value) any {
	switch fd.Kind() { //nolint:exhaustive // every scalar kind is handled by the default
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return messageToMap(v.Message())
	case protoreflect.EnumKind:
		if value := fd.Enum().Values().ByNumber(v.Enum()); value != nil {
			return string(value.Name())
		}
		return int32(v.Enum())
	default: // bool, the integer and float kinds, string, bytes
		return v.Interface()
	}
}

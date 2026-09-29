package sdkgen_test

import (
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/cli/internal/sdkgen"
)

// TestRenderGoWire_RefusesUnencodableShapes pins the three shapes ToProto/FromProto carry that
// SizeWire/AppendWire do not: a map, an optional scalar (*T) and a pointer to a message. Each is a
// blocking violation at discovery, so `world sdk generate` never reaches emission holding one — but
// RenderGoWire is exported, and MarshalWire is now the direct encoders rather than proto.Marshal, so
// emitting one would silently drop the field from the bytes. The refusal names the field.
func TestRenderGoWire_RefusesUnencodableShapes(t *testing.T) {
	t.Parallel()

	strKey := &sdkgen.Scalar{Family: "string", GoType: "string", Proto: "string"}
	i32 := &sdkgen.Scalar{Family: "varint", GoType: "int32", Proto: "int32"}

	for _, tc := range []struct {
		name  string
		field sdkgen.Field
		want  string
	}{
		{"map", sdkgen.Field{Name: "Scores", Number: 1, Kind: "map", Key: strKey, ValSc: i32}, "a map"},
		{"optional scalar", sdkgen.Field{Name: "Lives", Number: 1, Kind: "scalar", Scalar: i32, Optional: true}, "an optional scalar (*T)"},
		{"pointer message", sdkgen.Field{Name: "DeletedAt", Number: 1, Kind: "message", Timestamp: true, Pointer: true}, "a pointer to a message (*T)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			msgs := []sdkgen.Message{{
				Name:      "UpdateCommand",
				Wire:      "update",
				Kind:      sdkgen.KindCommand,
				PkgName:   "system",
				PkgPath:   "example.com/mygame/system",
				GenImport: "example.com/mygame/gen/system",
				Fields:    []sdkgen.Field{tc.field},
			}}
			_, err := sdkgen.RenderGoWire(msgs, nil)
			if err == nil {
				t.Fatalf("RenderGoWire accepted %s; want a refusal", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("refusal should name the shape %q, got: %v", tc.want, err)
			}
			if !strings.Contains(err.Error(), "UpdateCommand."+tc.field.Name) {
				t.Errorf("refusal should name the field, got: %v", err)
			}
		})
	}
}

// TestRenderGoWire_Timestamp verifies a time.Time field generates timestamppb conversions on both
// sides and pulls the timestamppb + time imports. The pointer and map-value shapes are refused
// outright (see TestRenderGoWire_RefusesUnencodableShapes), so only the value shape renders.
func TestRenderGoWire_Timestamp(t *testing.T) {
	t.Parallel()
	const cardinalImp = "github.com/argus-labs/world-engine/pkg/cardinal"
	msgs := []sdkgen.Message{{
		Name:      "AuditCommand",
		Wire:      "audit",
		Kind:      sdkgen.KindCommand,
		PkgName:   "system",
		PkgPath:   "example.com/mygame/system",
		GenImport: "example.com/mygame/gen/system",
		Fields: []sdkgen.Field{
			{Name: "CreatedAt", Number: 1, Kind: "message", Timestamp: true},
		},
	}}
	out, err := sdkgen.RenderGoWire(msgs, nil)
	if err != nil {
		t.Fatalf("RenderGoWire failed: %v", err)
	}
	for _, want := range []string{
		`"google.golang.org/protobuf/types/known/timestamppb"`,
		`"time"`,
		"p.CreatedAt = timestamppb.New(c.CreatedAt)",
		"c.CreatedAt = p.CreatedAt.AsTime()",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("timestamp wire missing %q:\n%s", want, out)
		}
	}
}

// TestRenderGoWire_ComponentCodec verifies a component emits a symmetric, registry-free snapshot codec:
// MarshalWire (proto bytes) and UnmarshalWire (proto bytes → a fresh concrete value via FromProto).
func TestRenderGoWire_ComponentCodec(t *testing.T) {
	t.Parallel()
	const cardinalImp = "github.com/argus-labs/world-engine/pkg/cardinal"
	sc := &sdkgen.Scalar{Family: "varint", GoType: "int32", Proto: "int32"}
	msgs := []sdkgen.Message{{
		Name:      "Position",
		Kind:      sdkgen.KindComponent,
		PkgName:   "component",
		PkgPath:   "example.com/mygame/component",
		GenImport: "example.com/mygame/gen/component",
		Fields: []sdkgen.Field{
			{Name: "X", Number: 1, Kind: "scalar", Scalar: sc},
			{Name: "Y", Number: 2, Kind: "scalar", Scalar: sc},
		},
	}}
	out, err := sdkgen.RenderGoWire(msgs, nil)
	if err != nil {
		t.Fatalf("RenderGoWire failed: %v", err)
	}
	for _, want := range []string{
		"func (c Position) MarshalWire() []byte {",
		// Derived from the direct encoders, not a second encoder (see TestRenderGoWireDirect_MarshalWireIsDerived).
		"return c.AppendWire(make([]byte, 0, c.SizeWire()))",
		"func (c Position) UnmarshalWire(data []byte) (any, error) {",
		"proto.Unmarshal(data, &p)",
		"return c.FromProto(&p), nil",
		"func (c Position) ProtoDescriptor() protoreflect.MessageDescriptor {",
		"return (&pbcomponent.Position{}).ProtoReflect().Descriptor()",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("component codec missing %q:\n%s", want, out)
		}
	}
	// Keep the generated API registry-free and descriptor-only.
	for _, unwanted := range []string{"RegisterCommandCodec", "PositionCodec", "FormSchema"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("component wiring contains %q:\n%s", unwanted, out)
		}
	}
}

// TestRenderGoWire_EventMarshalNoRegistry verifies an event emits a type-dispatched MarshalWire method
// and NO codec/registry: no Codec struct, no RegisterCommandCodec, no cardinal import. proto is still
// imported for proto.Marshal. This is the registry-free event wiring (dispatch by type, not by name).
func TestRenderGoWire_EventMarshalNoRegistry(t *testing.T) {
	t.Parallel()

	const cardinalImp = "github.com/argus-labs/world-engine/pkg/cardinal"

	msgs := []sdkgen.Message{{
		Name:      "PlayerDied",
		Wire:      "player_died",
		Kind:      sdkgen.KindEvent,
		PkgName:   "events",
		PkgPath:   "example.com/mygame/events",
		GenImport: "example.com/mygame/gen/events",
		Fields: []sdkgen.Field{{
			Name:   "PlayerID",
			Number: 1,
			Kind:   "scalar",
			Scalar: &sdkgen.Scalar{Family: "varint", GoType: "uint64", Proto: "uint64"},
		}},
	}}

	out, err := sdkgen.RenderGoWire(msgs, nil)
	if err != nil {
		t.Fatalf("RenderGoWire failed: %v", err)
	}
	if !strings.Contains(out, "func (c PlayerDied) MarshalWire() []byte {") {
		t.Errorf("event should emit a MarshalWire method:\n%s", out)
	}
	if !strings.Contains(out, "return c.AppendWire(make([]byte, 0, c.SizeWire()))") {
		t.Errorf("MarshalWire should be derived from the direct encoders:\n%s", out)
	}
	for _, unwanted := range []string{"RegisterCommandCodec", "RegisterEventCodec", "PlayerDiedCodec", cardinalImp} {
		if strings.Contains(out, unwanted) {
			t.Errorf("event wiring must be registry-free, but output contains %q:\n%s", unwanted, out)
		}
	}
}

// TestConvFuncKeepsUnderscoreRunsDistinct pins that a run of underscores in a package alias survives
// into the mirrored converter's name. AliasFromPath builds the alias with sanitizeToIdent, which maps
// every non-identifier rune to '_' — so "b--c" yields "b__c" while "b_c" yields "b_c", and the PascalCase
// pass used to drop empty segments and collapse both onto one name. Two mirrored types would then declare
// one function; checkNoRedeclaration refuses that, but only after the fact. The single-underscore
// spelling is every name generated today and must be untouched.
func TestConvFuncKeepsUnderscoreRunsDistinct(t *testing.T) {
	t.Parallel()

	single := sdkgen.TypeRef{PkgPath: "x/a/b_c", Name: "T"}.ConvFunc("ToProto")
	double := sdkgen.TypeRef{PkgPath: "x/a/b--c", Name: "T"}.ConvFunc("ToProto")
	if single == double {
		t.Errorf("aliases a_b_c and a_b__c must not collide, both gave %q", single)
	}
	if want := "mirrorABCTToProto"; single != want {
		t.Errorf("existing names must not change: want %q, got %q", want, single)
	}
}

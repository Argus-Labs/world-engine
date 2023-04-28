package component

import (
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type (
	TypeID int

	// IComponentType is a high level representation of a user defined component.
	IComponentType proto.Message
)

// ID returns the ID of the component; the proto message's fully qualified name.
func ID(ic IComponentType) string {
	if anyComp, ok := ic.(*anypb.Any); ok {
		return idFromAny(anyComp)
	}
	return string(ic.ProtoReflect().Descriptor().FullName())
}

// idFromAny extracts the underlying proto message name.
// when creating an "any" proto message, the typeURL is prefixed by
// "types.google.com/". Everything after the slash
// is the underlying full name of the underlying message.
func idFromAny(a *anypb.Any) string {
	_, after, _ := strings.Cut(a.TypeUrl, "/")
	return after
}

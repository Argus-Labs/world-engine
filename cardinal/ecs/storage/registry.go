package storage

import (
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
)

var _ protoregistry.MessageTypeResolver = &typeRegistry{}

type TypeRegistry interface {
	Register(...component.IComponentType)
	FindMessageByName(name protoreflect.FullName) (protoreflect.MessageType, error)
	FindMessageByURL(url string) (protoreflect.MessageType, error)
	FindExtensionByNumber(message protoreflect.FullName, field protoreflect.FieldNumber) (protoreflect.ExtensionType, error)
	FindExtensionByName(field protoreflect.FullName) (protoreflect.ExtensionType, error)
}

func NewTypeRegistry() TypeRegistry {
	return &typeRegistry{map[protoreflect.FullName]proto.Message{}}
}

// typeRegistry is a type that helps
type typeRegistry struct {
	reg map[protoreflect.FullName]proto.Message
}

func (t *typeRegistry) Register(msgs ...component.IComponentType) {
	for _, msg := range msgs {
		t.reg[msg.ProtoReflect().Descriptor().FullName()] = msg
	}
}

func (t *typeRegistry) FindMessageByName(name protoreflect.FullName) (protoreflect.MessageType, error) {
	msg, ok := t.reg[name]
	if !ok {
		return nil, protoregistry.NotFound
	}
	return msg.ProtoReflect().Type(), nil
}

func (t *typeRegistry) FindMessageByURL(url string) (protoreflect.MessageType, error) {
	// typeURLs in "any" are prefixed by types.google.any/, so we need to cut that part out.
	// when doing proto.Message().FullName() you get everything in an any _after_ the "/".
	_, name, _ := strings.Cut(url, "/")
	protoName := protoreflect.FullName(name)
	msg, ok := t.reg[protoName]
	if !ok {
		return nil, protoregistry.NotFound
	}
	return msg.ProtoReflect().Type(), nil
}

// FindExtensionByName -- we don't actually need/use this, but it's required by the unmarshal options.
func (t *typeRegistry) FindExtensionByName(field protoreflect.FullName) (protoreflect.ExtensionType, error) {
	return nil, nil
}

// FindExtensionByNumber -- we don't actually need/use this, but it's required by the unmarshal options.
func (t *typeRegistry) FindExtensionByNumber(message protoreflect.FullName, field protoreflect.FieldNumber) (protoreflect.ExtensionType, error) {
	return nil, nil
}

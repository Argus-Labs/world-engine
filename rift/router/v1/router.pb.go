// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.28.1
// 	protoc        (unknown)
// source: router/v1/router.proto

package routerv1

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type SendMessageRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// sender is the identifier of the message sender.
	Sender string `protobuf:"bytes,1,opt,name=sender,proto3" json:"sender,omitempty"`
	// message contains the underlying bytes of the message. typically, this is an abi encoded solidity struct.
	Message []byte `protobuf:"bytes,2,opt,name=message,proto3" json:"message,omitempty"`
	// message_id is the id of the message. this is needed to indicate to the server which concrete type to deserialize
	// the message bytes into.
	MessageId uint64 `protobuf:"varint,3,opt,name=message_id,json=messageId,proto3" json:"message_id,omitempty"`
	// evm_tx_hash is the tx hash of the evm transaction that triggered the request.
	EvmTxHash string `protobuf:"bytes,4,opt,name=evm_tx_hash,json=evmTxHash,proto3" json:"evm_tx_hash,omitempty"`
}

func (x *SendMessageRequest) Reset() {
	*x = SendMessageRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_router_v1_router_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SendMessageRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SendMessageRequest) ProtoMessage() {}

func (x *SendMessageRequest) ProtoReflect() protoreflect.Message {
	mi := &file_router_v1_router_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SendMessageRequest.ProtoReflect.Descriptor instead.
func (*SendMessageRequest) Descriptor() ([]byte, []int) {
	return file_router_v1_router_proto_rawDescGZIP(), []int{0}
}

func (x *SendMessageRequest) GetSender() string {
	if x != nil {
		return x.Sender
	}
	return ""
}

func (x *SendMessageRequest) GetMessage() []byte {
	if x != nil {
		return x.Message
	}
	return nil
}

func (x *SendMessageRequest) GetMessageId() uint64 {
	if x != nil {
		return x.MessageId
	}
	return 0
}

func (x *SendMessageRequest) GetEvmTxHash() string {
	if x != nil {
		return x.EvmTxHash
	}
	return ""
}

type SendMessageResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// errs contain any errors that occurred during the transaction execution.
	Errs string `protobuf:"bytes,1,opt,name=errs,proto3" json:"errs,omitempty"`
	// result is an ABI encoded struct of the transaction type's result.
	Result []byte `protobuf:"bytes,2,opt,name=result,proto3" json:"result,omitempty"`
	// evm_tx_hash is the tx hash of the evm transaction that triggered the request.
	EvmTxHash string `protobuf:"bytes,3,opt,name=evm_tx_hash,json=evmTxHash,proto3" json:"evm_tx_hash,omitempty"`
	// code is an arbitrary code that represents the result of the message execution. Refer to game shard documentation
	// for code definitions.
	Code uint32 `protobuf:"varint,4,opt,name=code,proto3" json:"code,omitempty"`
}

func (x *SendMessageResponse) Reset() {
	*x = SendMessageResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_router_v1_router_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SendMessageResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SendMessageResponse) ProtoMessage() {}

func (x *SendMessageResponse) ProtoReflect() protoreflect.Message {
	mi := &file_router_v1_router_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SendMessageResponse.ProtoReflect.Descriptor instead.
func (*SendMessageResponse) Descriptor() ([]byte, []int) {
	return file_router_v1_router_proto_rawDescGZIP(), []int{1}
}

func (x *SendMessageResponse) GetErrs() string {
	if x != nil {
		return x.Errs
	}
	return ""
}

func (x *SendMessageResponse) GetResult() []byte {
	if x != nil {
		return x.Result
	}
	return nil
}

func (x *SendMessageResponse) GetEvmTxHash() string {
	if x != nil {
		return x.EvmTxHash
	}
	return ""
}

func (x *SendMessageResponse) GetCode() uint32 {
	if x != nil {
		return x.Code
	}
	return 0
}

type QueryShardRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// resource is the resource to query for.
	Resource string `protobuf:"bytes,1,opt,name=resource,proto3" json:"resource,omitempty"`
	// request is an ABI encoded request struct.
	Request []byte `protobuf:"bytes,2,opt,name=request,proto3" json:"request,omitempty"`
}

func (x *QueryShardRequest) Reset() {
	*x = QueryShardRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_router_v1_router_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *QueryShardRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*QueryShardRequest) ProtoMessage() {}

func (x *QueryShardRequest) ProtoReflect() protoreflect.Message {
	mi := &file_router_v1_router_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use QueryShardRequest.ProtoReflect.Descriptor instead.
func (*QueryShardRequest) Descriptor() ([]byte, []int) {
	return file_router_v1_router_proto_rawDescGZIP(), []int{2}
}

func (x *QueryShardRequest) GetResource() string {
	if x != nil {
		return x.Resource
	}
	return ""
}

func (x *QueryShardRequest) GetRequest() []byte {
	if x != nil {
		return x.Request
	}
	return nil
}

type QueryShardResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// response is an ABI encoded response struct.
	Response []byte `protobuf:"bytes,1,opt,name=response,proto3" json:"response,omitempty"`
}

func (x *QueryShardResponse) Reset() {
	*x = QueryShardResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_router_v1_router_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *QueryShardResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*QueryShardResponse) ProtoMessage() {}

func (x *QueryShardResponse) ProtoReflect() protoreflect.Message {
	mi := &file_router_v1_router_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use QueryShardResponse.ProtoReflect.Descriptor instead.
func (*QueryShardResponse) Descriptor() ([]byte, []int) {
	return file_router_v1_router_proto_rawDescGZIP(), []int{3}
}

func (x *QueryShardResponse) GetResponse() []byte {
	if x != nil {
		return x.Response
	}
	return nil
}

var File_router_v1_router_proto protoreflect.FileDescriptor

var file_router_v1_router_proto_rawDesc = []byte{
	0x0a, 0x16, 0x72, 0x6f, 0x75, 0x74, 0x65, 0x72, 0x2f, 0x76, 0x31, 0x2f, 0x72, 0x6f, 0x75, 0x74,
	0x65, 0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x16, 0x77, 0x6f, 0x72, 0x6c, 0x64, 0x2e,
	0x65, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x2e, 0x72, 0x6f, 0x75, 0x74, 0x65, 0x72, 0x2e, 0x76, 0x31,
	0x22, 0x85, 0x01, 0x0a, 0x12, 0x53, 0x65, 0x6e, 0x64, 0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65,
	0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x16, 0x0a, 0x06, 0x73, 0x65, 0x6e, 0x64, 0x65,
	0x72, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x73, 0x65, 0x6e, 0x64, 0x65, 0x72, 0x12,
	0x18, 0x0a, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c,
	0x52, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x12, 0x1d, 0x0a, 0x0a, 0x6d, 0x65, 0x73,
	0x73, 0x61, 0x67, 0x65, 0x5f, 0x69, 0x64, 0x18, 0x03, 0x20, 0x01, 0x28, 0x04, 0x52, 0x09, 0x6d,
	0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x49, 0x64, 0x12, 0x1e, 0x0a, 0x0b, 0x65, 0x76, 0x6d, 0x5f,
	0x74, 0x78, 0x5f, 0x68, 0x61, 0x73, 0x68, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x65,
	0x76, 0x6d, 0x54, 0x78, 0x48, 0x61, 0x73, 0x68, 0x22, 0x75, 0x0a, 0x13, 0x53, 0x65, 0x6e, 0x64,
	0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12,
	0x12, 0x0a, 0x04, 0x65, 0x72, 0x72, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x65,
	0x72, 0x72, 0x73, 0x12, 0x16, 0x0a, 0x06, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x18, 0x02, 0x20,
	0x01, 0x28, 0x0c, 0x52, 0x06, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x12, 0x1e, 0x0a, 0x0b, 0x65,
	0x76, 0x6d, 0x5f, 0x74, 0x78, 0x5f, 0x68, 0x61, 0x73, 0x68, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x09, 0x65, 0x76, 0x6d, 0x54, 0x78, 0x48, 0x61, 0x73, 0x68, 0x12, 0x12, 0x0a, 0x04, 0x63,
	0x6f, 0x64, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x04, 0x63, 0x6f, 0x64, 0x65, 0x22,
	0x49, 0x0a, 0x11, 0x51, 0x75, 0x65, 0x72, 0x79, 0x53, 0x68, 0x61, 0x72, 0x64, 0x52, 0x65, 0x71,
	0x75, 0x65, 0x73, 0x74, 0x12, 0x1a, 0x0a, 0x08, 0x72, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x72, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65,
	0x12, 0x18, 0x0a, 0x07, 0x72, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x0c, 0x52, 0x07, 0x72, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x22, 0x30, 0x0a, 0x12, 0x51, 0x75,
	0x65, 0x72, 0x79, 0x53, 0x68, 0x61, 0x72, 0x64, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x12, 0x1a, 0x0a, 0x08, 0x72, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x0c, 0x52, 0x08, 0x72, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x32, 0xd2, 0x01, 0x0a,
	0x03, 0x4d, 0x73, 0x67, 0x12, 0x66, 0x0a, 0x0b, 0x53, 0x65, 0x6e, 0x64, 0x4d, 0x65, 0x73, 0x73,
	0x61, 0x67, 0x65, 0x12, 0x2a, 0x2e, 0x77, 0x6f, 0x72, 0x6c, 0x64, 0x2e, 0x65, 0x6e, 0x67, 0x69,
	0x6e, 0x65, 0x2e, 0x72, 0x6f, 0x75, 0x74, 0x65, 0x72, 0x2e, 0x76, 0x31, 0x2e, 0x53, 0x65, 0x6e,
	0x64, 0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a,
	0x2b, 0x2e, 0x77, 0x6f, 0x72, 0x6c, 0x64, 0x2e, 0x65, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x2e, 0x72,
	0x6f, 0x75, 0x74, 0x65, 0x72, 0x2e, 0x76, 0x31, 0x2e, 0x53, 0x65, 0x6e, 0x64, 0x4d, 0x65, 0x73,
	0x73, 0x61, 0x67, 0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x63, 0x0a, 0x0a,
	0x51, 0x75, 0x65, 0x72, 0x79, 0x53, 0x68, 0x61, 0x72, 0x64, 0x12, 0x29, 0x2e, 0x77, 0x6f, 0x72,
	0x6c, 0x64, 0x2e, 0x65, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x2e, 0x72, 0x6f, 0x75, 0x74, 0x65, 0x72,
	0x2e, 0x76, 0x31, 0x2e, 0x51, 0x75, 0x65, 0x72, 0x79, 0x53, 0x68, 0x61, 0x72, 0x64, 0x52, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x2a, 0x2e, 0x77, 0x6f, 0x72, 0x6c, 0x64, 0x2e, 0x65, 0x6e,
	0x67, 0x69, 0x6e, 0x65, 0x2e, 0x72, 0x6f, 0x75, 0x74, 0x65, 0x72, 0x2e, 0x76, 0x31, 0x2e, 0x51,
	0x75, 0x65, 0x72, 0x79, 0x53, 0x68, 0x61, 0x72, 0x64, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73,
	0x65, 0x42, 0xbd, 0x01, 0x0a, 0x1a, 0x63, 0x6f, 0x6d, 0x2e, 0x77, 0x6f, 0x72, 0x6c, 0x64, 0x2e,
	0x65, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x2e, 0x72, 0x6f, 0x75, 0x74, 0x65, 0x72, 0x2e, 0x76, 0x31,
	0x42, 0x0b, 0x52, 0x6f, 0x75, 0x74, 0x65, 0x72, 0x50, 0x72, 0x6f, 0x74, 0x6f, 0x50, 0x01, 0x5a,
	0x17, 0x72, 0x69, 0x66, 0x74, 0x2f, 0x72, 0x6f, 0x75, 0x74, 0x65, 0x72, 0x2f, 0x76, 0x31, 0x3b,
	0x72, 0x6f, 0x75, 0x74, 0x65, 0x72, 0x76, 0x31, 0xa2, 0x02, 0x03, 0x57, 0x45, 0x52, 0xaa, 0x02,
	0x16, 0x57, 0x6f, 0x72, 0x6c, 0x64, 0x2e, 0x45, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x2e, 0x52, 0x6f,
	0x75, 0x74, 0x65, 0x72, 0x2e, 0x56, 0x31, 0xca, 0x02, 0x16, 0x57, 0x6f, 0x72, 0x6c, 0x64, 0x5c,
	0x45, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x5c, 0x52, 0x6f, 0x75, 0x74, 0x65, 0x72, 0x5c, 0x56, 0x31,
	0xe2, 0x02, 0x22, 0x57, 0x6f, 0x72, 0x6c, 0x64, 0x5c, 0x45, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x5c,
	0x52, 0x6f, 0x75, 0x74, 0x65, 0x72, 0x5c, 0x56, 0x31, 0x5c, 0x47, 0x50, 0x42, 0x4d, 0x65, 0x74,
	0x61, 0x64, 0x61, 0x74, 0x61, 0xea, 0x02, 0x19, 0x57, 0x6f, 0x72, 0x6c, 0x64, 0x3a, 0x3a, 0x45,
	0x6e, 0x67, 0x69, 0x6e, 0x65, 0x3a, 0x3a, 0x52, 0x6f, 0x75, 0x74, 0x65, 0x72, 0x3a, 0x3a, 0x56,
	0x31, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_router_v1_router_proto_rawDescOnce sync.Once
	file_router_v1_router_proto_rawDescData = file_router_v1_router_proto_rawDesc
)

func file_router_v1_router_proto_rawDescGZIP() []byte {
	file_router_v1_router_proto_rawDescOnce.Do(func() {
		file_router_v1_router_proto_rawDescData = protoimpl.X.CompressGZIP(file_router_v1_router_proto_rawDescData)
	})
	return file_router_v1_router_proto_rawDescData
}

var file_router_v1_router_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_router_v1_router_proto_goTypes = []interface{}{
	(*SendMessageRequest)(nil),  // 0: world.engine.router.v1.SendMessageRequest
	(*SendMessageResponse)(nil), // 1: world.engine.router.v1.SendMessageResponse
	(*QueryShardRequest)(nil),   // 2: world.engine.router.v1.QueryShardRequest
	(*QueryShardResponse)(nil),  // 3: world.engine.router.v1.QueryShardResponse
}
var file_router_v1_router_proto_depIdxs = []int32{
	0, // 0: world.engine.router.v1.Msg.SendMessage:input_type -> world.engine.router.v1.SendMessageRequest
	2, // 1: world.engine.router.v1.Msg.QueryShard:input_type -> world.engine.router.v1.QueryShardRequest
	1, // 2: world.engine.router.v1.Msg.SendMessage:output_type -> world.engine.router.v1.SendMessageResponse
	3, // 3: world.engine.router.v1.Msg.QueryShard:output_type -> world.engine.router.v1.QueryShardResponse
	2, // [2:4] is the sub-list for method output_type
	0, // [0:2] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_router_v1_router_proto_init() }
func file_router_v1_router_proto_init() {
	if File_router_v1_router_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_router_v1_router_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SendMessageRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_router_v1_router_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SendMessageResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_router_v1_router_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*QueryShardRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_router_v1_router_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*QueryShardResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_router_v1_router_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   4,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_router_v1_router_proto_goTypes,
		DependencyIndexes: file_router_v1_router_proto_depIdxs,
		MessageInfos:      file_router_v1_router_proto_msgTypes,
	}.Build()
	File_router_v1_router_proto = out.File
	file_router_v1_router_proto_rawDesc = nil
	file_router_v1_router_proto_goTypes = nil
	file_router_v1_router_proto_depIdxs = nil
}

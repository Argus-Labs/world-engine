// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.28.1
// 	protoc        (unknown)
// source: shard/v2/shard.proto

package shardv2

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

type RegisterGameShardRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// namespace is the namespace of the game shard.
	Namespace string `protobuf:"bytes,1,opt,name=namespace,proto3" json:"namespace,omitempty"`
	// router_address is the address of the game shard's router service.
	RouterAddress string `protobuf:"bytes,2,opt,name=router_address,json=routerAddress,proto3" json:"router_address,omitempty"`
}

func (x *RegisterGameShardRequest) Reset() {
	*x = RegisterGameShardRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_shard_v2_shard_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *RegisterGameShardRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*RegisterGameShardRequest) ProtoMessage() {}

func (x *RegisterGameShardRequest) ProtoReflect() protoreflect.Message {
	mi := &file_shard_v2_shard_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use RegisterGameShardRequest.ProtoReflect.Descriptor instead.
func (*RegisterGameShardRequest) Descriptor() ([]byte, []int) {
	return file_shard_v2_shard_proto_rawDescGZIP(), []int{0}
}

func (x *RegisterGameShardRequest) GetNamespace() string {
	if x != nil {
		return x.Namespace
	}
	return ""
}

func (x *RegisterGameShardRequest) GetRouterAddress() string {
	if x != nil {
		return x.RouterAddress
	}
	return ""
}

type RegisterGameShardResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *RegisterGameShardResponse) Reset() {
	*x = RegisterGameShardResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_shard_v2_shard_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *RegisterGameShardResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*RegisterGameShardResponse) ProtoMessage() {}

func (x *RegisterGameShardResponse) ProtoReflect() protoreflect.Message {
	mi := &file_shard_v2_shard_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use RegisterGameShardResponse.ProtoReflect.Descriptor instead.
func (*RegisterGameShardResponse) Descriptor() ([]byte, []int) {
	return file_shard_v2_shard_proto_rawDescGZIP(), []int{1}
}

type SubmitTransactionsRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// epoch is the period in which the transactions occurred. For loop driven runtimes, such as cardinal,
	// this is often referred to as "tick number".
	Epoch         uint64 `protobuf:"varint,1,opt,name=epoch,proto3" json:"epoch,omitempty"`
	UnixTimestamp uint64 `protobuf:"varint,2,opt,name=unix_timestamp,json=unixTimestamp,proto3" json:"unix_timestamp,omitempty"`
	// namespace is the namespace of the game shard in which the transactions were executed in.
	Namespace string `protobuf:"bytes,3,opt,name=namespace,proto3" json:"namespace,omitempty"`
	// transactions is a mapping of game shard transaction ID's to the transactions themselves.
	//
	//	NOTE: if this message is being consumed via Golang, the transaction mapping MUST be converted to a
	//
	// slice with the transaction ID's sorted. Maps in Golang are NOT deterministic.
	Transactions map[uint64]*Transactions `protobuf:"bytes,4,rep,name=transactions,proto3" json:"transactions,omitempty" protobuf_key:"varint,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
}

func (x *SubmitTransactionsRequest) Reset() {
	*x = SubmitTransactionsRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_shard_v2_shard_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SubmitTransactionsRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SubmitTransactionsRequest) ProtoMessage() {}

func (x *SubmitTransactionsRequest) ProtoReflect() protoreflect.Message {
	mi := &file_shard_v2_shard_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SubmitTransactionsRequest.ProtoReflect.Descriptor instead.
func (*SubmitTransactionsRequest) Descriptor() ([]byte, []int) {
	return file_shard_v2_shard_proto_rawDescGZIP(), []int{2}
}

func (x *SubmitTransactionsRequest) GetEpoch() uint64 {
	if x != nil {
		return x.Epoch
	}
	return 0
}

func (x *SubmitTransactionsRequest) GetUnixTimestamp() uint64 {
	if x != nil {
		return x.UnixTimestamp
	}
	return 0
}

func (x *SubmitTransactionsRequest) GetNamespace() string {
	if x != nil {
		return x.Namespace
	}
	return ""
}

func (x *SubmitTransactionsRequest) GetTransactions() map[uint64]*Transactions {
	if x != nil {
		return x.Transactions
	}
	return nil
}

type SubmitTransactionsResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *SubmitTransactionsResponse) Reset() {
	*x = SubmitTransactionsResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_shard_v2_shard_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SubmitTransactionsResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SubmitTransactionsResponse) ProtoMessage() {}

func (x *SubmitTransactionsResponse) ProtoReflect() protoreflect.Message {
	mi := &file_shard_v2_shard_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SubmitTransactionsResponse.ProtoReflect.Descriptor instead.
func (*SubmitTransactionsResponse) Descriptor() ([]byte, []int) {
	return file_shard_v2_shard_proto_rawDescGZIP(), []int{3}
}

type Transactions struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Txs []*Transaction `protobuf:"bytes,1,rep,name=txs,proto3" json:"txs,omitempty"`
}

func (x *Transactions) Reset() {
	*x = Transactions{}
	if protoimpl.UnsafeEnabled {
		mi := &file_shard_v2_shard_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Transactions) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Transactions) ProtoMessage() {}

func (x *Transactions) ProtoReflect() protoreflect.Message {
	mi := &file_shard_v2_shard_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Transactions.ProtoReflect.Descriptor instead.
func (*Transactions) Descriptor() ([]byte, []int) {
	return file_shard_v2_shard_proto_rawDescGZIP(), []int{4}
}

func (x *Transactions) GetTxs() []*Transaction {
	if x != nil {
		return x.Txs
	}
	return nil
}

type Transaction struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	PersonaTag string `protobuf:"bytes,1,opt,name=PersonaTag,proto3" json:"PersonaTag,omitempty"`
	Namespace  string `protobuf:"bytes,2,opt,name=Namespace,proto3" json:"Namespace,omitempty"`
	Nonce      uint64 `protobuf:"varint,3,opt,name=Nonce,proto3" json:"Nonce,omitempty"`
	Signature  string `protobuf:"bytes,4,opt,name=Signature,proto3" json:"Signature,omitempty"`
	Body       []byte `protobuf:"bytes,5,opt,name=Body,proto3" json:"Body,omitempty"`
}

func (x *Transaction) Reset() {
	*x = Transaction{}
	if protoimpl.UnsafeEnabled {
		mi := &file_shard_v2_shard_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Transaction) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Transaction) ProtoMessage() {}

func (x *Transaction) ProtoReflect() protoreflect.Message {
	mi := &file_shard_v2_shard_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Transaction.ProtoReflect.Descriptor instead.
func (*Transaction) Descriptor() ([]byte, []int) {
	return file_shard_v2_shard_proto_rawDescGZIP(), []int{5}
}

func (x *Transaction) GetPersonaTag() string {
	if x != nil {
		return x.PersonaTag
	}
	return ""
}

func (x *Transaction) GetNamespace() string {
	if x != nil {
		return x.Namespace
	}
	return ""
}

func (x *Transaction) GetNonce() uint64 {
	if x != nil {
		return x.Nonce
	}
	return 0
}

func (x *Transaction) GetSignature() string {
	if x != nil {
		return x.Signature
	}
	return ""
}

func (x *Transaction) GetBody() []byte {
	if x != nil {
		return x.Body
	}
	return nil
}

var File_shard_v2_shard_proto protoreflect.FileDescriptor

var file_shard_v2_shard_proto_rawDesc = []byte{
	0x0a, 0x14, 0x73, 0x68, 0x61, 0x72, 0x64, 0x2f, 0x76, 0x32, 0x2f, 0x73, 0x68, 0x61, 0x72, 0x64,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x15, 0x77, 0x6f, 0x72, 0x6c, 0x64, 0x2e, 0x65, 0x6e,
	0x67, 0x69, 0x6e, 0x65, 0x2e, 0x73, 0x68, 0x61, 0x72, 0x64, 0x2e, 0x76, 0x32, 0x22, 0x5f, 0x0a,
	0x18, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x65, 0x72, 0x47, 0x61, 0x6d, 0x65, 0x53, 0x68, 0x61,
	0x72, 0x64, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x1c, 0x0a, 0x09, 0x6e, 0x61, 0x6d,
	0x65, 0x73, 0x70, 0x61, 0x63, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x6e, 0x61,
	0x6d, 0x65, 0x73, 0x70, 0x61, 0x63, 0x65, 0x12, 0x25, 0x0a, 0x0e, 0x72, 0x6f, 0x75, 0x74, 0x65,
	0x72, 0x5f, 0x61, 0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x0d, 0x72, 0x6f, 0x75, 0x74, 0x65, 0x72, 0x41, 0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x22, 0x1b,
	0x0a, 0x19, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x65, 0x72, 0x47, 0x61, 0x6d, 0x65, 0x53, 0x68,
	0x61, 0x72, 0x64, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0xc4, 0x02, 0x0a, 0x19,
	0x53, 0x75, 0x62, 0x6d, 0x69, 0x74, 0x54, 0x72, 0x61, 0x6e, 0x73, 0x61, 0x63, 0x74, 0x69, 0x6f,
	0x6e, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x14, 0x0a, 0x05, 0x65, 0x70, 0x6f,
	0x63, 0x68, 0x18, 0x01, 0x20, 0x01, 0x28, 0x04, 0x52, 0x05, 0x65, 0x70, 0x6f, 0x63, 0x68, 0x12,
	0x25, 0x0a, 0x0e, 0x75, 0x6e, 0x69, 0x78, 0x5f, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d,
	0x70, 0x18, 0x02, 0x20, 0x01, 0x28, 0x04, 0x52, 0x0d, 0x75, 0x6e, 0x69, 0x78, 0x54, 0x69, 0x6d,
	0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x12, 0x1c, 0x0a, 0x09, 0x6e, 0x61, 0x6d, 0x65, 0x73, 0x70,
	0x61, 0x63, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x6e, 0x61, 0x6d, 0x65, 0x73,
	0x70, 0x61, 0x63, 0x65, 0x12, 0x66, 0x0a, 0x0c, 0x74, 0x72, 0x61, 0x6e, 0x73, 0x61, 0x63, 0x74,
	0x69, 0x6f, 0x6e, 0x73, 0x18, 0x04, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x42, 0x2e, 0x77, 0x6f, 0x72,
	0x6c, 0x64, 0x2e, 0x65, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x2e, 0x73, 0x68, 0x61, 0x72, 0x64, 0x2e,
	0x76, 0x32, 0x2e, 0x53, 0x75, 0x62, 0x6d, 0x69, 0x74, 0x54, 0x72, 0x61, 0x6e, 0x73, 0x61, 0x63,
	0x74, 0x69, 0x6f, 0x6e, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x2e, 0x54, 0x72, 0x61,
	0x6e, 0x73, 0x61, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52, 0x0c,
	0x74, 0x72, 0x61, 0x6e, 0x73, 0x61, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x1a, 0x64, 0x0a, 0x11,
	0x54, 0x72, 0x61, 0x6e, 0x73, 0x61, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x45, 0x6e, 0x74, 0x72,
	0x79, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x04, 0x52, 0x03,
	0x6b, 0x65, 0x79, 0x12, 0x39, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01,
	0x28, 0x0b, 0x32, 0x23, 0x2e, 0x77, 0x6f, 0x72, 0x6c, 0x64, 0x2e, 0x65, 0x6e, 0x67, 0x69, 0x6e,
	0x65, 0x2e, 0x73, 0x68, 0x61, 0x72, 0x64, 0x2e, 0x76, 0x32, 0x2e, 0x54, 0x72, 0x61, 0x6e, 0x73,
	0x61, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x3a, 0x02,
	0x38, 0x01, 0x22, 0x1c, 0x0a, 0x1a, 0x53, 0x75, 0x62, 0x6d, 0x69, 0x74, 0x54, 0x72, 0x61, 0x6e,
	0x73, 0x61, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x22, 0x44, 0x0a, 0x0c, 0x54, 0x72, 0x61, 0x6e, 0x73, 0x61, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x73,
	0x12, 0x34, 0x0a, 0x03, 0x74, 0x78, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x22, 0x2e,
	0x77, 0x6f, 0x72, 0x6c, 0x64, 0x2e, 0x65, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x2e, 0x73, 0x68, 0x61,
	0x72, 0x64, 0x2e, 0x76, 0x32, 0x2e, 0x54, 0x72, 0x61, 0x6e, 0x73, 0x61, 0x63, 0x74, 0x69, 0x6f,
	0x6e, 0x52, 0x03, 0x74, 0x78, 0x73, 0x22, 0x93, 0x01, 0x0a, 0x0b, 0x54, 0x72, 0x61, 0x6e, 0x73,
	0x61, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x1e, 0x0a, 0x0a, 0x50, 0x65, 0x72, 0x73, 0x6f, 0x6e,
	0x61, 0x54, 0x61, 0x67, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x50, 0x65, 0x72, 0x73,
	0x6f, 0x6e, 0x61, 0x54, 0x61, 0x67, 0x12, 0x1c, 0x0a, 0x09, 0x4e, 0x61, 0x6d, 0x65, 0x73, 0x70,
	0x61, 0x63, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x4e, 0x61, 0x6d, 0x65, 0x73,
	0x70, 0x61, 0x63, 0x65, 0x12, 0x14, 0x0a, 0x05, 0x4e, 0x6f, 0x6e, 0x63, 0x65, 0x18, 0x03, 0x20,
	0x01, 0x28, 0x04, 0x52, 0x05, 0x4e, 0x6f, 0x6e, 0x63, 0x65, 0x12, 0x1c, 0x0a, 0x09, 0x53, 0x69,
	0x67, 0x6e, 0x61, 0x74, 0x75, 0x72, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x53,
	0x69, 0x67, 0x6e, 0x61, 0x74, 0x75, 0x72, 0x65, 0x12, 0x12, 0x0a, 0x04, 0x42, 0x6f, 0x64, 0x79,
	0x18, 0x05, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x04, 0x42, 0x6f, 0x64, 0x79, 0x32, 0xfb, 0x01, 0x0a,
	0x12, 0x54, 0x72, 0x61, 0x6e, 0x73, 0x61, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x48, 0x61, 0x6e, 0x64,
	0x6c, 0x65, 0x72, 0x12, 0x76, 0x0a, 0x11, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x65, 0x72, 0x47,
	0x61, 0x6d, 0x65, 0x53, 0x68, 0x61, 0x72, 0x64, 0x12, 0x2f, 0x2e, 0x77, 0x6f, 0x72, 0x6c, 0x64,
	0x2e, 0x65, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x2e, 0x73, 0x68, 0x61, 0x72, 0x64, 0x2e, 0x76, 0x32,
	0x2e, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x65, 0x72, 0x47, 0x61, 0x6d, 0x65, 0x53, 0x68, 0x61,
	0x72, 0x64, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x30, 0x2e, 0x77, 0x6f, 0x72, 0x6c,
	0x64, 0x2e, 0x65, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x2e, 0x73, 0x68, 0x61, 0x72, 0x64, 0x2e, 0x76,
	0x32, 0x2e, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x65, 0x72, 0x47, 0x61, 0x6d, 0x65, 0x53, 0x68,
	0x61, 0x72, 0x64, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x6d, 0x0a, 0x06, 0x53,
	0x75, 0x62, 0x6d, 0x69, 0x74, 0x12, 0x30, 0x2e, 0x77, 0x6f, 0x72, 0x6c, 0x64, 0x2e, 0x65, 0x6e,
	0x67, 0x69, 0x6e, 0x65, 0x2e, 0x73, 0x68, 0x61, 0x72, 0x64, 0x2e, 0x76, 0x32, 0x2e, 0x53, 0x75,
	0x62, 0x6d, 0x69, 0x74, 0x54, 0x72, 0x61, 0x6e, 0x73, 0x61, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x73,
	0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x31, 0x2e, 0x77, 0x6f, 0x72, 0x6c, 0x64, 0x2e,
	0x65, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x2e, 0x73, 0x68, 0x61, 0x72, 0x64, 0x2e, 0x76, 0x32, 0x2e,
	0x53, 0x75, 0x62, 0x6d, 0x69, 0x74, 0x54, 0x72, 0x61, 0x6e, 0x73, 0x61, 0x63, 0x74, 0x69, 0x6f,
	0x6e, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x42, 0xb5, 0x01, 0x0a, 0x19, 0x63,
	0x6f, 0x6d, 0x2e, 0x77, 0x6f, 0x72, 0x6c, 0x64, 0x2e, 0x65, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x2e,
	0x73, 0x68, 0x61, 0x72, 0x64, 0x2e, 0x76, 0x32, 0x42, 0x0a, 0x53, 0x68, 0x61, 0x72, 0x64, 0x50,
	0x72, 0x6f, 0x74, 0x6f, 0x50, 0x01, 0x5a, 0x15, 0x72, 0x69, 0x66, 0x74, 0x2f, 0x73, 0x68, 0x61,
	0x72, 0x64, 0x2f, 0x76, 0x32, 0x3b, 0x73, 0x68, 0x61, 0x72, 0x64, 0x76, 0x32, 0xa2, 0x02, 0x03,
	0x57, 0x45, 0x53, 0xaa, 0x02, 0x15, 0x57, 0x6f, 0x72, 0x6c, 0x64, 0x2e, 0x45, 0x6e, 0x67, 0x69,
	0x6e, 0x65, 0x2e, 0x53, 0x68, 0x61, 0x72, 0x64, 0x2e, 0x56, 0x32, 0xca, 0x02, 0x15, 0x57, 0x6f,
	0x72, 0x6c, 0x64, 0x5c, 0x45, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x5c, 0x53, 0x68, 0x61, 0x72, 0x64,
	0x5c, 0x56, 0x32, 0xe2, 0x02, 0x21, 0x57, 0x6f, 0x72, 0x6c, 0x64, 0x5c, 0x45, 0x6e, 0x67, 0x69,
	0x6e, 0x65, 0x5c, 0x53, 0x68, 0x61, 0x72, 0x64, 0x5c, 0x56, 0x32, 0x5c, 0x47, 0x50, 0x42, 0x4d,
	0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0xea, 0x02, 0x18, 0x57, 0x6f, 0x72, 0x6c, 0x64, 0x3a,
	0x3a, 0x45, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x3a, 0x3a, 0x53, 0x68, 0x61, 0x72, 0x64, 0x3a, 0x3a,
	0x56, 0x32, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_shard_v2_shard_proto_rawDescOnce sync.Once
	file_shard_v2_shard_proto_rawDescData = file_shard_v2_shard_proto_rawDesc
)

func file_shard_v2_shard_proto_rawDescGZIP() []byte {
	file_shard_v2_shard_proto_rawDescOnce.Do(func() {
		file_shard_v2_shard_proto_rawDescData = protoimpl.X.CompressGZIP(file_shard_v2_shard_proto_rawDescData)
	})
	return file_shard_v2_shard_proto_rawDescData
}

var file_shard_v2_shard_proto_msgTypes = make([]protoimpl.MessageInfo, 7)
var file_shard_v2_shard_proto_goTypes = []interface{}{
	(*RegisterGameShardRequest)(nil),   // 0: world.engine.shard.v2.RegisterGameShardRequest
	(*RegisterGameShardResponse)(nil),  // 1: world.engine.shard.v2.RegisterGameShardResponse
	(*SubmitTransactionsRequest)(nil),  // 2: world.engine.shard.v2.SubmitTransactionsRequest
	(*SubmitTransactionsResponse)(nil), // 3: world.engine.shard.v2.SubmitTransactionsResponse
	(*Transactions)(nil),               // 4: world.engine.shard.v2.Transactions
	(*Transaction)(nil),                // 5: world.engine.shard.v2.Transaction
	nil,                                // 6: world.engine.shard.v2.SubmitTransactionsRequest.TransactionsEntry
}
var file_shard_v2_shard_proto_depIdxs = []int32{
	6, // 0: world.engine.shard.v2.SubmitTransactionsRequest.transactions:type_name -> world.engine.shard.v2.SubmitTransactionsRequest.TransactionsEntry
	5, // 1: world.engine.shard.v2.Transactions.txs:type_name -> world.engine.shard.v2.Transaction
	4, // 2: world.engine.shard.v2.SubmitTransactionsRequest.TransactionsEntry.value:type_name -> world.engine.shard.v2.Transactions
	0, // 3: world.engine.shard.v2.TransactionHandler.RegisterGameShard:input_type -> world.engine.shard.v2.RegisterGameShardRequest
	2, // 4: world.engine.shard.v2.TransactionHandler.Submit:input_type -> world.engine.shard.v2.SubmitTransactionsRequest
	1, // 5: world.engine.shard.v2.TransactionHandler.RegisterGameShard:output_type -> world.engine.shard.v2.RegisterGameShardResponse
	3, // 6: world.engine.shard.v2.TransactionHandler.Submit:output_type -> world.engine.shard.v2.SubmitTransactionsResponse
	5, // [5:7] is the sub-list for method output_type
	3, // [3:5] is the sub-list for method input_type
	3, // [3:3] is the sub-list for extension type_name
	3, // [3:3] is the sub-list for extension extendee
	0, // [0:3] is the sub-list for field type_name
}

func init() { file_shard_v2_shard_proto_init() }
func file_shard_v2_shard_proto_init() {
	if File_shard_v2_shard_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_shard_v2_shard_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*RegisterGameShardRequest); i {
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
		file_shard_v2_shard_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*RegisterGameShardResponse); i {
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
		file_shard_v2_shard_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SubmitTransactionsRequest); i {
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
		file_shard_v2_shard_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SubmitTransactionsResponse); i {
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
		file_shard_v2_shard_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Transactions); i {
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
		file_shard_v2_shard_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Transaction); i {
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
			RawDescriptor: file_shard_v2_shard_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   7,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_shard_v2_shard_proto_goTypes,
		DependencyIndexes: file_shard_v2_shard_proto_depIdxs,
		MessageInfos:      file_shard_v2_shard_proto_msgTypes,
	}.Build()
	File_shard_v2_shard_proto = out.File
	file_shard_v2_shard_proto_rawDesc = nil
	file_shard_v2_shard_proto_goTypes = nil
	file_shard_v2_shard_proto_depIdxs = nil
}

// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: shard/v1/query.proto

package types

import (
	context "context"
	fmt "fmt"
	_ "github.com/cosmos/cosmos-proto"
	_ "github.com/cosmos/cosmos-sdk/types/msgservice"
	_ "github.com/cosmos/gogoproto/gogoproto"
	grpc1 "github.com/cosmos/gogoproto/grpc"
	proto "github.com/cosmos/gogoproto/proto"
	_ "google.golang.org/genproto/googleapis/api/annotations"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	io "io"
	math "math"
	math_bits "math/bits"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion3 // please upgrade the proto package

type QueryTransactionsRequest struct {
	Namespace string       `protobuf:"bytes,1,opt,name=namespace,proto3" json:"namespace,omitempty"`
	Page      *PageRequest `protobuf:"bytes,2,opt,name=page,proto3" json:"page,omitempty"`
}

func (m *QueryTransactionsRequest) Reset()         { *m = QueryTransactionsRequest{} }
func (m *QueryTransactionsRequest) String() string { return proto.CompactTextString(m) }
func (*QueryTransactionsRequest) ProtoMessage()    {}
func (*QueryTransactionsRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_1088f6b90570984a, []int{0}
}
func (m *QueryTransactionsRequest) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *QueryTransactionsRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_QueryTransactionsRequest.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *QueryTransactionsRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_QueryTransactionsRequest.Merge(m, src)
}
func (m *QueryTransactionsRequest) XXX_Size() int {
	return m.Size()
}
func (m *QueryTransactionsRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_QueryTransactionsRequest.DiscardUnknown(m)
}

var xxx_messageInfo_QueryTransactionsRequest proto.InternalMessageInfo

func (m *QueryTransactionsRequest) GetNamespace() string {
	if m != nil {
		return m.Namespace
	}
	return ""
}

func (m *QueryTransactionsRequest) GetPage() *PageRequest {
	if m != nil {
		return m.Page
	}
	return nil
}

type QueryTransactionsResponse struct {
	// txs contains the transactions. Each entry contains a tick, and a list of txs that occurred in that tick.
	Ticks []*Tick `protobuf:"bytes,1,rep,name=ticks,proto3" json:"ticks,omitempty"`
	// page contains information on how to query the next items in the collection, if any.
	// when page is nil/empty, there is nothing left to query.
	Page *PageResponse `protobuf:"bytes,2,opt,name=page,proto3" json:"page,omitempty"`
}

func (m *QueryTransactionsResponse) Reset()         { *m = QueryTransactionsResponse{} }
func (m *QueryTransactionsResponse) String() string { return proto.CompactTextString(m) }
func (*QueryTransactionsResponse) ProtoMessage()    {}
func (*QueryTransactionsResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_1088f6b90570984a, []int{1}
}
func (m *QueryTransactionsResponse) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *QueryTransactionsResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_QueryTransactionsResponse.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *QueryTransactionsResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_QueryTransactionsResponse.Merge(m, src)
}
func (m *QueryTransactionsResponse) XXX_Size() int {
	return m.Size()
}
func (m *QueryTransactionsResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_QueryTransactionsResponse.DiscardUnknown(m)
}

var xxx_messageInfo_QueryTransactionsResponse proto.InternalMessageInfo

func (m *QueryTransactionsResponse) GetTicks() []*Tick {
	if m != nil {
		return m.Ticks
	}
	return nil
}

func (m *QueryTransactionsResponse) GetPage() *PageResponse {
	if m != nil {
		return m.Page
	}
	return nil
}

// PageRequest represents a request for a paged query.
type PageRequest struct {
	// key is the cosmos SDK store key to begin the iteration on.
	Key []byte `protobuf:"bytes,1,opt,name=key,proto3" json:"key,omitempty"`
	// limit is the amount of items we want to limit in our return.
	// example, if the collection we are interested has 10 items,
	// and we set limit to 5, the query will only return 5 items.
	Limit uint32 `protobuf:"varint,2,opt,name=limit,proto3" json:"limit,omitempty"`
}

func (m *PageRequest) Reset()         { *m = PageRequest{} }
func (m *PageRequest) String() string { return proto.CompactTextString(m) }
func (*PageRequest) ProtoMessage()    {}
func (*PageRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_1088f6b90570984a, []int{2}
}
func (m *PageRequest) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *PageRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_PageRequest.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *PageRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_PageRequest.Merge(m, src)
}
func (m *PageRequest) XXX_Size() int {
	return m.Size()
}
func (m *PageRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_PageRequest.DiscardUnknown(m)
}

var xxx_messageInfo_PageRequest proto.InternalMessageInfo

func (m *PageRequest) GetKey() []byte {
	if m != nil {
		return m.Key
	}
	return nil
}

func (m *PageRequest) GetLimit() uint32 {
	if m != nil {
		return m.Limit
	}
	return 0
}

// PageResponse represents a response to a paged query.
type PageResponse struct {
	// if a key is present, that means there are more items from the collection to query, and the given key is the key for
	// the item after the last one returned. if key is nil, that means there are no more items in the collection to query.
	Key []byte `protobuf:"bytes,1,opt,name=key,proto3" json:"key,omitempty"`
}

func (m *PageResponse) Reset()         { *m = PageResponse{} }
func (m *PageResponse) String() string { return proto.CompactTextString(m) }
func (*PageResponse) ProtoMessage()    {}
func (*PageResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_1088f6b90570984a, []int{3}
}
func (m *PageResponse) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *PageResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_PageResponse.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *PageResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_PageResponse.Merge(m, src)
}
func (m *PageResponse) XXX_Size() int {
	return m.Size()
}
func (m *PageResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_PageResponse.DiscardUnknown(m)
}

var xxx_messageInfo_PageResponse proto.InternalMessageInfo

func (m *PageResponse) GetKey() []byte {
	if m != nil {
		return m.Key
	}
	return nil
}

func init() {
	proto.RegisterType((*QueryTransactionsRequest)(nil), "shard.v1.QueryTransactionsRequest")
	proto.RegisterType((*QueryTransactionsResponse)(nil), "shard.v1.QueryTransactionsResponse")
	proto.RegisterType((*PageRequest)(nil), "shard.v1.PageRequest")
	proto.RegisterType((*PageResponse)(nil), "shard.v1.PageResponse")
}

func init() { proto.RegisterFile("shard/v1/query.proto", fileDescriptor_1088f6b90570984a) }

var fileDescriptor_1088f6b90570984a = []byte{
	// 381 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x7c, 0x51, 0xdd, 0x6e, 0xda, 0x30,
	0x14, 0x26, 0x63, 0x4c, 0xc3, 0xb0, 0x69, 0xb2, 0xd8, 0x16, 0x10, 0x8a, 0xa2, 0x6c, 0x17, 0x6c,
	0x12, 0xb1, 0x60, 0x5a, 0x1f, 0xa0, 0x4f, 0x40, 0x23, 0xa4, 0x4a, 0xbd, 0x69, 0x8d, 0xb1, 0x8c,
	0x45, 0x62, 0x87, 0xd8, 0xa1, 0xe5, 0x2d, 0xfa, 0x58, 0xbd, 0xe4, 0xb2, 0x97, 0x15, 0xbc, 0x48,
	0x15, 0x3b, 0x28, 0xf4, 0xf7, 0xce, 0xe7, 0x3b, 0xe7, 0x7c, 0x3f, 0x3e, 0xa0, 0xa3, 0x16, 0x38,
	0x9b, 0xa3, 0xf5, 0x08, 0xad, 0x72, 0x9a, 0x6d, 0xc2, 0x34, 0x93, 0x5a, 0xc2, 0xcf, 0x06, 0x0d,
	0xd7, 0xa3, 0x5e, 0x97, 0x48, 0x95, 0x48, 0x75, 0x69, 0x70, 0x64, 0x0b, 0x3b, 0xd4, 0xfb, 0x69,
	0x2b, 0x94, 0x28, 0x56, 0xec, 0x27, 0x8a, 0x95, 0x8d, 0x0e, 0x93, 0x4c, 0xda, 0x85, 0xe2, 0x55,
	0xa2, 0x7d, 0x26, 0x25, 0x8b, 0x29, 0xc2, 0x29, 0x47, 0x58, 0x08, 0xa9, 0xb1, 0xe6, 0x52, 0x1c,
	0xc8, 0x2a, 0x1f, 0x7a, 0x93, 0xd2, 0x12, 0x0d, 0x08, 0x70, 0xcf, 0x0a, 0x5b, 0xd3, 0x0c, 0x0b,
	0x85, 0x89, 0x59, 0x88, 0xe8, 0x2a, 0xa7, 0x4a, 0xc3, 0x3e, 0x68, 0x0a, 0x9c, 0x50, 0x95, 0x62,
	0x42, 0x5d, 0xc7, 0x77, 0x06, 0xcd, 0xa8, 0x02, 0xe0, 0x1f, 0xf0, 0x31, 0xc5, 0x8c, 0xba, 0x1f,
	0x7c, 0x67, 0xd0, 0x1a, 0x7f, 0x0f, 0x0f, 0x81, 0xc2, 0x09, 0x66, 0xb4, 0xa4, 0x88, 0xcc, 0x48,
	0x90, 0x80, 0xee, 0x2b, 0x22, 0x2a, 0x95, 0x42, 0x51, 0xf8, 0x1b, 0x34, 0x34, 0x27, 0x4b, 0xe5,
	0x3a, 0x7e, 0x7d, 0xd0, 0x1a, 0x7f, 0xad, 0x88, 0xa6, 0x9c, 0x2c, 0x23, 0xdb, 0x84, 0x7f, 0x9f,
	0xa8, 0xfd, 0x78, 0xae, 0x66, 0xb9, 0x4a, 0xb9, 0xff, 0xa0, 0x75, 0xe4, 0x01, 0x7e, 0x03, 0xf5,
	0x25, 0xdd, 0x98, 0x00, 0xed, 0xa8, 0x78, 0xc2, 0x0e, 0x68, 0xc4, 0x3c, 0xe1, 0xda, 0xb0, 0x7d,
	0x89, 0x6c, 0x11, 0xf8, 0xa0, 0x7d, 0x4c, 0xf6, 0x72, 0x6f, 0x7c, 0x05, 0x1a, 0x26, 0x07, 0x3c,
	0x07, 0xed, 0xe3, 0x2c, 0x30, 0xa8, 0xfc, 0xbc, 0xf5, 0x9b, 0xbd, 0x5f, 0xef, 0xce, 0x58, 0xcd,
	0xd3, 0xc9, 0xdd, 0xce, 0x73, 0xb6, 0x3b, 0xcf, 0x79, 0xd8, 0x79, 0xce, 0xed, 0xde, 0xab, 0x6d,
	0xf7, 0x5e, 0xed, 0x7e, 0xef, 0xd5, 0x2e, 0x4e, 0x18, 0xd7, 0x8b, 0x7c, 0x16, 0x12, 0x99, 0x20,
	0x9c, 0xb1, 0x5c, 0x0d, 0x63, 0x3c, 0x53, 0xe8, 0x5a, 0x66, 0xf1, 0x7c, 0x48, 0x05, 0xe3, 0x82,
	0x22, 0xb2, 0xc0, 0x5c, 0xa0, 0x1b, 0x64, 0x2f, 0x6d, 0xce, 0x3c, 0xfb, 0x64, 0xee, 0xfc, 0xef,
	0x31, 0x00, 0x00, 0xff, 0xff, 0xf5, 0x2b, 0xc7, 0x3b, 0x87, 0x02, 0x00, 0x00,
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// QueryClient is the client API for Query service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type QueryClient interface {
	Transactions(ctx context.Context, in *QueryTransactionsRequest, opts ...grpc.CallOption) (*QueryTransactionsResponse, error)
}

type queryClient struct {
	cc grpc1.ClientConn
}

func NewQueryClient(cc grpc1.ClientConn) QueryClient {
	return &queryClient{cc}
}

func (c *queryClient) Transactions(ctx context.Context, in *QueryTransactionsRequest, opts ...grpc.CallOption) (*QueryTransactionsResponse, error) {
	out := new(QueryTransactionsResponse)
	err := c.cc.Invoke(ctx, "/shard.v1.Query/Transactions", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// QueryServer is the server API for Query service.
type QueryServer interface {
	Transactions(context.Context, *QueryTransactionsRequest) (*QueryTransactionsResponse, error)
}

// UnimplementedQueryServer can be embedded to have forward compatible implementations.
type UnimplementedQueryServer struct {
}

func (*UnimplementedQueryServer) Transactions(ctx context.Context, req *QueryTransactionsRequest) (*QueryTransactionsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Transactions not implemented")
}

func RegisterQueryServer(s grpc1.Server, srv QueryServer) {
	s.RegisterService(&_Query_serviceDesc, srv)
}

func _Query_Transactions_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(QueryTransactionsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(QueryServer).Transactions(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/shard.v1.Query/Transactions",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(QueryServer).Transactions(ctx, req.(*QueryTransactionsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _Query_serviceDesc = grpc.ServiceDesc{
	ServiceName: "shard.v1.Query",
	HandlerType: (*QueryServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Transactions",
			Handler:    _Query_Transactions_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "shard/v1/query.proto",
}

func (m *QueryTransactionsRequest) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *QueryTransactionsRequest) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *QueryTransactionsRequest) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.Page != nil {
		{
			size, err := m.Page.MarshalToSizedBuffer(dAtA[:i])
			if err != nil {
				return 0, err
			}
			i -= size
			i = encodeVarintQuery(dAtA, i, uint64(size))
		}
		i--
		dAtA[i] = 0x12
	}
	if len(m.Namespace) > 0 {
		i -= len(m.Namespace)
		copy(dAtA[i:], m.Namespace)
		i = encodeVarintQuery(dAtA, i, uint64(len(m.Namespace)))
		i--
		dAtA[i] = 0xa
	}
	return len(dAtA) - i, nil
}

func (m *QueryTransactionsResponse) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *QueryTransactionsResponse) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *QueryTransactionsResponse) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.Page != nil {
		{
			size, err := m.Page.MarshalToSizedBuffer(dAtA[:i])
			if err != nil {
				return 0, err
			}
			i -= size
			i = encodeVarintQuery(dAtA, i, uint64(size))
		}
		i--
		dAtA[i] = 0x12
	}
	if len(m.Ticks) > 0 {
		for iNdEx := len(m.Ticks) - 1; iNdEx >= 0; iNdEx-- {
			{
				size, err := m.Ticks[iNdEx].MarshalToSizedBuffer(dAtA[:i])
				if err != nil {
					return 0, err
				}
				i -= size
				i = encodeVarintQuery(dAtA, i, uint64(size))
			}
			i--
			dAtA[i] = 0xa
		}
	}
	return len(dAtA) - i, nil
}

func (m *PageRequest) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *PageRequest) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *PageRequest) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.Limit != 0 {
		i = encodeVarintQuery(dAtA, i, uint64(m.Limit))
		i--
		dAtA[i] = 0x10
	}
	if len(m.Key) > 0 {
		i -= len(m.Key)
		copy(dAtA[i:], m.Key)
		i = encodeVarintQuery(dAtA, i, uint64(len(m.Key)))
		i--
		dAtA[i] = 0xa
	}
	return len(dAtA) - i, nil
}

func (m *PageResponse) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *PageResponse) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *PageResponse) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if len(m.Key) > 0 {
		i -= len(m.Key)
		copy(dAtA[i:], m.Key)
		i = encodeVarintQuery(dAtA, i, uint64(len(m.Key)))
		i--
		dAtA[i] = 0xa
	}
	return len(dAtA) - i, nil
}

func encodeVarintQuery(dAtA []byte, offset int, v uint64) int {
	offset -= sovQuery(v)
	base := offset
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return base
}
func (m *QueryTransactionsRequest) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = len(m.Namespace)
	if l > 0 {
		n += 1 + l + sovQuery(uint64(l))
	}
	if m.Page != nil {
		l = m.Page.Size()
		n += 1 + l + sovQuery(uint64(l))
	}
	return n
}

func (m *QueryTransactionsResponse) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if len(m.Ticks) > 0 {
		for _, e := range m.Ticks {
			l = e.Size()
			n += 1 + l + sovQuery(uint64(l))
		}
	}
	if m.Page != nil {
		l = m.Page.Size()
		n += 1 + l + sovQuery(uint64(l))
	}
	return n
}

func (m *PageRequest) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = len(m.Key)
	if l > 0 {
		n += 1 + l + sovQuery(uint64(l))
	}
	if m.Limit != 0 {
		n += 1 + sovQuery(uint64(m.Limit))
	}
	return n
}

func (m *PageResponse) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = len(m.Key)
	if l > 0 {
		n += 1 + l + sovQuery(uint64(l))
	}
	return n
}

func sovQuery(x uint64) (n int) {
	return (math_bits.Len64(x|1) + 6) / 7
}
func sozQuery(x uint64) (n int) {
	return sovQuery(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *QueryTransactionsRequest) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowQuery
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: QueryTransactionsRequest: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: QueryTransactionsRequest: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Namespace", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowQuery
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthQuery
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthQuery
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Namespace = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Page", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowQuery
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthQuery
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthQuery
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Page == nil {
				m.Page = &PageRequest{}
			}
			if err := m.Page.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipQuery(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthQuery
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *QueryTransactionsResponse) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowQuery
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: QueryTransactionsResponse: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: QueryTransactionsResponse: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Ticks", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowQuery
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthQuery
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthQuery
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Ticks = append(m.Ticks, &Tick{})
			if err := m.Ticks[len(m.Ticks)-1].Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Page", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowQuery
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthQuery
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthQuery
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Page == nil {
				m.Page = &PageResponse{}
			}
			if err := m.Page.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipQuery(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthQuery
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *PageRequest) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowQuery
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: PageRequest: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: PageRequest: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Key", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowQuery
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				byteLen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if byteLen < 0 {
				return ErrInvalidLengthQuery
			}
			postIndex := iNdEx + byteLen
			if postIndex < 0 {
				return ErrInvalidLengthQuery
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Key = append(m.Key[:0], dAtA[iNdEx:postIndex]...)
			if m.Key == nil {
				m.Key = []byte{}
			}
			iNdEx = postIndex
		case 2:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Limit", wireType)
			}
			m.Limit = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowQuery
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Limit |= uint32(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		default:
			iNdEx = preIndex
			skippy, err := skipQuery(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthQuery
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *PageResponse) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowQuery
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: PageResponse: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: PageResponse: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Key", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowQuery
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				byteLen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if byteLen < 0 {
				return ErrInvalidLengthQuery
			}
			postIndex := iNdEx + byteLen
			if postIndex < 0 {
				return ErrInvalidLengthQuery
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Key = append(m.Key[:0], dAtA[iNdEx:postIndex]...)
			if m.Key == nil {
				m.Key = []byte{}
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipQuery(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthQuery
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func skipQuery(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	depth := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowQuery
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		wireType := int(wire & 0x7)
		switch wireType {
		case 0:
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowQuery
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				iNdEx++
				if dAtA[iNdEx-1] < 0x80 {
					break
				}
			}
		case 1:
			iNdEx += 8
		case 2:
			var length int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowQuery
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				length |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if length < 0 {
				return 0, ErrInvalidLengthQuery
			}
			iNdEx += length
		case 3:
			depth++
		case 4:
			if depth == 0 {
				return 0, ErrUnexpectedEndOfGroupQuery
			}
			depth--
		case 5:
			iNdEx += 4
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
		if iNdEx < 0 {
			return 0, ErrInvalidLengthQuery
		}
		if depth == 0 {
			return iNdEx, nil
		}
	}
	return 0, io.ErrUnexpectedEOF
}

var (
	ErrInvalidLengthQuery        = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowQuery          = fmt.Errorf("proto: integer overflow")
	ErrUnexpectedEndOfGroupQuery = fmt.Errorf("proto: unexpected end of group")
)

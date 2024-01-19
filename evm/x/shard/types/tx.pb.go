// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: shard/v1/tx.proto

package types

import (
	context "context"
	fmt "fmt"
	_ "github.com/cosmos/cosmos-proto"
	_ "github.com/cosmos/cosmos-sdk/types/msgservice"
	grpc1 "github.com/cosmos/gogoproto/grpc"
	proto "github.com/cosmos/gogoproto/proto"
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

type SubmitShardTxRequest struct {
	// sender is the address of the sender. this will be set to the module address.
	Sender string `protobuf:"bytes,1,opt,name=sender,proto3" json:"sender,omitempty"`
	// namespace is the namespace of the world the transactions originated from.
	Namespace string `protobuf:"bytes,2,opt,name=namespace,proto3" json:"namespace,omitempty"`
	// epoch is an arbitrary interval that this transaction was executed in.
	// for loop driven games, this is likely a tick. for event driven games,
	// this could be some general period of time.
	Epoch         uint64 `protobuf:"varint,3,opt,name=epoch,proto3" json:"epoch,omitempty"`
	UnixTimestamp uint64 `protobuf:"varint,4,opt,name=unix_timestamp,json=unixTimestamp,proto3" json:"unix_timestamp,omitempty"`
	// txs are the transactions that occurred in this tick.
	Txs []*Transaction `protobuf:"bytes,5,rep,name=txs,proto3" json:"txs,omitempty"`
}

func (m *SubmitShardTxRequest) Reset()         { *m = SubmitShardTxRequest{} }
func (m *SubmitShardTxRequest) String() string { return proto.CompactTextString(m) }
func (*SubmitShardTxRequest) ProtoMessage()    {}
func (*SubmitShardTxRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_2ea9067d7c94eab8, []int{0}
}
func (m *SubmitShardTxRequest) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *SubmitShardTxRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_SubmitShardTxRequest.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *SubmitShardTxRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SubmitShardTxRequest.Merge(m, src)
}
func (m *SubmitShardTxRequest) XXX_Size() int {
	return m.Size()
}
func (m *SubmitShardTxRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_SubmitShardTxRequest.DiscardUnknown(m)
}

var xxx_messageInfo_SubmitShardTxRequest proto.InternalMessageInfo

func (m *SubmitShardTxRequest) GetSender() string {
	if m != nil {
		return m.Sender
	}
	return ""
}

func (m *SubmitShardTxRequest) GetNamespace() string {
	if m != nil {
		return m.Namespace
	}
	return ""
}

func (m *SubmitShardTxRequest) GetEpoch() uint64 {
	if m != nil {
		return m.Epoch
	}
	return 0
}

func (m *SubmitShardTxRequest) GetUnixTimestamp() uint64 {
	if m != nil {
		return m.UnixTimestamp
	}
	return 0
}

func (m *SubmitShardTxRequest) GetTxs() []*Transaction {
	if m != nil {
		return m.Txs
	}
	return nil
}

type SubmitShardTxResponse struct {
}

func (m *SubmitShardTxResponse) Reset()         { *m = SubmitShardTxResponse{} }
func (m *SubmitShardTxResponse) String() string { return proto.CompactTextString(m) }
func (*SubmitShardTxResponse) ProtoMessage()    {}
func (*SubmitShardTxResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_2ea9067d7c94eab8, []int{1}
}
func (m *SubmitShardTxResponse) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *SubmitShardTxResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_SubmitShardTxResponse.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *SubmitShardTxResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SubmitShardTxResponse.Merge(m, src)
}
func (m *SubmitShardTxResponse) XXX_Size() int {
	return m.Size()
}
func (m *SubmitShardTxResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_SubmitShardTxResponse.DiscardUnknown(m)
}

var xxx_messageInfo_SubmitShardTxResponse proto.InternalMessageInfo

func init() {
	proto.RegisterType((*SubmitShardTxRequest)(nil), "shard.v1.SubmitShardTxRequest")
	proto.RegisterType((*SubmitShardTxResponse)(nil), "shard.v1.SubmitShardTxResponse")
}

func init() { proto.RegisterFile("shard/v1/tx.proto", fileDescriptor_2ea9067d7c94eab8) }

var fileDescriptor_2ea9067d7c94eab8 = []byte{
	// 370 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x74, 0x51, 0xcd, 0x4e, 0xea, 0x40,
	0x14, 0x66, 0x6e, 0x81, 0x5c, 0x86, 0x70, 0x93, 0x3b, 0x29, 0xa1, 0xb7, 0xb9, 0xa9, 0x84, 0xc4,
	0x48, 0x48, 0xe8, 0x08, 0xee, 0xdc, 0xc9, 0xca, 0x8d, 0x89, 0x29, 0xac, 0x5c, 0x48, 0x4a, 0x3b,
	0x29, 0x8d, 0x76, 0xa6, 0xf6, 0x0c, 0x58, 0x77, 0xc6, 0x27, 0xf0, 0x51, 0x58, 0xf8, 0x10, 0x2e,
	0x89, 0x2b, 0xdd, 0x19, 0x58, 0xf0, 0x1a, 0xa6, 0x3f, 0x84, 0x68, 0x74, 0x37, 0xdf, 0xcf, 0xf9,
	0x66, 0xe6, 0x7c, 0xf8, 0x2f, 0x4c, 0xed, 0xc8, 0xa5, 0xf3, 0x1e, 0x95, 0xb1, 0x19, 0x46, 0x42,
	0x0a, 0xf2, 0x3b, 0xa5, 0xcc, 0x79, 0x4f, 0xff, 0xe7, 0x08, 0x08, 0x04, 0x8c, 0x53, 0x9e, 0x66,
	0x20, 0x33, 0xe9, 0x8d, 0x0c, 0xd1, 0x00, 0xbc, 0x64, 0x38, 0x00, 0x2f, 0x17, 0xd4, 0x5d, 0xe0,
	0x5d, 0xc8, 0x72, 0x7b, 0xeb, 0x0d, 0x61, 0x75, 0x38, 0x9b, 0x04, 0xbe, 0x1c, 0x26, 0xf2, 0x28,
	0xb6, 0xd8, 0xcd, 0x8c, 0x81, 0x24, 0x87, 0xb8, 0x0c, 0x8c, 0xbb, 0x2c, 0xd2, 0x50, 0x13, 0xb5,
	0x2b, 0x03, 0xed, 0xe5, 0xa9, 0xab, 0xe6, 0x37, 0x9d, 0xb8, 0x6e, 0xc4, 0x00, 0x86, 0x32, 0xf2,
	0xb9, 0x67, 0xe5, 0x3e, 0xf2, 0x1f, 0x57, 0xb8, 0x1d, 0x30, 0x08, 0x6d, 0x87, 0x69, 0xbf, 0x92,
	0x21, 0x6b, 0x47, 0x10, 0x15, 0x97, 0x58, 0x28, 0x9c, 0xa9, 0xa6, 0x34, 0x51, 0xbb, 0x68, 0x65,
	0x80, 0xec, 0xe3, 0x3f, 0x33, 0xee, 0xc7, 0x63, 0xe9, 0x07, 0x0c, 0xa4, 0x1d, 0x84, 0x5a, 0x31,
	0x95, 0x6b, 0x09, 0x3b, 0xda, 0x92, 0xe4, 0x00, 0x2b, 0x32, 0x06, 0xad, 0xd4, 0x54, 0xda, 0xd5,
	0x7e, 0xdd, 0xdc, 0xee, 0xc1, 0x1c, 0x45, 0x36, 0x07, 0xdb, 0x91, 0xbe, 0xe0, 0x56, 0xe2, 0x38,
	0xae, 0x3e, 0x6c, 0x16, 0x9d, 0xfc, 0x41, 0xad, 0x06, 0xae, 0x7f, 0xf9, 0x1a, 0x84, 0x82, 0x03,
	0xeb, 0x5f, 0x62, 0xe5, 0x0c, 0x3c, 0x72, 0x8e, 0x6b, 0x9f, 0x74, 0x62, 0xec, 0x92, 0xbf, 0xdb,
	0x89, 0xbe, 0xf7, 0xa3, 0x9e, 0x05, 0xeb, 0xa5, 0xfb, 0xcd, 0xa2, 0x83, 0x06, 0xa7, 0xcf, 0x2b,
	0x03, 0x2d, 0x57, 0x06, 0x7a, 0x5f, 0x19, 0xe8, 0x71, 0x6d, 0x14, 0x96, 0x6b, 0xa3, 0xf0, 0xba,
	0x36, 0x0a, 0x17, 0x66, 0x78, 0xe5, 0x99, 0xb7, 0x22, 0xba, 0x76, 0x4d, 0x97, 0xcd, 0x69, 0x7a,
	0xea, 0x32, 0xee, 0xf9, 0x9c, 0x51, 0x67, 0x6a, 0xfb, 0x9c, 0xc6, 0x34, 0xeb, 0x29, 0x2d, 0x69,
	0x52, 0x4e, 0x5b, 0x3a, 0xfa, 0x08, 0x00, 0x00, 0xff, 0xff, 0x70, 0x9c, 0x49, 0x6d, 0x0e, 0x02,
	0x00, 0x00,
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// MsgClient is the client API for Msg service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type MsgClient interface {
	SubmitShardTx(ctx context.Context, in *SubmitShardTxRequest, opts ...grpc.CallOption) (*SubmitShardTxResponse, error)
}

type msgClient struct {
	cc grpc1.ClientConn
}

func NewMsgClient(cc grpc1.ClientConn) MsgClient {
	return &msgClient{cc}
}

func (c *msgClient) SubmitShardTx(ctx context.Context, in *SubmitShardTxRequest, opts ...grpc.CallOption) (*SubmitShardTxResponse, error) {
	out := new(SubmitShardTxResponse)
	err := c.cc.Invoke(ctx, "/shard.v1.Msg/SubmitShardTx", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// MsgServer is the server API for Msg service.
type MsgServer interface {
	SubmitShardTx(context.Context, *SubmitShardTxRequest) (*SubmitShardTxResponse, error)
}

// UnimplementedMsgServer can be embedded to have forward compatible implementations.
type UnimplementedMsgServer struct {
}

func (*UnimplementedMsgServer) SubmitShardTx(ctx context.Context, req *SubmitShardTxRequest) (*SubmitShardTxResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SubmitShardTx not implemented")
}

func RegisterMsgServer(s grpc1.Server, srv MsgServer) {
	s.RegisterService(&_Msg_serviceDesc, srv)
}

func _Msg_SubmitShardTx_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SubmitShardTxRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MsgServer).SubmitShardTx(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/shard.v1.Msg/SubmitShardTx",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MsgServer).SubmitShardTx(ctx, req.(*SubmitShardTxRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _Msg_serviceDesc = grpc.ServiceDesc{
	ServiceName: "shard.v1.Msg",
	HandlerType: (*MsgServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "SubmitShardTx",
			Handler:    _Msg_SubmitShardTx_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "shard/v1/tx.proto",
}

func (m *SubmitShardTxRequest) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *SubmitShardTxRequest) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *SubmitShardTxRequest) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if len(m.Txs) > 0 {
		for iNdEx := len(m.Txs) - 1; iNdEx >= 0; iNdEx-- {
			{
				size, err := m.Txs[iNdEx].MarshalToSizedBuffer(dAtA[:i])
				if err != nil {
					return 0, err
				}
				i -= size
				i = encodeVarintTx(dAtA, i, uint64(size))
			}
			i--
			dAtA[i] = 0x2a
		}
	}
	if m.UnixTimestamp != 0 {
		i = encodeVarintTx(dAtA, i, uint64(m.UnixTimestamp))
		i--
		dAtA[i] = 0x20
	}
	if m.Epoch != 0 {
		i = encodeVarintTx(dAtA, i, uint64(m.Epoch))
		i--
		dAtA[i] = 0x18
	}
	if len(m.Namespace) > 0 {
		i -= len(m.Namespace)
		copy(dAtA[i:], m.Namespace)
		i = encodeVarintTx(dAtA, i, uint64(len(m.Namespace)))
		i--
		dAtA[i] = 0x12
	}
	if len(m.Sender) > 0 {
		i -= len(m.Sender)
		copy(dAtA[i:], m.Sender)
		i = encodeVarintTx(dAtA, i, uint64(len(m.Sender)))
		i--
		dAtA[i] = 0xa
	}
	return len(dAtA) - i, nil
}

func (m *SubmitShardTxResponse) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *SubmitShardTxResponse) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *SubmitShardTxResponse) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	return len(dAtA) - i, nil
}

func encodeVarintTx(dAtA []byte, offset int, v uint64) int {
	offset -= sovTx(v)
	base := offset
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return base
}
func (m *SubmitShardTxRequest) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = len(m.Sender)
	if l > 0 {
		n += 1 + l + sovTx(uint64(l))
	}
	l = len(m.Namespace)
	if l > 0 {
		n += 1 + l + sovTx(uint64(l))
	}
	if m.Epoch != 0 {
		n += 1 + sovTx(uint64(m.Epoch))
	}
	if m.UnixTimestamp != 0 {
		n += 1 + sovTx(uint64(m.UnixTimestamp))
	}
	if len(m.Txs) > 0 {
		for _, e := range m.Txs {
			l = e.Size()
			n += 1 + l + sovTx(uint64(l))
		}
	}
	return n
}

func (m *SubmitShardTxResponse) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	return n
}

func sovTx(x uint64) (n int) {
	return (math_bits.Len64(x|1) + 6) / 7
}
func sozTx(x uint64) (n int) {
	return sovTx(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *SubmitShardTxRequest) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowTx
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
			return fmt.Errorf("proto: SubmitShardTxRequest: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: SubmitShardTxRequest: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Sender", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTx
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
				return ErrInvalidLengthTx
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthTx
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Sender = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Namespace", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTx
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
				return ErrInvalidLengthTx
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthTx
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Namespace = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 3:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Epoch", wireType)
			}
			m.Epoch = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTx
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Epoch |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 4:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field UnixTimestamp", wireType)
			}
			m.UnixTimestamp = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTx
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.UnixTimestamp |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 5:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Txs", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTx
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
				return ErrInvalidLengthTx
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthTx
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Txs = append(m.Txs, &Transaction{})
			if err := m.Txs[len(m.Txs)-1].Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipTx(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthTx
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
func (m *SubmitShardTxResponse) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowTx
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
			return fmt.Errorf("proto: SubmitShardTxResponse: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: SubmitShardTxResponse: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		default:
			iNdEx = preIndex
			skippy, err := skipTx(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthTx
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
func skipTx(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	depth := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowTx
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
					return 0, ErrIntOverflowTx
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
					return 0, ErrIntOverflowTx
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
				return 0, ErrInvalidLengthTx
			}
			iNdEx += length
		case 3:
			depth++
		case 4:
			if depth == 0 {
				return 0, ErrUnexpectedEndOfGroupTx
			}
			depth--
		case 5:
			iNdEx += 4
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
		if iNdEx < 0 {
			return 0, ErrInvalidLengthTx
		}
		if depth == 0 {
			return iNdEx, nil
		}
	}
	return 0, io.ErrUnexpectedEOF
}

var (
	ErrInvalidLengthTx        = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowTx          = fmt.Errorf("proto: integer overflow")
	ErrUnexpectedEndOfGroupTx = fmt.Errorf("proto: unexpected end of group")
)

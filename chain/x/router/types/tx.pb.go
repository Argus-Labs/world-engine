// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: router/v1/tx.proto

package types

import (
	context "context"
	fmt "fmt"
	_ "github.com/cosmos/cosmos-proto"
	_ "github.com/cosmos/cosmos-sdk/types/msgservice"
	_ "github.com/cosmos/gogoproto/gogoproto"
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

// `UpdateNamespaceRequest` is the Msg/UpdateNamespace request type.
type UpdateNamespaceRequest struct {
	// authority is the address that controls the module (defaults to x/gov unless overwritten).
	Authority string     `protobuf:"bytes,1,opt,name=authority,proto3" json:"authority,omitempty"`
	Namespace *Namespace `protobuf:"bytes,2,opt,name=namespace,proto3" json:"namespace,omitempty"`
}

func (m *UpdateNamespaceRequest) Reset()         { *m = UpdateNamespaceRequest{} }
func (m *UpdateNamespaceRequest) String() string { return proto.CompactTextString(m) }
func (*UpdateNamespaceRequest) ProtoMessage()    {}
func (*UpdateNamespaceRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_51d72ccbaea415e4, []int{0}
}
func (m *UpdateNamespaceRequest) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *UpdateNamespaceRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_UpdateNamespaceRequest.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *UpdateNamespaceRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_UpdateNamespaceRequest.Merge(m, src)
}
func (m *UpdateNamespaceRequest) XXX_Size() int {
	return m.Size()
}
func (m *UpdateNamespaceRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_UpdateNamespaceRequest.DiscardUnknown(m)
}

var xxx_messageInfo_UpdateNamespaceRequest proto.InternalMessageInfo

func (m *UpdateNamespaceRequest) GetAuthority() string {
	if m != nil {
		return m.Authority
	}
	return ""
}

func (m *UpdateNamespaceRequest) GetNamespace() *Namespace {
	if m != nil {
		return m.Namespace
	}
	return nil
}

// `UpdateNamespaceResponse` defines the response structure for executing a UpdateNamespaceResponse message.
type UpdateNamespaceResponse struct {
}

func (m *UpdateNamespaceResponse) Reset()         { *m = UpdateNamespaceResponse{} }
func (m *UpdateNamespaceResponse) String() string { return proto.CompactTextString(m) }
func (*UpdateNamespaceResponse) ProtoMessage()    {}
func (*UpdateNamespaceResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_51d72ccbaea415e4, []int{1}
}
func (m *UpdateNamespaceResponse) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *UpdateNamespaceResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_UpdateNamespaceResponse.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *UpdateNamespaceResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_UpdateNamespaceResponse.Merge(m, src)
}
func (m *UpdateNamespaceResponse) XXX_Size() int {
	return m.Size()
}
func (m *UpdateNamespaceResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_UpdateNamespaceResponse.DiscardUnknown(m)
}

var xxx_messageInfo_UpdateNamespaceResponse proto.InternalMessageInfo

func init() {
	proto.RegisterType((*UpdateNamespaceRequest)(nil), "router.v1.UpdateNamespaceRequest")
	proto.RegisterType((*UpdateNamespaceResponse)(nil), "router.v1.UpdateNamespaceResponse")
}

func init() { proto.RegisterFile("router/v1/tx.proto", fileDescriptor_51d72ccbaea415e4) }

var fileDescriptor_51d72ccbaea415e4 = []byte{
	// 334 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x7c, 0x90, 0x41, 0x4b, 0x02, 0x41,
	0x14, 0xc7, 0x9d, 0xa2, 0x60, 0x27, 0x28, 0x58, 0x2c, 0x75, 0x0f, 0x8b, 0x79, 0x12, 0xc1, 0x1d,
	0x34, 0x28, 0xe8, 0x96, 0xf7, 0x82, 0x8c, 0x20, 0xba, 0xc4, 0xb8, 0x3b, 0x8d, 0x0b, 0xee, 0xce,
	0x3a, 0x6f, 0xc6, 0xf4, 0x16, 0x7d, 0x82, 0x0e, 0x7d, 0x10, 0x0f, 0x7d, 0x88, 0x8e, 0xd2, 0xa9,
	0x63, 0xb8, 0x07, 0xbf, 0x46, 0xe8, 0x6c, 0x6d, 0x54, 0x74, 0x9b, 0xf7, 0x7f, 0x3f, 0xde, 0x6f,
	0xde, 0xc3, 0xb6, 0x14, 0x5a, 0x31, 0x49, 0x46, 0x2d, 0xa2, 0xc6, 0x5e, 0x22, 0x85, 0x12, 0xb6,
	0x65, 0x32, 0x6f, 0xd4, 0x72, 0x4a, 0xbe, 0x80, 0x48, 0x00, 0x89, 0x80, 0x2f, 0x91, 0x08, 0xb8,
	0x61, 0x9c, 0x8a, 0x69, 0xdc, 0xac, 0x2a, 0x62, 0x8a, 0xac, 0x55, 0xe4, 0x82, 0x0b, 0x93, 0x2f,
	0x5f, 0x59, 0xba, 0x9b, 0x8b, 0x86, 0x9a, 0xc9, 0x89, 0x89, 0x6b, 0x4f, 0x08, 0xef, 0x5d, 0x26,
	0x01, 0x55, 0xec, 0x8c, 0x46, 0x0c, 0x12, 0xea, 0xb3, 0x2e, 0x1b, 0x6a, 0x06, 0xca, 0x3e, 0xc4,
	0x16, 0xd5, 0xaa, 0x2f, 0x64, 0xa8, 0x26, 0x65, 0x54, 0x45, 0x75, 0xab, 0x53, 0x7e, 0x7d, 0x6e,
	0x16, 0x33, 0xd9, 0x49, 0x10, 0x48, 0x06, 0x70, 0xa1, 0x64, 0x18, 0xf3, 0x6e, 0x8e, 0xda, 0x6d,
	0x6c, 0xc5, 0x9f, 0xb3, 0xca, 0x6b, 0x55, 0x54, 0xdf, 0x6a, 0x17, 0xbd, 0xaf, 0x95, 0xbc, 0xdc,
	0x93, 0x63, 0xc7, 0xdb, 0x0f, 0x8b, 0x69, 0x23, 0x9f, 0x51, 0xab, 0xe0, 0xd2, 0xaf, 0x5f, 0x41,
	0x22, 0x62, 0x60, 0xed, 0x5b, 0xbc, 0x7e, 0x0a, 0xdc, 0xbe, 0xc2, 0x3b, 0x3f, 0x08, 0x7b, 0xff,
	0x9b, 0xe5, 0xef, 0x9d, 0x9c, 0xda, 0x7f, 0x88, 0x11, 0x38, 0x1b, 0xf7, 0x8b, 0x69, 0x03, 0x75,
	0xce, 0x5f, 0xe6, 0x2e, 0x9a, 0xcd, 0x5d, 0xf4, 0x3e, 0x77, 0xd1, 0x63, 0xea, 0x16, 0x66, 0xa9,
	0x5b, 0x78, 0x4b, 0xdd, 0xc2, 0xf5, 0x11, 0x0f, 0x55, 0x5f, 0xf7, 0x3c, 0x5f, 0x44, 0x84, 0x4a,
	0xae, 0xa1, 0x39, 0xa0, 0x3d, 0x20, 0x77, 0x42, 0x0e, 0x82, 0x26, 0x8b, 0x79, 0x18, 0x33, 0xe2,
	0xf7, 0x69, 0x18, 0x93, 0x31, 0xc9, 0xae, 0xae, 0x26, 0x09, 0x83, 0xde, 0xe6, 0xea, 0xe6, 0x07,
	0x1f, 0x01, 0x00, 0x00, 0xff, 0xff, 0x59, 0x6b, 0xd5, 0x9d, 0xf5, 0x01, 0x00, 0x00,
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
	UpdateNamespace(ctx context.Context, in *UpdateNamespaceRequest, opts ...grpc.CallOption) (*UpdateNamespaceResponse, error)
}

type msgClient struct {
	cc grpc1.ClientConn
}

func NewMsgClient(cc grpc1.ClientConn) MsgClient {
	return &msgClient{cc}
}

func (c *msgClient) UpdateNamespace(ctx context.Context, in *UpdateNamespaceRequest, opts ...grpc.CallOption) (*UpdateNamespaceResponse, error) {
	out := new(UpdateNamespaceResponse)
	err := c.cc.Invoke(ctx, "/router.v1.Msg/UpdateNamespace", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// MsgServer is the server API for Msg service.
type MsgServer interface {
	UpdateNamespace(context.Context, *UpdateNamespaceRequest) (*UpdateNamespaceResponse, error)
}

// UnimplementedMsgServer can be embedded to have forward compatible implementations.
type UnimplementedMsgServer struct {
}

func (*UnimplementedMsgServer) UpdateNamespace(ctx context.Context, req *UpdateNamespaceRequest) (*UpdateNamespaceResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UpdateNamespace not implemented")
}

func RegisterMsgServer(s grpc1.Server, srv MsgServer) {
	s.RegisterService(&_Msg_serviceDesc, srv)
}

func _Msg_UpdateNamespace_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UpdateNamespaceRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MsgServer).UpdateNamespace(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/router.v1.Msg/UpdateNamespace",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MsgServer).UpdateNamespace(ctx, req.(*UpdateNamespaceRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _Msg_serviceDesc = grpc.ServiceDesc{
	ServiceName: "router.v1.Msg",
	HandlerType: (*MsgServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "UpdateNamespace",
			Handler:    _Msg_UpdateNamespace_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "router/v1/tx.proto",
}

func (m *UpdateNamespaceRequest) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *UpdateNamespaceRequest) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *UpdateNamespaceRequest) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.Namespace != nil {
		{
			size, err := m.Namespace.MarshalToSizedBuffer(dAtA[:i])
			if err != nil {
				return 0, err
			}
			i -= size
			i = encodeVarintTx(dAtA, i, uint64(size))
		}
		i--
		dAtA[i] = 0x12
	}
	if len(m.Authority) > 0 {
		i -= len(m.Authority)
		copy(dAtA[i:], m.Authority)
		i = encodeVarintTx(dAtA, i, uint64(len(m.Authority)))
		i--
		dAtA[i] = 0xa
	}
	return len(dAtA) - i, nil
}

func (m *UpdateNamespaceResponse) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *UpdateNamespaceResponse) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *UpdateNamespaceResponse) MarshalToSizedBuffer(dAtA []byte) (int, error) {
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
func (m *UpdateNamespaceRequest) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = len(m.Authority)
	if l > 0 {
		n += 1 + l + sovTx(uint64(l))
	}
	if m.Namespace != nil {
		l = m.Namespace.Size()
		n += 1 + l + sovTx(uint64(l))
	}
	return n
}

func (m *UpdateNamespaceResponse) Size() (n int) {
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
func (m *UpdateNamespaceRequest) Unmarshal(dAtA []byte) error {
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
			return fmt.Errorf("proto: UpdateNamespaceRequest: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: UpdateNamespaceRequest: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Authority", wireType)
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
			m.Authority = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Namespace", wireType)
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
			if m.Namespace == nil {
				m.Namespace = &Namespace{}
			}
			if err := m.Namespace.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
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
func (m *UpdateNamespaceResponse) Unmarshal(dAtA []byte) error {
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
			return fmt.Errorf("proto: UpdateNamespaceResponse: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: UpdateNamespaceResponse: illegal tag %d (wire type %d)", fieldNum, wire)
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

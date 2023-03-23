// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: argus/adapter/v1/tx.proto

package v1

import (
	context "context"
	fmt "fmt"
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

// MsgClaimQuestReward is the Msg/ClaimQuestReward request type.
type MsgClaimQuestReward struct {
	// user_ID is the game client user_ID.
	User_ID string `protobuf:"bytes,1,opt,name=user_ID,json=userID,proto3" json:"user_ID,omitempty"`
	// quest_ID is the ID of the quest that was completed.
	Quest_ID string `protobuf:"bytes,2,opt,name=quest_ID,json=questID,proto3" json:"quest_ID,omitempty"`
}

func (m *MsgClaimQuestReward) Reset()         { *m = MsgClaimQuestReward{} }
func (m *MsgClaimQuestReward) String() string { return proto.CompactTextString(m) }
func (*MsgClaimQuestReward) ProtoMessage()    {}
func (*MsgClaimQuestReward) Descriptor() ([]byte, []int) {
	return fileDescriptor_664ac58aa66c04f8, []int{0}
}
func (m *MsgClaimQuestReward) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *MsgClaimQuestReward) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_MsgClaimQuestReward.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *MsgClaimQuestReward) XXX_Merge(src proto.Message) {
	xxx_messageInfo_MsgClaimQuestReward.Merge(m, src)
}
func (m *MsgClaimQuestReward) XXX_Size() int {
	return m.Size()
}
func (m *MsgClaimQuestReward) XXX_DiscardUnknown() {
	xxx_messageInfo_MsgClaimQuestReward.DiscardUnknown(m)
}

var xxx_messageInfo_MsgClaimQuestReward proto.InternalMessageInfo

func (m *MsgClaimQuestReward) GetUser_ID() string {
	if m != nil {
		return m.User_ID
	}
	return ""
}

func (m *MsgClaimQuestReward) GetQuest_ID() string {
	if m != nil {
		return m.Quest_ID
	}
	return ""
}

// MsgClaimQuestRewardResponse is the Msg/ClaimQuestReward response type.
type MsgClaimQuestRewardResponse struct {
	// reward_ID is the ID of the reward claimed.
	Reward_ID string `protobuf:"bytes,1,opt,name=reward_ID,json=rewardID,proto3" json:"reward_ID,omitempty"`
}

func (m *MsgClaimQuestRewardResponse) Reset()         { *m = MsgClaimQuestRewardResponse{} }
func (m *MsgClaimQuestRewardResponse) String() string { return proto.CompactTextString(m) }
func (*MsgClaimQuestRewardResponse) ProtoMessage()    {}
func (*MsgClaimQuestRewardResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_664ac58aa66c04f8, []int{1}
}
func (m *MsgClaimQuestRewardResponse) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *MsgClaimQuestRewardResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_MsgClaimQuestRewardResponse.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *MsgClaimQuestRewardResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_MsgClaimQuestRewardResponse.Merge(m, src)
}
func (m *MsgClaimQuestRewardResponse) XXX_Size() int {
	return m.Size()
}
func (m *MsgClaimQuestRewardResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_MsgClaimQuestRewardResponse.DiscardUnknown(m)
}

var xxx_messageInfo_MsgClaimQuestRewardResponse proto.InternalMessageInfo

func (m *MsgClaimQuestRewardResponse) GetReward_ID() string {
	if m != nil {
		return m.Reward_ID
	}
	return ""
}

func init() {
	proto.RegisterType((*MsgClaimQuestReward)(nil), "argus.adapter.v1.MsgClaimQuestReward")
	proto.RegisterType((*MsgClaimQuestRewardResponse)(nil), "argus.adapter.v1.MsgClaimQuestRewardResponse")
}

func init() { proto.RegisterFile("argus/adapter/v1/tx.proto", fileDescriptor_664ac58aa66c04f8) }

var fileDescriptor_664ac58aa66c04f8 = []byte{
	// 246 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0x92, 0x4c, 0x2c, 0x4a, 0x2f,
	0x2d, 0xd6, 0x4f, 0x4c, 0x49, 0x2c, 0x28, 0x49, 0x2d, 0xd2, 0x2f, 0x33, 0xd4, 0x2f, 0xa9, 0xd0,
	0x2b, 0x28, 0xca, 0x2f, 0xc9, 0x17, 0x12, 0x00, 0x4b, 0xe9, 0x41, 0xa5, 0xf4, 0xca, 0x0c, 0x95,
	0x3c, 0xb9, 0x84, 0x7d, 0x8b, 0xd3, 0x9d, 0x73, 0x12, 0x33, 0x73, 0x03, 0x4b, 0x53, 0x8b, 0x4b,
	0x82, 0x52, 0xcb, 0x13, 0x8b, 0x52, 0x84, 0xc4, 0xb9, 0xd8, 0x4b, 0x8b, 0x53, 0x8b, 0xe2, 0x3d,
	0x5d, 0x24, 0x18, 0x15, 0x18, 0x35, 0x38, 0x83, 0xd8, 0x40, 0x5c, 0x4f, 0x17, 0x21, 0x49, 0x2e,
	0x8e, 0x42, 0x90, 0x3a, 0x90, 0x0c, 0x13, 0x58, 0x86, 0x1d, 0xcc, 0xf7, 0x74, 0x51, 0xb2, 0xe2,
	0x92, 0xc6, 0x62, 0x54, 0x50, 0x6a, 0x71, 0x41, 0x7e, 0x5e, 0x71, 0xaa, 0x90, 0x34, 0x17, 0x67,
	0x11, 0x58, 0x04, 0x61, 0x28, 0x07, 0x44, 0xc0, 0xd3, 0xc5, 0x28, 0x9f, 0x8b, 0xd9, 0xb7, 0x38,
	0x5d, 0x28, 0x83, 0x4b, 0x00, 0xc3, 0x29, 0xaa, 0x7a, 0xe8, 0x8e, 0xd6, 0xc3, 0x62, 0x8d, 0x94,
	0x2e, 0x51, 0xca, 0x60, 0xae, 0x71, 0xf2, 0x38, 0xf1, 0x48, 0x8e, 0xf1, 0xc2, 0x23, 0x39, 0xc6,
	0x07, 0x8f, 0xe4, 0x18, 0x27, 0x3c, 0x96, 0x63, 0xb8, 0xf0, 0x58, 0x8e, 0xe1, 0xc6, 0x63, 0x39,
	0x86, 0x28, 0xbd, 0xf4, 0xcc, 0x92, 0x8c, 0xd2, 0x24, 0xbd, 0xe4, 0xfc, 0x5c, 0x7d, 0xb0, 0x91,
	0xba, 0x39, 0x89, 0x49, 0xc5, 0x10, 0xa6, 0x7e, 0x05, 0x3c, 0x58, 0x4b, 0x2a, 0x0b, 0x52, 0x8b,
	0xf5, 0xcb, 0x0c, 0x93, 0xd8, 0xc0, 0x41, 0x6b, 0x0c, 0x08, 0x00, 0x00, 0xff, 0xff, 0x54, 0x86,
	0xb4, 0xb4, 0x77, 0x01, 0x00, 0x00,
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
	// ClaimQuestReward claims a quest reward.
	ClaimQuestReward(ctx context.Context, in *MsgClaimQuestReward, opts ...grpc.CallOption) (*MsgClaimQuestRewardResponse, error)
}

type msgClient struct {
	cc grpc1.ClientConn
}

func NewMsgClient(cc grpc1.ClientConn) MsgClient {
	return &msgClient{cc}
}

func (c *msgClient) ClaimQuestReward(ctx context.Context, in *MsgClaimQuestReward, opts ...grpc.CallOption) (*MsgClaimQuestRewardResponse, error) {
	out := new(MsgClaimQuestRewardResponse)
	err := c.cc.Invoke(ctx, "/argus.adapter.v1.Msg/ClaimQuestReward", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// MsgServer is the server API for Msg service.
type MsgServer interface {
	// ClaimQuestReward claims a quest reward.
	ClaimQuestReward(context.Context, *MsgClaimQuestReward) (*MsgClaimQuestRewardResponse, error)
}

// UnimplementedMsgServer can be embedded to have forward compatible implementations.
type UnimplementedMsgServer struct {
}

func (*UnimplementedMsgServer) ClaimQuestReward(ctx context.Context, req *MsgClaimQuestReward) (*MsgClaimQuestRewardResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ClaimQuestReward not implemented")
}

func RegisterMsgServer(s grpc1.Server, srv MsgServer) {
	s.RegisterService(&_Msg_serviceDesc, srv)
}

func _Msg_ClaimQuestReward_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(MsgClaimQuestReward)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MsgServer).ClaimQuestReward(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/argus.adapter.v1.Msg/ClaimQuestReward",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MsgServer).ClaimQuestReward(ctx, req.(*MsgClaimQuestReward))
	}
	return interceptor(ctx, in, info, handler)
}

var _Msg_serviceDesc = grpc.ServiceDesc{
	ServiceName: "argus.adapter.v1.Msg",
	HandlerType: (*MsgServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "ClaimQuestReward",
			Handler:    _Msg_ClaimQuestReward_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "argus/adapter/v1/tx.proto",
}

func (m *MsgClaimQuestReward) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *MsgClaimQuestReward) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *MsgClaimQuestReward) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if len(m.Quest_ID) > 0 {
		i -= len(m.Quest_ID)
		copy(dAtA[i:], m.Quest_ID)
		i = encodeVarintTx(dAtA, i, uint64(len(m.Quest_ID)))
		i--
		dAtA[i] = 0x12
	}
	if len(m.User_ID) > 0 {
		i -= len(m.User_ID)
		copy(dAtA[i:], m.User_ID)
		i = encodeVarintTx(dAtA, i, uint64(len(m.User_ID)))
		i--
		dAtA[i] = 0xa
	}
	return len(dAtA) - i, nil
}

func (m *MsgClaimQuestRewardResponse) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *MsgClaimQuestRewardResponse) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *MsgClaimQuestRewardResponse) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if len(m.Reward_ID) > 0 {
		i -= len(m.Reward_ID)
		copy(dAtA[i:], m.Reward_ID)
		i = encodeVarintTx(dAtA, i, uint64(len(m.Reward_ID)))
		i--
		dAtA[i] = 0xa
	}
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
func (m *MsgClaimQuestReward) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = len(m.User_ID)
	if l > 0 {
		n += 1 + l + sovTx(uint64(l))
	}
	l = len(m.Quest_ID)
	if l > 0 {
		n += 1 + l + sovTx(uint64(l))
	}
	return n
}

func (m *MsgClaimQuestRewardResponse) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = len(m.Reward_ID)
	if l > 0 {
		n += 1 + l + sovTx(uint64(l))
	}
	return n
}

func sovTx(x uint64) (n int) {
	return (math_bits.Len64(x|1) + 6) / 7
}
func sozTx(x uint64) (n int) {
	return sovTx(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *MsgClaimQuestReward) Unmarshal(dAtA []byte) error {
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
			return fmt.Errorf("proto: MsgClaimQuestReward: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: MsgClaimQuestReward: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field User_ID", wireType)
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
			m.User_ID = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Quest_ID", wireType)
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
			m.Quest_ID = string(dAtA[iNdEx:postIndex])
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
func (m *MsgClaimQuestRewardResponse) Unmarshal(dAtA []byte) error {
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
			return fmt.Errorf("proto: MsgClaimQuestRewardResponse: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: MsgClaimQuestRewardResponse: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Reward_ID", wireType)
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
			m.Reward_ID = string(dAtA[iNdEx:postIndex])
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

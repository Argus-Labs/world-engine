// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: shard/v1/types.proto

package types

import (
	fmt "fmt"
	proto "github.com/cosmos/gogoproto/proto"
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

type Transaction struct {
	// tx_id is the ID associated with the payloads below. This is needed so we know which transaction struct
	// to unmarshal the payload.Body into.
	TxId uint64 `protobuf:"varint,1,opt,name=tx_id,json=txId,proto3" json:"tx_id,omitempty"`
	// signed_payload is a proto encoded SignedPayload
	// (https://buf.build/argus-labs/world-engine/file/main:shard/v1/shard.proto#L21).
	SignedPayload []byte `protobuf:"bytes,2,opt,name=signed_payload,json=signedPayload,proto3" json:"signed_payload,omitempty"`
}

func (m *Transaction) Reset()         { *m = Transaction{} }
func (m *Transaction) String() string { return proto.CompactTextString(m) }
func (*Transaction) ProtoMessage()    {}
func (*Transaction) Descriptor() ([]byte, []int) {
	return fileDescriptor_0a60f84bb846c47b, []int{0}
}
func (m *Transaction) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *Transaction) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_Transaction.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *Transaction) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Transaction.Merge(m, src)
}
func (m *Transaction) XXX_Size() int {
	return m.Size()
}
func (m *Transaction) XXX_DiscardUnknown() {
	xxx_messageInfo_Transaction.DiscardUnknown(m)
}

var xxx_messageInfo_Transaction proto.InternalMessageInfo

func (m *Transaction) GetTxId() uint64 {
	if m != nil {
		return m.TxId
	}
	return 0
}

func (m *Transaction) GetSignedPayload() []byte {
	if m != nil {
		return m.SignedPayload
	}
	return nil
}

type Transactions struct {
	Txs []*Transaction `protobuf:"bytes,1,rep,name=txs,proto3" json:"txs,omitempty"`
}

func (m *Transactions) Reset()         { *m = Transactions{} }
func (m *Transactions) String() string { return proto.CompactTextString(m) }
func (*Transactions) ProtoMessage()    {}
func (*Transactions) Descriptor() ([]byte, []int) {
	return fileDescriptor_0a60f84bb846c47b, []int{1}
}
func (m *Transactions) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *Transactions) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_Transactions.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *Transactions) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Transactions.Merge(m, src)
}
func (m *Transactions) XXX_Size() int {
	return m.Size()
}
func (m *Transactions) XXX_DiscardUnknown() {
	xxx_messageInfo_Transactions.DiscardUnknown(m)
}

var xxx_messageInfo_Transactions proto.InternalMessageInfo

func (m *Transactions) GetTxs() []*Transaction {
	if m != nil {
		return m.Txs
	}
	return nil
}

// TickedTransactions contains a tick number, and the transactions that occurred in that tick.
type TickedTransactions struct {
	Tick uint64        `protobuf:"varint,1,opt,name=tick,proto3" json:"tick,omitempty"`
	Txs  *Transactions `protobuf:"bytes,2,opt,name=txs,proto3" json:"txs,omitempty"`
}

func (m *TickedTransactions) Reset()         { *m = TickedTransactions{} }
func (m *TickedTransactions) String() string { return proto.CompactTextString(m) }
func (*TickedTransactions) ProtoMessage()    {}
func (*TickedTransactions) Descriptor() ([]byte, []int) {
	return fileDescriptor_0a60f84bb846c47b, []int{2}
}
func (m *TickedTransactions) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *TickedTransactions) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_TickedTransactions.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *TickedTransactions) XXX_Merge(src proto.Message) {
	xxx_messageInfo_TickedTransactions.Merge(m, src)
}
func (m *TickedTransactions) XXX_Size() int {
	return m.Size()
}
func (m *TickedTransactions) XXX_DiscardUnknown() {
	xxx_messageInfo_TickedTransactions.DiscardUnknown(m)
}

var xxx_messageInfo_TickedTransactions proto.InternalMessageInfo

func (m *TickedTransactions) GetTick() uint64 {
	if m != nil {
		return m.Tick
	}
	return 0
}

func (m *TickedTransactions) GetTxs() *Transactions {
	if m != nil {
		return m.Txs
	}
	return nil
}

func init() {
	proto.RegisterType((*Transaction)(nil), "shard.v1.Transaction")
	proto.RegisterType((*Transactions)(nil), "shard.v1.Transactions")
	proto.RegisterType((*TickedTransactions)(nil), "shard.v1.TickedTransactions")
}

func init() { proto.RegisterFile("shard/v1/types.proto", fileDescriptor_0a60f84bb846c47b) }

var fileDescriptor_0a60f84bb846c47b = []byte{
	// 271 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x6c, 0x90, 0x4f, 0x4b, 0xc3, 0x30,
	0x18, 0xc6, 0x17, 0x57, 0x45, 0xd2, 0xe9, 0x21, 0xfe, 0xa1, 0xa7, 0x50, 0x0a, 0x62, 0x2f, 0x6b,
	0xd8, 0x04, 0xbd, 0x7b, 0xdb, 0x6d, 0x94, 0x9d, 0xbc, 0x8c, 0x34, 0x09, 0x6d, 0x68, 0x4d, 0x4a,
	0x92, 0xcd, 0xee, 0x5b, 0xf8, 0xb1, 0x3c, 0xee, 0xe8, 0x51, 0xda, 0x2f, 0x22, 0x6b, 0x51, 0x26,
	0x78, 0x7d, 0x78, 0xde, 0xdf, 0xf3, 0xf2, 0x83, 0xd7, 0xb6, 0xa0, 0x86, 0x93, 0xed, 0x8c, 0xb8,
	0x5d, 0x2d, 0x6c, 0x52, 0x1b, 0xed, 0x34, 0x3a, 0xef, 0xd3, 0x64, 0x3b, 0x8b, 0x16, 0xd0, 0x5f,
	0x19, 0xaa, 0x2c, 0x65, 0x4e, 0x6a, 0x85, 0xae, 0xe0, 0xa9, 0x6b, 0xd6, 0x92, 0x07, 0x20, 0x04,
	0xb1, 0x97, 0x7a, 0xae, 0x59, 0x70, 0x74, 0x07, 0x2f, 0xad, 0xcc, 0x95, 0xe0, 0xeb, 0x9a, 0xee,
	0x2a, 0x4d, 0x79, 0x70, 0x12, 0x82, 0x78, 0x92, 0x5e, 0x0c, 0xe9, 0x72, 0x08, 0xa3, 0x27, 0x38,
	0x39, 0x42, 0x59, 0x74, 0x0f, 0xc7, 0xae, 0xb1, 0x01, 0x08, 0xc7, 0xb1, 0x3f, 0xbf, 0x49, 0x7e,
	0x26, 0x93, 0xa3, 0x52, 0x7a, 0x68, 0x44, 0x29, 0x44, 0x2b, 0xc9, 0x4a, 0xc1, 0xff, 0x9c, 0x23,
	0xe8, 0x39, 0xc9, 0xca, 0xdf, 0x4f, 0x24, 0x2b, 0x51, 0x3c, 0x20, 0x0f, 0xf3, 0xfe, 0xfc, 0xf6,
	0x5f, 0xa4, 0xed, 0x99, 0xcf, 0xcb, 0x8f, 0x16, 0x83, 0x7d, 0x8b, 0xc1, 0x57, 0x8b, 0xc1, 0x7b,
	0x87, 0x47, 0xfb, 0x0e, 0x8f, 0x3e, 0x3b, 0x3c, 0x7a, 0x79, 0xcc, 0xa5, 0x2b, 0x36, 0x59, 0xc2,
	0xf4, 0x2b, 0xa1, 0x26, 0xdf, 0xd8, 0x69, 0x45, 0x33, 0x4b, 0xde, 0xb4, 0xa9, 0xf8, 0x54, 0xa8,
	0x5c, 0x2a, 0x41, 0x58, 0x41, 0xa5, 0x22, 0x0d, 0x19, 0xe4, 0xf5, 0xe6, 0xb2, 0xb3, 0x5e, 0xdd,
	0xc3, 0x77, 0x00, 0x00, 0x00, 0xff, 0xff, 0xbd, 0xde, 0x45, 0xa3, 0x52, 0x01, 0x00, 0x00,
}

func (m *Transaction) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Transaction) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *Transaction) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if len(m.SignedPayload) > 0 {
		i -= len(m.SignedPayload)
		copy(dAtA[i:], m.SignedPayload)
		i = encodeVarintTypes(dAtA, i, uint64(len(m.SignedPayload)))
		i--
		dAtA[i] = 0x12
	}
	if m.TxId != 0 {
		i = encodeVarintTypes(dAtA, i, uint64(m.TxId))
		i--
		dAtA[i] = 0x8
	}
	return len(dAtA) - i, nil
}

func (m *Transactions) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Transactions) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *Transactions) MarshalToSizedBuffer(dAtA []byte) (int, error) {
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
				i = encodeVarintTypes(dAtA, i, uint64(size))
			}
			i--
			dAtA[i] = 0xa
		}
	}
	return len(dAtA) - i, nil
}

func (m *TickedTransactions) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *TickedTransactions) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *TickedTransactions) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.Txs != nil {
		{
			size, err := m.Txs.MarshalToSizedBuffer(dAtA[:i])
			if err != nil {
				return 0, err
			}
			i -= size
			i = encodeVarintTypes(dAtA, i, uint64(size))
		}
		i--
		dAtA[i] = 0x12
	}
	if m.Tick != 0 {
		i = encodeVarintTypes(dAtA, i, uint64(m.Tick))
		i--
		dAtA[i] = 0x8
	}
	return len(dAtA) - i, nil
}

func encodeVarintTypes(dAtA []byte, offset int, v uint64) int {
	offset -= sovTypes(v)
	base := offset
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return base
}
func (m *Transaction) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.TxId != 0 {
		n += 1 + sovTypes(uint64(m.TxId))
	}
	l = len(m.SignedPayload)
	if l > 0 {
		n += 1 + l + sovTypes(uint64(l))
	}
	return n
}

func (m *Transactions) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if len(m.Txs) > 0 {
		for _, e := range m.Txs {
			l = e.Size()
			n += 1 + l + sovTypes(uint64(l))
		}
	}
	return n
}

func (m *TickedTransactions) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.Tick != 0 {
		n += 1 + sovTypes(uint64(m.Tick))
	}
	if m.Txs != nil {
		l = m.Txs.Size()
		n += 1 + l + sovTypes(uint64(l))
	}
	return n
}

func sovTypes(x uint64) (n int) {
	return (math_bits.Len64(x|1) + 6) / 7
}
func sozTypes(x uint64) (n int) {
	return sovTypes(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *Transaction) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowTypes
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
			return fmt.Errorf("proto: Transaction: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Transaction: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field TxId", wireType)
			}
			m.TxId = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTypes
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.TxId |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field SignedPayload", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTypes
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
				return ErrInvalidLengthTypes
			}
			postIndex := iNdEx + byteLen
			if postIndex < 0 {
				return ErrInvalidLengthTypes
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.SignedPayload = append(m.SignedPayload[:0], dAtA[iNdEx:postIndex]...)
			if m.SignedPayload == nil {
				m.SignedPayload = []byte{}
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipTypes(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthTypes
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
func (m *Transactions) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowTypes
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
			return fmt.Errorf("proto: Transactions: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Transactions: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Txs", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTypes
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
				return ErrInvalidLengthTypes
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthTypes
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
			skippy, err := skipTypes(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthTypes
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
func (m *TickedTransactions) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowTypes
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
			return fmt.Errorf("proto: TickedTransactions: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: TickedTransactions: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Tick", wireType)
			}
			m.Tick = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTypes
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Tick |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Txs", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTypes
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
				return ErrInvalidLengthTypes
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthTypes
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Txs == nil {
				m.Txs = &Transactions{}
			}
			if err := m.Txs.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipTypes(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthTypes
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
func skipTypes(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	depth := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowTypes
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
					return 0, ErrIntOverflowTypes
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
					return 0, ErrIntOverflowTypes
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
				return 0, ErrInvalidLengthTypes
			}
			iNdEx += length
		case 3:
			depth++
		case 4:
			if depth == 0 {
				return 0, ErrUnexpectedEndOfGroupTypes
			}
			depth--
		case 5:
			iNdEx += 4
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
		if iNdEx < 0 {
			return 0, ErrInvalidLengthTypes
		}
		if depth == 0 {
			return iNdEx, nil
		}
	}
	return 0, io.ErrUnexpectedEOF
}

var (
	ErrInvalidLengthTypes        = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowTypes          = fmt.Errorf("proto: integer overflow")
	ErrUnexpectedEndOfGroupTypes = fmt.Errorf("proto: unexpected end of group")
)

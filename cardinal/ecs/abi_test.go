package ecs

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"math/big"
	"testing"
)

// TestNoTagPanics tests that it panics when a struct field is of type *big.Int and does not have a `solidity` struct
// field tag.
func TestNoTagPanics(t *testing.T) {
	type FooReadBroken struct {
		Large *big.Int
	}
	_, err := GenerateABIType(FooReadBroken{})
	assert.Error(t, err)

	type FooReadBrokenSlice struct {
		SliceBig []*big.Int
	}

	_, err = GenerateABIType(FooReadBrokenSlice{})
	assert.Error(t, err)
}

func TestGenerateABIType_AllValidTypes(t *testing.T) {
	type SomeOtherType struct {
		X uint64
		Y int64
		Z common.Address
		D *big.Int `evm:"int128"`
	}
	type BigType struct {
		Uint8      uint8
		Uint32     uint32
		Uint64     uint64
		SliceUint8 []uint8

		Int8       int8
		Int64      int64
		SliceInt64 []int64

		String      string
		SliceString []string

		Bool      bool
		SliceBool []bool

		Address      common.Address
		SliceAddress []common.Address

		Bytes []byte

		BigInt      *big.Int   `evm:"uint256"`
		SliceBigInt []*big.Int `evm:"int256"`

		SomeOtherStruct SomeOtherType

		SliceOfSomeOtherStruct []SomeOtherType
	}
	at, err := GenerateABIType(BigType{})
	assert.Nil(t, err)
	args := abi.Arguments{{Type: *at}}

	b := BigType{
		Uint8:        30,
		Uint32:       22,
		Uint64:       39,
		SliceUint8:   []uint8{2, 3, 1},
		Int8:         32,
		Int64:        11,
		SliceInt64:   []int64{4, 56},
		String:       "hello world",
		SliceString:  []string{"hello", "world"},
		Bool:         true,
		SliceBool:    []bool{true, false, true},
		Address:      common.BigToAddress(big.NewInt(40502305)),
		SliceAddress: []common.Address{common.BigToAddress(big.NewInt(42)), common.BigToAddress(big.NewInt(3250235))},
		Bytes:        []byte("hello"),
		BigInt:       big.NewInt(3520),
		SliceBigInt:  []*big.Int{big.NewInt(32), big.NewInt(40)},
		SomeOtherStruct: SomeOtherType{
			X: 40,
			Y: 50,
			Z: common.BigToAddress(big.NewInt(234235)),
			D: big.NewInt(3884),
		},
		SliceOfSomeOtherStruct: []SomeOtherType{
			{
				X: 32,
				Y: 3252,
				Z: common.BigToAddress(big.NewInt(1248)),
				D: big.NewInt(44),
			},
		},
	}
	bz, err := args.Pack(b)
	assert.Nil(t, err)

	unpacked, err := args.Unpack(bz)
	assert.Nil(t, err)
	assert.Len(t, unpacked, 1)

	got, err := SerdeInto[BigType](unpacked[0])
	assert.Nil(t, err)
	assert.Equal(t, got, b)
}

func TestGenerateABIType_PanicOnImportedTypes(t *testing.T) {
	type InvalidType struct {
		X common.Decimal
	}
	_, err := GenerateABIType(InvalidType{})
	assert.Error(t, err)
}

func TestGenerateABIType_NoSizeOnInt(t *testing.T) {
	type InvalidUint struct {
		Uint uint
	}

	type InvalidInt struct {
		Int int
	}

	_, err := GenerateABIType(InvalidUint{})
	assert.Error(t, err)

	_, err = GenerateABIType(InvalidInt{})
	assert.Error(t, err)
}

func TestGenerateABIType_StructInStruct(t *testing.T) {
	type Bar struct {
		HelloWorld uint64 `json:"HelloWorld"`
	}

	type Foo struct {
		Y uint64
		B Bar
	}
	foo := Foo{32, Bar{42}}
	a, err := GenerateABIType(foo)
	assert.Nil(t, err)
	args := abi.Arguments{{Type: *a}}
	bz, err := args.Pack(foo)
	assert.Nil(t, err)

	unpacked, err := args.Unpack(bz)
	assert.Nil(t, err)
	assert.Len(t, unpacked, 1)

	got, err := SerdeInto[Foo](unpacked[0])
	assert.Nil(t, err)

	assert.Equal(t, got, foo)
}

func TestGenerateABIType_SliceOfStructInStruct(t *testing.T) {
	type Bar struct {
		HelloWorld uint64 `json:"HelloWorld"`
	}

	type Foo struct {
		Y uint64
		B []Bar
	}
	foo := Foo{32, []Bar{{899789}}}
	a, err := GenerateABIType(foo)
	assert.Nil(t, err)
	args := abi.Arguments{{Type: *a}}
	bz, err := args.Pack(foo)
	assert.Nil(t, err)

	unpacked, err := args.Unpack(bz)
	assert.Nil(t, err)
	assert.Len(t, unpacked, 1)

	underlyingFoo, err := SerdeInto[Foo](unpacked[0])
	assert.Nil(t, err)

	assert.Equal(t, underlyingFoo, foo)
}

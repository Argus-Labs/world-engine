package ecs

import (
	"github.com/ethereum/go-ethereum/common"
	propAssert "github.com/magiconair/properties/assert"
	"gotest.tools/v3/assert"
	"math/big"
	"testing"
)

type FooReply struct {
	Hi bool
}

// TestNoTagPanics tests that it panics when a struct field is of type *big.Int and does not have a `solidity` struct
// field tag.
func TestNoTagPanics(t *testing.T) {
	type FooReadBroken struct {
		Large *big.Int
	}
	propAssert.Panic(t, func() {
		NewReadType[FooReadBroken, FooReply]("foo", nil, true)
	}, ".*")

}

func TestWorksWithTag(t *testing.T) {
	type FooReadWorks struct {
		Large *big.Int `solidity:"uint256"`
	}
	read := NewReadType[FooReadWorks, FooReply]("foo", nil, true)

	_, err := read.EncodeAsABI(FooReadWorks{big.NewInt(300000000)})
	assert.NilError(t, err, nil)
}

func TestAddrWorks(t *testing.T) {
	type FooReadAddr struct {
		Addr common.Address
	}
	read := NewReadType[FooReadAddr, FooReply]("foo", nil, true)
	_, err := read.EncodeAsABI(FooReadAddr{common.HexToAddress("0x6265617665726275696c642e6f7267")})
	assert.NilError(t, err)
}

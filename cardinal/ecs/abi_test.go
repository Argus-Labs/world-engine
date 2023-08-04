package ecs

import (
	"github.com/ethereum/go-ethereum/common"
	"math/big"
	"testing"
)

func TestThing(t *testing.T) {
	type Foo struct {
		Address        common.Address
		Big            *big.Int
		SliceOfUint64  []uint64
		SliceOfBigInt  []*big.Int
		String         string
		Bool           bool
		SliceOfAddress []common.Address
	}
}

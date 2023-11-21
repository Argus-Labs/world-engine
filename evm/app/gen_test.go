package app

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
	"math"
	"math/big"
	"testing"

	"pkg.berachain.dev/polaris/eth/core"
)

func TestFoo(t *testing.T) {
	addr := common.HexToAddress("0xaa9288F88233Eb887d194fF2215Cf1776a6FEE41")
	gen := core.Genesis{}
	gen.Alloc = core.GenesisAlloc{
		addr: core.GenesisAccount{Balance: big.NewInt(math.MaxInt64)},
	}

	bz, err := gen.MarshalJSON()
	require.NoError(t, err)
	fmt.Println(string(bz))
}

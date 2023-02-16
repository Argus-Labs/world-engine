package evm_test

import (
	"math/big"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/suite"

	argus "github.com/argus-labs/argus/app"
)

type PrecompileSuite struct {
	suite.Suite

	ctx     sdk.Context
	handler sdk.Handler
	app     *argus.ArgusApp
	codec   codec.Codec
	chainID *big.Int

	signer    keyring.Signer
	ethSigner ethtypes.Signer
	from      common.Address
	to        sdk.AccAddress

	dynamicTxFee bool
}

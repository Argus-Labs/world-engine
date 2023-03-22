package argus

import (
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	sdkcryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/x/auth/tx"

	"github.com/cosmos/cosmos-sdk/types/module"

	"github.com/argus-labs/argus/app/simparams"
)

// MakeEncodingConfig creates the application encoding configuration.
func MakeEncodingConfig(mb module.BasicManager) simparams.EncodingConfig {
	cdc := codec.NewLegacyAmino()
	ir := types.NewInterfaceRegistry()
	protoCodec := codec.NewProtoCodec(ir)

	cryptocodec.RegisterInterfaces(ir)
	sdkcryptocodec.RegisterInterfaces(ir)

	txConfig := tx.NewTxConfig(protoCodec, tx.DefaultSignModes)

	mb.RegisterLegacyAminoCodec(cdc)
	mb.RegisterInterfaces(ir)

	return simparams.EncodingConfig{
		InterfaceRegistry: ir,
		Codec:             protoCodec,
		TxConfig:          txConfig,
		Amino:             cdc,
	}
}

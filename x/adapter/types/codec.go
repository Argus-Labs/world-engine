package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"

	"github.com/argus-labs/argus/x/adapter/types/v1"
)

var (
	amino     = codec.NewLegacyAmino()
	ModuleCdc = codec.NewAminoCodec(amino)
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(v1.MsgClaimQuestReward{}, "adapter/MsgClaimQuestReward", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	msgservice.RegisterMsgServiceDesc(registry, &v1._Msg_serviceDesc)
	registry.RegisterImplementations(
		(*sdk.Msg)(nil),
		&v1.MsgClaimQuestReward{},
	)
}

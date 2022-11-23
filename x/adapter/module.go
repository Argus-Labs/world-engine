package adapter

import (
	"encoding/json"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/spf13/cobra"
	abci "github.com/tendermint/tendermint/abci/types"

	adapterkeeper "github.com/argus-labs/argus/x/adapter/keeper"
	"github.com/argus-labs/argus/x/adapter/types/v1"
)

const (
	Name             = "adapter"
	StoreKey         = Name
	RouterKey        = Name
	QuerierRoute     = Name
	ConsensusVersion = uint64(1)
)

var (
	_ module.AppModule      = AppModule{}
	_ module.AppModuleBasic = AppModuleBasic{}
)

// AppModuleBasic implements the AppModuleBasic interface for the capability module.
type AppModuleBasic struct {
	cdc codec.Codec
}

// AppModule implements the AppModule interface for the capability module.
type AppModule struct {
	AppModuleBasic

	k adapterkeeper.Keeper
}

func NewAppModule(cdc codec.Codec, k adapterkeeper.Keeper) AppModule {
	return AppModule{
		AppModuleBasic: AppModuleBasic{cdc},
		k:              k,
	}
}

func (a AppModule) RegisterGRPCGatewayRoutes(clientCtx client.Context, mux *runtime.ServeMux) {}

func (a AppModule) InitGenesis(context sdk.Context, jsonCodec codec.JSONCodec, message json.RawMessage) []abci.ValidatorUpdate {
	return []abci.ValidatorUpdate{}
}

func (a AppModule) ExportGenesis(context sdk.Context, jsonCodec codec.JSONCodec) json.RawMessage {
	return nil
}

func (a AppModule) RegisterInvariants(registry sdk.InvariantRegistry) {}

func (a AppModule) Route() sdk.Route {
	return sdk.NewRoute(Name, nil)
}

func (a AppModule) QuerierRoute() string {
	return Name
}

func (a AppModule) LegacyQuerierHandler(amino *codec.LegacyAmino) sdk.Querier {
	return nil
}

func (a AppModule) RegisterServices(cfg module.Configurator) {
	v1.RegisterMsgServer(cfg.MsgServer(), a.k)
}

func (a AppModule) ConsensusVersion() uint64 {
	return ConsensusVersion
}

func (a AppModuleBasic) Name() string {
	return Name
}

func (a AppModuleBasic) RegisterLegacyAminoCodec(amino *codec.LegacyAmino) {
	v1.RegisterCodec(amino)
}

func (a AppModuleBasic) RegisterInterfaces(registry types.InterfaceRegistry) {
	v1.RegisterInterfaces(registry)
}

func (a AppModuleBasic) DefaultGenesis(jsonCodec codec.JSONCodec) json.RawMessage {
	return nil
}

func (a AppModuleBasic) ValidateGenesis(jsonCodec codec.JSONCodec, config client.TxEncodingConfig, message json.RawMessage) error {
	return nil
}

func (a AppModuleBasic) RegisterGRPCGatewayRoutes(context client.Context, mux *runtime.ServeMux) {}

func (a AppModuleBasic) GetTxCmd() *cobra.Command {
	return nil
}

func (a AppModuleBasic) GetQueryCmd() *cobra.Command {
	return nil
}

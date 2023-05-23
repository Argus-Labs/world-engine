package router

import (
	gwruntime "github.com/grpc-ecosystem/grpc-gateway/runtime"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	types2 "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/spf13/cobra"

	"github.com/argus-labs/world-engine/chain/x/router/keeper"
	routertypes "github.com/argus-labs/world-engine/chain/x/router/types"
)

const (
	Name             = "Router"
	ConsensusVersion = 1
)

var (
	_ module.HasServices    = AppModule{}
	_ module.AppModuleBasic = AppModuleBasic{}
)

// ==============================================================================
// AppModuleBasic
// ==============================================================================

// AppModuleBasic defines the basic application module used by the evm module.
type AppModuleBasic struct{}

// Name returns the evm module's name.
func (AppModuleBasic) Name() string {
	return Name
}

// RegisterLegacyAminoCodec registers the evm module's types on the given LegacyAmino codec.
func (AppModuleBasic) RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	// types.RegisterLegacyAminoCodec(cdc)
}

// RegisterInterfaces registers the module's interface types.
func (b AppModuleBasic) RegisterInterfaces(r types2.InterfaceRegistry) {
	routertypes.RegisterInterfaces(r)
}

// RegisterGRPCGatewayRoutes registers the gRPC Gateway routes for the evm module.
func (AppModuleBasic) RegisterGRPCGatewayRoutes(clientCtx client.Context, mux *gwruntime.ServeMux) {
	// if err := types.RegisterQueryServiceHandlerClient(context.Background(), mux,
	// types.NewQueryClient(clientCtx)); err != nil {
	// 	panic(err)
	// }
}

// GetTxCmd returns no root tx command for the evm module.
func (AppModuleBasic) GetTxCmd() *cobra.Command {
	return nil
}

// GetQueryCmd returns the root query command for the evm module.
func (AppModuleBasic) GetQueryCmd() *cobra.Command {
	return nil
}

// ==============================================================================
// AppModule
// ==============================================================================

// AppModule implements an application module for the evm module.
type AppModule struct {
	AppModuleBasic
	keeper *keeper.Keeper
}

// NewAppModule creates a new AppModule object.
func NewAppModule(
	keeper *keeper.Keeper,
) AppModule {
	return AppModule{
		AppModuleBasic: AppModuleBasic{},
		keeper:         keeper,
	}
}

// IsOnePerModuleType implements the depinject.OnePerModuleType interface.
func (am AppModule) IsOnePerModuleType() {}

// IsAppModule implements the appmodule.AppModule interface.
func (am AppModule) IsAppModule() {}

// RegisterInvariants registers the evm module invariants.
func (am AppModule) RegisterInvariants(_ sdk.InvariantRegistry) {}

// RegisterServices registers a gRPC query service to respond to the
// module-specific gRPC queries.
func (am AppModule) RegisterServices(cfg module.Configurator) {
	// TODO: register the services
	routertypes.RegisterMsgServiceServer(cfg.MsgServer(), am.keeper)
	routertypes.RegisterQueryServiceServer(cfg.QueryServer(), am.keeper)
	// types.RegisterMsgServiceServer(cfg.MsgServer(), am.keeper)
	// types.RegisterQueryServer(cfg.QueryServer(), am.keeper)
}

// ConsensusVersion implements AppModule/ConsensusVersion.
func (AppModule) ConsensusVersion() uint64 { return ConsensusVersion }

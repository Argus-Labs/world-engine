package namespace

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	gwruntime "github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/spf13/cobra"

	"pkg.world.dev/world-engine/evm/x/namespace/cli/query"
	"pkg.world.dev/world-engine/evm/x/namespace/cli/tx"
	"pkg.world.dev/world-engine/evm/x/namespace/keeper"
	namespacetypes "pkg.world.dev/world-engine/evm/x/namespace/types"
)

const (
	ConsensusVersion = 1
)

var (
	_ module.HasServices    = AppModule{}
	_ module.AppModuleBasic = AppModuleBasic{}
	_ module.HasGenesis     = AppModule{}
)

// ==============================================================================
// AppModuleBasic
// ==============================================================================

// AppModuleBasic defines the basic application module used by the module.
type AppModuleBasic struct{}

// Name returns the module's name.
func (AppModuleBasic) Name() string {
	return namespacetypes.ModuleName
}

// RegisterLegacyAminoCodec registers the module's types on the given LegacyAmino codec.
func (AppModuleBasic) RegisterLegacyAminoCodec(_ *codec.LegacyAmino) {
	// types.RegisterLegacyAminoCodec(cdc)
}

// RegisterInterfaces registers the module's interface types.
func (b AppModuleBasic) RegisterInterfaces(r codectypes.InterfaceRegistry) {
	namespacetypes.RegisterInterfaces(r)
}

// RegisterGRPCGatewayRoutes registers the gRPC Gateway routes for the module.
func (AppModuleBasic) RegisterGRPCGatewayRoutes(ctx client.Context, mux *gwruntime.ServeMux) {
	err := namespacetypes.RegisterQueryServiceHandlerClient(
		context.Background(),
		mux,
		namespacetypes.NewQueryServiceClient(ctx),
	)
	if err != nil {
		panic(err)
	}
}

// GetTxCmd returns no root tx command for the module.
func (AppModuleBasic) GetTxCmd() *cobra.Command {
	return tx.NewTxCmd()
}

// GetQueryCmd returns the root query command for the module.
func (AppModuleBasic) GetQueryCmd() *cobra.Command {
	return query.NewQueryCmd()
}

// ==============================================================================
// AppModule
// ==============================================================================

// AppModule implements an application module for the module.
type AppModule struct { //nolint:decorder
	AppModuleBasic
	keeper *keeper.Keeper
}

func (am AppModule) DefaultGenesis(cdc codec.JSONCodec) json.RawMessage {
	return cdc.MustMarshalJSON(namespacetypes.DefaultGenesis())
}

func (am AppModule) ValidateGenesis(cdc codec.JSONCodec, _ client.TxEncodingConfig, bz json.RawMessage) error {
	var g namespacetypes.Genesis
	if err := cdc.UnmarshalJSON(bz, &g); err != nil {
		return fmt.Errorf("failed to unmarshal %s genesis state: %w", namespacetypes.ModuleName, err)
	}
	return g.Validate()
}

func (am AppModule) InitGenesis(
	ctx sdk.Context,
	cdc codec.JSONCodec,
	bz json.RawMessage,
) {
	var g namespacetypes.Genesis
	cdc.MustUnmarshalJSON(bz, &g)
	am.keeper.InitGenesis(ctx, &g)
}

func (am AppModule) ExportGenesis(ctx sdk.Context, cdc codec.JSONCodec) json.RawMessage {
	g := am.keeper.ExportGenesis(ctx)
	return cdc.MustMarshalJSON(g)
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

// RegisterInvariants registers the module invariants.
func (am AppModule) RegisterInvariants(_ sdk.InvariantRegistry) {}

// RegisterServices registers a gRPC query service to respond to the
// module-specific gRPC queries.
func (am AppModule) RegisterServices(cfg module.Configurator) {
	namespacetypes.RegisterMsgServer(cfg.MsgServer(), am.keeper)
	namespacetypes.RegisterQueryServiceServer(cfg.QueryServer(), am.keeper)
}

// ConsensusVersion implements AppModule/ConsensusVersion.
func (AppModule) ConsensusVersion() uint64 { return ConsensusVersion }

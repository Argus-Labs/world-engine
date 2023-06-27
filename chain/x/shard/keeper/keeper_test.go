package keeper_test

import (
	storetypes "cosmossdk.io/store/types"
	"github.com/argus-labs/world-engine/chain/x/shard/keeper"
	"github.com/argus-labs/world-engine/chain/x/shard/module"
	"github.com/argus-labs/world-engine/chain/x/shard/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	cmttime "github.com/cometbft/cometbft/types/time"
	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/testutil"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/stretchr/testify/suite"
	"testing"
)

type TestSuite struct {
	suite.Suite
	ctx    sdk.Context
	addrs  []sdk.AccAddress
	auth   string
	keeper *keeper.Keeper
	encCfg moduletestutil.TestEncodingConfig
}

func (s *TestSuite) SetupTest() {
	s.addrs = simtestutil.CreateIncrementalAccounts(3)
	s.auth = s.addrs[0].String()
	s.encCfg = moduletestutil.MakeTestEncodingConfig(module.AppModuleBasic{})
	key := storetypes.NewKVStoreKey(module.ModuleName)
	storeService := runtime.NewKVStoreService(key)
	testCtx := testutil.DefaultContextWithDB(s.T(), key, storetypes.NewTransientStoreKey("transient_test"))
	ctx := testCtx.Ctx.WithBlockHeader(cmtproto.Header{Time: cmttime.Now()})
	shardKeeper := keeper.NewKeeper(storeService, s.auth)
	s.keeper = shardKeeper
	s.ctx = ctx
}

func (s *TestSuite) TestSubmitBatch() {
	batch := []byte("hello world")
	res, err := s.keeper.SubmitBatch(s.ctx, &types.SubmitBatchRequest{
		Sender: s.auth,
		Batch:  batch,
	})
	s.Require().NoError(err)
	s.Require().NotNil(res)
	newBatch := []byte("goodbye world")
	res, err = s.keeper.SubmitBatch(s.ctx, &types.SubmitBatchRequest{
		Sender: s.auth,
		Batch:  newBatch,
	})
	s.Require().NoError(err)
	s.Require().NotNil(res)
	genesis := s.keeper.ExportGenesis(s.ctx)
	s.Require().Len(genesis.Batches, 2)
	s.Require().Equal(genesis.Batches[0], batch)
	s.Require().Equal(genesis.Batches[1], newBatch)
	s.Require().Equal(genesis.Index, uint64(2))
}

func (s *TestSuite) TestSubmitBatch_Unauthorized() {
	_, err := s.keeper.SubmitBatch(s.ctx, &types.SubmitBatchRequest{
		Sender: s.addrs[1].String(),
		Batch:  []byte("foo"),
	})
	s.Require().ErrorIs(err, sdkerrors.ErrUnauthorized)
}

func TestTestSuite(t *testing.T) {
	suite.Run(t, new(TestSuite))
}

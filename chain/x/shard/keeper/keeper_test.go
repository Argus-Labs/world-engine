package keeper_test

import (
	"testing"

	storetypes "cosmossdk.io/store/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	cmttime "github.com/cometbft/cometbft/types/time"
	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/testutil"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/stretchr/testify/suite"

	"github.com/argus-labs/world-engine/chain/x/shard/keeper"
	"github.com/argus-labs/world-engine/chain/x/shard/module"
	"github.com/argus-labs/world-engine/chain/x/shard/types"
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
	batch := &types.TransactionBatch{
		Namespace: "cardinal1",
		Tick:      420,
		Batch:     []byte("data"),
	}
	res, err := s.keeper.SubmitBatch(s.ctx, &types.SubmitBatchRequest{
		Sender:           s.auth,
		TransactionBatch: batch,
	})
	s.Require().NoError(err)
	s.Require().NotNil(res)
	newBatch := &types.TransactionBatch{
		Namespace: "cardinal2",
		Tick:      320,
		Batch:     []byte("data2"),
	}
	res, err = s.keeper.SubmitBatch(s.ctx, &types.SubmitBatchRequest{
		Sender:           s.auth,
		TransactionBatch: newBatch,
	})
	s.Require().NoError(err)
	s.Require().NotNil(res)
	genesis := s.keeper.ExportGenesis(s.ctx)
	s.Require().Len(genesis.Batches, 2)
	s.Require().Equal(*genesis.Batches[0], *batch)
	s.Require().Equal(*genesis.Batches[1], *newBatch)
}

func (s *TestSuite) TestSubmitBatch_Unauthorized() {
	_, err := s.keeper.SubmitBatch(s.ctx, &types.SubmitBatchRequest{
		Sender: s.addrs[1].String(),
		TransactionBatch: &types.TransactionBatch{
			Namespace: "cardinal",
			Tick:      420,
			Batch:     []byte("some data"),
		},
	})
	s.Require().ErrorIs(err, sdkerrors.ErrUnauthorized)
}

func (s *TestSuite) TestQueryBatches() {
	ns := "cardinal1"
	batches := []*types.TransactionBatch{
		{
			Namespace: ns,
			Tick:      42,
			Batch:     []byte("x"),
		},
		{
			Namespace: ns,
			Tick:      43,
			Batch:     []byte("y"),
		},
		{
			Namespace: ns,
			Tick:      44,
			Batch:     []byte("z"),
		},
	}
	for _, batch := range batches {
		_, err := s.keeper.SubmitBatch(s.ctx, &types.SubmitBatchRequest{
			Sender:           s.auth,
			TransactionBatch: batch,
		})
		s.Require().NoError(err)
	}

	// submit one not relevant to our namespace.
	_, err := s.keeper.SubmitBatch(s.ctx, &types.SubmitBatchRequest{
		Sender: s.auth,
		TransactionBatch: &types.TransactionBatch{
			Namespace: "notcardinal",
			Tick:      32,
			Batch:     []byte("foo"),
		},
	})
	s.Require().NoError(err)

	res, err := s.keeper.Batches(s.ctx, &types.QueryBatchesRequest{
		Namespace: ns,
		Page:      nil,
	})
	s.Require().NoError(err)
	s.Require().Len(res.Batches, len(batches))

	// limit the request to only 2.
	limit := uint32(2)
	res, err = s.keeper.Batches(s.ctx, &types.QueryBatchesRequest{
		Namespace: ns,
		Page: &types.PageRequest{
			Key:   nil,
			Limit: limit,
		},
	})
	s.Require().NoError(err)
	s.Require().Len(res.Batches, int(limit))

	res, err = s.keeper.Batches(s.ctx, &types.QueryBatchesRequest{
		Namespace: ns,
		Page: &types.PageRequest{
			Key: res.Page.Key,
		},
	})
	s.Require().NoError(err)
	s.Require().Len(res.Batches, len(batches)-int(limit))
}

// TestSubmitBatch_DuplicateTick tests that when duplicate ticks are submitted, the data is overwritten.
func (s *TestSuite) TestSubmitBatch_DuplicateTick() {
	batch := &types.TransactionBatch{
		Namespace: "cardinal",
		Tick:      4,
		Batch:     []byte("data"),
	}

	_, err := s.keeper.SubmitBatch(s.ctx, &types.SubmitBatchRequest{
		Sender:           s.auth,
		TransactionBatch: batch,
	})
	s.Require().NoError(err)

	// change the data
	batch.Batch = []byte("different data")
	_, err = s.keeper.SubmitBatch(s.ctx, &types.SubmitBatchRequest{
		Sender:           s.auth,
		TransactionBatch: batch,
	})
	s.Require().ErrorContains(err, "already submitted")
}

func TestTestSuite(t *testing.T) {
	suite.Run(t, new(TestSuite))
}

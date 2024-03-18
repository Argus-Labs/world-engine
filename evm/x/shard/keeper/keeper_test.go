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
	"google.golang.org/protobuf/proto"

	"pkg.world.dev/world-engine/evm/x/shard"
	"pkg.world.dev/world-engine/evm/x/shard/keeper"
	"pkg.world.dev/world-engine/evm/x/shard/types"
	shardv1 "pkg.world.dev/world-engine/rift/shard/v1"
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
	s.encCfg = moduletestutil.MakeTestEncodingConfig(shard.AppModuleBasic{})
	key := storetypes.NewKVStoreKey(shard.ModuleName)
	storeService := runtime.NewKVStoreService(key)
	testCtx := testutil.DefaultContextWithDB(s.T(), key, storetypes.NewTransientStoreKey("transient_test"))
	ctx := testCtx.Ctx.WithBlockHeader(cmtproto.Header{Time: cmttime.Now()})
	shardKeeper := keeper.NewKeeper(storeService, s.auth)
	s.keeper = shardKeeper
	s.ctx = ctx
}

func (s *TestSuite) TestSubmitTransactions() {
	epoch := uint64(2)
	tx := &shardv1.Transaction{
		PersonaTag: "meow",
		Namespace:  "darkforest-west1",
		Nonce:      1,
		Signature:  "0xfooooooooo",
		Body:       []byte("transaction"),
	}
	txBz, err := proto.Marshal(tx)
	s.Require().NoError(err)
	txs := []*types.Transaction{
		{3, txBz},
		{4, txBz},
	}
	_, err = s.keeper.SubmitShardTx(
		s.ctx,
		&types.SubmitShardTxRequest{
			Sender:    s.auth,
			Namespace: tx.GetNamespace(),
			Epoch:     epoch,
			Txs:       txs,
		},
	)
	s.Require().NoError(err)

	// submit some transactions for a different namespace..
	_, err = s.keeper.SubmitShardTx(
		s.ctx,
		&types.SubmitShardTxRequest{
			Sender:    s.auth,
			Namespace: "foo",
			Epoch:     epoch,
			Txs: []*types.Transaction{
				{3, txBz},
				{4, txBz},
			},
		},
	)
	s.Require().NoError(err)

	res, err := s.keeper.Transactions(s.ctx, &types.QueryTransactionsRequest{Namespace: tx.GetNamespace()})
	s.Require().NoError(err)
	// we only submitted transactions for 1 epoch, so there should only be 1.
	s.Require().Len(res.Epochs, 1)
	// should have equal amount of txs within the epoch.
	s.Require().Len(res.Epochs[0].Txs, len(txs))
}

func (s *TestSuite) TestPagedQueryTransactions() {
	epoch := uint64(15)
	tx := &shardv1.Transaction{
		PersonaTag: "ffzz",
		Namespace:  "somegame-west1",
		Nonce:      3,
		Signature:  "0xfooooooooo",
		Body:       []byte("txtxtx"),
	}
	txBz, err := proto.Marshal(tx)
	s.Require().NoError(err)
	txs := []*types.Transaction{
		{1, txBz},
		{4, txBz},
	}
	_, err = s.keeper.SubmitShardTx(
		s.ctx,
		&types.SubmitShardTxRequest{
			Sender:    s.auth,
			Namespace: tx.GetNamespace(),
			Epoch:     epoch,
			Txs:       txs,
		},
	)
	s.Require().NoError(err)
	_, err = s.keeper.SubmitShardTx(
		s.ctx,
		&types.SubmitShardTxRequest{
			Sender:    s.auth,
			Namespace: tx.GetNamespace(),
			Epoch:     epoch + 1,
			Txs:       txs,
		},
	)
	s.Require().NoError(err)

	// ensure limiting works
	res, err := s.keeper.Transactions(s.ctx, &types.QueryTransactionsRequest{
		Namespace: tx.GetNamespace(),
		Page: &types.PageRequest{
			Key:   nil,
			Limit: 1,
		},
	})
	s.Require().NoError(err)
	s.Require().Len(res.Epochs, 1)

	// ensure that no page returns both epochs
	res, err = s.keeper.Transactions(s.ctx, &types.QueryTransactionsRequest{
		Namespace: tx.GetNamespace(),
		Page:      nil,
	})
	s.Require().NoError(err)
	s.Require().Len(res.Epochs, 2)
}

func (s *TestSuite) TestSubmitBatch_Unauthorized() {
	_, err := s.keeper.SubmitShardTx(s.ctx, &types.SubmitShardTxRequest{
		Sender:    s.addrs[1].String(),
		Namespace: "foo",
		Epoch:     4,
		Txs:       nil,
	})
	s.Require().ErrorIs(err, sdkerrors.ErrUnauthorized)
}

func (s *TestSuite) TestExportGenesis() {
	submit1 := &types.SubmitShardTxRequest{
		Sender:    s.auth,
		Namespace: "foo",
		Epoch:     1,
		Txs: []*types.Transaction{
			{1, []byte("foo")},
			{10, []byte("bar")},
			{1, []byte("baz")},
		},
	}

	submit2 := &types.SubmitShardTxRequest{
		Sender:    s.auth,
		Namespace: "bar",
		Epoch:     3,
		Txs: []*types.Transaction{
			{15, []byte("qux")},
			{2, []byte("quiz")},
		},
	}

	submit3 := &types.SubmitShardTxRequest{
		Sender:    s.auth,
		Namespace: "foo",
		Epoch:     2,
		Txs: []*types.Transaction{
			{4, []byte("qux")},
			{9, []byte("quiz")},
		},
	}

	reqs := []*types.SubmitShardTxRequest{submit1, submit2, submit3}
	for _, req := range reqs {
		_, err := s.keeper.SubmitShardTx(s.ctx, req)
		s.Require().NoError(err)
	}

	gen := s.keeper.ExportGenesis(s.ctx)
	// there should only be 2 namespaced txs, because we only submitted 2 diff ones.
	s.Require().Len(gen.NamespaceTransactions, 2)
	s.Require().Len(gen.NamespaceTransactions[1].Epochs, 2)        // we submitted 2 epochs for namespace foo
	s.Require().Len(gen.NamespaceTransactions[1].Epochs[0].Txs, 3) // the first epoch had 3 txs
	s.Require().Len(gen.NamespaceTransactions[1].Epochs[1].Txs, 2) // the second epoch had 2 txs

	s.Require().Len(gen.NamespaceTransactions[0].Epochs, 1)        // only one epoch under namespace "bar"
	s.Require().Len(gen.NamespaceTransactions[0].Epochs[0].Txs, 2) // only 2 txs in the epoch.

	// importing back the genesis should not panic
	s.Require().NotPanics(func() {
		s.keeper.InitGenesis(s.ctx, gen)
	})
}

func TestTestSuite(t *testing.T) {
	suite.Run(t, new(TestSuite))
}

package keeper_test

import (
	shardv1 "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/shard/v1"
	"github.com/argus-labs/world-engine/chain/x/shard"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"google.golang.org/protobuf/proto"
	"testing"

	storetypes "cosmossdk.io/store/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	cmttime "github.com/cometbft/cometbft/types/time"
	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/testutil"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/stretchr/testify/suite"

	"github.com/argus-labs/world-engine/chain/x/shard/keeper"
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
	tick := uint64(2)
	sp := &shardv1.SignedPayload{
		PersonaTag: "meow",
		Namespace:  "darkforest-west1",
		Nonce:      1,
		Signature:  "0xfooooooooo",
		Body:       []byte("transaction"),
	}
	signedPayloadBz, err := proto.Marshal(sp)
	s.Require().NoError(err)
	txs := &types.Transactions{Txs: []*types.Transaction{
		{3, signedPayloadBz},
		{4, signedPayloadBz},
	}}
	_, err = s.keeper.SubmitCardinalTx(
		s.ctx,
		&types.SubmitCardinalTxRequest{
			Sender:    s.auth,
			Namespace: sp.Namespace,
			Tick:      tick,
			Txs:       txs,
		},
	)
	s.Require().NoError(err)

	// submit some transactions for a different namespace..
	_, err = s.keeper.SubmitCardinalTx(
		s.ctx,
		&types.SubmitCardinalTxRequest{
			Sender:    s.auth,
			Namespace: "foo",
			Tick:      tick,
			Txs: &types.Transactions{Txs: []*types.Transaction{
				{3, signedPayloadBz},
				{4, signedPayloadBz},
			}},
		},
	)
	s.Require().NoError(err)

	res, err := s.keeper.Transactions(s.ctx, &types.QueryTransactionsRequest{Namespace: sp.Namespace})
	s.Require().NoError(err)
	// we only submitted transactions for 1 tick, so there should only be 1.
	s.Require().Len(res.Txs, 1)
	// should have equal amount of txs within the tick.
	s.Require().Len(res.Txs[0].Txs.Txs, len(txs.Txs))
}

func (s *TestSuite) TestSubmitBatch_Unauthorized() {
	_, err := s.keeper.SubmitCardinalTx(s.ctx, &types.SubmitCardinalTxRequest{
		Sender:    s.addrs[1].String(),
		Namespace: "foo",
		Tick:      4,
		Txs:       nil,
	})
	s.Require().ErrorIs(err, sdkerrors.ErrUnauthorized)
}

func (s *TestSuite) TestExportGenesis() {
	submit1 := &types.SubmitCardinalTxRequest{
		Sender:    s.auth,
		Namespace: "foo",
		Tick:      1,
		Txs: &types.Transactions{Txs: []*types.Transaction{
			{1, []byte("foo")},
			{10, []byte("bar")},
			{1, []byte("baz")},
		}},
	}

	submit2 := &types.SubmitCardinalTxRequest{
		Sender:    s.auth,
		Namespace: "bar",
		Tick:      3,
		Txs: &types.Transactions{Txs: []*types.Transaction{
			{15, []byte("qux")},
			{2, []byte("quiz")},
		}},
	}

	submit3 := &types.SubmitCardinalTxRequest{
		Sender:    s.auth,
		Namespace: "foo",
		Tick:      2,
		Txs: &types.Transactions{Txs: []*types.Transaction{
			{4, []byte("qux")},
			{9, []byte("quiz")},
		}},
	}

	reqs := []*types.SubmitCardinalTxRequest{submit1, submit2, submit3}
	for _, req := range reqs {
		_, err := s.keeper.SubmitCardinalTx(s.ctx, req)
		s.Require().NoError(err)
	}

	gen := s.keeper.ExportGenesis(s.ctx)
	// there should only be 2 namespaced txs, because we only submitted 2 diff ones.
	s.Require().Len(gen.Txs, 2)
	s.Require().Len(gen.Txs[1].Txs, 2)            // we submitted 2 ticks for namespace "foo"
	s.Require().Len(gen.Txs[1].Txs[0].Txs.Txs, 3) // the first tick had 3 txs
	s.Require().Len(gen.Txs[1].Txs[1].Txs.Txs, 2) // the second tick had 2 txs

	s.Require().Len(gen.Txs[0].Txs, 1)            // only one tick under namespace "bar"
	s.Require().Len(gen.Txs[0].Txs[0].Txs.Txs, 2) // only 2 txs in the tick.

	// importing back the genesis should not panic
	s.Require().NotPanics(func() {
		s.keeper.InitGenesis(s.ctx, gen)
	})
}

func TestTestSuite(t *testing.T) {
	suite.Run(t, new(TestSuite))
}

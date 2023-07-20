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

func (s *TestSuite) TestSubmitBatch() {
	tx := &shardv1.SignedPayload{
		PersonaTag: "meow",
		Namespace:  "darkforest-west1",
		Nonce:      1,
		Signature:  "0xfooooooooo",
		Body:       []byte("transaction"),
	}
	bz, err := proto.Marshal(tx)
	s.Require().NoError(err)
	res, err := s.keeper.SubmitCardinalTx(s.ctx, &types.SubmitCardinalTxRequest{
		Sender: s.auth, CardinalTx: bz,
	})
	s.Require().NoError(err)
	s.Require().NotNil(res)
	newTx := &shardv1.SignedPayload{
		PersonaTag: "bark",
		Namespace:  "darkforest-east1",
		Nonce:      13,
		Signature:  "0xbarrrrrrrrrr",
		Body:       []byte("some tx"),
	}
	bz, err = proto.Marshal(newTx)
	s.Require().NoError(err)
	res, err = s.keeper.SubmitCardinalTx(s.ctx, &types.SubmitCardinalTxRequest{
		Sender:     s.auth,
		CardinalTx: bz,
	})
	s.Require().NoError(err)
	s.Require().NotNil(res)

	genesis := s.keeper.ExportGenesis(s.ctx)
	s.Require().Len(genesis.Transactions, 2)
	// check the darkforest-east1 transactions.. should only be 1.
	s.Require().Len(genesis.Transactions[0].Txs, 1)
	// now check the tx was saved properly
	gotTx := new(shardv1.SignedPayload)
	err = proto.Unmarshal(genesis.Transactions[0].Txs[0], gotTx)
	s.Require().NoError(err)
	s.Require().True(proto.Equal(gotTx, newTx))

	// now lets check darkforest-west1 transactions
	s.Require().Len(genesis.Transactions[1].Txs, 1)
	// now check the tx was saved properly
	gotTx = new(shardv1.SignedPayload)
	err = proto.Unmarshal(genesis.Transactions[1].Txs[0], gotTx)
	s.Require().NoError(err)
	s.Require().True(proto.Equal(gotTx, tx))
}

func (s *TestSuite) TestSubmitBatch_Unauthorized() {
	_, err := s.keeper.SubmitCardinalTx(s.ctx, &types.SubmitCardinalTxRequest{
		Sender:     s.addrs[1].String(),
		CardinalTx: nil,
	})
	s.Require().ErrorIs(err, sdkerrors.ErrUnauthorized)
}

func (s *TestSuite) TestQueryBatches() {
	ns := "cardinal1"
	transactions := []*shardv1.SignedPayload{
		{
			PersonaTag: "dark_mage1",
			Namespace:  ns,
			Nonce:      3,
			Signature:  "0xfoo",
			Body:       []byte("tx_data"),
		},
		{
			PersonaTag: "dark_mage2",
			Namespace:  ns,
			Nonce:      4,
			Signature:  "0xfoo1",
			Body:       []byte("tx_data"),
		},
		{
			PersonaTag: "dark_mage3",
			Namespace:  ns,
			Nonce:      5,
			Signature:  "0xfoo2",
			Body:       []byte("tx_data"),
		},
	}
	for _, tx := range transactions {
		bz, err := proto.Marshal(tx)
		s.Require().NoError(err)
		_, err = s.keeper.SubmitCardinalTx(s.ctx, &types.SubmitCardinalTxRequest{
			Sender:     s.auth,
			CardinalTx: bz,
		})
		s.Require().NoError(err)
	}

	otherTx := &shardv1.SignedPayload{
		PersonaTag: "dark_mage2",
		Namespace:  "darkforest-east1",
		Nonce:      3,
		Signature:  "0xfoo",
		Body:       []byte("tx_data"),
	}
	bz, err := proto.Marshal(otherTx)
	// submit one not relevant to our namespace.
	_, err = s.keeper.SubmitCardinalTx(s.ctx, &types.SubmitCardinalTxRequest{
		Sender:     s.auth,
		CardinalTx: bz,
	})
	s.Require().NoError(err)

	res, err := s.keeper.Transactions(s.ctx, &types.QueryTransactionsRequest{
		Namespace: ns,
		Page:      nil,
	})
	s.Require().NoError(err)
	s.Require().Len(res.Transactions, len(transactions))

	// limit the request to only 2.
	limit := uint32(2)
	res, err = s.keeper.Transactions(s.ctx, &types.QueryTransactionsRequest{
		Namespace: ns,
		Page: &types.PageRequest{
			Key:   nil,
			Limit: limit,
		},
	})
	s.Require().NoError(err)
	// should only have received 2.
	s.Require().Len(res.Transactions, int(limit))

	// query again with the key in page response should give us the remaining batch.
	res, err = s.keeper.Transactions(s.ctx, &types.QueryTransactionsRequest{
		Namespace: ns,
		Page: &types.PageRequest{
			Key: res.Page.Key,
		},
	})
	s.Require().NoError(err)
	s.Require().Len(res.Transactions, len(transactions)-int(limit))
}

func TestTestSuite(t *testing.T) {
	suite.Run(t, new(TestSuite))
}

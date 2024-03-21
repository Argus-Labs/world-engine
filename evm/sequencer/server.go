package sequencer

import (
	"context"
	"net"
	"os"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/gogoproto/proto"
	"github.com/rotisserie/eris"
	zerolog "github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	namespacetypes "pkg.world.dev/world-engine/evm/x/namespace/types"
	"pkg.world.dev/world-engine/evm/x/shard/types"
	shard "pkg.world.dev/world-engine/rift/shard/v2"
)

const (
	defaultPort = "9601"
)

var (
	Name                                = "shard_sequencer"
	_    shard.TransactionHandlerServer = &Sequencer{}
)

// Sequencer handles sequencing game shard transactions.
type Sequencer struct {
	shard.UnimplementedTransactionHandlerServer
	moduleAddr sdk.AccAddress
	tq         *TxQueue

	// opts
	key string
}

// NewShardSequencer returns a new game shardsequencer server. It runs on a default port of 9601,
// unless the SHARD_SEQUENCER_PORT environment variable is set.
//
// The sequencer exposes a single gRPC endpoint, SubmitShardTx, which will take in transactions from game shards,
// indexed by namespace. At every block, the sequencer tx queue is flushed, and processed in the storage shard storage
// module, persisting the data to the blockchain.
func NewShardSequencer(opts ...Option) *Sequencer {
	addr := authtypes.NewModuleAddress(Name)
	s := &Sequencer{
		moduleAddr: addr,
		tq:         NewTxQueue(addr.String()),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Serve serves the server in a new go routine.
func (s *Sequencer) Serve() {
	grpcServer := grpc.NewServer(grpc.Creds(s.creds))
	shard.RegisterTransactionHandlerServer(grpcServer, s)
	port := defaultPort
	// check if a custom port was set
	if setPort := os.Getenv("SHARD_SEQUENCER_PORT"); setPort != "" {
		if _, err := strconv.Atoi(setPort); err == nil {
			port = setPort
		}
	}
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		zerolog.Fatal().Err(err).Msg("game shard sequencer failed to open listener")
	}
	go func() {
		err = grpcServer.Serve(listener)
		if err != nil {
			zerolog.Fatal().Err(err).Msg("game shard sequencer failed to serve grpc server")
		}
	}()
}

// FlushMessages empties and returns all messages stored in the queue.
func (s *Sequencer) FlushMessages() ([]*types.SubmitShardTxRequest, []*namespacetypes.UpdateNamespaceRequest) {
	return s.tq.FlushTxQueue(), s.tq.FlushInitQueue()
}

// Submit appends the game shard tx submission to the tx queue.
func (s *Sequencer) Submit(_ context.Context, req *shard.SubmitTransactionsRequest) (
	*shard.SubmitTransactionsResponse, error,
) {
	txIDs := sortMapKeys(req.GetTransactions())
	for _, txID := range txIDs {
		txs := req.GetTransactions()[txID].GetTxs()
		for _, tx := range txs {
			bz, err := proto.Marshal(tx)
			if err != nil {
				return nil, eris.Wrap(err, "failed to marshal transaction")
			}
			s.tq.AddTx(req.GetNamespace(), req.GetEpoch(), req.GetUnixTimestamp(), txID, bz)
		}
	}
	return &shard.SubmitTransactionsResponse{}, nil
}

func (s *Sequencer) RegisterGameShard(
	_ context.Context,
	req *shard.RegisterGameShardRequest,
) (*shard.RegisterGameShardResponse, error) {
	s.tq.AddInitMsg(req.GetNamespace(), req.GetRouterAddress())
	return &shard.RegisterGameShardResponse{}, nil
}

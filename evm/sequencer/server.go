package sequencer

import (
	"context"
	"crypto/tls"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/gogoproto/proto"
	"github.com/rotisserie/eris"
	zerolog "github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"net"
	"os"
	"pkg.world.dev/world-engine/evm/x/shard/types"
	shard "pkg.world.dev/world-engine/rift/shard/v2"
	"strconv"
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
	creds credentials.TransportCredentials
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

func loadCredentials(certPath, keyPath string) (credentials.TransportCredentials, error) {
	// Load server's certificate and private key
	sc, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	sk, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	serverCert, err := tls.X509KeyPair(sc, sk)
	if err != nil {
		return nil, err
	}

	// Create the credentials and return it
	config := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.NoClientCert,
	}

	return credentials.NewTLS(config), nil
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
func (s *Sequencer) FlushMessages() []*types.SubmitShardTxRequest {
	return s.tq.GetTxs()
}

// Submit appends the game shard tx submission to the tx queue.
func (s *Sequencer) Submit(_ context.Context, req *shard.SubmitTransactionsRequest) (
	*shard.SubmitTransactionsResponse, error) {
	zerolog.Logger.Info().Msgf("got transaction from shard: %s", req.Namespace)
	txIDs := sortMapKeys(req.Transactions)
	for _, txID := range txIDs {
		txs := req.Transactions[txID].Txs
		for _, tx := range txs {
			bz, err := proto.Marshal(tx)
			if err != nil {
				return nil, eris.Wrap(err, "failed to marshal transaction")
			}
			s.tq.AddTx(req.Namespace, req.Epoch, txID, bz)
		}

	}
	return &shard.SubmitTransactionsResponse{}, nil
}

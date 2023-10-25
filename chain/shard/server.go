package shard

import (
	shardgrpc "buf.build/gen/go/argus-labs/world-engine/grpc/go/shard/v1/shardv1grpc"
	shard "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/shard/v1"
	"context"
	"crypto/tls"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	zerolog "github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/proto"
	"net"
	"os"
	"strconv"
	"sync"

	"pkg.world.dev/world-engine/chain/x/shard/types"
)

const (
	defaultPort = "9601"
)

var (
	Name                              = "shard_sequencer"
	_    shardgrpc.ShardHandlerServer = &Sequencer{}
)

// Sequencer handles sequencing game shard transactions.
type Sequencer struct {
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
		tq: &TxQueue{
			lock:       sync.Mutex{},
			ntx:        make(NamespacedTxs),
			outbox:     make([]*types.SubmitShardTxRequest, 0),
			moduleAddr: addr.String(),
		},
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
	shardgrpc.RegisterShardHandlerServer(grpcServer, s)
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

// SubmitShardTx appends the game shard tx submission to the tx queue.
func (s *Sequencer) SubmitShardTx(_ context.Context, req *shard.SubmitShardTxRequest) (
	*shard.SubmitShardTxResponse, error) {
	zerolog.Logger.Info().Msgf("got transaction from shard: %s", req.Tx.Namespace)
	bz, err := proto.Marshal(req.Tx)
	if err != nil {
		return nil, err
	}

	s.tq.AddTx(req.Tx.Namespace, req.Epoch, req.TxId, bz)

	return &shard.SubmitShardTxResponse{}, nil
}

package shard

import (
	shardgrpc "buf.build/gen/go/argus-labs/world-engine/grpc/go/shard/v1/shardv1grpc"
	shard "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/shard/v1"
	"context"
	"crypto/tls"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/proto"
	"log"
	"net"
	"os"
	"sync"

	"github.com/argus-labs/world-engine/chain/x/shard/types"
)

var (
	Name                              = "shard_handler_server"
	_    shardgrpc.ShardHandlerServer = &Server{}
)

type Server struct {
	moduleAddr sdk.AccAddress
	tq         *TxQueue
	serverOpts []grpc.ServerOption
}

func NewShardServer(opts ...Option) *Server {
	addr := authtypes.NewModuleAddress(Name)
	s := &Server{
		moduleAddr: addr,
		tq: &TxQueue{
			lock:       sync.Mutex{},
			ntx:        make(NamespacedTxs, 0),
			outbox:     make([]*types.SubmitCardinalTxRequest, 0),
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

// Serve serves the application in a new go routine. The routine panics if serve fails.
func (s *Server) Serve(listenAddr string) {
	grpcServer := grpc.NewServer(s.serverOpts...)
	shardgrpc.RegisterShardHandlerServer(grpcServer, s)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		err = grpcServer.Serve(listener)
		if err != nil {
			panic(err)
		}
	}()
}

// FlushMessages gets the next available batch of transactions ready to be stored.
func (s *Server) FlushMessages() []*types.SubmitCardinalTxRequest {
	return s.tq.GetTxs()
}

// SubmitCardinalTx appends the cardinal tx submission to the tx queue, which eventually gets executed during
// abci.EndBlock
func (s *Server) SubmitCardinalTx(_ context.Context, req *shard.SubmitCardinalTxRequest) (
	*shard.SubmitCardinalTxResponse, error) {

	bz, err := proto.Marshal(req.Tx)
	if err != nil {
		return nil, err
	}

	s.tq.AddTx(req.Tx.Namespace, req.Tick, req.TxId, bz)

	return &shard.SubmitCardinalTxResponse{}, nil
}

package evm

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/argus-labs/world-engine/cardinal/ecs"
	"net"
	"os"

	"github.com/argus-labs/world-engine/cardinal/ecs/transaction"
	"github.com/argus-labs/world-engine/sign"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"buf.build/gen/go/argus-labs/world-engine/grpc/go/router/v1/routerv1grpc"
	"buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/router/v1"
)

var (
	_ routerv1grpc.MsgServer = &srv{}
)

// ITransactionTypes maps transaction type ID's to transaction types.
type ITransactionTypes map[transaction.TypeID]transaction.ITransaction

// IReadTypes maps read resource names to the underlying IRead
type IReadTypes map[string]ecs.IRead

var _ IWorld = &ecs.World{}

// IWorld is a type that gives access to transaction and read data in the ecs.World, as well as access to queue
// transactions.
type IWorld interface {
	AddTransaction(transaction.TypeID, any, *sign.SignedPayload) int
	ListTransactions() ([]transaction.ITransaction, error)
	ListReads() []ecs.IRead
}

type srv struct {
	txMap      ITransactionTypes
	readMap    IReadTypes
	world      *ecs.World
	serverOpts []grpc.ServerOption
}

func NewServer(w *ecs.World, opts ...Option) (routerv1grpc.MsgServer, error) {
	// setup txs
	txs, err := w.ListTransactions()
	if err != nil {
		return nil, err
	}
	it := make(ITransactionTypes, len(txs))
	for _, tx := range txs {
		it[tx.ID()] = tx
	}

	reads := w.ListReads()
	ir := make(IReadTypes, len(reads))
	for _, r := range reads {
		ir[r.Name()] = r
	}

	s := &srv{txMap: it, readMap: ir, world: w}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

func loadCredentials(serverCertPath, serverKeyPath string) (credentials.TransportCredentials, error) {
	// Load server's certificate and private key
	sc, err := os.ReadFile(serverCertPath)
	if err != nil {
		return nil, err
	}
	sk, err := os.ReadFile(serverKeyPath)
	if err != nil {
		return nil, err
	}
	serverCert, err := tls.X509KeyPair(sc, sk)
	if err != nil {
		return nil, err
	}

	// Create the credentials and return txMap
	config := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.NoClientCert,
	}

	return credentials.NewTLS(config), nil
}

// Serve serves the application in a new go routine.
func (s *srv) Serve(addr string) error {
	server := grpc.NewServer(s.serverOpts...)
	routerv1grpc.RegisterMsgServer(server, s)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	go func() {
		err = server.Serve(listener)
		if err != nil {
			panic(err)
		}
	}()
	return nil
}

func (s *srv) SendMsg(ctx context.Context, msg *routerv1.MsgSend) (*routerv1.MsgSendResponse, error) {
	// first we check if we can extract the transaction associated with the id
	itx, ok := s.txMap[transaction.TypeID(msg.MessageId)]
	if !ok {
		return nil, fmt.Errorf("no transaction with ID %d is registerd in this world", msg.MessageId)
	}
	// decode the evm bytes into the transaction
	tx, err := itx.DecodeEVMBytes(msg.Message)
	if err != nil {
		return nil, err
	}
	// add transaction to the world queue
	s.world.AddTransaction(itx.ID(), tx, nil)
	return &routerv1.MsgSendResponse{}, nil
}

func (s *srv) QueryShard(ctx context.Context, req *routerv1.QueryShardRequest) (*routerv1.QueryShardResponse, error) {
	read, ok := s.readMap[req.Resource]
	if !ok {
		return nil, fmt.Errorf("no read with name %s found", req.Resource)
	}
	ecsRequest, err := read.DecodeEVMRequest(req.Request)
	if err != nil {
		return nil, err
	}
	reply, err := read.HandleRead(s.world, ecsRequest)
	if err != nil {
		return nil, err
	}
	bz, err := read.EncodeEVMReply(reply)
	if err != nil {
		return nil, err
	}
	return &routerv1.QueryShardResponse{Response: bz}, nil
}

package evm

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/argus-labs/world-engine/cardinal/ecs/transaction"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"net"
	"os"

	"buf.build/gen/go/argus-labs/world-engine/grpc/go/router/v1/routerv1grpc"
	"buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/router/v1"
)

var (
	_ routerv1grpc.MsgServer = &srv{}
)

// ITransactionTypes is a map that maps transaction type ID's to transaction types.
type ITransactionTypes map[transaction.TypeID]transaction.ITransaction

// TxHandler is a type that gives access to transaction data in the ecs.World, as well as access to queue transactions.
type TxHandler interface {
	AddTransaction(transaction.TypeID, any)
	ListTransactions() ([]transaction.ITransaction, error)
}

type srv struct {
	it    ITransactionTypes
	txh   TxHandler
	creds credentials.TransportCredentials
}

func NewServer(txh TxHandler, opts ...Option) (routerv1grpc.MsgServer, error) {
	txs, err := txh.ListTransactions()
	if err != nil {
		return nil, err
	}
	it := make(ITransactionTypes, len(txs))
	for _, tx := range txs {
		it[tx.ID()] = tx
	}
	s := &srv{it: it, txh: txh}
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

	// Create the credentials and return it
	config := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.NoClientCert,
	}

	return credentials.NewTLS(config), nil
}

// Serve serves the application in a new go routine.
func (s *srv) Serve(addr string) error {
	server := grpc.NewServer(grpc.Creds(s.creds))
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
	itx, ok := s.it[transaction.TypeID(msg.MessageId)]
	if !ok {
		return nil, fmt.Errorf("no transaction with ID %d is registerd in this world", msg.MessageId)
	}
	// decode the evm bytes into the transaction
	tx, err := itx.DecodeEVMBytes(msg.Message)
	if err != nil {
		return nil, err
	}
	// add transaction to the world queue
	s.txh.AddTransaction(itx.ID(), tx)
	return &routerv1.MsgSendResponse{}, nil
}

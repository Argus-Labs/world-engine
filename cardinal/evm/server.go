package evm

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"sync/atomic"

	"github.com/argus-labs/world-engine/cardinal/ecs"

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

// ITransactionTypes is a map that maps transaction type ID's to transaction types.
type ITransactionTypes map[transaction.TypeID]transaction.ITransaction

var _ TxHandler = &ecs.World{}

// TxHandler is a type that gives access to transaction data in the ecs.World, as well as access to queue transactions.
type TxHandler interface {
	AddTransaction(transaction.TypeID, any, *sign.SignedPayload) (uint64, transaction.TxID)
	ListTransactions() ([]transaction.ITransaction, error)
}

type srv struct {
	it         ITransactionTypes
	txh        TxHandler
	serverOpts []grpc.ServerOption
	nextNonce  *atomic.Uint64
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
	s := &srv{it: it, txh: txh, nextNonce: &atomic.Uint64{}}
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

// nextSig produces a signature that has a static PersonaTag and a unique nonce. srv's TxHandler wants a unique
// signature to help identify transactions, however this signature information is not readily available in evm
// messages. nextSig is a currently workaround to ensure signatures assocaited with transactions are unique.
// See https://linear.app/arguslabs/issue/CAR-133/mechanism-to-tie-evm-0x-address-to-cardinal-persona
func (s *srv) nextSig() *sign.SignedPayload {
	return &sign.SignedPayload{
		PersonaTag: "internal-persona-tag-for-evm-server",
		Nonce:      s.nextNonce.Add(1),
	}
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
	sig := s.nextSig()
	// add transaction to the world queue
	s.txh.AddTransaction(itx.ID(), tx, sig)
	return &routerv1.MsgSendResponse{}, nil
}

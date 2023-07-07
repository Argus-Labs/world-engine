package evm

import (
	"context"
	"crypto/tls"
	"embed"
	"fmt"
	"github.com/argus-labs/world-engine/cardinal/ecs/transaction"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"net"

	"buf.build/gen/go/argus-labs/world-engine/grpc/go/router/v1/routerv1grpc"
	"buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/router/v1"
)

var (
	//go:embed cert
	f embed.FS
	_ routerv1grpc.MsgServer = &srv{}
)

// ITransactionTypes is a map that maps transaction type ID's to transaction types.
type ITransactionTypes map[transaction.TypeID]transaction.ITransaction

// TxQueuer is a type that provides the function found in ecs.World which adds transactions to the tx queue.
// this is needed, so that we do not need to have a full reference to the world object here.
type TxQueuer interface {
	AddTransaction(transaction.TypeID, any)
}

type srv struct {
	it  ITransactionTypes
	txq TxQueuer
}

func NewServer(it ITransactionTypes, txq TxQueuer) routerv1grpc.MsgServer {
	return &srv{it, txq}
}

func loadCredentials() (credentials.TransportCredentials, error) {
	// Load server's certificate and private key
	sc, err := f.ReadFile("cert/server-cert.pem")
	if err != nil {
		return nil, err
	}
	sk, err := f.ReadFile("cert/server-key.pem")
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
	creds, err := loadCredentials()
	if err != nil {
		return err
	}
	server := grpc.NewServer(grpc.Creds(creds))
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
	s.txq.AddTransaction(itx.ID(), tx)
	return &routerv1.MsgSendResponse{}, nil
}

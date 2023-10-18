package evm

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
	"pkg.world.dev/world-engine/sign"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	routerv1 "pkg.world.dev/world-engine/rift/router/v1"
)

var (
	_ routerv1.MsgServer = &msgServerImpl{}

	defaultPort = "9020"

	cardinalEvmPortEnv = "CARDINAL_EVM_PORT"
)

type Server interface {
	routerv1.MsgServer
	// Serve serves the application in a new go routine.
	Serve() error
}

// txByID maps transaction type ID's to transaction types.
type txByID map[transaction.TypeID]transaction.ITransaction

// queryByName maps query resource names to the underlying IQuery
type queryByName map[string]ecs.IQuery

type msgServerImpl struct {
	// required embed
	routerv1.UnimplementedMsgServer

	txMap    txByID
	queryMap queryByName
	world    *ecs.World

	// opts
	creds credentials.TransportCredentials
	port  string
}

// NewServer returns a new EVM connection server. This server is responsible for handling requests originating from
// the EVM. It runs on a default port of 9020, but a custom port can be set using options, or by setting an env variable
// with key CARDINAL_EVM_PORT.
func NewServer(w *ecs.World, opts ...Option) (Server, error) {
	// setup txs
	txs, err := w.ListTransactions()
	if err != nil {
		return nil, err
	}
	it := make(txByID, len(txs))
	for _, tx := range txs {
		it[tx.ID()] = tx
	}

	queries := w.ListQueries()
	ir := make(queryByName, len(queries))
	for _, q := range queries {
		ir[q.Name()] = q
	}

	s := &msgServerImpl{txMap: it, queryMap: ir, world: w, port: defaultPort}
	for _, opt := range opts {
		opt(s)
	}
	if s.port == defaultPort {
		port := os.Getenv(cardinalEvmPortEnv)
		if port != "" {
			s.port = port
		}
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
func (s *msgServerImpl) Serve() error {
	server := grpc.NewServer(grpc.Creds(s.creds))
	routerv1.RegisterMsgServer(server, s)
	listener, err := net.Listen("tcp", ":"+s.port)
	if err != nil {
		return err
	}
	go func() {
		err = server.Serve(listener)
		if err != nil {
			log.Fatal(err)
		}
	}()
	return nil
}

const (
	CodeSuccess = iota
	CodeTxFailed
	CodeNoResult
	CodeServerUnresponsive
	CodeUnauthorized
	CodeUnsupportedTransaction
	CodeInvalidFormat
)

func (s *msgServerImpl) SendMessage(ctx context.Context, msg *routerv1.SendMessageRequest) (*routerv1.SendMessageResponse, error) {
	// first we check if we can extract the transaction associated with the id
	itx, ok := s.txMap[transaction.TypeID(msg.MessageId)]
	if !ok {
		return &routerv1.SendMessageResponse{
			Errs:      fmt.Errorf("no transaction with ID %d is registerd in this world", msg.MessageId).Error(),
			EvmTxHash: msg.EvmTxHash,
			Code:      CodeUnsupportedTransaction,
		}, nil
	}

	// decode the evm bytes into the transaction
	tx, err := itx.DecodeEVMBytes(msg.Message)
	if err != nil {
		return &routerv1.SendMessageResponse{
			Errs:      fmt.Errorf("failed to decode ABI encoded bytes into ABI type: %w", err).Error(),
			EvmTxHash: msg.EvmTxHash,
			Code:      CodeInvalidFormat,
		}, nil
	}

	// check if the sender has a linked persona address. if not don't process the transaction.
	sc, err := s.getSignerComponentForAuthorizedAddr(msg.Sender)
	if err != nil {
		return &routerv1.SendMessageResponse{
			Errs:      fmt.Errorf("failed to authorize EVM address with persona tag: %w", err).Error(),
			EvmTxHash: msg.EvmTxHash,
			Code:      CodeUnauthorized,
		}, nil
	}

	// since we are injecting the tx directly, all we need is the persona tag in the signed payload.
	// the sig checking happens in the server's Handler, not in ecs.World.
	sig := &sign.SignedPayload{PersonaTag: sc.PersonaTag}
	s.world.AddEVMTransaction(itx.ID(), tx, sig, msg.EvmTxHash)

	// wait for the next tick so the tx gets processed
	timedOut := s.world.WaitForNextTick()
	if timedOut {
		return &routerv1.SendMessageResponse{
			EvmTxHash: msg.EvmTxHash,
			Code:      CodeServerUnresponsive,
		}, nil
	}

	// check for the tx receipt.
	receipt, ok := s.world.ConsumeEVMTxResult(msg.EvmTxHash)
	if !ok {
		return &routerv1.SendMessageResponse{
			EvmTxHash: msg.EvmTxHash,
			Code:      CodeNoResult,
		}, nil
	}

	// we got a receipt, so lets clean it up and return it.
	var errStr string
	code := CodeSuccess
	if retErr := errors.Join(receipt.Errs...); retErr != nil {
		code = CodeTxFailed
		errStr = retErr.Error()
	}
	return &routerv1.SendMessageResponse{
		Errs:      errStr,
		Result:    receipt.ABIResult,
		EvmTxHash: receipt.EVMTxHash,
		Code:      uint32(code),
	}, nil
}

// getSignerComponentForAuthorizedAddr attempts to find a stored SignerComponent which contains the provided `addr`
// within its authorized addresses slice.
func (s *msgServerImpl) getSignerComponentForAuthorizedAddr(addr string) (*ecs.SignerComponent, error) {
	var sc *ecs.SignerComponent
	var err error
	q, err := s.world.NewSearch(ecs.Exact(ecs.SignerComponent{}))
	if err != nil {
		return nil, err
	}
	q.Each(s.world, func(id entity.ID) bool {
		var signerComp *ecs.SignerComponent
		signerComp, err = ecs.GetComponent[ecs.SignerComponent](s.world, id)
		if err != nil {
			return false
		}
		for _, authAddr := range signerComp.AuthorizedAddresses {
			if authAddr == addr {
				sc = signerComp
				return false
			}
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	if sc == nil {
		return nil, fmt.Errorf("address %s does not have a linked persona tag", addr)
	}
	return sc, nil
}

func (s *msgServerImpl) QueryShard(ctx context.Context, req *routerv1.QueryShardRequest) (*routerv1.QueryShardResponse, error) {
	query, ok := s.queryMap[req.Resource]
	if !ok {
		return nil, fmt.Errorf("no query with name %s found", req.Resource)
	}
	ecsRequest, err := query.DecodeEVMRequest(req.Request)
	if err != nil {
		return nil, err
	}
	reply, err := query.HandleQuery(s.world, ecsRequest)
	if err != nil {
		return nil, err
	}
	bz, err := query.EncodeEVMReply(reply)
	if err != nil {
		return nil, err
	}
	return &routerv1.QueryShardResponse{Response: bz}, nil
}

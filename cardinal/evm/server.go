package evm

import (
	"context"
	"crypto/tls"
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

	"buf.build/gen/go/argus-labs/world-engine/grpc/go/router/v1/routerv1grpc"
	"buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/router/v1"
)

var (
	_ routerv1grpc.MsgServer = &msgServerImpl{}

	defaultPort = "9020"

	cardinalEvmPortEnv = "CARDINAL_EVM_PORT"
)

type Server interface {
	routerv1grpc.MsgServer
	// Serve serves the application in a new go routine.
	Serve() error
}

// txByID maps transaction type ID's to transaction types.
type txByID map[transaction.TypeID]transaction.ITransaction

// readByName maps read resource names to the underlying IRead
type readByName map[string]ecs.IRead

type msgServerImpl struct {
	txMap   txByID
	readMap readByName
	world   *ecs.World

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

	reads := w.ListReads()
	ir := make(readByName, len(reads))
	for _, r := range reads {
		ir[r.Name()] = r
	}

	s := &msgServerImpl{txMap: it, readMap: ir, world: w, port: defaultPort}
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
	routerv1grpc.RegisterMsgServer(server, s)
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

func (s *msgServerImpl) SendMessage(ctx context.Context, msg *routerv1.SendMessageRequest,
) (*routerv1.SendMessageResponse, error) {
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

	// check if the sender has a linked persona address. if not don't process the transaction.
	sc, err := s.getSignerComponentForAuthorizedAddr(msg.Sender)
	if err != nil {
		return nil, err
	}
	// since we are injecting directly, all we need is the persona tag in the signed payload. the sig checking happens
	// in the server's Handler, not in `World`.
	sig := &sign.SignedPayload{PersonaTag: sc.PersonaTag}
	// add transaction to the world queue
	s.world.AddTransaction(itx.ID(), tx, sig)
	return &routerv1.SendMessageResponse{}, nil
}

// getSignerComponentForAuthorizedAddr attempts to find a stored SignerComponent which contains the provided `addr`
// within its authorized addresses slice.
func (s *msgServerImpl) getSignerComponentForAuthorizedAddr(addr string) (*ecs.SignerComponent, error) {
	var sc *ecs.SignerComponent
	var err error
	q, err := s.world.NewQuery(ecs.Exact(ecs.SignerComponent{}))
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

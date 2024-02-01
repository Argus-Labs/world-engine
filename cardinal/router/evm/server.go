package evm

/*
import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"os"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"

	"github.com/rotisserie/eris"
	zerolog "github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/types/message"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/types/entity"
	"pkg.world.dev/world-engine/sign"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	routerv1 "pkg.world.dev/world-engine/rift/router/v1"
)

var (
	_ routerv1.MsgServer = &msgServerImpl{}

	defaultPort = "9020"

	cardinalEvmPortEnv    = "CARDINAL_EVM_PORT"
	serverCertFilePathEnv = "SERVER_CERT_PATH"
	serverKeyFilePathEnv  = "SERVER_KEY_PATH"
)

var (
	ErrNoEVMTypes = errors.New("no evm types were given to the server")
)

type Server interface {
	routerv1.MsgServer
	// Serve serves the application in a new go routine.
	Serve() error
	Shutdown()
}

// txByName maps transaction type ID's to transaction types.
type txByName map[string]message.Message

// queryByName maps query resource names to the underlying Query.
type queryByName map[string]ecs.Query

type msgServerImpl struct {
	// required embed
	routerv1.UnimplementedMsgServer

	txMap    txByName
	queryMap queryByName
	world    *ecs.Engine

	// opts
	creds credentials.TransportCredentials
	port  string

	shutdown func()
}

// NewServer returns a new EVM connection server. This server is responsible for handling requests originating from
// the EVM. It runs on a default port of 9020, but a custom port can be set using options, or by setting an env variable
// with key CARDINAL_EVM_PORT.
//
// NewServer will return ErrNoEvmTypes if no transactions OR queries were given with EVM support.
func NewServer(w *ecs.Engine, opts ...Option) (Server, error) {
	hasEVMTxsOrQueries := false

	txs := w.ListMessages()
	it := make(txByName, len(txs))
	for _, tx := range txs {
		if tx.IsEVMCompatible() {
			hasEVMTxsOrQueries = true
			it[tx.Name()] = tx
		}
	}

	queries := w.ListQueries()
	ir := make(queryByName, len(queries))
	for _, q := range queries {
		if q.IsEVMCompatible() {
			hasEVMTxsOrQueries = true
			ir[q.Name()] = q
		}
	}

	if !hasEVMTxsOrQueries {
		return nil, eris.Wrap(ErrNoEVMTypes, "no evm txs or queries")
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
	w.Logger.Debug().Msgf("EVM listener running on port %s", s.port)
	if s.creds == nil {
		var err error
		s.creds, err = tryLoadCredentials()
		if err != nil {
			return nil, err
		}
	}
	if s.creds == nil {
		w.Logger.Warn().
			Msg(
				"running EVM server without credentials. if running on production, please " +
					"shut down and supply the proper credentials for the EVM server",
			)
	}
	return s, nil
}

// tryLoadCredentials will attempt to load the server cert and key file paths from env.
// if the envs are not set, this is a noop and will return nil,nil.
func tryLoadCredentials() (credentials.TransportCredentials, error) {
	cert := os.Getenv(serverCertFilePathEnv)
	if cert != "" {
		key := os.Getenv(serverKeyFilePathEnv)
		if key != "" {
			zerolog.Debug().Msg("running EVM server with SSL credentials")
			return loadCredentials(cert, key)
		}
	}
	zerolog.Debug().
		Msg(
			"running EVM server without SSL credentials. if this is a production application, " +
				"please set provide SSL credentials",
		)
	return nil, nil
}

// loadCredentials loads the TLS credentials for the server from the given file paths.
func loadCredentials(
	serverCertPath, serverKeyPath string,
) (credentials.TransportCredentials, error) {
	// Load server's certificate and private key
	sc, err := os.ReadFile(serverCertPath)
	if err != nil {
		return nil, eris.Wrapf(err, "error reading %s", serverCertPath)
	}
	sk, err := os.ReadFile(serverKeyPath)
	if err != nil {
		return nil, eris.Wrapf(err, "error reading %s", serverKeyPath)
	}
	serverCert, err := tls.X509KeyPair(sc, sk)
	if err != nil {
		return nil, eris.Wrap(err, "error creating serverCert")
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
		return eris.Wrapf(err, "error listening to port %s", s.port)
	}
	go func() {
		err = eris.Wrap(server.Serve(listener), "error serving server")
		if err != nil {
			zerolog.Fatal().Err(err).Msg(eris.ToString(err, true))
		}
	}()
	s.shutdown = server.GracefulStop
	return nil
}

func (s *msgServerImpl) Shutdown() {
	if s.shutdown != nil {
		s.shutdown()
	}
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

func (s *msgServerImpl) SendMessage(_ context.Context, msg *routerv1.SendMessageRequest) (
	*routerv1.SendMessageResponse, error,
) {
	// first we check if we can extract the transaction associated with the id
	itx, ok := s.txMap[msg.MessageId]
	if !ok {
		return &routerv1.SendMessageResponse{
			Errs: fmt.Errorf(
				"transaction with name %s either does not exist, or did not have EVM support "+
					"enabled", msg.MessageId,
			).
				Error(),
			EvmTxHash: msg.EvmTxHash,
			Code:      CodeUnsupportedTransaction,
		}, nil
	}

	// decode the evm bytes into the transaction
	tx, err := itx.DecodeEVMBytes(msg.Message)
	if err != nil {
		return &routerv1.SendMessageResponse{
			Errs: fmt.Errorf("failed to decode ABI encoded bytes into ABI type: %w", err).
				Error(),
			EvmTxHash: msg.EvmTxHash,
			Code:      CodeInvalidFormat,
		}, nil
	}

	// check if the sender has a linked persona address. if not don't process the transaction.
	sc, err := s.getSignerComponentForAuthorizedAddr(msg.Sender)
	if err != nil {
		return &routerv1.SendMessageResponse{
			Errs: fmt.Errorf("failed to authorize EVM address with persona tag: %w", err).
				Error(),
			EvmTxHash: msg.EvmTxHash,
			Code:      CodeUnauthorized,
		}, nil
	}

	// since we are injecting the tx directly, all we need is the persona tag in the signed payload.
	// the sig checking happens in the server's Handler, not in ecs.Engine.
	sig := &sign.Transaction{PersonaTag: sc.PersonaTag}
	s.world.AddEVMTransaction(itx.ID(), tx, sig, msg.EvmTxHash)

	// wait for the next tick so the tx gets processed
	success := s.world.WaitForNextTick()
	if !success {
		return &routerv1.SendMessageResponse{
			EvmTxHash: msg.EvmTxHash,
			Code:      CodeServerUnresponsive,
		}, nil
	}

	// check for the tx receipt.
	receipt, ok := s.world.GetEVMMsgResult(msg.EvmTxHash)
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
func (s *msgServerImpl) getSignerComponentForAuthorizedAddr(
	addr string,
) (*ecs.SignerComponent, error) {
	var sc *ecs.SignerComponent
	eCtx := ecs.NewReadOnlyEngineContext(s.world)
	q := eCtx.NewSearch(filter.Exact(ecs.SignerComponent{}))
	var getComponentErr error
	searchIterationErr := eris.Wrap(
		q.Each(
			eCtx, func(id entity.ID) bool {
				var signerComp *ecs.SignerComponent
				signerComp, getComponentErr = ecs.GetComponent[ecs.SignerComponent](eCtx, id)
				getComponentErr = eris.Wrap(getComponentErr, "")
				if getComponentErr != nil {
					return false
				}
				for _, authAddr := range signerComp.AuthorizedAddresses {
					if authAddr == addr {
						sc = signerComp
						return false
					}
				}
				return true
			},
		), "",
	)
	if getComponentErr != nil {
		return nil, getComponentErr
	}
	if searchIterationErr != nil {
		return nil, searchIterationErr
	}
	if sc == nil {
		return nil, eris.Errorf("address %s does not have a linked persona tag", addr)
	}
	return sc, nil
}

func (s *msgServerImpl) QueryShard(_ context.Context, req *routerv1.QueryShardRequest) (
	*routerv1.QueryShardResponse, error,
) {
	zerolog.Logger.Debug().Msgf("get request for %q", req.Resource)
	query, ok := s.queryMap[req.Resource]
	if !ok {
		return nil, eris.Errorf("no query with name %s found", req.Resource)
	}
	ecsRequest, err := query.DecodeEVMRequest(req.Request)
	if err != nil {
		zerolog.Logger.Error().Err(err).Msg("failed to decode query request")
		return nil, err
	}
	reply, err := query.HandleQuery(ecs.NewReadOnlyEngineContext(s.world), ecsRequest)
	if err != nil {
		zerolog.Logger.Error().Err(err).Msg("failed to handle query")
		return nil, err
	}
	zerolog.Logger.Debug().Msg("successfully handled query")
	bz, err := query.EncodeEVMReply(reply)
	if err != nil {
		zerolog.Logger.Error().Err(err).Msg("failed to encode query reply for EVM")
		return nil, err
	}
	zerolog.Logger.Debug().Msgf("sending back reply: %v", reply)
	return &routerv1.QueryShardResponse{Response: bz}, nil
}

*/

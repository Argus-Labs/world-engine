package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/argus-labs/world-engine/cardinal/chain"
	"github.com/ethereum/go-ethereum/common"
	"io"
	"log"
	"net/http"

	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/transaction"
	"github.com/argus-labs/world-engine/sign"
)

// Handler is a type that contains endpoints for transactions and queries in a given ecs world.
type Handler struct {
	w                      *ecs.World
	mux                    *http.ServeMux
	server                 *http.Server
	disableSigVerification bool

	// plugins
	adapter chain.Writer
}

var (
	// ErrorInvalidSignature is returned when a signature is incorrect in some way (e.g. namespace mismatch, nonce invalid,
	// the actual Verify fails). Other failures (e.g. Redis is down) should not wrap this error.
	ErrorInvalidSignature = errors.New("invalid signature")
)

const (
	listTxEndpoint   = "/cardinal/list-tx-endpoints"
	listReadEndpoint = "/cardinal/list-read-endpoints"

	readPrefix = "read-"
	txPrefix   = "tx-"

	getSignerForPersonaStatusUnknown   = "unknown"
	getSignerForPersonaStatusAvailable = "available"
	getSignerForPersonaStatusAssigned  = "assigned"
)

// NewHandler returns a new Handler that can handle HTTP requests. An HTTP endpoint for each
// transaction and read registered with the given world is automatically created.
func NewHandler(w *ecs.World, opts ...Option) (*Handler, error) {
	th := &Handler{
		w:   w,
		mux: http.NewServeMux(),
	}
	for _, opt := range opts {
		opt(th)
	}

	// make the transaction endpoints
	txs, err := w.ListTransactions()
	if err != nil {
		return nil, err
	}
	txEndpoints := make([]string, 0, len(txs))
	for _, tx := range txs {
		endpoint := conformPath(txPrefix + tx.Name())
		if tx.Name() == ecs.CreatePersonaTx.Name() {
			// The CreatePersonaTx is different from normal transactions because it doesn't look up a signer
			// address from the world to verify the transaction.
			th.mux.HandleFunc(endpoint, th.makeCreatePersonaHandler(tx))
		} else {
			th.mux.HandleFunc(endpoint, th.makeTxHandler(tx))
		}
		txEndpoints = append(txEndpoints, endpoint)
	}
	th.mux.HandleFunc(listTxEndpoint, func(writer http.ResponseWriter, request *http.Request) {
		writeResult(writer, txEndpoints)
	})

	// make the read endpoints
	reads := w.ListReads()
	readEndpoints := make([]string, 0, len(reads))
	for _, q := range reads {
		endpoint := conformPath(readPrefix + q.Name())
		th.mux.HandleFunc(endpoint, th.makeReadHandler(q))
		readEndpoints = append(readEndpoints, endpoint)
	}
	th.mux.HandleFunc(listReadEndpoint, func(writer http.ResponseWriter, request *http.Request) {
		writeResult(writer, readEndpoints)
	})

	readPersonaSignerEndpoint := conformPath(readPrefix + "persona-signer")
	th.mux.HandleFunc(readPersonaSignerEndpoint, th.handleReadPersonaSigner)
	txEndpoints = append(txEndpoints, readPersonaSignerEndpoint)
	return th, nil
}

func getSignerAddressFromPayload(sp sign.SignedPayload) (string, error) {
	createPersonaTx, err := decode[ecs.CreatePersonaTransaction](common.Hex2Bytes(sp.Body))
	if err != nil {
		return "", err
	}
	return createPersonaTx.SignerAddress, nil
}

func (t *Handler) verifySignature(request *http.Request, getSignedAddressFromWorld bool) (payload []byte, sig *sign.SignedPayload, err error) {
	buf, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, nil, errors.New("unable to read body")
	}

	sp, err := sign.UnmarshalSignedPayload(buf)
	if err != nil {
		return nil, nil, err
	}
	if t.disableSigVerification {
		// During testing (with signature verification disabled), a request body can either be wrapped in a signed payload,
		// or the request body can be sent as is.
		if len(sp.Body) == 0 {
			return buf, sp, nil
		}
		return common.Hex2Bytes(sp.Body), sp, nil
	}

	if sp.Namespace != t.w.GetNamespace() {
		return nil, nil, fmt.Errorf("%w: namespace must be %q", ErrorInvalidSignature, t.w.GetNamespace())
	}

	var signerAddress string
	if getSignedAddressFromWorld {
		// Use -1 as the tick. We don't care about any pending CreatePersonaTxs, we just want to know the
		// current signer address for the given persona. Any error will fail this request.
		signerAddress, err = t.w.GetSignerForPersonaTag(sp.PersonaTag, -1)
	} else {
		signerAddress, err = getSignerAddressFromPayload(*sp)
	}
	if err != nil {
		return nil, nil, err
	}

	nonce, err := t.w.GetNonce(signerAddress)
	if err != nil {
		return nil, nil, err
	}
	if sp.Nonce <= nonce {
		return nil, nil, fmt.Errorf("invalid nonce: %w", ErrorInvalidSignature)
	}

	if err := sp.Verify(signerAddress); err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrorInvalidSignature, err)
	}
	if err := t.w.SetNonce(signerAddress, sp.Nonce); err != nil {
		return nil, nil, err
	}

	if len(sp.Body) == 0 {
		return buf, sp, nil
	}
	return common.Hex2Bytes(sp.Body), sp, nil
}

func (t *Handler) makeTxHandler(tx transaction.ITransaction) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		payload, sp, err := t.verifySignature(request, true)
		if errors.Is(err, ErrorInvalidSignature) {
			writeUnauthorized(writer, err)
			return
		} else if err != nil {
			writeError(writer, "unable to verify signature", err)
			return
		}
		txVal, err := tx.Decode(payload)
		if err != nil {
			writeError(writer, "unable to decode transaction", err)
			return
		}
		t.w.AddTransaction(tx.ID(), txVal, sp)
		writeResult(writer, "ok")
		if t.adapter != nil {
			err = t.adapter.Submit(context.Background(), sp)
			if err != nil {
				writeError(writer, "error submitting transaction to blockchain", err)
			}
		}
	}
}

func (t *Handler) makeReadHandler(q ecs.IRead) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		buf, err := io.ReadAll(request.Body)
		if err != nil {
			writeError(writer, "unable to read request body", err)
			return
		}
		res, err := q.HandleRead(t.w, buf)
		if err != nil {
			writeError(writer, "error handling read", err)
			return
		}
		writer.Write(res)
	}
}

// Serve sets up the endpoints passed in by the user, as well as a special "/list" endpoint, that informs consumers
// what endpoints the user set up in the Handler. Then, it serves the application, blocking the main thread.
// Please us `go txh.Serve(host,port)` if you do not want to block execution after calling this function.
func (t *Handler) Serve(host, port string) {
	t.server = &http.Server{
		Addr:    fmt.Sprintf("%s:%s", host, port),
		Handler: t.mux,
	}
	err := t.server.ListenAndServe()
	if err != nil {
		log.Print(err)
	}
}

func (t *Handler) Close() error {
	if err := t.server.Close(); err != nil {
		return err
	}
	return nil
}

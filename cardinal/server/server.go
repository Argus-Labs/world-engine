package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/argus-labs/world-engine/cardinal/shard"
	"github.com/invopop/jsonschema"

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
	adapter shard.WriteAdapter
}

var (
	// ErrorInvalidSignature is returned when a signature is incorrect in some way (e.g. namespace mismatch, nonce invalid,
	// the actual Verify fails). Other failures (e.g. Redis is down) should not wrap this error.
	ErrorInvalidSignature = errors.New("invalid signature")
)

const (
	listTxEndpoint   = "/list/tx-endpoints"
	listReadEndpoint = "/list/read-endpoints"

	schemaEndpointPrefix = "/schema/"

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
	txEndpoints := make([]string, 0, len(txs)*2)
	for _, tx := range txs {
		endpoint := conformPath(txPrefix + tx.Name())
		schemaEndpoint := conformPath(schemaEndpointPrefix + txPrefix + tx.Name())
		if tx.Name() == ecs.CreatePersonaTx.Name() {
			// The CreatePersonaTx is different from normal transactions because it doesn't look up a signer
			// address from the world to verify the transaction.
			th.mux.HandleFunc(endpoint, th.makeCreatePersonaHandler(tx))
		} else {
			th.mux.HandleFunc(endpoint, th.makeTxHandler(tx))
		}
		th.mux.HandleFunc(schemaEndpoint, th.makeSchemaHandler(tx.Schema()))
		txEndpoints = append(txEndpoints, endpoint, schemaEndpoint)
	}
	th.mux.HandleFunc(listTxEndpoint, func(writer http.ResponseWriter, request *http.Request) {
		// marshall txEndpoints to JSON and write to writer
		res, err := json.Marshal(txEndpoints)
		if err != nil {
			writeError(writer, "unable to marshal response", err)
			return
		}

		writeResult(writer, res)
	})

	// make the read endpoints
	reads := w.ListReads()
	readEndpoints := make([]string, 0, len(reads)*2)
	for _, r := range reads {
		endpoint := conformPath(readPrefix + r.Name())
		schemaEndpoint := conformPath(schemaEndpointPrefix + readPrefix + r.Name())
		th.mux.HandleFunc(endpoint, th.makeReadHandler(r))
		th.mux.HandleFunc(schemaEndpoint, th.makeSchemaHandler(r.Schema()))
		readEndpoints = append(readEndpoints, endpoint, schemaEndpoint)
	}
	readPersonaSignerEndpoint := conformPath(readPrefix + "persona-signer")
	readPersonaSignerSchemaEndpoint := conformPath(schemaEndpointPrefix + readPrefix + "persona-signer")
	th.mux.HandleFunc(readPersonaSignerEndpoint, th.handleReadPersonaSigner)
	th.mux.HandleFunc(readPersonaSignerSchemaEndpoint, th.handleReadPersonaSignerSchema)
	readEndpoints = append(readEndpoints, readPersonaSignerEndpoint, readPersonaSignerSchemaEndpoint)
	th.mux.HandleFunc(listReadEndpoint, func(writer http.ResponseWriter, request *http.Request) {
		res, err := json.Marshal(readEndpoints)
		if err != nil {
			writeError(writer, "unable to marshal response", err)
			return
		}

		writeResult(writer, res)
	})

	return th, nil
}

func getSignerAddressFromPayload(sp sign.SignedPayload) (string, error) {
	createPersonaTx, err := decode[ecs.CreatePersonaTransaction](sp.Body)
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

	if sp.PersonaTag == "" {
		return nil, nil, errors.New("PersonaTag must not be empty")
	}

	// Handle the case where signature is disabled
	if t.disableSigVerification {
		return sp.Body, sp, nil
	}
	///////////////////////////////////////////////

	// Check that the namespace is correct
	if sp.Namespace != t.w.GetNamespace() {
		return nil, nil, fmt.Errorf("%w: got namespace %q but it must be %q", ErrorInvalidSignature, sp.Namespace, t.w.GetNamespace())
	}

	var signerAddress string
	if getSignedAddressFromWorld {
		// Use 0 as the tick. We don't care about any pending CreatePersonaTxs, we just want to know the
		// current signer address for the given persona. Any error will fail this request.
		signerAddress, err = t.w.GetSignerForPersonaTag(sp.PersonaTag, 0)
	} else {
		signerAddress, err = getSignerAddressFromPayload(*sp)
	}
	if err != nil {
		return nil, nil, err
	}

	// Check the nonce
	nonce, err := t.w.GetNonce(signerAddress)
	if err != nil {
		return nil, nil, err
	}
	if sp.Nonce <= nonce {
		return nil, nil, fmt.Errorf("%w: got nonce %d, but must be greater than %d",
			ErrorInvalidSignature, sp.Nonce, nonce)
	}

	// Verify signature
	if err := sp.Verify(signerAddress); err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrorInvalidSignature, err)
	}
	// Update nonce
	if err := t.w.SetNonce(signerAddress, sp.Nonce); err != nil {
		return nil, nil, err
	}

	if len(sp.Body) == 0 {
		return buf, sp, nil
	}
	return sp.Body, sp, nil
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

		submitTx := func() uint64 {
			tick := t.w.AddTransaction(tx.ID(), txVal, sp)

			res, err := json.Marshal("ok")
			if err != nil {
				writeError(writer, "unable to marshal response", err)
				return 0
			}
			writeResult(writer, res)
			return uint64(tick)
		}

		// check if we have an adapter
		if t.adapter != nil {
			// if the world is recovering via adapter, we shouldn't accept transactions.
			if t.w.IsRecovering() {
				writeError(writer, "unable to submit transactions: game world is recovering state", nil)
			} else {
				tick := submitTx()
				err = t.adapter.Submit(context.Background(), sp, uint64(tx.ID()), tick)
				if err != nil {
					writeError(writer, "error submitting transaction to blockchain", err)
				}
			}
		} else {
			// if there is no adapter, then we can just put the tx in the queue.
			submitTx()
		}
	}
}

func (t *Handler) makeReadHandler(r ecs.IRead) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		buf, err := io.ReadAll(request.Body)
		if err != nil {
			writeError(writer, "unable to read request body", err)
			return
		}
		res, err := r.HandleReadRaw(t.w, buf)
		if err != nil {
			writeError(writer, "error handling read", err)
			return
		}
		writeResult(writer, res)
	}
}

func (t *Handler) makeSchemaHandler(schema *jsonschema.Schema) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		res, err := json.Marshal(schema)
		if err != nil {
			writeError(writer, "unable to marshal response", err)
			return
		}

		writeResult(writer, res)
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

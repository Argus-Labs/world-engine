package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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
}

var (
	// ErrorInvalidSignature is returned when a signature is incorrect in some way (e.g. namespace mismatch, nonce invalid,
	// the actual Verify fails). Other failures (e.g. Redis is down) should not wrap this error.
	ErrorInvalidSignature = errors.New("invalid signature")
)

const (
	listTxEndpoint    = "/cardinal/list-tx-endpoints"
	listQueryEndpoint = "/cardinal/list-query-endpoints"

	getSignerForPersonaStatusUnknown   = "unknown"
	getSignerForPersonaStatusAvailable = "available"
	getSignerForPersonaStatusAssigned  = "assigned"
)

// NewHandler returns a new Handler that can handle HTTP requests. An HTTP endpoint for each
// transaction and query registered with the given world is automatically created.
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
		endpoint := conformPath("tx-" + tx.Name())
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

	// make the query endpoints
	queries := w.ListQueries()
	queryEndpoints := make([]string, 0, len(queries))
	for _, q := range queries {
		endpoint := conformPath("query-" + q.Name())
		th.mux.HandleFunc(endpoint, th.makeQueryHandler(q))
		queryEndpoints = append(queryEndpoints, endpoint)
	}
	th.mux.HandleFunc(listQueryEndpoint, func(writer http.ResponseWriter, request *http.Request) {
		writeResult(writer, queryEndpoints)
	})

	queryPersonaSignerEndpoint := conformPath("query-persona-signer")
	th.mux.HandleFunc(queryPersonaSignerEndpoint, th.handleQueryPersonaSigner)
	txEndpoints = append(txEndpoints, queryPersonaSignerEndpoint)
	return th, nil
}

// CreatePersonaResponse is returned from a tx-create-persona request. It contains the current tick of the game
// (needed to call the query-persona-signer endpoint).
type CreatePersonaResponse struct {
	Tick   int
	Status string
}

// QueryPersonaSignerRequest is the desired request body for the query-persona-signer endpoint.
type QueryPersonaSignerRequest struct {
	PersonaTag string
	Tick       int
}

// QueryPersonaSignerResponse is used as the response body for the query-persona-signer endpoint. Status can be:
// "assigned": The requested persona tag has been assigned the returned SignerAddress
// "unknown": The game tick has not advanced far enough to know what the signer address. SignerAddress will be empty.
// "available": The game tick has advanced, and no signer address has been assigned. SignerAddress will be empty.
type QueryPersonaSignerResponse struct {
	Status        string
	SignerAddress string
}

func getSignerAddressFromPayload(sp sign.SignedPayload) (string, error) {
	createPersonaTx, err := decode[ecs.CreatePersonaTransaction](sp.Body)
	if err != nil {
		return "", err
	}
	return createPersonaTx.SignerAddress, nil
}

func (t *Handler) verifySignature(request *http.Request, getSignedAddressFromWorld bool) (payload []byte, err error) {
	buf, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, errors.New("unable to read body")
	}
	sp, err := decode[sign.SignedPayload](buf)
	if err != nil {
		return nil, err
	}
	if t.disableSigVerification {
		// During testing (with signature verification disabled), a request body can either be wrapped in a signed payload,
		// or the request body can be sent as is.
		if len(sp.Body) == 0 {
			return buf, nil
		}
		return sp.Body, nil
	}

	if sp.Namespace != t.w.GetNamespace() {
		return nil, fmt.Errorf("%w: namespace must be %q", ErrorInvalidSignature, t.w.GetNamespace())
	}

	var signerAddress string
	if getSignedAddressFromWorld {
		// Use -1 as the tick. We don't care about any pending CreatePersonaTxs, we just want to know the
		// current signer address for the given persona. Any error will fail this request.
		signerAddress, err = t.w.GetSignerForPersonaTag(sp.PersonaTag, -1)
	} else {
		signerAddress, err = getSignerAddressFromPayload(sp)
	}
	if err != nil {
		return nil, err
	}

	nonce, err := t.w.GetNonce(signerAddress)
	if err != nil {
		return nil, err
	}
	if sp.Nonce <= nonce {
		return nil, fmt.Errorf("invalid nonce: %w", ErrorInvalidSignature)
	}

	if err := sp.Verify(signerAddress); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrorInvalidSignature, err)
	}
	if err := t.w.SetNonce(signerAddress, sp.Nonce); err != nil {
		return nil, err
	}

	if len(sp.Body) == 0 {
		return buf, nil
	}
	return sp.Body, nil
}

func (t *Handler) handleQueryPersonaSigner(w http.ResponseWriter, r *http.Request) {
	buf, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, "unable to read body", err)
		return
	}

	req, err := decode[QueryPersonaSignerRequest](buf)
	if err != nil {
		writeError(w, "unable to decode body", err)
		return
	}

	var status string
	addr, err := t.w.GetSignerForPersonaTag(req.PersonaTag, req.Tick)
	if err == ecs.ErrorPersonaTagHasNoSigner {
		status = getSignerForPersonaStatusAvailable
	} else if err == ecs.ErrorCreatePersonaTxsNotProcessed {
		status = getSignerForPersonaStatusUnknown
	} else if err != nil {
		writeError(w, "query persona signer error", err)
		return
	} else {
		status = getSignerForPersonaStatusAssigned
	}
	writeResult(w, QueryPersonaSignerResponse{
		Status:        status,
		SignerAddress: addr,
	})
}

func (t *Handler) makeCreatePersonaHandler(tx transaction.ITransaction) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		payload, err := t.verifySignature(request, false)
		if err != nil {
			if errors.Is(err, ErrorInvalidSignature) {
				writeUnauthorized(writer, err)
				return
			}
			writeError(writer, "unable to verify signature", err)
			return
		}

		txVal, err := tx.Decode(payload)
		if err != nil {
			writeError(writer, "unable to decode transaction", err)
			return
		}
		t.w.AddTransaction(tx.ID(), txVal)
		writeResult(writer, CreatePersonaResponse{
			Tick:   t.w.CurrentTick(),
			Status: "ok",
		})
	}
}

func (t *Handler) makeTxHandler(tx transaction.ITransaction) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		payload, err := t.verifySignature(request, true)
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
		t.w.AddTransaction(tx.ID(), txVal)
		writeResult(writer, "ok")
	}
}

func (t *Handler) makeQueryHandler(q ecs.IQuery) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		buf, err := io.ReadAll(request.Body)
		if err != nil {
			writeError(writer, "unable to read request body", err)
			return
		}
		res, err := q.HandleQuery(t.w, buf)
		if err != nil {
			writeError(writer, "error handling query", err)
			return
		}
		writer.Write(res)
	}
}

// fixes a path to contain a leading slash.
// if the path already contains a leading slash, it is simply returned as is.
func conformPath(p string) string {
	if p[0] != '/' {
		p = "/" + p
	}
	return p
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

func writeUnauthorized(w http.ResponseWriter, err error) {
	w.WriteHeader(401)
	fmt.Fprintf(w, "unauthorized: %v", err)
}

func writeError(w http.ResponseWriter, msg string, err error) {
	w.WriteHeader(500)
	fmt.Fprintf(w, "%s: %v", msg, err)
}

func writeResult(w http.ResponseWriter, v any) {
	if s, ok := v.(string); ok {
		v = struct{ Msg string }{Msg: s}
	}
	enc := json.NewEncoder(w)
	if err := enc.Encode(v); err != nil {
		writeError(w, "can't encode", err)
		return
	}
}

func decode[T any](buf []byte) (T, error) {
	var val T
	r := bytes.NewReader(buf)
	dec := json.NewDecoder(r)
	if err := dec.Decode(&val); err != nil {
		return val, err
	}
	return val, nil
}

type Option func(th *Handler)

func DisableSignatureVerification() Option {
	return func(th *Handler) {
		th.disableSigVerification = true
	}
}

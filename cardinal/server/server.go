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

// TransactionHandler is a type that contains endpoints for transactions in a given ecs world.
type TransactionHandler struct {
	w                      *ecs.World
	mux                    *http.ServeMux
	server                 *http.Server
	disableSigVerification bool
}

// NewTransactionHandler returns a new TransactionHandler
func NewTransactionHandler(w *ecs.World, opts ...Option) (*TransactionHandler, error) {
	th := &TransactionHandler{
		w:   w,
		mux: http.NewServeMux(),
	}
	for _, opt := range opts {
		opt(th)
	}

	txs, err := w.ListTransactions()
	if err != nil {
		return nil, err
	}
	var endpoints []string
	for _, tx := range txs {
		endpoint := conformPath("tx_" + tx.Name())
		if tx.Name() == ecs.CreatePersonaTx.Name() {
			th.mux.HandleFunc(endpoint, th.makeCreatePersonaHandler(tx))
		} else {
			th.mux.HandleFunc(endpoint, th.makeTxHandler(tx))
		}
		endpoints = append(endpoints, endpoint)
	}

	th.mux.HandleFunc("/cardinal/list_endpoints", func(writer http.ResponseWriter, request *http.Request) {
		writeResult(writer, endpoints)
	})

	th.mux.HandleFunc("/query_persona_signer", th.handleQueryPersonaSigner)
	return th, nil
}

type CreatePersonaResponse struct {
	Tick   int
	Status string
}

type QueryPersonaSignerRequest struct {
	PersonaTag string
	Tick       int
}

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

func getSignerAddressFromWorld(world *ecs.World, personaTag string) (string, error) {
	addr, err := world.GetSignerForPersonaTag(personaTag, -1)
	if err != nil {
		return "", err
	}
	return addr, nil
}

func (t *TransactionHandler) verifySignature(request *http.Request, getSignedAddressFromWorld bool) (payload []byte, err error) {
	buf, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, errors.New("unable to read body")
	}
	sp, err := decode[sign.SignedPayload](buf)
	if err != nil {
		return nil, err
	}

	if !t.disableSigVerification {
		var signerAddress string
		var err error
		if getSignedAddressFromWorld {
			signerAddress, err = getSignerAddressFromWorld(t.w, sp.PersonaTag)
		} else {
			signerAddress, err = getSignerAddressFromPayload(sp)
		}
		if err != nil {
			return nil, err
		}

		if err := sp.Verify(signerAddress); err != nil {
			return nil, err

		}
		// The signature is valid. Drop out of this branch to return the payload.
	}
	// For testing, it would be nice to be able to bypass signature verification and just
	// pass in the raw transaction data. If it looks like the signed payload is empty,
	// just return the original request body.
	if len(sp.Body) == 0 {
		return buf, nil
	}
	return sp.Body, nil
}

func (t *TransactionHandler) handleQueryPersonaSigner(w http.ResponseWriter, r *http.Request) {
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
		status = "available"
	} else if err == ecs.ErrorCreatePersonaTxsNotProcessed {
		status = "unknown"
	} else if err != nil {
		writeError(w, "query persona signer error", err)
		return
	} else {
		status = "assigned"
	}
	writeResult(w, QueryPersonaSignerResponse{
		Status:        status,
		SignerAddress: addr,
	})
}

func (t *TransactionHandler) makeCreatePersonaHandler(tx transaction.ITransaction) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		payload, err := t.verifySignature(request, false)
		if err != nil {
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

func (t *TransactionHandler) makeTxHandler(tx transaction.ITransaction) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		payload, err := t.verifySignature(request, true)
		if err != nil {
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

// fixes a path to contain a leading slash.
// if the path already contains a leading slash, it is simply returned as is.
func conformPath(p string) string {
	if p[0] != '/' {
		p = "/" + p
	}
	return p
}

// Serve sets up the endpoints passed in by the user, as well as a special "/list" endpoint, that informs consumers
// what endpoints the user set up in the TransactionHandler. Then, it serves the application, blocking the main thread.
// Please us `go txh.Serve(host,port)` if you do not want to block execution after calling this function.
func (t *TransactionHandler) Serve(host, port string) {
	t.server = &http.Server{
		Addr:    fmt.Sprintf("%s:%s", host, port),
		Handler: t.mux,
	}
	err := t.server.ListenAndServe()
	if err != nil {
		log.Print(err)
	}
}

func (t *TransactionHandler) Close() error {
	if err := t.server.Close(); err != nil {
		return err
	}
	return nil
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

type Option func(th *TransactionHandler)

func DisableSignatureVerification() Option {
	return func(th *TransactionHandler) {
		th.disableSigVerification = true
	}
}

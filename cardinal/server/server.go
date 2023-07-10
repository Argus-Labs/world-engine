package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/transaction"
)

// TransactionHandler is a type that contains endpoints for transactions in a given ecs world.
type TransactionHandler struct {
	w                      *ecs.World
	disableSigVerificaiton bool
}

// NewTransactionHandler returns a new TransactionHandler
func NewTransactionHandler(w *ecs.World, opts ...Option) (*TransactionHandler, error) {
	th := &TransactionHandler{w: w}
	for _, opt := range opts {
		opt(th)
	}

	txs, err := w.ListTransactions()
	if err != nil {
		return nil, err
	}
	var endpoints []string
	for _, tx := range txs {
		path := "tx_" + tx.Name()
		endpoint := conformPath(path)
		http.HandleFunc(endpoint, th.makeTxHandler(tx))
		endpoints = append(endpoints, endpoint)
	}

	http.HandleFunc("/cardinal/list_endpoints", func(writer http.ResponseWriter, request *http.Request) {
		writeResult(writer, endpoints)
	})

	return th, nil
}

type SignedPayload struct {
	PersonaTag    string
	SignerAddress string
	Signature     []byte
	Payload       []byte
}

func (th *TransactionHandler) verifySignature(request *http.Request) (payload []byte, err error) {
	buf, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, errors.New("unable to read body")
	}
	sp, err := decode[SignedPayload](buf)
	if err != nil {
		return nil, err
	}

	if !th.disableSigVerificaiton {
		// Actually verify the signature
		panic("signature verification not implemented")
	}
	// For testing, it would be nice to be able to bypass signature verification and just
	// pass in the raw transaction data. If it looks like the signed payload is empty,
	// just return the original request body.
	if len(sp.Payload) == 0 {
		return buf, nil
	}
	return sp.Payload, nil
}

func (th *TransactionHandler) makeTxHandler(tx transaction.ITransaction) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		payload, err := th.verifySignature(request)
		if err != nil {
			writeError(writer, "unable to verify signature", err)
			return
		}
		txVal, err := tx.Decode(payload)
		if err != nil {
			writeError(writer, "unable to decode transaction", err)
			return
		}
		th.w.AddTransaction(tx.ID(), txVal)
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
	err := http.ListenAndServe(fmt.Sprintf("%s:%s", host, port), nil)
	if err != nil {
		panic(err)
	}
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
		th.disableSigVerificaiton = true
	}
}

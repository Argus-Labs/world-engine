package server

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/transaction"
	"github.com/invopop/jsonschema"
	"io"
	"net/http"
)

func registerReadHandlers(w *ecs.World, th *Handler) error {
	var readEndpoints []string

	// Register user defined read endpoints
	reads := w.ListReads()
	for _, r := range reads {
		endpoint := readPrefix + r.Name()
		schemaEndpoint := schemaEndpointPrefix + readPrefix + r.Name()
		readEndpoints = append(readEndpoints, endpoint, schemaEndpoint)

		th.mux.HandleFunc(conformPath(endpoint), th.makeReadHandler(r))
		th.mux.HandleFunc(conformPath(schemaEndpoint), th.makeSchemaHandler(r.Schema()))
	}

	// Register persona read endpoints
	readPersonaSignerEndpoint := readPrefix + "persona-signer"
	readPersonaSignerSchemaEndpoint := schemaEndpointPrefix + readPrefix + "persona-signer"
	readEndpoints = append(readEndpoints, readPersonaSignerEndpoint, readPersonaSignerSchemaEndpoint)
	th.mux.HandleFunc(conformPath(readPersonaSignerEndpoint), th.handleReadPersonaSigner)
	th.mux.HandleFunc(conformPath(readPersonaSignerSchemaEndpoint), th.handleReadPersonaSignerSchema)

	// Register list read endpoints
	th.mux.HandleFunc(conformPath(listReadEndpoint), writeHandler(readEndpoints))

	return nil
}

func registerTxHandlers(w *ecs.World, th *Handler) error {
	var txEndpoints []string

	// Register tx endpoints
	txs, err := w.ListTransactions()
	if err != nil {
		return err
	}
	for _, tx := range txs {
		endpoint := txPrefix + tx.Name()
		schemaEndpoint := schemaEndpointPrefix + txPrefix + tx.Name()
		txEndpoints = append(txEndpoints, endpoint, schemaEndpoint)

		if tx.Name() == ecs.CreatePersonaTx.Name() {
			// Register persona tx endpoint
			// Note: The CreatePersonaTx is different from normal transactions because it doesn't look up a signer
			// address from the world to verify the transaction.
			th.mux.HandleFunc(conformPath(endpoint), th.makeCreatePersonaHandler(tx))
		} else {
			// Register user defined tx endpoints
			th.mux.HandleFunc(conformPath(endpoint), th.makeTxHandler(tx))
		}
		th.mux.HandleFunc(conformPath(schemaEndpoint), th.makeSchemaHandler(tx.Schema()))
	}

	// Register list tx endpoints
	th.mux.HandleFunc(conformPath(listTxEndpoint), writeHandler(txEndpoints))

	return nil
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
			return tick
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

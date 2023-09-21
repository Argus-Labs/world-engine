package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/invopop/jsonschema"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
	"pkg.world.dev/world-engine/sign"
)

func (t *Handler) makeSchemaHandler(inSchema, outSchema *jsonschema.Schema) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		requestAndReply := map[string]*jsonschema.Schema{
			"request": inSchema,
			"reply":   outSchema,
		}
		res, err := json.Marshal(requestAndReply)
		if err != nil {
			writeError(writer, "unable to marshal response", err)
			return
		}

		writeResult(writer, res)
	}
}

func (t *Handler) processTransaction(tx transaction.ITransaction, payload []byte, sp *sign.SignedPayload) ([]byte, error) {
	txVal, err := tx.Decode(payload)
	if err != nil {
		return nil, fmt.Errorf("unable to decode transaction: %w", err)
	}

	submitTx := func() (uint64, []byte, error) {
		tick, txHash := t.w.AddTransaction(tx.ID(), txVal, sp)

		res, err := json.Marshal(TransactionReply{
			TxHash: string(txHash),
			Tick:   tick,
		})
		if err != nil {
			return 0, nil, fmt.Errorf("unable to marshal response: %w", err)
		}
		return tick, res, nil
	}

	// check if we have an adapter
	if t.adapter != nil {
		// if the world is recovering via adapter, we shouldn't accept transactions.
		if t.w.IsRecovering() {
			return nil, errors.New("unable to submit transactions: game world is recovering state")
		} else {
			tick, res, err := submitTx()
			if err != nil {
				return nil, err
			}
			err = t.adapter.Submit(context.Background(), sp, uint64(tx.ID()), tick)
			if err != nil {
				return nil, fmt.Errorf("error submitting transaction to blockchain: %w", err)
			}
			return res, nil
		}
	} else {
		// if there is no adapter, then we can just put the tx in the queue.
		_, res, err := submitTx()
		if err != nil {
			return nil, err
		}
		return res, nil
	}
}

func (t *Handler) makeTxHandler(tx transaction.ITransaction) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		payload, sp, err := t.verifySignatureOfHTTPRequest(request, false)
		if errors.Is(err, ErrorInvalidSignature) {
			writeUnauthorized(writer, err)
			return
		} else if errors.Is(err, ErrorSystemTransactionForbidden) {
			writeUnauthorized(writer, err)
			return
		} else if err != nil {
			writeError(writer, "unable to verify signature", err)
			return
		}
		res, err := t.processTransaction(tx, payload, sp)
		if err != nil {
			writeError(writer, "", err)
		} else {
			writeResult(writer, res)
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

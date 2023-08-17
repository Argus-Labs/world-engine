package server

import (
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

// ReadPersonaSignerRequest is the desired request body for the read-persona-signer endpoint.
type ReadPersonaSignerRequest struct {
	PersonaTag string
	Tick       uint64
}

// ReadPersonaSignerResponse is used as the response body for the read-persona-signer endpoint. Status can be:
// "assigned": The requested persona tag has been assigned the returned SignerAddress
// "unknown": The game tick has not advanced far enough to know what the signer address. SignerAddress will be empty.
// "available": The game tick has advanced, and no signer address has been assigned. SignerAddress will be empty.
type ReadPersonaSignerResponse struct {
	Status        string
	SignerAddress string
}

func (t *Handler) handleReadPersonaSigner(w http.ResponseWriter, r *http.Request) {
	buf, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, "unable to read body", err)
		return
	}

	req, err := decode[ReadPersonaSignerRequest](buf)
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
		writeError(w, "read persona signer error", err)
		return
	} else {
		status = getSignerForPersonaStatusAssigned
	}

	res := ReadPersonaSignerResponse{
		Status:        status,
		SignerAddress: addr,
	}
	resJson, err := json.Marshal(res)
	if err != nil {
		writeError(w, "unable to marshal response", err)
		return
	}

	writeResult(w, resJson)
}

func (t *Handler) handleReadPersonaSignerSchema(w http.ResponseWriter, _ *http.Request) {
	jsonSchema, err := json.Marshal(jsonschema.Reflect(new(ReadPersonaSignerRequest)))
	if err != nil {
		writeError(w, "unable to marshal response", err)
		return
	}

	writeResult(w, jsonSchema)
}

func (t *Handler) makeCreatePersonaHandler(tx transaction.ITransaction) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		payload, sp, err := t.verifySignature(request, true)
		if err != nil {
			if errors.Is(err, ErrorInvalidSignature) {
				writeUnauthorized(writer, err)
				return
			} else if errors.Is(err, ErrorSystemTransactionRequired) {
				writeUnauthorized(writer, err)
				return
			}
			writeError(writer, "unable to verify signature", err)
			return
		}
		if !sp.IsSystemPayload() {
			writeUnauthorized(writer, fmt.Errorf("persona tag %q is required for this transaction", sign.SystemPersonaTag))
			return
		}

		txVal, err := tx.Decode(payload)
		if err != nil {
			writeError(writer, "unable to decode transaction", err)
			return
		}
		tick, id := t.w.AddTransaction(tx.ID(), txVal, sp)

		res, err := json.Marshal(ReceiptID{
			ID:   id.String(),
			Tick: tick,
		})
		if err != nil {
			writeError(writer, "unable to marshal response", err)
			return
		}

		writeResult(writer, res)
	}
}

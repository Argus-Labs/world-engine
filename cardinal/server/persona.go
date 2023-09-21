package server

import (
	"encoding/json"
	"errors"
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

func (t *Handler) getPersonaSignerResponse(req *ReadPersonaSignerRequest) (*ReadPersonaSignerResponse, error) {
	var status string
	addr, err := t.w.GetSignerForPersonaTag(req.PersonaTag, req.Tick)
	if errors.Is(err, ecs.ErrorPersonaTagHasNoSigner) {
		status = getSignerForPersonaStatusAvailable
	} else if errors.Is(err, ecs.ErrorCreatePersonaTxsNotProcessed) {
		status = getSignerForPersonaStatusUnknown
	} else if err != nil {
		return nil, err
	} else {
		status = getSignerForPersonaStatusAssigned
	}

	res := ReadPersonaSignerResponse{
		Status:        status,
		SignerAddress: addr,
	}
	return &res, nil
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

	res, err := t.getPersonaSignerResponse(&req)
	if err != nil {
		writeError(w, "read persona signer error", err)
		return
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

func generateCreatePersonaResponseFromPayload(payload []byte, sp *sign.SignedPayload, tx transaction.ITransaction, world *ecs.World) (*TransactionReply, error) {
	txVal, err := tx.Decode(payload)
	if err != nil {
		return nil, errors.New("unable to decode transaction")
	}
	tick, txHash := world.AddTransaction(tx.ID(), txVal, sp)
	res := TransactionReply{
		TxHash: string(txHash),
		Tick:   tick,
	}
	return &res, nil
}

func (t *Handler) makeCreatePersonaHandler(tx transaction.ITransaction) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		payload, sp, err := t.verifySignatureOfHTTPRequest(request, true)
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
		txReply, err := generateCreatePersonaResponseFromPayload(payload, sp, tx, t.w)
		if err != nil {
			writeError(writer, "", err)
			return
		}
		res, err := json.Marshal(txReply)
		if err != nil {
			writeError(writer, "unable to marshal response", err)
			return
		}

		writeResult(writer, res)
	}
}

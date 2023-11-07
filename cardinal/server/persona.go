package server

import (
	"errors"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
	"pkg.world.dev/world-engine/sign"
)

const (
	getSignerForPersonaStatusUnknown   = "unknown"
	getSignerForPersonaStatusAvailable = "available"
	getSignerForPersonaStatusAssigned  = "assigned"
)

// QueryPersonaSignerRequest is the desired request body for the query-persona-signer endpoint.
type QueryPersonaSignerRequest struct {
	PersonaTag string `json:"personaTag"`
	Tick       uint64 `json:"tick"`
}

// QueryPersonaSignerResponse is used as the response body for the query-persona-signer endpoint. Status can be:
// "assigned": The requested persona tag has been assigned the returned SignerAddress
// "unknown": The game tick has not advanced far enough to know what the signer address. SignerAddress will be empty.
// "available": The game tick has advanced, and no signer address has been assigned. SignerAddress will be empty.
type QueryPersonaSignerResponse struct {
	Status        string `json:"status"`
	SignerAddress string `json:"signerAddress"`
}

func (handler *Handler) getPersonaSignerResponse(req *QueryPersonaSignerRequest) (*QueryPersonaSignerResponse, error) {
	var status string
	addr, err := handler.w.GetSignerForPersonaTag(req.PersonaTag, req.Tick)
	//nolint:gocritic // its ok.
	if errors.Is(err, ecs.ErrPersonaTagHasNoSigner) {
		status = getSignerForPersonaStatusAvailable
	} else if errors.Is(err, ecs.ErrCreatePersonaTxsNotProcessed) {
		status = getSignerForPersonaStatusUnknown
	} else if err != nil {
		return nil, err
	} else {
		status = getSignerForPersonaStatusAssigned
	}

	res := QueryPersonaSignerResponse{
		Status:        status,
		SignerAddress: addr,
	}
	return &res, nil
}

func (handler *Handler) generateCreatePersonaResponseFromPayload(payload []byte, sp *sign.Transaction,
	tx transaction.ITransaction) (*TransactionReply, error) {
	txVal, err := tx.Decode(payload)
	if err != nil {
		return nil, errors.New("unable to decode transaction")
	}
	return handler.submitTransaction(txVal, tx, sp)
}

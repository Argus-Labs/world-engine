package server

import (
	"errors"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/public"
	"pkg.world.dev/world-engine/sign"
)

const (
	getSignerForPersonaStatusUnknown   = "unknown"
	getSignerForPersonaStatusAvailable = "available"
	getSignerForPersonaStatusAssigned  = "assigned"
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

func (handler *Handler) getPersonaSignerResponse(req *ReadPersonaSignerRequest) (*ReadPersonaSignerResponse, error) {
	var status string
	addr, err := handler.w.GetSignerForPersonaTag(req.PersonaTag, req.Tick)
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

func (handler *Handler) generateCreatePersonaResponseFromPayload(payload []byte, sp *sign.SignedPayload, tx public.ITransaction) (*TransactionReply, error) {
	txVal, err := tx.Decode(payload)
	if err != nil {
		return nil, errors.New("unable to decode transaction")
	}
	return handler.submitTransaction(txVal, tx, sp)
}

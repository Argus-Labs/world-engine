package query

import (
	"errors"
	"pkg.world.dev/world-engine/cardinal/persona"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

const (
	PersonaStatusUnknown   = "unknown"
	PersonaStatusAvailable = "available"
	PersonaStatusAssigned  = "assigned"
)

// PersonaSignerQueryRequest is the desired request body for the query-persona-signer endpoint.
type PersonaSignerQueryRequest struct {
	PersonaTag string `json:"personaTag"`
	Tick       uint64 `json:"tick"`
}

// PersonaSignerQueryResponse is used as the response body for the query-persona-signer endpoint. Status can be:
// "assigned": The requested persona tag has been assigned the returned SignerAddress
// "unknown": The game tick has not advanced far enough to know what the signer address. SignerAddress will be empty.
// "available": The game tick has advanced, and no signer address has been assigned. SignerAddress will be empty.
type PersonaSignerQueryResponse struct {
	Status        string `json:"status"`
	SignerAddress string `json:"signerAddress"`
}

func PersonaSignerQuery(wCtx engine.Context, req *PersonaSignerQueryRequest) (*PersonaSignerQueryResponse, error) {
	var status string

	addr, err := wCtx.GetSignerForPersonaTag(req.PersonaTag, req.Tick)
	if err != nil {
		//nolint:gocritic // cant switch case this.
		if errors.Is(err, persona.ErrPersonaTagHasNoSigner) {
			status = PersonaStatusAvailable
		} else if errors.Is(err, persona.ErrCreatePersonaTxsNotProcessed) {
			status = PersonaStatusUnknown
		} else {
			return nil, err
		}
	} else {
		status = PersonaStatusAssigned
	}

	res := PersonaSignerQueryResponse{
		Status:        status,
		SignerAddress: addr,
	}
	return &res, nil
}

package query

import (
	"errors"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

var (
	ErrPersonaTagHasNoSigner        = errors.New("persona tag does not have a signer")
	ErrCreatePersonaTxsNotProcessed = errors.New("create persona txs have not been processed for the given tick")
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

func QueryPersonaSigner(eCtx engine.Context, req *QueryPersonaSignerRequest) (*QueryPersonaSignerResponse, error) {
	var status string

	addr, err := eCtx.GetSignerForPersonaTag(req.PersonaTag, req.Tick)
	if err != nil {
		//nolint:gocritic // cant switch case this.
		if errors.Is(err, ErrPersonaTagHasNoSigner) {
			status = getSignerForPersonaStatusAvailable
		} else if errors.Is(err, ErrCreatePersonaTxsNotProcessed) {
			status = getSignerForPersonaStatusUnknown
		} else {
			return nil, err
		}
	} else {
		status = getSignerForPersonaStatusAssigned
	}

	res := QueryPersonaSignerResponse{
		Status:        status,
		SignerAddress: addr,
	}
	return &res, nil
}

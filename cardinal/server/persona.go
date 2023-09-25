package server

import (
	"context"
	"errors"
	"fmt"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
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

func (t *Handler) generateCreatePersonaResponseFromPayload(payload []byte, sp *sign.SignedPayload, tx transaction.ITransaction) (*TransactionReply, error) {
	txVal, err := tx.Decode(payload)
	if err != nil {
		return nil, errors.New("unable to decode transaction")
	}
	return t.submitTransaction(txVal, tx, sp)
}

// submitTransaction submits a transaction to the game world, as well as the blockchain.
func (t *Handler) submitTransaction(txVal any, tx transaction.ITransaction, sp *sign.SignedPayload) (*TransactionReply, error) {

	submitTx := func() *TransactionReply {
		tick, txHash := t.w.AddTransaction(tx.ID(), txVal, sp)

		return &TransactionReply{
			TxHash: string(txHash),
			Tick:   tick,
		}
	}

	// check if we have an adapter
	if t.adapter != nil {
		// if the world is recovering via adapter, we shouldn't accept transactions.
		if t.w.IsRecovering() {
			return nil, errors.New("unable to submit transactions: game world is recovering state")
		} else {
			txReply := submitTx()
			err := t.adapter.Submit(context.Background(), sp, uint64(tx.ID()), txReply.Tick)
			if err != nil {
				return nil, fmt.Errorf("error submitting transaction to blockchain: %w", err)
			}
			return txReply, nil
		}
	} else {
		// if there is no adapter, then we can just put the tx in the queue.
		return submitTx(), nil
	}
}

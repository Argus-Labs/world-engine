package ecs

import (
	"errors"

	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
)

// CreatePersonaTransaction allows for the associating of a persona tag with a signer address.
type CreatePersonaTransaction struct {
	PersonaTag    string
	SignerAddress string
}

type CreatePersonaTransactionResult struct {
	Success bool
}

// CreatePersonaTx is a concrete ECS transaction.
var CreatePersonaTx = NewTransactionType[CreatePersonaTransaction, CreatePersonaTransactionResult]("create-persona")

type SignerComponent struct {
	PersonaTag    string
	SignerAddress string
}

// SignerComp is the concrete ECS component that pairs a persona tag to a signer address.
var SignerComp = NewComponentType[SignerComponent]()

// RegisterPersonaSystem is an ecs.System that will associate persona tags with signature addresses. Each persona tag
// may have at most 1 signer, so additional attempts to register a signer with a persona tag will be ignored.
func RegisterPersonaSystem(world *World, queue *TransactionQueue) error {
	createTxs := CreatePersonaTx.In(queue)
	if len(createTxs) == 0 {
		return nil
	}
	personaTagToAddress := map[string]string{}
	var errs []error
	NewQuery(filter.Exact(SignerComp)).Each(world, func(id storage.EntityID) bool {
		sc, err := SignerComp.Get(world, id)
		if err != nil {
			errs = append(errs, err)
			return true
		}
		personaTagToAddress[sc.PersonaTag] = sc.SignerAddress
		return true
	})
	if len(errs) != 0 {
		return errors.Join(errs...)
	}
	for _, txData := range createTxs {
		tx := txData.Value
		if _, ok := personaTagToAddress[tx.PersonaTag]; ok {
			// This PersonaTag has already been registered. Don't do anything
			continue
		}
		id, err := world.Create(SignerComp)
		if err != nil {
			CreatePersonaTx.AddError(world, txData.ID, err)
			continue
		}
		if err := SignerComp.Set(world, id, SignerComponent{
			PersonaTag:    tx.PersonaTag,
			SignerAddress: tx.SignerAddress,
		}); err != nil {
			CreatePersonaTx.AddError(world, txData.ID, err)
			continue
		}
		personaTagToAddress[tx.PersonaTag] = tx.SignerAddress
		CreatePersonaTx.SetResult(world, txData.ID, CreatePersonaTransactionResult{
			Success: true,
		})
	}

	return nil
}

var (
	ErrorPersonaTagHasNoSigner        = errors.New("persona tag does not have a signer")
	ErrorCreatePersonaTxsNotProcessed = errors.New("create persona txs have not been processed for the given tick")
)

// GetSignerForPersonaTag returns the signer address that has been registered for the given persona tag after the
// given tick. If the world's tick is less than or equal to the given tick, ErrorCreatePersonaTXsNotProcessed is returned.
// If the given personaTag has no signer address, ErrorPersonaTagHasNoSigner is returned.
func (w *World) GetSignerForPersonaTag(personaTag string, tick uint64) (addr string, err error) {
	if tick >= w.tick {
		return "", ErrorCreatePersonaTxsNotProcessed
	}
	var errs []error
	NewQuery(filter.Exact(SignerComp)).Each(w, func(id storage.EntityID) bool {
		sc, err := SignerComp.Get(w, id)
		if err != nil {
			errs = append(errs, err)
		}
		if sc.PersonaTag == personaTag {
			addr = sc.SignerAddress
			return false
		}
		return true
	})
	if len(errs) > 0 {
		return "", errors.Join(errs...)
	}

	if addr == "" {
		return "", ErrorPersonaTagHasNoSigner
	}
	return addr, nil
}

package ecs

import (
	"errors"

	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
)

type CreatePersonaTransaction struct {
	PersonaTag    string
	SignerAddress string
}

var CreatePersonaTx = NewTransactionType[CreatePersonaTransaction]("create_persona")

type SignerComponent struct {
	PersonaTag    string
	SignerAddress string
}

var SignerComp = NewComponentType[SignerComponent]()

func RegisterPersonaSystem(world *World, queue *TransactionQueue) error {
	createTxs := CreatePersonaTx.In(queue)
	if len(createTxs) == 0 {
		return nil
	}
	personaTagToAddress := map[string]string{}
	var errs []error
	NewQuery(filter.Exact(SignerComp)).Each(world, func(id storage.EntityID) {
		sc, err := SignerComp.Get(world, id)
		if err != nil {
			errs = append(errs, err)
			return
		}
		personaTagToAddress[sc.PersonaTag] = sc.SignerAddress
	})
	if len(errs) != 0 {
		return errors.Join(errs...)
	}
	for _, tx := range createTxs {
		if _, ok := personaTagToAddress[tx.PersonaTag]; ok {
			// This PersonaTag has already been registered. Don't do anything
			continue
		}
		id, err := world.Create(SignerComp)
		if err != nil {
			return err
		}
		if err := SignerComp.Set(world, id, SignerComponent{
			PersonaTag:    tx.PersonaTag,
			SignerAddress: tx.SignerAddress,
		}); err != nil {
			return err
		}
		personaTagToAddress[tx.PersonaTag] = tx.SignerAddress
	}

	return nil
}

var (
	ErrorPersonaTagHasNoSigner        = errors.New("persona tag does not have a signer")
	ErrorCreatePersonaTxsNotProcessed = errors.New("create persona txs have not been processed for the given tick")
)

func (w *World) GetSignerForPersonaTag(personaTag string, tick int) (addr string, err error) {
	if tick >= w.tick {
		return "", ErrorCreatePersonaTxsNotProcessed
	}
	var errs []error
	NewQuery(filter.Exact(SignerComp)).Each(w, func(id storage.EntityID) {
		if addr != "" {
			return
		}
		sc, err := SignerComp.Get(w, id)
		if err != nil {
			errs = append(errs, err)
		}
		if sc.PersonaTag == personaTag {
			addr = sc.SignerAddress
		}
	})
	if addr == "" {
		return "", ErrorPersonaTagHasNoSigner
	}
	return addr, nil
}

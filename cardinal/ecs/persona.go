package ecs

import (
	"errors"
	"fmt"

	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/log"
	"pkg.world.dev/world-engine/cardinal/ecs/query"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
	"pkg.world.dev/world-engine/cardinal/world_namespace"
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
var CreatePersonaTx = transaction.NewTransactionType[CreatePersonaTransaction, CreatePersonaTransactionResult](
	"create-persona",
	transaction.WithTxEVMSupport[CreatePersonaTransaction, CreatePersonaTransactionResult],
)

type AuthorizePersonaAddress struct {
	PersonaTag string
	Address    string
}

type AuthorizePersonaAddressResult struct {
	Success bool
}

var AuthorizePersonaAddressTx = transaction.NewTransactionType[AuthorizePersonaAddress, AuthorizePersonaAddressResult](
	"authorize-persona-address",
)

type SignerComponent struct {
	PersonaTag          string
	SignerAddress       string
	AuthorizedAddresses []string
}

// SignerComp is the concrete ECS component that pairs a persona tag to a signer address.
var SignerComp = component.NewComponentType[SignerComponent]("SignerComponent")

type personaTagComponentData struct {
	SignerAddress string
	EntityID      entity.ID
}

func buildPersonaTagMapping(world *World) (map[string]personaTagComponentData, error) {
	personaTagToAddress := map[string]personaTagComponentData{}
	var errs []error
	query.NewQuery(filter.Exact(SignerComp)).Each(world_namespace.Namespace(world.Namespace()), world.Store(), func(id entity.ID) bool {
		sc, err := SignerComp.Get(world.StoreManager(), id)
		if err != nil {
			errs = append(errs, err)
			return true
		}
		personaTagToAddress[sc.PersonaTag] = personaTagComponentData{
			SignerAddress: sc.SignerAddress,
			EntityID:      id,
		}
		return true
	})
	if len(errs) != 0 {
		return nil, errors.Join(errs...)
	}
	return personaTagToAddress, nil
}

// RegisterPersonaSystem is an ecs.System that will associate persona tags with signature addresses. Each persona tag
// may have at most 1 signer, so additional attempts to register a signer with a persona tag will be ignored.
func RegisterPersonaSystem(world *World, queue *transaction.TxQueue, _ *log.Logger) error {
	createTxs := CreatePersonaTx.In(queue)
	if len(createTxs) == 0 {
		return nil
	}
	personaTagToAddress, err := buildPersonaTagMapping(world)
	if err != nil {
		return err
	}
	for _, txData := range createTxs {
		tx := txData.Value
		if _, ok := personaTagToAddress[tx.PersonaTag]; ok {
			// This PersonaTag has already been registered. Don't do anything
			continue
		}
		id, err := world.StoreManager().CreateEntity(SignerComp)
		if err != nil {
			CreatePersonaTx.AddError(world.receiptHistory, txData.TxHash, err)
			continue
		}
		if err := SignerComp.Set(world.Logger, world.NameToComponent(), world.StoreManager(), id, SignerComponent{
			PersonaTag:    tx.PersonaTag,
			SignerAddress: tx.SignerAddress,
		}); err != nil {
			CreatePersonaTx.AddError(world.receiptHistory, txData.TxHash, err)
			continue
		}
		personaTagToAddress[tx.PersonaTag] = personaTagComponentData{
			SignerAddress: tx.SignerAddress,
			EntityID:      id,
		}
		CreatePersonaTx.SetResult(world.GetReceiptHistory(), txData.TxHash, CreatePersonaTransactionResult{
			Success: true,
		})
	}

	return nil
}

var (
	ErrorPersonaTagHasNoSigner        = errors.New("persona tag does not have a signer")
	ErrorCreatePersonaTxsNotProcessed = errors.New("create persona txs have not been processed for the given tick")
)

// AuthorizePersonaAddressSystem enables users to authorize an address to a persona tag. This is mostly used so that
// users who want to interact with the game via smart contract can link their EVM address to their persona tag, enabling
// them to mutate their owned state from the context of the EVM.
func AuthorizePersonaAddressSystem(world *World, queue *transaction.TxQueue, _ *log.Logger) error {
	txs := AuthorizePersonaAddressTx.In(queue)
	if len(txs) == 0 {
		return nil
	}
	personaTagToAddress, err := buildPersonaTagMapping(world)
	if err != nil {
		return err
	}
	for _, tx := range txs {
		if tx.Sig.PersonaTag != tx.Value.PersonaTag {
			AuthorizePersonaAddressTx.AddError(world.receiptHistory, tx.TxHash, fmt.Errorf("signer does not match request"))
			AuthorizePersonaAddressTx.SetResult(world.GetReceiptHistory(), tx.TxHash, AuthorizePersonaAddressResult{Success: false})
			continue
		}
		data, ok := personaTagToAddress[tx.Value.PersonaTag]
		if !ok {
			// This PersonaTag has not been registered.
			AuthorizePersonaAddressTx.AddError(world.receiptHistory, tx.TxHash, fmt.Errorf("persona does not exist"))
			AuthorizePersonaAddressTx.SetResult(world.GetReceiptHistory(), tx.TxHash, AuthorizePersonaAddressResult{Success: false})
			continue
		}
		err = SignerComp.Update(world.Logger, world.NameToComponent(), world.StoreManager(), data.EntityID, func(component SignerComponent) SignerComponent {
			// check if this address already exists
			for _, addr := range component.AuthorizedAddresses {
				// if its already in the authorized addresses slice, just return the component.
				if addr == tx.Value.Address {
					return component
				}
			}
			component.AuthorizedAddresses = append(component.AuthorizedAddresses, tx.Value.Address)
			return component
		})
		if err != nil {
			AuthorizePersonaAddressTx.AddError(world.receiptHistory, tx.TxHash, err)
			AuthorizePersonaAddressTx.SetResult(world.GetReceiptHistory(), tx.TxHash, AuthorizePersonaAddressResult{Success: false})
			continue
		}
		AuthorizePersonaAddressTx.SetResult(world.GetReceiptHistory(), tx.TxHash, AuthorizePersonaAddressResult{Success: true})
	}
	return nil
}

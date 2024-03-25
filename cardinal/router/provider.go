package router

import (
	"pkg.world.dev/world-engine/cardinal/persona/component"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/sign"
)

//go:generate mockgen -source=provider.go -package mocks -destination=mocks/provider.go

// Provider provides the Router with the necessary functionality to handle API requests from the EVM.
// The ecs.Engine is expected to implement these methods.
type Provider interface {
	GetMessageByFullName(string) (types.Message, bool)
	GetMessageByID(id types.MessageID) (types.Message, bool)
	HandleEVMQuery(name string, abiRequest []byte) ([]byte, error)
	GetSignerComponentForPersona(string) (*component.SignerComponent, error)
	WaitForNextTick() bool

	AddEVMTransaction(id types.MessageID, msgValue any, tx *sign.Transaction, evmTxHash string) (
		tick uint64, txHash types.TxHash,
	)
	ConsumeEVMMsgResult(evmTxHash string) ([]byte, []error, string, bool)
}

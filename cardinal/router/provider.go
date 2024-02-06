package router

import (
	"pkg.world.dev/world-engine/cardinal/types/message"
	"pkg.world.dev/world-engine/sign"
)

//go:generate mockgen -source=provider.go -package mocks -destination=mocks/provider.go

// Provider provides the Router with the necessary functionality to handle API requests from the EVM.
// The ecs.Engine is expected to implement these methods.
type Provider interface {
	GetMessageByName(string) (message.Message, bool)
	HandleEVMQuery(name string, abiRequest []byte) ([]byte, error)
	GetPersonaForEVMAddress(string) (string, error)
	WaitForNextTick() bool
	AddEVMTransaction(id message.TypeID, msgValue any, tx *sign.Transaction, evmTxHash string) (
		tick uint64, txHash message.TxHash,
	)
	ConsumeEVMMsgResult(evmTxHash string) ([]byte, []error, string, bool)
}

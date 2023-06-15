package main

import (
	"reflect"
	"testing"

	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/inmem"
	"github.com/argus-labs/world-engine/cardinal/evm"
)

type mockTxQueuer struct {
	c int
}

func (txq mockTxQueuer) AddTransaction(n string, v any) {
	txq.c++
}

func TestReceiverSimple(t *testing.T) {
	w := inmem.NewECSWorldForTest(t)
	handlers := map[string]evm.TxHandler{reflect.TypeOf(SendEnergyTx{}).Name(): SendEnergyTxAbiType}
	tx := ecs.NewTransactionType[SendEnergyTx](w, reflect.TypeOf(SendEnergyTx{}).Name())
	rvr := evm.NewReceiver(mockTxQueuer{}, handlers)
	rvr.SendMsg()

}

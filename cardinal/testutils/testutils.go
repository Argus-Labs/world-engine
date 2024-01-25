package testutils

import (
	"crypto/ecdsa"
	"fmt"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/sign"
)

func SetTestTimeout(t *testing.T, timeout time.Duration) {
	if _, ok := t.Deadline(); ok {
		// A deadline has already been set. Don't add an additional deadline.
		return
	}
	success := make(chan bool)
	t.Cleanup(func() {
		success <- true
	})
	go func() {
		select {
		case <-success:
			// test was successful. Do nothing
		case <-time.After(timeout):
			panic("test timed out")
		}
	}()
}

func WorldToWorldContext(world *cardinal.World) cardinal.WorldContext {
	return cardinal.TestingWorldToWorldContext(world)
}

var (
	nonce      uint64
	privateKey *ecdsa.PrivateKey
)

func UniqueSignatureWithName(name string) *sign.Transaction {
	if privateKey == nil {
		var err error
		privateKey, err = crypto.GenerateKey()
		if err != nil {
			panic(err)
		}
	}
	nonce++
	// We only verify signatures when hitting the HTTP server, and in tests we're likely just adding transactions
	// directly to the Engine queue. It's OK if the signature does not match the payload.
	sig, err := sign.NewTransaction(privateKey, name, "namespace", nonce, `{"some":"data"}`)
	if err != nil {
		panic(err)
	}
	return sig
}

func UniqueSignature() *sign.Transaction {
	return UniqueSignatureWithName("some_persona_tag")
}

func AddTransactionToWorldByAnyTransaction(
	world *cardinal.World,
	cardinalTx cardinal.AnyMessage,
	value any,
	tx *sign.Transaction) {
	worldCtx := WorldToWorldContext(world)
	ecsWorld := cardinal.TestingWorldContextToECSWorld(worldCtx)

	txs := ecsWorld.ListMessages()
	txID := cardinalTx.Convert().ID()
	found := false
	for _, tx := range txs {
		if tx.ID() == txID {
			found = true
			break
		}
	}
	if !found {
		panic(
			fmt.Sprintf(
				"cannot find transaction %q in registered transactions. Did you register it?",
				cardinalTx.Convert().Name(),
			),
		)
	}

	_, _ = ecsWorld.AddTransaction(txID, value, tx)
}

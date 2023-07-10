package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/inmem"
)

func TestCanListTransactionEndpoints(t *testing.T) {
	type SendEnergyTx struct {
		From, To string
		Amount   uint64
	}
	w := inmem.NewECSWorldForTest(t)
	alphaTx := ecs.NewTransactionType[SendEnergyTx]("alpha")
	betaTx := ecs.NewTransactionType[SendEnergyTx]("beta")
	gammaTx := ecs.NewTransactionType[SendEnergyTx]("gamma")
	assert.NilError(t, w.RegisterTransactions(alphaTx, betaTx, gammaTx))
	txh, err := NewTransactionHandler(w, DisableSignatureVerification())

	port := "4040"
	fullUrl := "http://localhost:" + port
	go txh.Serve("", "4040")

	req, err := http.NewRequest("GET", fullUrl+"/cardinal/list_endpoints", nil)
	assert.NilError(t, err)
	resp, err := http.DefaultClient.Do(req)
	assert.NilError(t, err)
	assert.Equal(t, resp.StatusCode, 200)
	bz, err := io.ReadAll(resp.Body)
	assert.NilError(t, err)
	gotEndpoints := []string{}
	assert.NilError(t, json.Unmarshal(bz, &gotEndpoints))
	wantEndpoints := []string{
		"/tx_alpha",
		"/tx_beta",
		"/tx_gamma",
	}
	assert.DeepEqual(t, wantEndpoints, gotEndpoints)
}

func TestHandleTransactionWithNoSignatureVerification(t *testing.T) {
	type SendEnergyTx struct {
		From, To string
		Amount   uint64
	}
	count := 0
	w := inmem.NewECSWorldForTest(t)
	endpoint := "move"
	sendTx := ecs.NewTransactionType[SendEnergyTx](endpoint)
	assert.NilError(t, w.RegisterTransactions(sendTx))
	w.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue) error {
		txs := sendTx.In(queue)
		assert.Equal(t, 1, len(txs))
		tx := txs[0]
		assert.Equal(t, tx.From, "me")
		assert.Equal(t, tx.To, "you")
		assert.Equal(t, tx.Amount, uint64(420))
		count++
		return nil
	})
	txh, err := NewTransactionHandler(w, DisableSignatureVerification())
	port := "4040"
	fullUrl := "http://localhost:" + port
	go txh.Serve("", "4040")

	req, err := http.NewRequest("GET", fullUrl+"/tx_"+endpoint, nil)
	assert.NilError(t, err)
	_, err = http.DefaultClient.Do(req)
	assert.NilError(t, err)

	assert.NilError(t, w.LoadGameState())
	assert.NilError(t, w.Tick(context.Background()))
	assert.Equal(t, 1, count)
}

func TestHandleWrappedTransactionWithNoSignatureVerification(t *testing.T) {
	type SendEnergyTx struct {
		From, To string
		Amount   uint64
	}
	count := 0
	endpoint := "move"
	w := inmem.NewECSWorldForTest(t)
	sendTx := ecs.NewTransactionType[SendEnergyTx](endpoint)
	assert.NilError(t, w.RegisterTransactions(sendTx))
	w.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue) error {
		txs := sendTx.In(queue)
		assert.Equal(t, 1, len(txs))
		tx := txs[0]
		assert.Equal(t, tx.From, "me")
		assert.Equal(t, tx.To, "you")
		assert.Equal(t, tx.Amount, uint64(420))
		count++
		return nil
	})

	txh, err := NewTransactionHandler(w, DisableSignatureVerification())
	port := "4040"
	fullUrl := "http://localhost:" + port
	go txh.Serve("", "4040")

	tx := SendEnergyTx{
		From:   "me",
		To:     "you",
		Amount: 420,
	}
	bz, err := json.Marshal(tx)
	assert.NilError(t, err)
	signedTx := SignedPayload{
		PersonaTag:    "some_persona",
		SignerAddress: "some_address",
		Signature:     []byte{1, 2, 3, 4},
		Payload:       bz,
	}

	bz, err = json.Marshal(&signedTx)
	assert.NilError(t, err)
	req, err := http.NewRequest("GET", fullUrl+"/tx_"+endpoint, bytes.NewReader(bz))
	assert.NilError(t, err)
	_, err = http.DefaultClient.Do(req)
	assert.NilError(t, err)

	assert.NilError(t, w.LoadGameState())
	assert.NilError(t, w.Tick(context.Background()))
	assert.Equal(t, 1, count)
}

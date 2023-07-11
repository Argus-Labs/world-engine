package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/argus-labs/world-engine/sign"
	"gotest.tools/v3/assert"

	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/inmem"
	"github.com/ethereum/go-ethereum/crypto"
)

type SendEnergyTx struct {
	From, To string
	Amount   uint64
}

func TestCanListTransactionEndpoints(t *testing.T) {
	w := inmem.NewECSWorldForTest(t)
	alphaTx := ecs.NewTransactionType[SendEnergyTx]("alpha")
	betaTx := ecs.NewTransactionType[SendEnergyTx]("beta")
	gammaTx := ecs.NewTransactionType[SendEnergyTx]("gamma")
	assert.NilError(t, w.RegisterTransactions(alphaTx, betaTx, gammaTx))
	txh, err := NewTransactionHandler(w, DisableSignatureVerification())

	port := "4040"
	fullUrl := "http://localhost:" + port
	t.Cleanup(func() { assert.NilError(t, txh.Close()) })
	go txh.Serve("", port)

	resp, err := http.Get(fullUrl + "/cardinal/list_endpoints")
	assert.NilError(t, err)
	assert.Equal(t, resp.StatusCode, 200)
	gotEndpoints := []string{}
	assert.NilError(t, json.NewDecoder(resp.Body).Decode(&gotEndpoints))

	// Make sure the gotEndpoints contains alpha, beta and gamma. It's ok
	// to have extra endpoints
	foundEndpoints := map[string]bool{
		"/tx_alpha": false,
		"/tx_beta":  false,
		"/tx_gamma": false,
	}

	for _, e := range gotEndpoints {
		if _, ok := foundEndpoints[e]; ok {
			foundEndpoints[e] = true
		}
	}

	for endpoint, found := range foundEndpoints {
		assert.Check(t, found, "endpoint %q not found", endpoint)
	}
}

func mustReadBody(t *testing.T, resp *http.Response) string {
	buf, err := io.ReadAll(resp.Body)
	assert.NilError(t, err)
	return string(buf)
}

func TestHandleTransactionWithNoSignatureVerification(t *testing.T) {
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
	assert.NilError(t, w.LoadGameState())

	tx := SendEnergyTx{
		From:   "me",
		To:     "you",
		Amount: 420,
	}
	bz, err := json.Marshal(tx)
	assert.NilError(t, err)

	txh, err := NewTransactionHandler(w, DisableSignatureVerification())
	port := "4040"
	fullUrl := "http://localhost:" + port
	t.Cleanup(func() { assert.NilError(t, txh.Close()) })
	go txh.Serve("", port)

	resp, err := http.Post(fullUrl+"/tx_"+endpoint, "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)
	assert.Equal(t, 200, resp.StatusCode, "request failed with body: %v", mustReadBody(t, resp))

	assert.NilError(t, w.Tick(context.Background()))
	assert.Equal(t, 1, count)
}

func TestHandleWrappedTransactionWithNoSignatureVerification(t *testing.T) {
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
	t.Cleanup(func() { assert.NilError(t, txh.Close()) })
	go txh.Serve("", "4040")

	tx := SendEnergyTx{
		From:   "me",
		To:     "you",
		Amount: 420,
	}
	bz, err := json.Marshal(tx)
	assert.NilError(t, err)
	signedTx := sign.SignedPayload{
		PersonaTag: "some_persona",
		Namespace:  "some_namespace",
		Nonce:      100,
		Signature:  []byte{1, 2, 3, 4},
		Body:       bz,
	}

	bz, err = json.Marshal(&signedTx)
	assert.NilError(t, err)
	_, err = http.Post(fullUrl+"/tx_"+endpoint, "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)

	assert.NilError(t, w.LoadGameState())
	assert.NilError(t, w.Tick(context.Background()))
	assert.Equal(t, 1, count)
}

func TestCanCreateAndVerifyPersonaSigner(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	tx := ecs.NewTransactionType[SendEnergyTx]("some_tx")
	world.RegisterTransactions(tx)
	assert.NilError(t, world.LoadGameState())

	txh, err := NewTransactionHandler(world)
	t.Cleanup(func() { assert.NilError(t, txh.Close()) })
	go txh.Serve("", "4040")

	personaTag := "CoolMage"

	privateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	signerAddr := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	createPersonaTx := ecs.CreatePersonaTransaction{
		PersonaTag:    personaTag,
		SignerAddress: signerAddr,
	}

	signedPayload, err := sign.NewSignedPayload(privateKey, personaTag, "world-1", 100, createPersonaTx)
	assert.NilError(t, err)

	bz, err := signedPayload.Marshal()
	assert.NilError(t, err)

	resp, err := http.Post("http://localhost:4040/tx_create_persona", "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)
	body := mustReadBody(t, resp)
	assert.Equal(t, 200, resp.StatusCode, "request failed with body: %s", body)

	var createPersonaResponse CreatePersonaResponse
	assert.NilError(t, json.Unmarshal([]byte(body), &createPersonaResponse))
	assert.Equal(t, createPersonaResponse.Status, "ok")
	tick := createPersonaResponse.Tick

	// postQueryPersonaSigner is a helper that makes a request to the query_persona_signer endpoint and returns the response
	postQueryPersonaSigner := func(personaTag string, tick int) QueryPersonaSignerResponse {
		bz, err = json.Marshal(QueryPersonaSignerRequest{
			PersonaTag: personaTag,
			Tick:       tick,
		})
		assert.NilError(t, err)
		resp, err = http.Post("http://localhost:4040/query_persona_signer", "application/json", bytes.NewReader(bz))
		assert.NilError(t, err)
		assert.Equal(t, resp.StatusCode, 200)
		var queryPersonaSignerResponse QueryPersonaSignerResponse
		assert.NilError(t, json.NewDecoder(resp.Body).Decode(&queryPersonaSignerResponse))
		return queryPersonaSignerResponse
	}

	// Check some random person tag against a tick far in the past. This should be unassigned.
	personaSignerResp := postQueryPersonaSigner("some_other_persona_tag", -100)
	assert.Equal(t, personaSignerResp.Status, "available")

	// If the game tick matches the passed in game tick, there hasn't been enough time to process the create persona tx.
	personaSignerResp = postQueryPersonaSigner(personaTag, tick)
	assert.Equal(t, personaSignerResp.Status, "unknown")

	// Tick the game state so that the persona can actually be registered
	assert.NilError(t, world.Tick(context.Background()))

	// The persona tag should now be registered with our signer address.
	personaSignerResp = postQueryPersonaSigner(personaTag, tick)
	assert.Equal(t, personaSignerResp.Status, "assigned")
	assert.Equal(t, personaSignerResp.SignerAddress, signerAddr)
}

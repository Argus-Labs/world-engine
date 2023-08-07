package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/ethereum/go-ethereum/common"

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

type SendEnergyTxResult struct{}

// testTransactionHandler is a helper struct that can start an HTTP server on port 4040 with the given world.
type testTransactionHandler struct {
	*Handler
	t         *testing.T
	urlPrefix string
}

func (t *testTransactionHandler) makeURL(path string) string {
	return t.urlPrefix + path
}

func makeTestTransactionHandler(t *testing.T, world *ecs.World, opts ...Option) *testTransactionHandler {
	txh, err := NewHandler(world, opts...)
	assert.NilError(t, err)
	t.Cleanup(func() {
		assert.NilError(t, txh.Close())
	})
	port := "4040"
	go txh.Serve("", port)
	urlPrefix := "http://localhost:" + port

	return &testTransactionHandler{
		Handler:   txh,
		t:         t,
		urlPrefix: urlPrefix,
	}
}

func TestCanListTransactionEndpoints(t *testing.T) {
	w := inmem.NewECSWorldForTest(t)
	alphaTx := ecs.NewTransactionType[SendEnergyTx, SendEnergyTxResult]("alpha")
	betaTx := ecs.NewTransactionType[SendEnergyTx, SendEnergyTxResult]("beta")
	gammaTx := ecs.NewTransactionType[SendEnergyTx, SendEnergyTxResult]("gamma")
	assert.NilError(t, w.RegisterTransactions(alphaTx, betaTx, gammaTx))
	txh := makeTestTransactionHandler(t, w, DisableSignatureVerification())

	resp, err := http.Get(txh.makeURL(listTxEndpoint))
	assert.NilError(t, err)
	assert.Equal(t, resp.StatusCode, 200)
	var gotEndpoints []string
	assert.NilError(t, json.NewDecoder(resp.Body).Decode(&gotEndpoints))

	// Make sure the gotEndpoints contains alpha, beta and gamma. It's ok to have extra endpoints
	foundEndpoints := map[string]bool{
		"/tx-alpha": false,
		"/tx-beta":  false,
		"/tx-gamma": false,
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

	w := inmem.NewECSWorldForTest(t)
	endpoint := "move"
	sendTx := ecs.NewTransactionType[SendEnergyTx, SendEnergyTxResult](endpoint)
	assert.NilError(t, w.RegisterTransactions(sendTx))
	count := 0
	w.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue) error {
		txs := sendTx.In(queue)
		assert.Equal(t, 1, len(txs))
		tx := txs[0]
		assert.Equal(t, tx.Value.From, "me")
		assert.Equal(t, tx.Value.To, "you")
		assert.Equal(t, tx.Value.Amount, uint64(420))
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
	payload := &sign.SignedPayload{
		PersonaTag: "meow",
		Namespace:  w.GetNamespace(),
		Nonce:      40,
		Signature:  "doesnt matter what goes in here",
		Body:       bz,
	}
	bogusSignatureBz, err := json.Marshal(payload)
	assert.NilError(t, err)

	txh := makeTestTransactionHandler(t, w, DisableSignatureVerification())

	resp, err := http.Post(txh.makeURL("/tx-"+endpoint), "application/json", bytes.NewReader(bogusSignatureBz))
	assert.NilError(t, err)
	assert.Equal(t, 200, resp.StatusCode, "request failed with body: %v", mustReadBody(t, resp))

	assert.NilError(t, w.Tick(context.Background()))
	assert.Equal(t, 1, count)
}

func TestHandleWrappedTransactionWithNoSignatureVerification(t *testing.T) {
	// skipping as this does not always work 100% of the time.
	// https://linear.app/arguslabs/issue/CAR-115/refactor-tests-that-use-http-calls-to-use-a-mock-http-connection
	t.Skip("this test is flaky and does not always pass")
	count := 0
	endpoint := "move"
	w := inmem.NewECSWorldForTest(t)
	sendTx := ecs.NewTransactionType[SendEnergyTx, SendEnergyTxResult](endpoint)
	assert.NilError(t, w.RegisterTransactions(sendTx))
	w.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue) error {
		txs := sendTx.In(queue)
		assert.Equal(t, 1, len(txs))
		tx := txs[0]
		assert.Equal(t, tx.Value.From, "me")
		assert.Equal(t, tx.Value.To, "you")
		assert.Equal(t, tx.Value.Amount, uint64(420))
		count++
		return nil
	})

	txh := makeTestTransactionHandler(t, w, DisableSignatureVerification())

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
		// this bogus signature is OK because DisableSignatureVerification was used
		Signature: common.Bytes2Hex([]byte{1, 2, 3, 4}),
		Body:      bz,
	}

	bz, err = json.Marshal(&signedTx)
	assert.NilError(t, err)
	_, err = http.Post(txh.makeURL("/tx-"+endpoint), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)

	assert.NilError(t, w.LoadGameState())
	assert.NilError(t, w.Tick(context.Background()))
	assert.Equal(t, 1, count)
}

func TestCanCreateAndVerifyPersonaSigner(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	tx := ecs.NewTransactionType[SendEnergyTx, SendEnergyTxResult]("some_tx")
	assert.NilError(t, world.RegisterTransactions(tx))
	assert.NilError(t, world.LoadGameState())
	assert.NilError(t, world.Tick(context.Background()))

	txh := makeTestTransactionHandler(t, world)

	personaTag := "CoolMage"
	privateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	signerAddr := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	createPersonaTx := ecs.CreatePersonaTransaction{
		PersonaTag:    personaTag,
		SignerAddress: signerAddr,
	}

	signedPayload, err := sign.NewSignedPayload(privateKey, personaTag, world.GetNamespace(), 100, createPersonaTx)
	assert.NilError(t, err)

	bz, err := signedPayload.Marshal()
	assert.NilError(t, err)

	resp, err := http.Post(txh.makeURL("/tx-create-persona"), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)
	body := mustReadBody(t, resp)
	assert.Equal(t, 200, resp.StatusCode, "request failed with body: %s", body)

	var createPersonaResponse CreatePersonaResponse
	assert.NilError(t, json.Unmarshal([]byte(body), &createPersonaResponse))
	assert.Equal(t, createPersonaResponse.Status, "ok")
	tick := createPersonaResponse.Tick

	// postReadPersonaSigner is a helper that makes a request to the read-persona-signer endpoint and returns the response
	postReadPersonaSigner := func(personaTag string, tick uint64) ReadPersonaSignerResponse {
		bz, err = json.Marshal(ReadPersonaSignerRequest{
			PersonaTag: personaTag,
			Tick:       tick,
		})
		assert.NilError(t, err)
		resp, err = http.Post(txh.makeURL("/"+readPrefix+"persona-signer"), "application/json", bytes.NewReader(bz))
		assert.NilError(t, err)
		assert.Equal(t, resp.StatusCode, 200)
		var readPersonaSignerResponse ReadPersonaSignerResponse
		assert.NilError(t, json.NewDecoder(resp.Body).Decode(&readPersonaSignerResponse))
		return readPersonaSignerResponse
	}

	// Check some random person tag against a tick far in the past. This should be available.
	personaSignerResp := postReadPersonaSigner("some_other_persona_tag", 0)
	assert.Equal(t, personaSignerResp.Status, "available")

	// If the game tick matches the passed in game tick, there hasn't been enough time to process the create persona tx.
	personaSignerResp = postReadPersonaSigner(personaTag, tick)
	assert.Equal(t, personaSignerResp.Status, "unknown")

	// Tick the game state so that the persona can actually be registered
	assert.NilError(t, world.Tick(context.Background()))

	// The persona tag should now be registered with our signer address.
	personaSignerResp = postReadPersonaSigner(personaTag, tick)
	assert.Equal(t, personaSignerResp.Status, "assigned")
	assert.Equal(t, personaSignerResp.SignerAddress, signerAddr)
}

func TestSigVerificationChecksNamespace(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	assert.NilError(t, world.LoadGameState())
	privateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)

	txh := makeTestTransactionHandler(t, world)

	personaTag := "some_dude"
	signerAddr := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	createPersonaTx := ecs.CreatePersonaTransaction{
		PersonaTag:    personaTag,
		SignerAddress: signerAddr,
	}
	sigPayload, err := sign.NewSignedPayload(privateKey, personaTag, "bad_namespace", 100, createPersonaTx)
	assert.NilError(t, err)

	bz, err := sigPayload.Marshal()
	assert.NilError(t, err)
	resp, err := http.Post(txh.makeURL("/tx-create-persona"), "application/json", bytes.NewReader(bz))
	// This should fail because the namespace does not match the world's namespace
	assert.Equal(t, resp.StatusCode, 401)

	// The namespace now matches the world
	sigPayload, err = sign.NewSignedPayload(privateKey, personaTag, world.GetNamespace(), 100, createPersonaTx)
	assert.NilError(t, err)
	bz, err = sigPayload.Marshal()
	assert.NilError(t, err)
	resp, err = http.Post(txh.makeURL("/tx-create-persona"), "application/json", bytes.NewReader(bz))
	assert.Equal(t, resp.StatusCode, 200)
}

func TestSigVerificationChecksNonce(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	assert.NilError(t, world.LoadGameState())
	privateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)

	txh := makeTestTransactionHandler(t, world)

	personaTag := "some_dude"
	signerAddr := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	namespace := world.GetNamespace()

	createPersonaTx := ecs.CreatePersonaTransaction{
		PersonaTag:    personaTag,
		SignerAddress: signerAddr,
	}
	sigPayload, err := sign.NewSignedPayload(privateKey, personaTag, namespace, 100, createPersonaTx)
	assert.NilError(t, err)
	bz, err := sigPayload.Marshal()
	assert.NilError(t, err)

	// Register a persona. This should succeed
	resp, err := http.Post(txh.makeURL("/tx-create-persona"), "application/json", bytes.NewReader(bz))
	assert.Equal(t, resp.StatusCode, 200)

	// Repeat the request. Since the nonce is the same, this should fail
	resp, err = http.Post(txh.makeURL("/tx-create-persona"), "application/json", bytes.NewReader(bz))
	assert.Equal(t, resp.StatusCode, 401)

	// Using an old nonce should fail
	sigPayload, err = sign.NewSignedPayload(privateKey, personaTag, namespace, 50, createPersonaTx)
	assert.NilError(t, err)
	bz, err = sigPayload.Marshal()
	assert.NilError(t, err)
	resp, err = http.Post(txh.makeURL("/tx-create-persona"), "application/json", bytes.NewReader(bz))
	assert.Equal(t, resp.StatusCode, 401)

	// But increasing the nonce should work
	sigPayload, err = sign.NewSignedPayload(privateKey, personaTag, namespace, 101, createPersonaTx)
	assert.NilError(t, err)
	bz, err = sigPayload.Marshal()
	assert.NilError(t, err)
	resp, err = http.Post(txh.makeURL("/tx-create-persona"), "application/json", bytes.NewReader(bz))
	assert.Equal(t, resp.StatusCode, 200)
}

// TestCanListReads tests that we can list the available queries in the handler.
func TestCanListReads(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	type FooRequest struct {
		Foo  int    `json:"foo,omitempty"`
		Meow string `json:"bar,omitempty"`
	}

	type FooResponse struct {
		Meow string `json:"meow,omitempty"`
	}

	fooRead := ecs.NewReadType[FooRequest, FooResponse]("foo", func(world *ecs.World, req FooRequest) (FooResponse, error) {
		return FooResponse{Meow: req.Meow}, nil
	})
	barRead := ecs.NewReadType[FooRequest, FooResponse]("bar", func(world *ecs.World, req FooRequest) (FooResponse, error) {

		return FooResponse{Meow: req.Meow}, nil
	})
	bazRead := ecs.NewReadType[FooRequest, FooResponse]("baz", func(world *ecs.World, req FooRequest) (FooResponse, error) {
		return FooResponse{Meow: req.Meow}, nil
	})

	assert.NilError(t, world.RegisterReads(fooRead, barRead, bazRead))
	assert.NilError(t, world.LoadGameState())

	txh := makeTestTransactionHandler(t, world, DisableSignatureVerification())

	resp, err := http.Get(txh.makeURL(listReadEndpoint))
	assert.NilError(t, err)
	assert.Equal(t, resp.StatusCode, 200)
	var gotEndpoints []string
	assert.NilError(t, json.NewDecoder(resp.Body).Decode(&gotEndpoints))

	endpoints := []string{"/read-foo", "/schema/read-foo", "/read-bar", "/schema/read-bar", "/read-baz", "/schema/read-baz", "/read-persona-signer", "/schema/read-persona-signer"}
	for i, e := range gotEndpoints {
		assert.Equal(t, e, endpoints[i])
	}
}

// TestReadEncodeDecode tests that read requests/responses are properly marshalled/unmarshalled in the context of http communication.
// We do not necessarily need to test anything w/r/t world storage, as what users decide to do within the context
// of their read requests are up to them, and not necessarily required for this feature to provably work.
func TestReadEncodeDecode(t *testing.T) {
	// setup this read business stuff

	type FooRequest struct {
		Foo  int    `json:"foo,omitempty"`
		Meow string `json:"bar,omitempty"`
	}

	type FooResponse struct {
		Meow string `json:"meow,omitempty"`
	}
	endpoint := "foo"
	fq := ecs.NewReadType[FooRequest, FooResponse](endpoint, func(world *ecs.World, req FooRequest) (FooResponse, error) {
		return FooResponse{Meow: req.Meow}, nil
	})

	// set up the world, register the reads, load.
	world := inmem.NewECSWorldForTest(t)
	assert.NilError(t, world.RegisterReads(fq))
	assert.NilError(t, world.LoadGameState())

	// make our test tx handler
	txh := makeTestTransactionHandler(t, world, DisableSignatureVerification())

	// now we set up a request, and marshal it to json to send to the handler
	req := FooRequest{Foo: 12, Meow: "hello"}
	bz, err := json.Marshal(req)
	assert.NilError(t, err)

	res, err := http.Post(txh.makeURL("/"+readPrefix+endpoint), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)

	buf, err := io.ReadAll(res.Body)
	assert.NilError(t, err)

	var fooRes FooResponse
	err = json.Unmarshal(buf, &fooRes)
	assert.NilError(t, err)

	assert.Equal(t, fooRes.Meow, req.Meow)
}

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/inmem"
	"pkg.world.dev/world-engine/sign"
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
	return t.urlPrefix + "/" + path
}

func (t *testTransactionHandler) post(path string, payload any) *http.Response {
	bz, err := json.Marshal(payload)
	assert.NilError(t.t, err)

	res, err := http.Post(t.makeURL(path), "application/json", bytes.NewReader(bz))
	assert.NilError(t.t, err)
	return res
}

func makeTestTransactionHandler(t *testing.T, world *ecs.World, swaggerFilePath string, opts ...Option) *testTransactionHandler {
	port := "4040"
	opts = append(opts, WithPort(port))
	var txh *Handler
	var err error
	if len(swaggerFilePath) == 0 {
		txh, err = NewHandler(world, opts...)
	} else {
		txh, err = NewSwaggerHandler(world, swaggerFilePath, opts...)
	}
	assert.NilError(t, err)

	healthPath := "/health"

	// Add a health check endpoint, so we can make sure the server is up and running before allowing any test
	// logic to run.
	txh.mux.HandleFunc(healthPath, func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(200)
	})

	t.Cleanup(func() {
		assert.NilError(t, txh.Close())
	})

	go func() {
		err = txh.Serve()
		// ErrServerClosed is returned from txh.Serve after txh.Close is called. This is
		// normal.
		if err != http.ErrServerClosed {
			assert.NilError(t, err)
		}
	}()

	urlPrefix := "http://localhost:" + port
	healthURL := urlPrefix + healthPath
	start := time.Now()
	for {
		assert.Check(t, time.Since(start) < time.Second, "timeout while waiting for a healthy server")

		resp, err := http.Get(healthURL)
		if err == nil && resp.StatusCode == 200 {
			// the health check endpoint was successfully queried.
			break
		}
	}

	return &testTransactionHandler{
		Handler:   txh,
		t:         t,
		urlPrefix: urlPrefix,
	}
}

func TestIfServeSetEnvVarForPort(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	alphaTx := ecs.NewTransactionType[SendEnergyTx, SendEnergyTxResult]("alpha")
	assert.NilError(t, world.RegisterTransactions(alphaTx))
	txh, err := NewHandler(world, DisableSignatureVerification())
	assert.NilError(t, err)
	t.Cleanup(func() {
		assert.NilError(t, txh.Close())
	})
	txh.port = ""
	err = os.Setenv("CARDINAL_PORT", "1337")
	assert.NilError(t, err)
	txh.initialize()
	assert.Equal(t, txh.port, "1337")
	txh.port = ""
	err = os.Setenv("CARDINAL_PORT", "133asdfsdgdfdfgdf7")
	assert.NilError(t, err)
	txh.initialize()
	assert.Equal(t, txh.port, "4040")
	err = os.Setenv("CARDINAL_PORT", "4555")
	txh.port = "bad"
	txh.initialize()
	assert.Equal(t, txh.port, "4555")
}

func TestCanListTransactionEndpoints(t *testing.T) {
	w := inmem.NewECSWorldForTest(t)
	alphaTx := ecs.NewTransactionType[SendEnergyTx, SendEnergyTxResult]("alpha")
	betaTx := ecs.NewTransactionType[SendEnergyTx, SendEnergyTxResult]("beta")
	gammaTx := ecs.NewTransactionType[SendEnergyTx, SendEnergyTxResult]("gamma")
	assert.NilError(t, w.RegisterTransactions(alphaTx, betaTx, gammaTx))
	txh := makeTestTransactionHandler(t, w, "", DisableSignatureVerification())

	resp, err := http.Get(txh.makeURL(listTxEndpoint))
	assert.NilError(t, err)
	assert.Equal(t, resp.StatusCode, 200)
	var gotEndpoints []string
	assert.NilError(t, json.NewDecoder(resp.Body).Decode(&gotEndpoints))

	// Make sure the gotEndpoints contains alpha, beta and gamma. It's ok to have extra endpoints
	foundEndpoints := map[string]bool{
		"tx-alpha": false,
		"tx-beta":  false,
		"tx-gamma": false,
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
	w.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue, _ *ecs.Logger) error {
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
		Namespace:  w.Namespace(),
		Nonce:      40,
		Signature:  "doesnt matter what goes in here",
		Body:       bz,
	}
	bogusSignatureBz, err := json.Marshal(payload)
	assert.NilError(t, err)

	txh := makeTestTransactionHandler(t, w, "", DisableSignatureVerification())

	resp, err := http.Post(txh.makeURL("tx-"+endpoint), "application/json", bytes.NewReader(bogusSignatureBz))
	assert.NilError(t, err)
	assert.Equal(t, 200, resp.StatusCode, "request failed with body: %v", mustReadBody(t, resp))

	assert.NilError(t, w.Tick(context.Background()))
	assert.Equal(t, 1, count)
}

func TestHandleSwaggerServer(t *testing.T) {
	w := inmem.NewECSWorldForTest(t)
	sendTx := ecs.NewTransactionType[SendEnergyTx, SendEnergyTxResult]("send-energy")
	assert.NilError(t, w.RegisterTransactions(sendTx))
	w.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue, _ *ecs.Logger) error {
		return nil
	})

	// Queue up a CreatePersonaTx
	personaTag := "foobar"
	signerAddress := "xyzzy"
	ecs.CreatePersonaTx.AddToQueue(w, ecs.CreatePersonaTransaction{
		PersonaTag:    personaTag,
		SignerAddress: signerAddress,
	})

	//create readers
	type FooRequest struct {
		ID string
	}
	type FooReply struct {
		Name string
		Age  uint64
	}

	expectedReply := FooReply{
		Name: "Chad",
		Age:  22,
	}
	fooRead := ecs.NewReadType[FooRequest, FooReply]("foo", func(world *ecs.World, req FooRequest) (FooReply, error) {
		return expectedReply, nil
	}, ecs.WithReadEVMSupport[FooRequest, FooReply])
	assert.NilError(t, w.RegisterReads(fooRead))

	txh := makeTestTransactionHandler(t, w, "./swagger.yml", DisableSignatureVerification())

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

	bz, err = signedTx.Marshal()
	assert.NilError(t, err)

	//Test /query/http/endpoints
	expectedEndpointResult := EndpointsResult{
		TxEndpoints:    []string{"/tx/persona/create-persona", "/tx/persona/authorize-persona-address", "/tx/game/send-energy"},
		QueryEndpoints: []string{"/query/game/foo", "/query/http/endpoints", "/query/persona/signer", "/query/receipt/list"},
	}
	resp1, err := http.Post(txh.makeURL("query/http/endpoints"), "application/json", nil)
	assert.NilError(t, err)
	defer resp1.Body.Close()
	var endpointResult EndpointsResult
	err = json.NewDecoder(resp1.Body).Decode(&endpointResult)
	assert.NilError(t, err)
	assert.Assert(t, reflect.DeepEqual(endpointResult, expectedEndpointResult))

	//Test /query/persona/signer
	gotReadPersonaSignerResponse := ReadPersonaSignerResponse{}
	expectedReadPersonaSignerResponse := ReadPersonaSignerResponse{Status: personaTag, SignerAddress: signerAddress}
	readPersonaRequest := ReadPersonaSignerRequest{
		PersonaTag: personaTag,
		Tick:       0,
	}
	readPersonaRequestData, err := json.Marshal(readPersonaRequest)
	assert.NilError(t, err)
	req, err := http.NewRequest("POST", txh.makeURL("query/persona/signer"), bytes.NewBuffer(readPersonaRequestData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Length", strconv.Itoa(len(readPersonaRequestData)))
	req.Header.Set("Accept", "application/json")
	client := http.Client{}
	ctx := context.Background()
	err = w.LoadGameState()
	assert.NilError(t, err)
	err = w.Tick(ctx)
	assert.NilError(t, err)
	resp2, err := client.Do(req)
	assert.NilError(t, err)
	defer resp2.Body.Close()
	err = json.NewDecoder(resp2.Body).Decode(&gotReadPersonaSignerResponse)
	assert.NilError(t, err)
	reflect.DeepEqual(gotReadPersonaSignerResponse, expectedReadPersonaSignerResponse)

}

func TestHandleWrappedTransactionWithNoSignatureVerification(t *testing.T) {
	count := 0
	endpoint := "move"
	w := inmem.NewECSWorldForTest(t)
	sendTx := ecs.NewTransactionType[SendEnergyTx, SendEnergyTxResult](endpoint)
	assert.NilError(t, w.RegisterTransactions(sendTx))
	w.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue, _ *ecs.Logger) error {
		txs := sendTx.In(queue)
		assert.Equal(t, 1, len(txs))
		tx := txs[0]
		assert.Equal(t, tx.Value.From, "me")
		assert.Equal(t, tx.Value.To, "you")
		assert.Equal(t, tx.Value.Amount, uint64(420))
		count++
		return nil
	})

	txh := makeTestTransactionHandler(t, w, "", DisableSignatureVerification())

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
	_, err = http.Post(txh.makeURL("tx-"+endpoint), "application/json", bytes.NewReader(bz))
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

	txh := makeTestTransactionHandler(t, world, "")

	personaTag := "CoolMage"
	privateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	signerAddr := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	createPersonaTx := ecs.CreatePersonaTransaction{
		PersonaTag:    personaTag,
		SignerAddress: signerAddr,
	}

	signedPayload, err := sign.NewSystemSignedPayload(privateKey, world.Namespace(), 100, createPersonaTx)
	assert.NilError(t, err)

	bz, err := signedPayload.Marshal()
	assert.NilError(t, err)

	resp, err := http.Post(txh.makeURL("tx-create-persona"), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)
	body := mustReadBody(t, resp)
	assert.Equal(t, 200, resp.StatusCode, "request failed with body: %s", body)

	var txReply TransactionReply
	assert.NilError(t, json.Unmarshal([]byte(body), &txReply))
	assert.Equal(t, txReply.Tick, world.CurrentTick())
	tick := txReply.Tick

	// postReadPersonaSigner is a helper that makes a request to the read-persona-signer endpoint and returns the response
	postReadPersonaSigner := func(personaTag string, tick uint64) ReadPersonaSignerResponse {
		bz, err = json.Marshal(ReadPersonaSignerRequest{
			PersonaTag: personaTag,
			Tick:       tick,
		})
		assert.NilError(t, err)
		resp, err = http.Post(txh.makeURL(readPrefix+"persona-signer"), "application/json", bytes.NewReader(bz))
		assert.NilError(t, err)
		assert.Equal(t, resp.StatusCode, 200)
		var readPersonaSignerResponse ReadPersonaSignerResponse
		assert.NilError(t, json.NewDecoder(resp.Body).Decode(&readPersonaSignerResponse))
		return readPersonaSignerResponse
	}

	// Check some random person tag against a tick far in the past. This should be available.
	personaSignerResp := postReadPersonaSigner("some_other_persona_tag", 0)
	assert.Equal(t, personaSignerResp.Status, "available")

	// If the game tick matches the passed in game tick, there hasn't been enough time to process the create_persona_tx.
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

	txh := makeTestTransactionHandler(t, world, "")

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
	resp, err := http.Post(txh.makeURL("tx-create-persona"), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)
	// This should fail because the namespace does not match the world's namespace
	assert.Equal(t, resp.StatusCode, 401)

	// The namespace now matches the world
	sigPayload, err = sign.NewSystemSignedPayload(privateKey, world.Namespace(), 100, createPersonaTx)
	assert.NilError(t, err)
	bz, err = sigPayload.Marshal()
	assert.NilError(t, err)
	resp, err = http.Post(txh.makeURL("tx-create-persona"), "application/json", bytes.NewReader(bz))
	assert.Equal(t, resp.StatusCode, 200)
}

func TestSigVerificationChecksNonce(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	assert.NilError(t, world.LoadGameState())
	privateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)

	txh := makeTestTransactionHandler(t, world, "")

	personaTag := "some_dude"
	signerAddr := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	namespace := world.Namespace()

	createPersonaTx := ecs.CreatePersonaTransaction{
		PersonaTag:    personaTag,
		SignerAddress: signerAddr,
	}
	sigPayload, err := sign.NewSystemSignedPayload(privateKey, namespace, 100, createPersonaTx)
	assert.NilError(t, err)
	bz, err := sigPayload.Marshal()
	assert.NilError(t, err)

	// Register a persona. This should succeed
	resp, err := http.Post(txh.makeURL("tx-create-persona"), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)
	assert.Equal(t, resp.StatusCode, 200)

	// Repeat the request. Since the nonce is the same, this should fail
	resp, err = http.Post(txh.makeURL("tx-create-persona"), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)
	assert.Equal(t, resp.StatusCode, 401)

	// Using an old nonce should fail
	sigPayload, err = sign.NewSystemSignedPayload(privateKey, namespace, 50, createPersonaTx)
	assert.NilError(t, err)
	bz, err = sigPayload.Marshal()
	assert.NilError(t, err)
	resp, err = http.Post(txh.makeURL("tx-create-persona"), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)
	assert.Equal(t, resp.StatusCode, 401)

	// But increasing the nonce should work
	sigPayload, err = sign.NewSystemSignedPayload(privateKey, namespace, 101, createPersonaTx)
	assert.NilError(t, err)
	bz, err = sigPayload.Marshal()
	assert.NilError(t, err)
	resp, err = http.Post(txh.makeURL("tx-create-persona"), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)
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

	txh := makeTestTransactionHandler(t, world, "", DisableSignatureVerification())

	resp, err := http.Get(txh.makeURL(listReadEndpoint))
	assert.NilError(t, err)
	assert.Equal(t, resp.StatusCode, 200)
	var gotEndpoints []string
	assert.NilError(t, json.NewDecoder(resp.Body).Decode(&gotEndpoints))

	endpoints := []string{"read-foo",
		"schema/read-foo",
		"read-bar",
		"schema/read-bar",
		"read-baz",
		"schema/read-baz",
		"read-persona-signer",
		"schema/read-persona-signer",
	}
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
	txh := makeTestTransactionHandler(t, world, "", DisableSignatureVerification())

	// now we set up a request, and marshal it to json to send to the handler
	req := FooRequest{Foo: 12, Meow: "hello"}
	bz, err := json.Marshal(req)
	assert.NilError(t, err)

	res, err := http.Post(txh.makeURL(readPrefix+endpoint), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)

	buf, err := io.ReadAll(res.Body)
	assert.NilError(t, err)

	var fooRes FooResponse
	err = json.Unmarshal(buf, &fooRes)
	assert.NilError(t, err)

	assert.Equal(t, fooRes.Meow, req.Meow)
}

func TestMalformedRequestToGetTransactionReceiptsProducesError(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	assert.NilError(t, world.LoadGameState())
	txh := makeTestTransactionHandler(t, world, "", DisableSignatureVerification())
	res := txh.post(txReceiptsEndpoint, map[string]any{
		"missing_start_tick": 0,
	})
	assert.Check(t, 400 <= res.StatusCode && res.StatusCode <= 499)
}

func TestTransactionReceiptReturnCorrectTickWindows(t *testing.T) {
	historySize := uint64(10)
	world := inmem.NewECSWorldForTest(t, ecs.WithReceiptHistorySize(int(historySize)))
	assert.NilError(t, world.LoadGameState())
	txh := makeTestTransactionHandler(t, world, "", DisableSignatureVerification())

	// getReceipts is a helper that hits the txReceiptsEndpoint endpoint.
	getReceipts := func(start uint64) ListTxReceiptsReply {
		res := txh.post(txReceiptsEndpoint, ListTxReceiptsRequest{
			StartTick: start,
		})
		assert.Equal(t, 200, res.StatusCode)
		var reply ListTxReceiptsReply
		assert.NilError(t, json.NewDecoder(res.Body).Decode(&reply))
		return reply
	}
	tick := world.CurrentTick()
	// Attempting to get receipt data for the current tick should return no valid ticks as the
	// transactions have not yet been processed.
	reply := getReceipts(tick)
	tickCount := reply.EndTick - reply.StartTick
	assert.Equal(t, uint64(0), tickCount)

	// Attempting to get ticks in the future should also produce no valid ticks.
	reply = getReceipts(tick + 10000)
	tickCount = reply.EndTick - reply.StartTick
	assert.Equal(t, uint64(0), tickCount)

	// Tick once
	ctx := context.Background()
	assert.NilError(t, world.Tick(ctx))

	// The world ticked one time, so we should find 1 valid tick.
	reply = getReceipts(tick)
	tickCount = reply.EndTick - reply.StartTick
	assert.Equal(t, uint64(1), tickCount)
	assert.Equal(t, tick, reply.StartTick)

	// tick a bunch so that the tick history becomes fully populated
	jumpAhead := historySize * 2
	for i := uint64(0); i < jumpAhead; i++ {
		assert.NilError(t, world.Tick(ctx))
	}

	reply = getReceipts(tick)
	// We should find at most historySize valid ticks
	tickCount = reply.EndTick - reply.StartTick
	// EndTick is not actually included in the results. e.g. if StartTick and EndTick are equal,
	// tickCount will be 0, meaning no ticks are included in the results.
	assert.Equal(t, historySize, tickCount-1)
	// We jumped ahead quite a bit, so the returned StartTick should be ahead of the tick we asked for
	wantStartTick := tick + jumpAhead - historySize
	assert.Equal(t, wantStartTick, reply.StartTick)

	// Another way to figure out what StartTick should be is to subtract historySize from the current tick.
	// This is the oldest tick available to us.
	wantStartTick = world.CurrentTick() - historySize - 1
	assert.Equal(t, wantStartTick, reply.StartTick)

	// assuming wantStartTick is the oldest tick we can ask for if we ask for 3 ticks after that we
	// should get the remaining of historySize.
	tick = wantStartTick + 3
	reply = getReceipts(tick)
	tickCount = reply.EndTick - reply.StartTick - 1
	assert.Equal(t, historySize-3, tickCount)
}

func TestCanGetTransactionReceipts(t *testing.T) {
	// IncRequest in a transaction that increments the given number by 1.
	type IncRequest struct {
		Number int
	}
	type IncReply struct {
		Number int
	}

	// DupeRequest is a transaction that appends a copy of the given string to itself.
	type DupeRequest struct {
		Str string
	}
	type DupeReply struct {
		Str string
	}

	// ErrRequest is a transaction that will produce an error
	type ErrRequest struct{}
	type ErrReply struct{}

	incTx := ecs.NewTransactionType[IncRequest, IncReply]("increment")
	dupeTx := ecs.NewTransactionType[DupeRequest, DupeReply]("duplicate")
	errTx := ecs.NewTransactionType[ErrRequest, ErrReply]("error")

	world := inmem.NewECSWorldForTest(t)
	assert.NilError(t, world.RegisterTransactions(incTx, dupeTx, errTx))
	// System to handle incrementing numbers
	world.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue, _ *ecs.Logger) error {
		for _, tx := range incTx.In(queue) {
			incTx.SetResult(world, tx.TxHash, IncReply{
				Number: tx.Value.Number + 1,
			})
		}
		return nil
	})
	// System to handle duplicating strings
	world.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue, _ *ecs.Logger) error {
		for _, tx := range dupeTx.In(queue) {
			dupeTx.SetResult(world, tx.TxHash, DupeReply{
				Str: tx.Value.Str + tx.Value.Str,
			})
		}
		return nil
	})
	wantError := errors.New("some error")
	// System to handle error production
	world.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue, _ *ecs.Logger) error {
		for _, tx := range errTx.In(queue) {
			errTx.AddError(world, tx.TxHash, wantError)
			errTx.AddError(world, tx.TxHash, wantError)
		}
		return nil
	})
	assert.NilError(t, world.LoadGameState())

	// World setup is done. First check that there are no transactions.
	ctx := context.Background()
	assert.NilError(t, world.Tick(ctx))

	txh := makeTestTransactionHandler(t, world, "", DisableSignatureVerification())

	// We're going to be getting the list of receipts a lot, so make a helper to fetch the receipts
	getReceipts := func(start uint64) ListTxReceiptsReply {
		res := txh.post(txReceiptsEndpoint, ListTxReceiptsRequest{
			StartTick: start,
		})
		assert.Equal(t, 200, res.StatusCode)

		var txReceipts ListTxReceiptsReply
		assert.NilError(t, json.NewDecoder(res.Body).Decode(&txReceipts))
		return txReceipts
	}

	txReceipts := getReceipts(0)
	// Receipts should start out empty
	assert.Equal(t, uint64(0), txReceipts.StartTick)
	assert.Equal(t, 0, len(txReceipts.Receipts))

	nonce := uint64(0)
	privateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	nextSig := func() *sign.SignedPayload {
		sig, err := sign.NewSignedPayload(privateKey, "my-persona-tag", "namespace", nonce, `{"data": "stuff"}`)
		assert.NilError(t, err)
		nonce++
		return sig
	}

	incID := incTx.AddToQueue(world, IncRequest{99}, nextSig())
	dupeID := dupeTx.AddToQueue(world, DupeRequest{"foobar"}, nextSig())
	errID := errTx.AddToQueue(world, ErrRequest{}, nextSig())
	assert.Check(t, incID != dupeID)
	assert.Check(t, dupeID != errID)
	assert.Check(t, errID != incID)

	wantTick := world.CurrentTick()
	assert.NilError(t, world.Tick(ctx))

	txReceipts = getReceipts(0)
	assert.Equal(t, uint64(0), txReceipts.StartTick)
	assert.Equal(t, uint64(2), txReceipts.EndTick)
	assert.Equal(t, 3, len(txReceipts.Receipts))

	foundInc, foundDupe, foundErr := false, false, false
	for _, r := range txReceipts.Receipts {
		assert.Equal(t, wantTick, r.Tick)
		if len(r.Errors) > 0 {
			foundErr = true
			assert.Equal(t, 2, len(r.Errors))
			assert.Equal(t, wantError.Error(), r.Errors[0])
			assert.Equal(t, wantError.Error(), r.Errors[1])
			continue
		}
		m, ok := r.Result.(map[string]any)
		assert.Check(t, ok)
		if _, ok := m["Number"]; ok {
			foundInc = true
			num, ok := m["Number"].(float64)
			assert.Check(t, ok)
			assert.Equal(t, 100, int(num))
		} else if _, ok := m["Str"]; ok {
			foundDupe = true
			str, ok := m["Str"].(string)
			assert.Check(t, ok)
			assert.Equal(t, "foobarfoobar", str)
		} else {
			assert.Assert(t, false, "unknown transaction result", r.Result)
		}
	}

	assert.Check(t, foundInc)
	assert.Check(t, foundDupe)
	assert.Check(t, foundErr)
}

func TestTransactionIDIsReturned(t *testing.T) {
	type MoveTx struct{}
	world := inmem.NewECSWorldForTest(t)
	moveTx := ecs.NewTransactionType[MoveTx, MoveTx]("move")
	world.RegisterTransactions(moveTx)
	assert.NilError(t, world.LoadGameState())
	ctx := context.Background()
	// Preemptive tick so the tick isn't the zero value
	assert.NilError(t, world.Tick(ctx))
	privateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	txh := makeTestTransactionHandler(t, world, "")

	personaTag := "clifford_the_big_red_dog"
	signerAddr := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	namespace := world.Namespace()
	nonce := uint64(99)

	createPersonaTx := ecs.CreatePersonaTransaction{
		PersonaTag:    personaTag,
		SignerAddress: signerAddr,
	}

	sigPayload, err := sign.NewSystemSignedPayload(privateKey, namespace, nonce, createPersonaTx)
	assert.NilError(t, err)
	bz, err := sigPayload.Marshal()
	assert.NilError(t, err)

	resp, err := http.Post(txh.makeURL("tx-create-persona"), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var txReply TransactionReply
	assert.NilError(t, json.NewDecoder(resp.Body).Decode(&txReply))

	// The hash field should not be empty
	assert.Check(t, txReply.TxHash != "")
	// The tick should equal the current tick
	assert.Equal(t, world.CurrentTick(), txReply.Tick)

	assert.NilError(t, world.Tick(ctx))

	// Also check to make sure transaction IDs are returned for other kinds of transactions
	nonce++
	emptyData := map[string]any{}
	sigPayload, err = sign.NewSignedPayload(privateKey, personaTag, namespace, nonce, emptyData)

	bz, err = sigPayload.Marshal()
	assert.NilError(t, err)

	resp, err = http.Post(txh.makeURL("tx-move"), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.NilError(t, json.NewDecoder(resp.Body).Decode(&txReply))

	// The hash field should not be empty
	assert.Check(t, txReply.TxHash != "")
	// The tick should equal the current tick
	assert.Equal(t, world.CurrentTick(), txReply.Tick)
}

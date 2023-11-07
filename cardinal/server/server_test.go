package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"testing"
	"time"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"github.com/gorilla/websocket"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/shard"
	"pkg.world.dev/world-engine/chain/x/shard/types"

	"gotest.tools/v3/assert"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/cql"
	"pkg.world.dev/world-engine/cardinal/server"
	"pkg.world.dev/world-engine/sign"
)

type SendEnergyTx struct {
	From, To string
	Amount   uint64
}

type SendEnergyTxResult struct{}

func TestHealthEndpoint(t *testing.T) {
	testutils.SetTestTimeout(t, 10*time.Second)
	w := ecs.NewTestWorld(t)
	assert.NilError(t, w.LoadGameState())
	testutils.MakeTestTransactionHandler(t, w, server.DisableSignatureVerification())
	resp, err := http.Get("http://localhost:4040/health")
	assert.NilError(t, err)
	assert.Equal(t, resp.StatusCode, 200)
	var healthResponse server.HealthReply
	err = json.NewDecoder(resp.Body).Decode(&healthResponse)
	assert.NilError(t, err)
	assert.Assert(t, healthResponse.IsServerRunning)
	assert.Assert(t, !healthResponse.IsGameLoopRunning)
	ctx := context.Background()
	w.StartGameLoop(ctx, time.Tick(1*time.Second), nil)
	isGameLoopRunning := false
	for !isGameLoopRunning {
		time.Sleep(200 * time.Millisecond)
		resp, err = http.Get("http://localhost:4040/health")
		assert.NilError(t, err)
		assert.Equal(t, resp.StatusCode, 200)
		err = json.NewDecoder(resp.Body).Decode(&healthResponse)
		assert.NilError(t, err)
		assert.Assert(t, healthResponse.IsServerRunning)
		isGameLoopRunning = healthResponse.IsGameLoopRunning
	}
}

type Alpha struct{}

func (Alpha) Name() string { return "alpha" }

type Beta struct{}

func (Beta) Name() string { return "beta" }

type Gamma struct{}

func (Gamma) Name() string { return "gamma" }

func TestDebugEndpoint(t *testing.T) {
	world := ecs.NewTestWorld(t)

	assert.NilError(t, ecs.RegisterComponent[Alpha](world))
	assert.NilError(t, ecs.RegisterComponent[Beta](world))
	assert.NilError(t, ecs.RegisterComponent[Gamma](world))

	assert.NilError(t, world.LoadGameState())
	ctx := context.Background()
	worldCtx := ecs.NewWorldContext(world)
	_, err := component.CreateMany(worldCtx, 10, Alpha{})
	assert.NilError(t, err)
	_, err = component.CreateMany(worldCtx, 10, Beta{})
	assert.NilError(t, err)
	_, err = component.CreateMany(worldCtx, 10, Gamma{})
	assert.NilError(t, err)
	_, err = component.CreateMany(worldCtx, 10, Alpha{}, Beta{})
	assert.NilError(t, err)
	_, err = component.CreateMany(worldCtx, 10, Alpha{}, Gamma{})
	assert.NilError(t, err)
	_, err = component.CreateMany(worldCtx, 10, Beta{}, Gamma{})
	assert.NilError(t, err)
	_, err = component.CreateMany(worldCtx, 10, Alpha{}, Beta{}, Gamma{})
	assert.NilError(t, err)
	err = world.Tick(ctx)
	assert.NilError(t, err)
	txh := testutils.MakeTestTransactionHandler(t, world, server.DisableSignatureVerification())
	resp := txh.Get("debug/state")
	assert.Equal(t, resp.StatusCode, 200)
	bz, err := io.ReadAll(resp.Body)
	assert.NilError(t, err)
	data := make([]json.RawMessage, 0)
	err = json.Unmarshal(bz, &data)
	assert.NilError(t, err)
	assert.Equal(t, len(data), 10*7)
}

func TestShutDownViaMethod(t *testing.T) {
	// If this test is frozen then it failed to shut down, create failure with panic.
	testutils.SetTestTimeout(t, 10*time.Second)
	w := ecs.NewTestWorld(t)
	assert.NilError(t, w.LoadGameState())
	txh := testutils.MakeTestTransactionHandler(t, w, server.DisableSignatureVerification())
	resp, err := http.Get("http://localhost:4040/health")
	assert.NilError(t, err)
	assert.Equal(t, resp.StatusCode, 200)
	ctx := context.Background()
	w.StartGameLoop(ctx, time.Tick(1*time.Second), nil)
	for !w.IsGameLoopRunning() {
		// wait until game loop is running.
		time.Sleep(1 * time.Millisecond)
	}
	gameObject := server.NewGameManager(w, txh.Handler)
	err = gameObject.Shutdown() // Should block until loop is down.
	assert.NilError(t, err)
	assert.Assert(t, !w.IsGameLoopRunning())
	_, err = http.Get("http://localhost:4040/health")
	assert.Check(t, err != nil)
}

func TestShutDownViaSignal(t *testing.T) {
	// If this test is frozen then it failed to shut down, create a failure with panic.
	testutils.SetTestTimeout(t, 10*time.Second)
	w := ecs.NewTestWorld(t)
	sendTx := ecs.NewTransactionType[SendEnergyTx, SendEnergyTxResult]("sendTx")
	assert.NilError(t, w.RegisterTransactions(sendTx))
	w.AddSystem(func(ecs.WorldContext) error {
		return nil
	})
	assert.NilError(t, w.LoadGameState())
	txh := testutils.MakeTestTransactionHandler(t, w, server.DisableSignatureVerification())
	resp, err := http.Get("http://localhost:4040/health")
	assert.NilError(t, err)
	assert.Equal(t, resp.StatusCode, 200)
	ctx := context.Background()
	w.StartGameLoop(ctx, time.Tick(1*time.Second), nil)
	for !w.IsGameLoopRunning() {
		// wait until game loop is running
		time.Sleep(500 * time.Millisecond)
	}
	_ = server.NewGameManager(w, txh.Handler)

	// Send a SIGINT signal.
	cmd := exec.Command("kill", "-INT", strconv.Itoa(os.Getpid()))
	err = cmd.Run()
	assert.NilError(t, err)

	// wait for game loop and server to shut down.
	for w.IsGameLoopRunning() {
		time.Sleep(500 * time.Millisecond)
	}
	_, err = http.Get("http://localhost:4040/health")
	assert.Check(t, err != nil) // Server must shutdown before game loop. So if the gameloop turned off
}

func TestIfServeSetEnvVarForPort(t *testing.T) {
	world := ecs.NewTestWorld(t)
	alphaTx := ecs.NewTransactionType[SendEnergyTx, SendEnergyTxResult]("alpha")
	assert.NilError(t, world.RegisterTransactions(alphaTx))
	txh, err := server.NewHandler(world, nil, server.DisableSignatureVerification())
	assert.NilError(t, err)
	t.Cleanup(func() {
		assert.NilError(t, txh.Close())
	})
	txh.Port = ""
	t.Setenv("CARDINAL_PORT", "1337")
	txh.Initialize()
	assert.Equal(t, txh.Port, "1337")
	txh.Port = ""
	t.Setenv("CARDINAL_PORT", "133asdfsdgdfdfgdf7")
	txh.Initialize()
	assert.Equal(t, txh.Port, "4040")
	t.Setenv("CARDINAL_PORT", "4555")
	txh.Port = "bad"
	txh.Initialize()
	assert.Equal(t, txh.Port, "4555")
}

func TestCanListTransactionEndpoints(t *testing.T) {
	w := ecs.NewTestWorld(t)
	alphaTx := ecs.NewTransactionType[SendEnergyTx, SendEnergyTxResult]("alpha")
	betaTx := ecs.NewTransactionType[SendEnergyTx, SendEnergyTxResult]("beta")
	gammaTx := ecs.NewTransactionType[SendEnergyTx, SendEnergyTxResult]("gamma")
	assert.NilError(t, w.RegisterTransactions(alphaTx, betaTx, gammaTx))
	txh := testutils.MakeTestTransactionHandler(t, w, server.DisableSignatureVerification())

	resp, err := http.Post(txh.MakeHTTPURL("query/http/endpoints"), "application/json", nil)
	assert.NilError(t, err)
	assert.Equal(t, resp.StatusCode, 200)
	var gotEndpoints map[string][]string
	assert.NilError(t, json.NewDecoder(resp.Body).Decode(&gotEndpoints))

	// Make sure the gotEndpoints contains alpha, beta and gamma. It's ok to have extra endpoints
	foundEndpoints := map[string]bool{
		"/tx/game/alpha": false,
		"/tx/game/beta":  false,
		"/tx/game/gamma": false,
	}

	for _, e := range gotEndpoints["txEndpoints"] {
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
	endpoint := "move"
	url := "tx/game/" + endpoint
	w := ecs.NewTestWorld(t)
	sendTx := ecs.NewTransactionType[SendEnergyTx, SendEnergyTxResult](endpoint)
	assert.NilError(t, w.RegisterTransactions(sendTx))
	count := 0
	w.AddSystem(func(wCtx ecs.WorldContext) error {
		txs := sendTx.In(wCtx)
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
	payload := &sign.Transaction{
		PersonaTag: "meow",
		Namespace:  w.Namespace().String(),
		Nonce:      40,
		Signature:  "doesnt matter what goes in here",
		Body:       bz,
	}
	bogusSignatureBz, err := json.Marshal(payload)
	assert.NilError(t, err)

	txh := testutils.MakeTestTransactionHandler(t, w, server.DisableSignatureVerification())

	resp, err := http.Post(txh.MakeHTTPURL(url), "application/json", bytes.NewReader(bogusSignatureBz))
	assert.NilError(t, err)
	assert.Equal(t, 200, resp.StatusCode, "request failed with body: %v", mustReadBody(t, resp))

	assert.NilError(t, w.Tick(context.Background()))
	assert.Equal(t, 1, count)
	err = txh.Close()
	assert.NilError(t, err)
}

type garbageStructAlpha struct {
	Something int `json:"something"`
}

func (garbageStructAlpha) Name() string { return "alpha" }

type garbageStructBeta struct {
	Something int `json:"something"`
}

func (garbageStructBeta) Name() string { return "beta" }

func TestHandleSwaggerServer(t *testing.T) {
	w := ecs.NewTestWorld(t)
	sendTx := ecs.NewTransactionType[SendEnergyTx, SendEnergyTxResult]("send-energy")
	assert.NilError(t, w.RegisterTransactions(sendTx))
	w.AddSystem(func(ecs.WorldContext) error {
		return nil
	})

	assert.NilError(t, ecs.RegisterComponent[garbageStructAlpha](w))
	assert.NilError(t, ecs.RegisterComponent[garbageStructBeta](w))
	alphaCount := 75
	wCtx := ecs.NewWorldContext(w)
	_, err := component.CreateMany(wCtx, alphaCount, garbageStructAlpha{})
	assert.NilError(t, err)
	bothCount := 100
	_, err = component.CreateMany(wCtx, bothCount, garbageStructAlpha{}, garbageStructBeta{})
	assert.NilError(t, err)

	// Queue up a CreatePersonaTx
	personaTag := "foobar"
	signerAddress := "xyzzy"
	ecs.CreatePersonaTx.AddToQueue(w, ecs.CreatePersonaTransaction{
		PersonaTag:    personaTag,
		SignerAddress: signerAddress,
	})
	authorizedPersonaAddress := ecs.AuthorizePersonaAddress{
		Address: signerAddress,
	}
	ecs.AuthorizePersonaAddressTx.AddToQueue(w, authorizedPersonaAddress, &sign.Transaction{PersonaTag: personaTag})
	// PersonaTag registration doesn't take place until the relevant system is run during a game tick.

	// create readers
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
	fooQuery := ecs.NewQueryType[FooRequest, FooReply](
		"foo",
		func(wCtx ecs.WorldContext, req FooRequest,
		) (FooReply, error) {
			return expectedReply, nil
		})
	assert.NilError(t, w.RegisterQueries(fooQuery))

	txh := testutils.MakeTestTransactionHandler(t, w, server.DisableSignatureVerification())

	// Test /query/http/endpoints
	expectedEndpointResult := server.EndpointsResult{
		TxEndpoints: []string{
			"/tx/persona/create-persona", "/tx/game/authorize-persona-address", "/tx/game/send-energy"},
		QueryEndpoints: []string{
			"/query/game/foo", "/query/http/endpoints", "/query/persona/signer",
			"/query/receipt/list", "/query/game/cql",
		},
	}
	resp1, err := http.Post(txh.MakeHTTPURL("query/http/endpoints"), "application/json", nil)
	assert.NilError(t, err)
	defer resp1.Body.Close()
	var endpointResult server.EndpointsResult
	err = json.NewDecoder(resp1.Body).Decode(&endpointResult)
	assert.NilError(t, err)
	assert.Assert(t, reflect.DeepEqual(endpointResult, expectedEndpointResult))

	// Test /query/persona/signer
	gotQueryPersonaSignerResponse := server.QueryPersonaSignerResponse{}
	expectedQueryPersonaSignerResponse := server.QueryPersonaSignerResponse{
		Status:        personaTag,
		SignerAddress: signerAddress,
	}
	queryPersonaRequest := server.QueryPersonaSignerRequest{
		PersonaTag: personaTag,
		Tick:       0,
	}
	queryPersonaRequestData, err := json.Marshal(queryPersonaRequest)
	assert.NilError(t, err)
	req, err := http.NewRequest(
		http.MethodPost,
		txh.MakeHTTPURL("query/persona/signer"),
		bytes.NewBuffer(queryPersonaRequestData),
	)
	assert.NilError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Length", strconv.Itoa(len(queryPersonaRequestData)))
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
	err = json.NewDecoder(resp2.Body).Decode(&gotQueryPersonaSignerResponse)
	assert.NilError(t, err)
	reflect.DeepEqual(gotQueryPersonaSignerResponse, expectedQueryPersonaSignerResponse)

	// Test /query/game/foo
	fooRequest := FooRequest{ID: "1"}
	fooData, err := json.Marshal(fooRequest)
	if err != nil {
		assert.NilError(t, err)
	}
	resp3, err := http.Post(txh.MakeHTTPURL("query/game/foo"), "application/json", bytes.NewBuffer(fooData))
	if err != nil {
		assert.NilError(t, err)
	}
	defer resp3.Body.Close()
	actualFooReply := FooReply{
		Name: "",
		Age:  0,
	}
	err = json.NewDecoder(resp3.Body).Decode(&actualFooReply)
	if err != nil {
		assert.NilError(t, err)
	}
	assert.DeepEqual(t, actualFooReply, expectedReply)
	personaAddressJSON, err := json.Marshal(authorizedPersonaAddress)
	assert.NilError(t, err)
	// tx/game/authorize-persona-address
	signedTxPayload := sign.Transaction{
		PersonaTag: personaTag,
		Namespace:  "some_namespace",
		Nonce:      100,
		// this bogus signature is OK because DisableSignatureVerification was used
		Signature: common.Bytes2Hex([]byte{1, 2, 3, 4}),
		Body:      personaAddressJSON,
	}
	signedTxJSON, err := json.Marshal(signedTxPayload)
	assert.NilError(t, err)
	gotTxReply := server.TransactionReply{}
	resp4, err := http.Post(txh.MakeHTTPURL("tx/game/authorize-persona-address"), "application/json",
		bytes.NewBuffer(signedTxJSON))
	assert.NilError(t, err)
	err = json.NewDecoder(resp4.Body).Decode(&gotTxReply)
	assert.NilError(t, err)
	assert.Check(t, gotTxReply.Tick > 0)
	assert.Check(t, gotTxReply.TxHash != "")

	resp5, err := http.Post(txh.MakeHTTPURL("tx/game/dsakjsdlfksdj"), "application/json",
		bytes.NewBuffer(signedTxJSON))
	assert.NilError(t, err)
	assert.Equal(t, resp5.StatusCode, 404)
	resp6, err := http.Post(txh.MakeHTTPURL("query/game/sdsdfsdfsdf"), "application/json",
		bytes.NewBuffer(signedTxJSON))
	assert.NilError(t, err)
	assert.Equal(t, resp6.StatusCode, 404)

	// Test query/game/cql
	for _, v := range []struct {
		cql            string
		expectedStatus int
		amount         int
	}{
		{cql: "CONTAINS(alpha) & CONTAINS(beta)", expectedStatus: 200, amount: bothCount},
		{cql: "CONTAINS(alpha) | CONTAINS(beta)", expectedStatus: 200, amount: bothCount + alphaCount},
		{cql: "CONTAINS(beta)", expectedStatus: 200, amount: bothCount},
		{cql: "EXACT(alpha)", expectedStatus: 200, amount: alphaCount},
		{cql: "EXACT(beta)", expectedStatus: 200, amount: 0},
		{cql: "!(CONTAINS(alpha) | CONTAINS(beta))", expectedStatus: 200, amount: 1},
		{cql: "!CONTAINS(alpha) & CONTAINS(beta)", expectedStatus: 200, amount: 0},
	} {
		jsonQuery := struct{ CQL string }{v.cql}
		jsonQueryBytes, err := json.Marshal(jsonQuery)
		assert.NilError(t, err)
		resp7, err := http.Post(txh.MakeHTTPURL("query/game/cql"), "application/json", bytes.NewBuffer(jsonQueryBytes))
		assert.NilError(t, err)
		assert.Equal(t, resp7.StatusCode, v.expectedStatus)
		var entities []cql.QueryResponse
		err = json.NewDecoder(resp7.Body).Decode(&entities)
		assert.NilError(t, err)
		assert.Equal(t, len(entities), v.amount)
	}

	jsonQuery := struct{ CQL string }{"blah"}
	jsonQueryBytes, err := json.Marshal(jsonQuery)
	assert.NilError(t, err)
	resp8, err := http.Post(txh.MakeHTTPURL("query/game/cql"), "application/json", bytes.NewBuffer(jsonQueryBytes))
	assert.NilError(t, err)
	assert.Equal(t, resp8.StatusCode, 422)
}

func TestHandleWrappedTransactionWithNoSignatureVerification(t *testing.T) {
	endpoint := "move"
	url := fmt.Sprintf("tx/game/%s", endpoint)
	count := 0
	w := ecs.NewTestWorld(t)
	sendTx := ecs.NewTransactionType[SendEnergyTx, SendEnergyTxResult](endpoint)
	assert.NilError(t, w.RegisterTransactions(sendTx))
	w.AddSystem(func(wCtx ecs.WorldContext) error {
		txs := sendTx.In(wCtx)
		assert.Equal(t, 1, len(txs))
		tx := txs[0]
		assert.Equal(t, tx.Value.From, "me")
		assert.Equal(t, tx.Value.To, "you")
		assert.Equal(t, tx.Value.Amount, uint64(420))
		count++
		return nil
	})
	txh := testutils.MakeTestTransactionHandler(t, w, server.DisableSignatureVerification())
	tx := SendEnergyTx{
		From:   "me",
		To:     "you",
		Amount: 420,
	}
	bz, err := json.Marshal(tx)
	assert.NilError(t, err)
	signedTx := sign.Transaction{
		PersonaTag: "some_persona",
		Namespace:  "some_namespace",
		Nonce:      100,
		// this bogus signature is OK because DisableSignatureVerification was used
		Signature: common.Bytes2Hex([]byte{1, 2, 3, 4}),
		Body:      bz,
	}

	bz, err = json.Marshal(&signedTx)
	assert.NilError(t, err)
	_, err = http.Post(txh.MakeHTTPURL(url), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)

	assert.NilError(t, w.LoadGameState())
	assert.NilError(t, w.Tick(context.Background()))
	assert.Equal(t, 1, count)
	err = txh.Close()
	assert.NilError(t, err)
}

func TestCanCreateAndVerifyPersonaSigner(t *testing.T) {
	urlSet := []string{"tx/persona/create-persona", "query/persona/signer"}
	world := ecs.NewTestWorld(t)
	tx := ecs.NewTransactionType[SendEnergyTx, SendEnergyTxResult]("some_tx")
	assert.NilError(t, world.RegisterTransactions(tx))
	assert.NilError(t, world.LoadGameState())
	assert.NilError(t, world.Tick(context.Background()))
	txh := testutils.MakeTestTransactionHandler(t, world)

	personaTag := "CoolMage"
	privateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	signerAddr := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	createPersonaTx := ecs.CreatePersonaTransaction{
		PersonaTag:    personaTag,
		SignerAddress: signerAddr,
	}

	systemTx, err := sign.NewSystemTransaction(privateKey, world.Namespace().String(), 100, createPersonaTx)
	assert.NilError(t, err)

	bz, err := systemTx.Marshal()
	assert.NilError(t, err)

	resp, err := http.Post(txh.MakeHTTPURL(urlSet[0]), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)
	body := mustReadBody(t, resp)
	assert.Equal(t, 200, resp.StatusCode, "request failed with body: %s", body)

	var txReply server.TransactionReply
	assert.NilError(t, json.Unmarshal([]byte(body), &txReply))
	assert.Equal(t, txReply.Tick, world.CurrentTick())
	tick := txReply.Tick

	// postQueryPersonaSigner is a helper that makes a request to the query-persona-signer endpoint and returns
	// the response
	postQueryPersonaSigner := func(personaTag string, tick uint64) server.QueryPersonaSignerResponse {
		bz, err = json.Marshal(server.QueryPersonaSignerRequest{
			PersonaTag: personaTag,
			Tick:       tick,
		})
		assert.NilError(t, err)
		resp, err = http.Post(txh.MakeHTTPURL(urlSet[1]), "application/json", bytes.NewReader(bz))
		assert.NilError(t, err)
		assert.Equal(t, resp.StatusCode, 200)
		var queryPersonaSignerResponse server.QueryPersonaSignerResponse
		assert.NilError(t, json.NewDecoder(resp.Body).Decode(&queryPersonaSignerResponse))
		return queryPersonaSignerResponse
	}

	// Check some random person tag against a tick far in the past. This should be available.
	personaSignerResp := postQueryPersonaSigner("some_other_persona_tag", 0)
	assert.Equal(t, personaSignerResp.Status, "available")

	// If the game tick matches the passed in game tick, there hasn't been enough time to process the create_persona_tx.
	personaSignerResp = postQueryPersonaSigner(personaTag, tick)
	assert.Equal(t, personaSignerResp.Status, "unknown")

	// Tick the game state so that the persona can actually be registered
	assert.NilError(t, world.Tick(context.Background()))

	// The persona tag should now be registered with our signer address.
	personaSignerResp = postQueryPersonaSigner(personaTag, tick)
	assert.Equal(t, personaSignerResp.Status, "assigned")
	assert.Equal(t, personaSignerResp.SignerAddress, signerAddr)
	err = txh.Close()
	assert.NilError(t, err)
}

func TestSigVerificationChecksNamespace(t *testing.T) {
	url := "tx/persona/create-persona"
	world := ecs.NewTestWorld(t)
	assert.NilError(t, world.LoadGameState())
	privateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)

	txh := testutils.MakeTestTransactionHandler(t, world)

	personaTag := "some_dude"
	signerAddr := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	createPersonaTx := ecs.CreatePersonaTransaction{
		PersonaTag:    personaTag,
		SignerAddress: signerAddr,
	}
	sigPayload, err := sign.NewTransaction(privateKey, personaTag, "bad_namespace", 100, createPersonaTx)
	assert.NilError(t, err)

	bz, err := sigPayload.Marshal()
	assert.NilError(t, err)
	resp, err := http.Post(txh.MakeHTTPURL(url), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)
	// This should fail because the namespace does not match the world's namespace
	assert.Equal(t, resp.StatusCode, 401)

	// The namespace now matches the world
	sigPayload, err = sign.NewSystemTransaction(privateKey, world.Namespace().String(), 100, createPersonaTx)
	assert.NilError(t, err)
	bz, err = sigPayload.Marshal()
	assert.NilError(t, err)
	resp, err = http.Post(txh.MakeHTTPURL(url), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)
	assert.Equal(t, resp.StatusCode, 200)
	txh.Close()
}

func TestSigVerificationChecksNonce(t *testing.T) {
	url := "tx/persona/create-persona"
	world := ecs.NewTestWorld(t)
	assert.NilError(t, world.LoadGameState())
	privateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)

	txh := testutils.MakeTestTransactionHandler(t, world)

	personaTag := "some_dude"
	signerAddr := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	namespace := world.Namespace().String()

	createPersonaTx := ecs.CreatePersonaTransaction{
		PersonaTag:    personaTag,
		SignerAddress: signerAddr,
	}
	sigPayload, err := sign.NewSystemTransaction(privateKey, namespace, 100, createPersonaTx)
	assert.NilError(t, err)
	bz, err := sigPayload.Marshal()
	assert.NilError(t, err)

	// Register a persona. This should succeed
	resp, err := http.Post(txh.MakeHTTPURL(url), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)
	assert.Equal(t, resp.StatusCode, 200)

	// Repeat the request. Since the nonce is the same, this should fail
	resp, err = http.Post(txh.MakeHTTPURL(url), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)
	assert.Equal(t, resp.StatusCode, 401)

	// Using an old nonce should fail
	sigPayload, err = sign.NewSystemTransaction(privateKey, namespace, 50, createPersonaTx)
	assert.NilError(t, err)
	bz, err = sigPayload.Marshal()
	assert.NilError(t, err)
	resp, err = http.Post(txh.MakeHTTPURL(url), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)
	assert.Equal(t, resp.StatusCode, 401)

	// But increasing the nonce should work
	sigPayload, err = sign.NewSystemTransaction(privateKey, namespace, 101, createPersonaTx)
	assert.NilError(t, err)
	bz, err = sigPayload.Marshal()
	assert.NilError(t, err)
	resp, err = http.Post(txh.MakeHTTPURL(url), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)
	assert.Equal(t, resp.StatusCode, 200)
	err = txh.Close()
	assert.NilError(t, err)
}

// TestCanListQueries tests that we can list the available queries in the handler.
func TestCanListQueries(t *testing.T) {
	world := ecs.NewTestWorld(t)
	type FooRequest struct {
		Foo  int    `json:"foo,omitempty"`
		Meow string `json:"bar,omitempty"`
	}

	type FooResponse struct {
		Meow string `json:"meow,omitempty"`
	}

	fooQuery := ecs.NewQueryType[FooRequest, FooResponse]("foo", func(wCtx ecs.WorldContext, req FooRequest,
	) (FooResponse, error) {
		return FooResponse{Meow: req.Meow}, nil
	})
	barQuery := ecs.NewQueryType[FooRequest, FooResponse]("bar", func(wCtx ecs.WorldContext, req FooRequest,
	) (FooResponse, error) {
		return FooResponse{Meow: req.Meow}, nil
	})
	bazQuery := ecs.NewQueryType[FooRequest, FooResponse]("baz", func(wCtx ecs.WorldContext, req FooRequest,
	) (FooResponse, error) {
		return FooResponse{Meow: req.Meow}, nil
	})

	assert.NilError(t, world.RegisterQueries(fooQuery, barQuery, bazQuery))
	assert.NilError(t, world.LoadGameState())

	txh := testutils.MakeTestTransactionHandler(t, world, server.DisableSignatureVerification())

	resp, err := http.Post(txh.MakeHTTPURL("query/http/endpoints"), "application/json", nil)
	assert.NilError(t, err)
	assert.Equal(t, resp.StatusCode, 200)
	var gotEndpoints map[string][]string
	assert.NilError(t, json.NewDecoder(resp.Body).Decode(&gotEndpoints))

	endpoints := []string{
		"/query/game/foo",
		"/query/game/bar",
		"/query/game/baz",
		"/query/http/endpoints",
		"/query/persona/signer",
		"/query/receipt/list",
		"/query/game/cql",
	}
	assert.Equal(t, len(endpoints), len(gotEndpoints["queryEndpoints"]))
	for i, e := range gotEndpoints["queryEndpoints"] {
		assert.Equal(t, e, endpoints[i])
	}
}

// TestQueryEncodeDecode tests that query requests/responses are properly marshalled/unmarshalled in the context of
// http communication. We do not necessarily need to test anything w/r/t world storage, as what users decide to do
// within the context of their query requests are up to them, and not necessarily required for this feature to provably
// work.
func TestQueryEncodeDecode(t *testing.T) {
	// setup this read business stuff
	endpoint := "foo"
	type FooRequest struct {
		Foo  int    `json:"foo,omitempty"`
		Meow string `json:"bar,omitempty"`
	}

	type FooResponse struct {
		Meow string `json:"meow,omitempty"`
	}
	fq := ecs.NewQueryType[FooRequest, FooResponse](endpoint, func(wCtx ecs.WorldContext, req FooRequest,
	) (FooResponse, error) {
		return FooResponse{Meow: req.Meow}, nil
	})

	url := "query/game/" + endpoint
	// set up the world, register the queries, load.
	world := ecs.NewTestWorld(t)
	assert.NilError(t, world.RegisterQueries(fq))
	assert.NilError(t, world.LoadGameState())

	// make our test tx handler
	txh := testutils.MakeTestTransactionHandler(t, world, server.DisableSignatureVerification())

	// now we set up a request, and marshal it to json to send to the handler
	req := FooRequest{Foo: 12, Meow: "hello"}
	bz, err := json.Marshal(req)
	assert.NilError(t, err)

	res, err := http.Post(txh.MakeHTTPURL(url), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)

	buf, err := io.ReadAll(res.Body)
	assert.NilError(t, err)

	var fooRes FooResponse
	err = json.Unmarshal(buf, &fooRes)
	assert.NilError(t, err)

	assert.Equal(t, fooRes.Meow, req.Meow)
	err = txh.Close()
	assert.NilError(t, err)
}

func TestMalformedRequestToGetTransactionReceiptsProducesError(t *testing.T) {
	url := "query/receipts/list"
	world := ecs.NewTestWorld(t)
	assert.NilError(t, world.LoadGameState())
	txh := testutils.MakeTestTransactionHandler(t, world, server.DisableSignatureVerification())
	res := txh.Post(url, map[string]any{
		"missing_start_tick": 0,
	})
	assert.Check(t, 400 <= res.StatusCode && res.StatusCode <= 499)
	err := txh.Close()
	assert.NilError(t, err)
}

func TestTransactionReceiptReturnCorrectTickWindows(t *testing.T) {
	url := "query/receipts/list"

	historySize := uint64(10)
	world := ecs.NewTestWorld(t, ecs.WithReceiptHistorySize(int(historySize)))
	assert.NilError(t, world.LoadGameState())
	txh := testutils.MakeTestTransactionHandler(t, world, server.DisableSignatureVerification())

	// getReceipts is a helper that hits the txReceiptsEndpoint endpoint.
	getReceipts := func(start uint64) server.ListTxReceiptsReply {
		res := txh.Post(url, server.ListTxReceiptsRequest{
			StartTick: start,
		})
		assert.Equal(t, 200, res.StatusCode)
		var reply server.ListTxReceiptsReply
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
	err := txh.Close()
	assert.NilError(t, err)
}
func TestCanGetTransactionReceiptsSwagger(t *testing.T) {
	receiptEndpoint := "query/receipts/list"
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

	world := ecs.NewTestWorld(t)
	assert.NilError(t, world.RegisterTransactions(incTx, dupeTx, errTx))
	// System to handle incrementing numbers
	world.AddSystem(func(wCtx ecs.WorldContext) error {
		for _, tx := range incTx.In(wCtx) {
			incTx.SetResult(wCtx, tx.TxHash, IncReply{
				Number: tx.Value.Number + 1,
			})
		}
		return nil
	})
	// System to handle duplicating strings
	world.AddSystem(func(wCtx ecs.WorldContext) error {
		for _, tx := range dupeTx.In(wCtx) {
			dupeTx.SetResult(wCtx, tx.TxHash, DupeReply{
				Str: tx.Value.Str + tx.Value.Str,
			})
		}
		return nil
	})
	wantError := errors.New("some error")
	// System to handle error production
	world.AddSystem(func(wCtx ecs.WorldContext) error {
		for _, tx := range errTx.In(wCtx) {
			errTx.AddError(wCtx, tx.TxHash, wantError)
			errTx.AddError(wCtx, tx.TxHash, wantError)
		}
		return nil
	})
	assert.NilError(t, world.LoadGameState())

	// World setup is done. First check that there are no transactions.
	ctx := context.Background()
	assert.NilError(t, world.Tick(ctx))

	txh := testutils.MakeTestTransactionHandler(t, world, server.DisableSignatureVerification())

	// We're going to be getting the list of receipts a lot, so make a helper to fetch the receipts
	getReceipts := func(start uint64) server.ListTxReceiptsReply {
		res := txh.Post(receiptEndpoint, server.ListTxReceiptsRequest{
			StartTick: start,
		})
		assert.Equal(t, 200, res.StatusCode)

		var txReceipts server.ListTxReceiptsReply
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
	nextSig := func() *sign.Transaction {
		var sig *sign.Transaction
		sig, err = sign.NewTransaction(privateKey, "my-persona-tag", "namespace", nonce,
			`{"data": "stuff"}`)
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
		if _, ok = m["Number"]; ok {
			foundInc = true
			var num float64
			num, ok = m["Number"].(float64)
			assert.Check(t, ok)
			assert.Equal(t, 100, int(num))
		} else if _, ok := m["Str"]; ok {
			foundDupe = true
			var str string
			str, ok = m["Str"].(string)
			assert.Check(t, ok)
			assert.Equal(t, "foobarfoobar", str)
		} else {
			assert.Assert(t, false, "unknown transaction result", r.Result)
		}
	}

	assert.Check(t, foundInc)
	assert.Check(t, foundDupe)
	assert.Check(t, foundErr)

	err = txh.Close()
	assert.NilError(t, err)
}

func TestTransactionIDIsReturned(t *testing.T) {
	swaggerCreatePersonURL := "tx/persona/create-persona"
	swaggerUrls := []string{swaggerCreatePersonURL, "tx/game/move"}
	urls := swaggerUrls
	type MoveTx struct{}
	world := ecs.NewTestWorld(t)
	moveTx := ecs.NewTransactionType[MoveTx, MoveTx]("move")
	assert.NilError(t, world.RegisterTransactions(moveTx))
	assert.NilError(t, world.LoadGameState())
	ctx := context.Background()
	// Preemptive tick so the tick isn't the zero value
	assert.NilError(t, world.Tick(ctx))
	privateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	txh := testutils.MakeTestTransactionHandler(t, world)

	personaTag := "clifford_the_big_red_dog"
	signerAddr := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	namespace := world.Namespace().String()
	nonce := uint64(99)

	createPersonaTx := ecs.CreatePersonaTransaction{
		PersonaTag:    personaTag,
		SignerAddress: signerAddr,
	}

	sigPayload, err := sign.NewSystemTransaction(privateKey, namespace, nonce, createPersonaTx)
	assert.NilError(t, err)
	bz, err := sigPayload.Marshal()
	assert.NilError(t, err)

	resp, err := http.Post(txh.MakeHTTPURL(urls[0]), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var txReply server.TransactionReply
	assert.NilError(t, json.NewDecoder(resp.Body).Decode(&txReply))

	// The hash field should not be empty
	assert.Check(t, txReply.TxHash != "")
	// The tick should equal the current tick
	assert.Equal(t, world.CurrentTick(), txReply.Tick)

	assert.NilError(t, world.Tick(ctx))

	// Also check to make sure transaction IDs are returned for other kinds of transactions
	nonce++
	emptyData := map[string]any{}
	sigPayload, err = sign.NewTransaction(privateKey, personaTag, namespace, nonce, emptyData)
	assert.NilError(t, err)

	bz, err = sigPayload.Marshal()
	assert.NilError(t, err)

	resp, err = http.Post(txh.MakeHTTPURL(urls[1]), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.NilError(t, json.NewDecoder(resp.Body).Decode(&txReply))

	// The hash field should not be empty
	assert.Check(t, txReply.TxHash != "")
	// The tick should equal the current tick
	assert.Equal(t, world.CurrentTick(), txReply.Tick)
	txh.Close()
}

var _ shard.Adapter = &adapterMock{}

type adapterMock struct {
	called int
	hold   chan bool
}

func (a *adapterMock) Submit(_ context.Context, _ *sign.Transaction, _, _ uint64) error {
	a.called++
	return nil
}

func (a *adapterMock) QueryTransactions(_ context.Context, _ *types.QueryTransactionsRequest) (
	*types.QueryTransactionsResponse, error) {
	<-a.hold
	//nolint:nilnil // its ok. its a mock.
	return nil, nil
}

func TestTransactionsSubmittedToChain(t *testing.T) {
	createPersonaEndpoint := "tx/persona/create-persona"
	moveEndpoint := "tx/game/move"
	type MoveTx struct {
		Direction string
	}
	world := ecs.NewTestWorld(t)
	moveTx := ecs.NewTransactionType[MoveTx, MoveTx]("move")
	assert.NilError(t, world.RegisterTransactions(moveTx))
	assert.NilError(t, world.LoadGameState())
	adapter := adapterMock{}
	txh := testutils.MakeTestTransactionHandler(t, world, server.WithAdapter(&adapter),
		server.DisableSignatureVerification())

	privateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	personaTag := "clifford_the_big_red_dog"
	signerAddr := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	sigPayload, err := sign.NewSystemTransaction(privateKey, world.Namespace().String(), 1,
		ecs.CreatePersonaTransaction{
			PersonaTag:    personaTag,
			SignerAddress: signerAddr,
		})
	assert.NilError(t, err)
	bz, err := sigPayload.Marshal()
	assert.NilError(t, err)

	resp, err := http.Post(txh.MakeHTTPURL(createPersonaEndpoint), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, adapter.called, 1)

	sigPayload, err = sign.NewTransaction(privateKey, personaTag, world.Namespace().String(), 2,
		MoveTx{Direction: "up"})
	assert.NilError(t, err)
	bz, err = sigPayload.Marshal()
	assert.NilError(t, err)
	resp, err = http.Post(txh.MakeHTTPURL(moveEndpoint), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, adapter.called, 2)
}

func TestTransactionNotSubmittedWhenRecovering(t *testing.T) {
	moveEndpoint := "tx/game/move"
	type MoveTx struct {
		Direction string
	}
	holdChan := make(chan bool)
	adapter := adapterMock{hold: holdChan}
	world := ecs.NewTestWorld(t, ecs.WithAdapter(&adapter))
	go func() {
		err := world.RecoverFromChain(context.Background())
		assert.NilError(t, err)
	}()
	moveTx := ecs.NewTransactionType[MoveTx, MoveTx]("move")
	err := world.RegisterTransactions(moveTx)
	assert.NilError(t, err)
	assert.NilError(t, world.LoadGameState())
	txh := testutils.MakeTestTransactionHandler(t, world, server.WithAdapter(&adapter),
		server.DisableSignatureVerification())

	privateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	personaTag := "clifford_the_big_red_dog"

	sigPayload, err := sign.NewTransaction(privateKey, personaTag, world.Namespace().String(), 2,
		MoveTx{Direction: "up"})
	assert.NilError(t, err)
	bz, err := sigPayload.Marshal()
	assert.NilError(t, err)
	resp, err := http.Post(txh.MakeHTTPURL(moveEndpoint), "application/json", bytes.NewReader(bz))
	assert.NilError(t, err)
	assert.Equal(t, 500, resp.StatusCode)
	bz, err = io.ReadAll(resp.Body)
	assert.NilError(t, err)
	assert.ErrorContains(t, errors.New(string(bz)), "game world is recovering state")
}

func TestWebSocket(t *testing.T) {
	w := ecs.NewTestWorld(t)
	assert.NilError(t, w.LoadGameState())
	txh := testutils.MakeTestTransactionHandler(t, w, server.DisableSignatureVerification())
	url := txh.MakeWebSocketURL("echo")
	dial, _, err := websocket.DefaultDialer.Dial(url, nil)
	assert.NilError(t, err)
	messageToSend := "test"
	err = dial.WriteMessage(websocket.TextMessage, []byte(messageToSend))
	assert.NilError(t, err)
	messageType, message, err := dial.ReadMessage()
	assert.NilError(t, err)
	assert.Equal(t, messageType, websocket.TextMessage)
	assert.Equal(t, string(message), messageToSend)
	err = dial.Close()
	assert.NilError(t, err)
}

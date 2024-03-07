package nakama

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"

	"pkg.world.dev/world-engine/assert"

	"github.com/argus-labs/world-engine/e2e/tests/clients"
)

func TestEvents(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	signerAddr := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	username, deviceID, personaTag := triple(randomString())
	c := clients.NewNakamaClient(t)
	assert.NilError(t, c.RegisterDevice(username, deviceID))

	resp, err := c.RPC("nakama/claim-persona", map[string]any{
		"personaTag":    personaTag,
		"signerAddress": signerAddr,
	})
	assert.NilError(t, err, "claim-persona failed")
	assert.Equal(t, 200, resp.StatusCode, clients.CopyBody(resp))

	assert.NilError(t, waitForAcceptedPersonaTag(c))
	type JoinInput struct {
	}
	payload := JoinInput{}

	// Emit events by creating players
	amountOfPlayers := 3
	for i := 0; i < amountOfPlayers; i++ {
		resp, err = c.RPC("tx/game/join", payload)
		assert.NilError(t, err)
		assert.Equal(t, 200, resp.StatusCode, clients.CopyBody(resp))
	}

	// Fetch events and verify
	var events []clients.Event
	timeout := time.After(5 * time.Second)
	for len(events) < amountOfPlayers {
		select {
		case e := <-c.EventCh:
			events = append(events, e)
		case <-timeout:
			assert.FailNow(t, "timeout whiel waiting for events")
		}
	}

	assert.Equal(t, len(events), amountOfPlayers, "Expected number of player creation events does not match")
	for _, event := range events {
		assert.Contains(t, event.Message, "player created", "Event message does not contain expected text")
	}
}

func TestReceipts(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	signerAddr := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	username, deviceID, personaTag := triple(randomString())
	c := clients.NewNakamaClient(t)
	assert.NilError(t, c.RegisterDevice(username, deviceID))

	resp, err := c.RPC("nakama/claim-persona", map[string]any{
		"personaTag":    personaTag,
		"signerAddress": signerAddr,
	})
	assert.NilError(t, err, "claim-persona failed")
	assert.Equal(t, 200, resp.StatusCode, clients.CopyBody(resp))

	assert.NilError(t, waitForAcceptedPersonaTag(c))

	type JoinInput struct {
	}
	payload := JoinInput{}

	// Emit events and thus generate receipts by creating players
	amountOfPlayers := 3
	for i := 0; i < amountOfPlayers; i++ {
		resp, err = c.RPC("tx/game/join", payload)
		assert.NilError(t, err)
		assert.Equal(t, 200, resp.StatusCode, clients.CopyBody(resp))
	}

	var receipts []clients.Receipt
	timeout := time.After(5 * time.Second)
	for len(receipts) <= amountOfPlayers {
		select {
		case r := <-c.ReceiptCh:
			receipts = append(receipts, r)
		case <-timeout:
			assert.FailNow(t, "timeout while waiting receipts")
		}
	}

	assert.Equal(t, len(receipts), amountOfPlayers+1, "Expected number of receipts does not match")
	for i, receipt := range receipts {
		if i == 0 {
			// Assert that the persona creation receipt was successful
			assert.Equal(t, receipt.Result["success"], true)
			continue
		}

		// Assert that tx/game/join receipts returned successful
		assert.Equal(t, len(receipt.Errors), 0)
		value, ok := receipt.Result["Success"]
		assert.True(t, ok)
		success, ok := value.(bool)
		assert.True(t, ok)
		assert.Equal(t, success, true)
	}
}

//nolint:gocognit
func TestTransactionAndCQLAndRead(t *testing.T) {
	// Test persona
	privateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	signerAddr := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	username, deviceID, personaTag := triple(randomString())
	c := clients.NewNakamaClient(t)
	assert.NilError(t, c.RegisterDevice(username, deviceID))

	resp, err := c.RPC("nakama/claim-persona", map[string]any{
		"personaTag":    personaTag,
		"signerAddress": signerAddr,
	})
	assert.NilError(t, err, "claim-persona failed")
	assert.Equal(t, 200, resp.StatusCode, clients.CopyBody(resp))

	assert.NilError(t, waitForAcceptedPersonaTag(c))
	payload := map[string]any{}
	resp, err = c.RPC("tx/game/join", payload)
	assert.NilError(t, err)
	body := clients.CopyBody(resp)
	assert.Equal(t, 200, resp.StatusCode, body)

	// Moving "up" will increase the Y coordinate by 1 and leave the X coordinate unchanged.
	payload = map[string]any{
		"Direction": "up",
	}
	resp, err = c.RPC("tx/game/move", payload)
	assert.NilError(t, err)
	body = clients.CopyBody(resp)
	assert.Equal(t, 200, resp.StatusCode, body)

	type Item struct {
		ID   int              `json:"id"`
		Data []map[string]any `json:"data"`
	}
	type CQLResponse struct {
		Results []Item `json:"results"`
	}
	var finalResults []Item
	currentTime := time.Now()
	maxTime := 10 * time.Second

	// hits the cql endpoint and eventually expects both the y coordinate and name to be matched in the same set of
	// final results. Since the tests and http server move faster than the game loop, initial queries will happen before
	// the game tick has executed. We spin on this check until we find the desired results or until we time out.
	yAndNameNotFound := true
	for yAndNameNotFound {
		resp, err = c.RPC("query/game/cql", struct {
			CQL string `json:"CQL"`
		}{"CONTAINS(player)"})
		assert.NilError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		results, err := io.ReadAll(resp.Body)
		assert.NilError(t, err)
		var result CQLResponse
		err = json.Unmarshal(results, &result)
		finalResults = result.Results
		assert.NilError(t, err)
		for _, res := range finalResults {
			foundYValue := false
			foundName := false
			for _, v := range res.Data {
				if yValue, ok := v["Y"]; ok {
					if yValue.(float64) == 1 {
						foundYValue = true
					}
				} else if nameValue, ok := v["Name"]; ok {
					if nameValue.(string) == personaTag {
						foundName = true
					}
				} else {
					t.Fatal("unknown data: ", v)
				}
			}
			if foundYValue && foundName {
				yAndNameNotFound = false
			}
		}
		if time.Since(currentTime) > maxTime {
			assert.Assert(t, false, "timeout occurred here, CQL query should return some results eventually")
		}
	}

	// Test Read
	type LocationRequest struct {
		ID string
	}
	type LocationReply struct {
		X, Y int
	}
	resp, err = c.RPC("query/game/location", LocationRequest{personaTag})
	assert.NilError(t, err)
	bodyBytes, err := io.ReadAll(resp.Body)
	assert.NilError(t, err)
	typedResp := LocationReply{}
	err = json.Unmarshal(bodyBytes, &typedResp)
	assert.NilError(t, err)
	assert.Equal(t, typedResp.Y, 1)
}

func TestCanShowPersona(t *testing.T) {
	username, deviceID, personaTag := triple(randomString())
	c := clients.NewNakamaClient(t)
	assert.NilError(t, c.RegisterDevice(username, deviceID))

	resp, err := c.RPC("nakama/claim-persona", map[string]any{
		"personaTag": personaTag,
	})
	assert.NilError(t, err, "claim-persona failed")
	assert.Equal(t, 200, resp.StatusCode, clients.CopyBody(resp))

	assert.NilError(t, waitForAcceptedPersonaTag(c))
}

func TestDifferentUsersCannotClaimSamePersonaTag(t *testing.T) {
	userA, deviceA, ptA := triple(randomString())

	aClient := clients.NewNakamaClient(t)
	assert.NilError(t, aClient.RegisterDevice(userA, deviceA))

	resp, err := aClient.RPC("nakama/claim-persona", map[string]any{
		"personaTag": ptA,
	})
	assert.NilError(t, err)
	assert.Equal(t, 200, resp.StatusCode, clients.CopyBody(resp))

	userB, deviceB, _ := triple(randomString())
	// user B will try to claim the same persona tag as user A
	ptB := ptA
	bClient := clients.NewNakamaClient(t)
	assert.NilError(t, bClient.RegisterDevice(userB, deviceB))
	resp, err = bClient.RPC("nakama/claim-persona", map[string]any{
		"personaTag": ptB,
	})
	assert.NilError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, clients.CopyBody(resp))
}

func TestConcurrentlyClaimSamePersonaTag(t *testing.T) {
	userCount := 10
	users := make([]string, userCount)
	for i := range users {
		users[i] = randomString()
	}

	nakamaClients := []*clients.NakamaClient{}
	// The claim-persona requests should all happen in quick succession, so register all devices (essentially logging in)
	// before making any calls to claim-persona.
	for i := range users {
		name := users[i]
		c := clients.NewNakamaClient(t)
		assert.NilError(t, c.RegisterDevice(name, name))
		nakamaClients = append(nakamaClients, c)
	}

	// This is the single persona tag that all users will try to claim
	personaTag := randomString()
	type result struct {
		resp *http.Response
		err  error
	}
	ch := make(chan result)
	for _, client := range nakamaClients {
		c := client
		go func() {
			resp, err := c.RPC("nakama/claim-persona", map[string]any{
				"personaTag": personaTag,
			})
			ch <- result{resp, err}
		}()
	}

	codeCount := map[int]int{}
	for i := 0; i < userCount; i++ {
		r := <-ch
		assert.NilError(t, r.err)
		codeCount[r.resp.StatusCode]++
	}
	assert.Equal(t, 2, len(codeCount), "expected status codes of 200 and 409, got %v", codeCount)
	assert.Equal(t, 1, codeCount[200], "expected exactly 1 success")
	assert.Equal(t, userCount-1, codeCount[400], "expected exactly %d failures", userCount-1)
}

func TestCannotClaimAdditionalPersonATag(t *testing.T) {
	user, device, tag := triple(randomString())
	c := clients.NewNakamaClient(t)
	assert.NilError(t, c.RegisterDevice(user, device))

	resp, err := c.RPC("nakama/claim-persona", map[string]any{
		"personaTag": tag,
	})
	assert.NilError(t, err)
	assert.Equal(t, 200, resp.StatusCode, clients.CopyBody(resp))

	// Trying to request a different persona tag right away should fail.
	resp, err = c.RPC("nakama/claim-persona", map[string]any{
		"personaTag": "some-other-persona-tag",
	})
	assert.NilError(t, err)
	assert.Equal(t, 400, resp.StatusCode, clients.CopyBody(resp))

	assert.NilError(t, waitForAcceptedPersonaTag(c))
	// Trying to request a different persona tag after the original has been accepted
	// should fail
	resp, err = c.RPC("nakama/claim-persona", map[string]any{
		"personaTag": "some-other-persona-tag",
	})
	assert.NilError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestPersonaTagFieldCannotBeEmpty(t *testing.T) {
	user, device, _ := triple(randomString())
	c := clients.NewNakamaClient(t)
	assert.NilError(t, c.RegisterDevice(user, device))

	resp, err := c.RPC("nakama/claim-persona", map[string]any{
		"ignore_me": "foobar",
	})
	assert.NilError(t, err)
	assert.Equal(t, 400, resp.StatusCode, clients.CopyBody(resp))
}

func TestPersonaTagsShouldBeCaseInsensitive(t *testing.T) {
	clientA, clientB := clients.NewNakamaClient(t), clients.NewNakamaClient(t)
	userA, userB := randomString(), randomString()

	assert.NilError(t, clientA.RegisterDevice(userA, userA))
	assert.NilError(t, clientB.RegisterDevice(userB, userB))

	lowerCase := randomString()
	upperCase := strings.ToUpper(lowerCase)
	_, err := clientA.RPC("nakama/claim-persona", map[string]any{
		"personaTag": lowerCase,
	})
	assert.NilError(t, err)
	_, err = clientB.RPC("nakama/claim-persona", map[string]any{
		"personaTag": upperCase,
	})
	assert.NilError(t, err)

	assert.NilError(t, waitForAcceptedPersonaTag(clientA))

	respA, err := clientA.RPC("nakama/show-persona", nil)
	assert.NilError(t, err)
	respB, err := clientB.RPC("nakama/show-persona", nil)
	assert.NilError(t, err)

	showA := map[string]any{}
	showB := map[string]any{}
	assert.NilError(t, json.NewDecoder(respA.Body).Decode(&showA))
	assert.NilError(t, json.NewDecoder(respB.Body).Decode(&showB))

	assert.Equal(t, showA["status"], "accepted")
	assert.Equal(t, showB["status"], "rejected")
}

func TestReceiptsCanContainErrors(t *testing.T) {
	client := clients.NewNakamaClient(t)
	user := randomString()
	assert.NilError(t, client.RegisterDevice(user, user))

	resp, err := client.RPC("nakama/claim-persona", map[string]any{
		"personaTag": user,
	})
	assert.NilError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// This is the receipt for persona tag claiming
	receipt := <-client.ReceiptCh
	assert.Len(t, receipt.Errors, 0)

	assert.NilError(t, waitForAcceptedPersonaTag(client))

	wantErrorMsg := "SOME_ERROR_MESSAGE"
	resp, err = client.RPC("tx/game/error", map[string]any{
		"ErrorMsg": wantErrorMsg,
	})
	// The error for this message won't be generated until after the tick has been processed.
	assert.NilError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	timeout := time.After(5 * time.Second)
	select {
	case receipt = <-client.ReceiptCh:
		assert.Len(t, receipt.Errors, 1)
		assert.Contains(t, receipt.Errors[0], wantErrorMsg)
	case <-timeout:
		assert.FailNow(t, "timeout while waiting for receipt")
	}
}

// waitForAcceptedPersonaTag periodically queries the show-persona endpoint until a previously claimed persona tag
// is "accepted". A response of "pending" will wait a short period of time, then repeat the request. After 1 second,
// this helper returns an error.
func waitForAcceptedPersonaTag(c *clients.NakamaClient) error {
	timeout := time.After(2 * time.Second)
	retry := time.Tick(10 * time.Millisecond)
	for {
		resp, err := c.RPC("nakama/show-persona", nil)
		if err != nil {
			return err
		}
		status, err := getStatusFromResponse(resp)
		if err == nil {
			if status == "accepted" {
				break
			} else if status != "pending" {
				return fmt.Errorf("bad status %q while waiting for persona tag to be accepted", status)
			}
		}

		select {
		case <-timeout:
			return errors.New("timeout while waiting for persona tag to be accepted")
		case <-retry:
			continue
		}
	}
	return nil
}

func getStatusFromResponse(resp *http.Response) (string, error) {
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("got status code %d, want 200; response body: %v", resp.StatusCode, clients.CopyBody(resp))
	}
	m := map[string]any{}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return "", err
	}
	statusIface, ok := m["status"]
	if !ok {
		return "", fmt.Errorf("field 'status' not found in response body; got %v", m)
	}
	status, ok := statusIface.(string)
	if !ok {
		return "", fmt.Errorf("unable to cast value %v to string", statusIface)
	}

	return status, nil
}

const chars = "abcdefghijklmnopqrstuvwxyz"

func randomString() string {
	b := &strings.Builder{}
	for i := 0; i < 10; i++ {
		n := rand.Intn(len(chars))
		b.WriteString(chars[n : n+1])
	}
	return b.String()
}

func triple(s string) (string, string, string) {
	return s, s, s
}

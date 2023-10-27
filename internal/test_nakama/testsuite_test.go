//go:build integration

package test_nakama

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
	"gotest.tools/v3/assert"
)

func TestTransactionAndCQLAndRead(t *testing.T) {

	//Test persona
	privateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	signerAddr := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	username, deviceID, personaTag := triple(randomString())
	c := newClient(t)
	assert.NilError(t, c.registerDevice(username, deviceID))

	resp, err := c.rpc("nakama/claim-persona", map[string]any{
		"personaTag":    personaTag,
		"signerAddress": signerAddr,
	})
	assert.NilError(t, err, "claim-persona failed")
	assert.Equal(t, 200, resp.StatusCode, copyBody(resp))

	assert.NilError(t, waitForAcceptedPersonaTag(c))
	payload := map[string]any{}
	resp, err = c.rpc("tx/game/join", payload)
	assert.NilError(t, err)
	body := copyBody(resp)
	assert.Equal(t, 200, resp.StatusCode, body)

	// Moving "up" will increase the Y coordinate by 1 and leave the X coordinate unchanged.
	payload = map[string]any{
		"Direction": "up",
	}
	resp, err = c.rpc("tx/game/move", payload)
	assert.NilError(t, err)
	body = copyBody(resp)
	assert.Equal(t, 200, resp.StatusCode, body)

	type Item struct {
		ID   int              `json:"id"`
		Data []map[string]any `json:"data"`
	}
	finalResults := []Item{}
	currentTs := time.Now()
	maxTime := 10 * time.Second

	// hits the cql endpoint and eventually expects both the y coordinate and name to be matched in the same set of final results.
	// Since the tests and http server move faster than the game loop, initial queries will happen before the game tick has executed.
	// We spin on this check until we find the desired results or until we time out.
	yAndNameNotFound := true
	for yAndNameNotFound {
		resp, err = c.rpc("query/game/cql", struct {
			CQL string `json:CQL`
		}{"CONTAINS(player)"})
		assert.NilError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		results, err := io.ReadAll(resp.Body)

		err = json.Unmarshal(results, &finalResults)
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
		if time.Since(currentTs) > maxTime {
			assert.Assert(t, false, "timeout occurred here, CQL query should return some results eventually")
		}
	}

	//Test Read
	type LocationRequest struct {
		ID string
	}
	type LocationReply struct {
		X, Y int
	}
	resp, err = c.rpc("query/game/location", LocationRequest{personaTag})
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
	c := newClient(t)
	assert.NilError(t, c.registerDevice(username, deviceID))

	resp, err := c.rpc("nakama/claim-persona", map[string]any{
		"personaTag": personaTag,
	})
	assert.NilError(t, err, "claim-persona failed")
	assert.Equal(t, 200, resp.StatusCode, copyBody(resp))

	assert.NilError(t, waitForAcceptedPersonaTag(c))
}

func TestDifferentUsersCannotClaimSamePersonaTag(t *testing.T) {
	userA, deviceA, ptA := triple(randomString())

	aClient := newClient(t)
	assert.NilError(t, aClient.registerDevice(userA, deviceA))

	resp, err := aClient.rpc("nakama/claim-persona", map[string]any{
		"personaTag": ptA,
	})
	assert.NilError(t, err)
	assert.Equal(t, 200, resp.StatusCode, copyBody(resp))

	userB, deviceB, ptB := triple(randomString())
	// user B will try to claim the same persona tag as user A
	ptB = ptA
	bClient := newClient(t)
	assert.NilError(t, bClient.registerDevice(userB, deviceB))
	resp, err = bClient.rpc("nakama/claim-persona", map[string]any{
		"personaTag": ptB,
	})
	assert.NilError(t, err)
	assert.Equal(t, 409, resp.StatusCode, copyBody(resp))
}

func TestConcurrentlyClaimSamePersonaTag(t *testing.T) {
	userCount := 10
	users := make([]string, userCount)
	for i := range users {
		users[i] = randomString()
	}

	clients := []*nakamaClient{}
	// The claim-persona requests should all happen in quick succession, so register all devices (essentially logging in)
	// before making any calls to claim-persona.
	for i := range users {
		name := users[i]
		c := newClient(t)
		assert.NilError(t, c.registerDevice(name, name))
		clients = append(clients, c)
	}

	// This is the single persona tag that all users will try to claim
	personaTag := randomString()
	type result struct {
		resp *http.Response
		err  error
	}
	ch := make(chan result)
	for _, client := range clients {
		c := client
		go func() {
			resp, err := c.rpc("nakama/claim-persona", map[string]any{
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
	assert.Equal(t, userCount-1, codeCount[409], "expected exactly %d failures", userCount-1)
}

func TestCannotClaimAdditionalPersonATag(t *testing.T) {
	user, device, tag := triple(randomString())
	c := newClient(t)
	assert.NilError(t, c.registerDevice(user, device))

	resp, err := c.rpc("nakama/claim-persona", map[string]any{
		"personaTag": tag,
	})
	assert.NilError(t, err)
	assert.Equal(t, 200, resp.StatusCode, copyBody(resp))

	// Trying to request a different persona tag right away should fail.
	resp, err = c.rpc("nakama/claim-persona", map[string]any{
		"personaTag": "some-other-persona-tag",
	})
	assert.NilError(t, err)
	assert.Equal(t, 409, resp.StatusCode, copyBody(resp))

	assert.NilError(t, waitForAcceptedPersonaTag(c))
	// Trying to request a different persona tag after the original has been accepted
	// should fail
	resp, err = c.rpc("nakama/claim-persona", map[string]any{
		"personaTag": "some-other-persona-tag",
	})
	assert.NilError(t, err)
	assert.Equal(t, 409, resp.StatusCode)

}

func TestPersonaTagFieldCannotBeEmpty(t *testing.T) {
	user, device, _ := triple(randomString())
	c := newClient(t)
	assert.NilError(t, c.registerDevice(user, device))

	resp, err := c.rpc("nakama/claim-persona", map[string]any{
		"ignore_me": "foobar",
	})
	assert.NilError(t, err)
	assert.Equal(t, 400, resp.StatusCode, copyBody(resp))
}

// waitForAcceptedPersonaTag periodically queries the show-persona endpoint until a previously claimed persona tag
// is "accepted". A response of "pending" will wait a short period of time, then repeat the request. After 1 second,
// this helper returns an error.
func waitForAcceptedPersonaTag(c *nakamaClient) error {
	timeout := time.After(2 * time.Second)
	retry := time.Tick(10 * time.Millisecond)
	for {
		resp, err := c.rpc("nakama/show-persona", nil)
		if err != nil {
			return err
		}
		status, err := getStatusFromResponse(resp)
		if err != nil {
			return fmt.Errorf("unable to get 'status' field from response: %v", err)
		}
		if status == "accepted" {
			break
		} else if status != "pending" {
			return fmt.Errorf("bad status %q while waiting for persona tag to be accepted", status)
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
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("got status code %d, want 200; response body: %v", resp.StatusCode, copyBody(resp))
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

package testsuite

import (
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func TestCanShowPersona(t *testing.T) {
	username, deviceID, personaTag := triple(randomString())
	c := newClient()
	c.registerDevice(username, deviceID)

	resp, err := c.rpc("nakama/claim-persona", map[string]any{
		"persona_tag": personaTag,
	})
	assert.NilError(t, err, "claim-persona failed")
	assert.Equal(t, 200, resp.StatusCode, copyBody(resp))

	assert.NilError(t, waitForAcceptedPersonaTag(t, c))
}

func TestDifferentUsersCannotClaimSamePersonaTag(t *testing.T) {
	userA, deviceA, ptA := triple(randomString())

	aClient := newClient()
	aClient.registerDevice(userA, deviceA)

	resp, err := aClient.rpc("nakama/claim-persona", map[string]any{
		"persona_tag": ptA,
	})
	assert.NilError(t, err)
	assert.Equal(t, 200, resp.StatusCode, copyBody(resp))

	userB, deviceB, ptB := triple(randomString())
	// user B will try to claim the same persona tag as user A
	ptB = ptA
	bClient := newClient()
	bClient.registerDevice(userB, deviceB)
	resp, err = bClient.rpc("nakama/claim-persona", map[string]any{
		"persona_tag": ptB,
	})
	assert.NilError(t, err)
	assert.Equal(t, 409, resp.StatusCode, copyBody(resp))
}

func TestConcurrentlyClaimSamePersonaTag(t *testing.T) {
	userCount := 100
	users := make([]string, userCount)
	for i := range users {
		users[i] = randomString()
	}

	clients := []*nakamaClient{}
	// The claim-persona requests should all happen in quick succession, so register all devices (essentially logging in)
	// before making any calls to claim-persona.
	for i := range users {
		name := users[i]
		c := newClient()
		c.registerDevice(name, name)
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
				"persona_tag": personaTag,
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
	c := newClient()
	c.registerDevice(user, device)

	resp, err := c.rpc("nakama/claim-persona", map[string]any{
		"persona_tag": tag,
	})
	assert.NilError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Trying to request a different persona tag right away should fail.
	resp, err = c.rpc("nakama/claim-persona", map[string]any{
		"persona_tag": "some-other-persona-tag",
	})
	assert.NilError(t, err)
	assert.Equal(t, 409, resp.StatusCode, copyBody(resp))

	assert.NilError(t, waitForAcceptedPersonaTag(t, c))
	// Trying to request a different persona tag right away should fail.
	resp, err = c.rpc("nakama/claim-persona", map[string]any{
		"persona_tag": "some-other-persona-tag",
	})
	assert.NilError(t, err)
	assert.Equal(t, 409, resp.StatusCode)

}

func TestPersonaTagFieldCannotBeEmpty(t *testing.T) {
	user, device, _ := triple(randomString())
	c := newClient()
	c.registerDevice(user, device)

	resp, err := c.rpc("nakama/claim-persona", map[string]any{
		"ignore_me": "foobar",
	})
	assert.NilError(t, err)
	assert.Equal(t, 400, resp.StatusCode, copyBody(resp))
}

// waitForAcceptedPersonaTag periodically queries the show-persona endpoint until a previously claimed persona tag
// is "accepted". A response of "pending" will wait a short period of time, then repeat the request. After 1 second,
// this helper returns an error.
func waitForAcceptedPersonaTag(t *testing.T, c *nakamaClient) (err error) {
	timeout := time.After(time.Second)
	tick := time.Tick(10 * time.Millisecond)
	for {
		resp, err := c.rpc("nakama/show-persona", nil)
		assert.NilError(t, err)
		assert.Equal(t, 200, resp.StatusCode, copyBody(resp))
		status := bodyToMap(t, resp)["status"].(string)
		if status == "accepted" {
			break
		} else if status == "pending" {
			continue
		} else {
			return fmt.Errorf("bad status %q while waiting for persona tag to be accepted", status)
		}
		select {
		case <-timeout:
			return errors.New("timeout while waiting for persona tag to be accepted")
		case <-tick:
			continue
		}
	}
	return nil
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

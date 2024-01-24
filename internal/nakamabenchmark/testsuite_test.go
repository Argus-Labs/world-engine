package nakamabenchmark

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
)

const chars = "abcdefghijklmnopqrstuvwxyz"

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
			return fmt.Errorf("unable to get 'status' field from response: %w", err)
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
	if resp.StatusCode != http.StatusOK {
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

func triple(s string) (string, string, string) {
	return s, s, s
}

func randomString() string {
	b := &strings.Builder{}
	for i := 0; i < 10; i++ {
		n := rand.Intn(len(chars))
		b.WriteString(chars[n : n+1])
	}
	return b.String()
}

//nolint:gocognit
func TestCQLLoad(t *testing.T) {
	t.Logf("Starting loop for http load.")
	// Test persona
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

	var finalResults []interface{}

	// hits the cql endpoint to simulate http load for profiling.
	cqlQuery := "ALL()"
	for true {
		resp, err = c.rpc("query/game/cql", struct {
			CQL string `json:"CQL"`
		}{cqlQuery})
		assert.NilError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		results, err := io.ReadAll(resp.Body)
		assert.NilError(t, err)

		err = json.Unmarshal(results, &finalResults)
		if err != nil {
			t.Logf("There was an error: %s", err.Error())
		}
		t.Logf("http cql query: \"%s\" called", cqlQuery)
		time.Sleep(2 * time.Second)

	}

}

package nakamabenchmark

import (
	"encoding/json"
	"io"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"pkg.world.dev/world-engine/assert"
)

const chars = "abcdefghijklmnopqrstuvwxyz"

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
	for {
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

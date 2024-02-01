package benchmarkclient

import (
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/nakama_test/clientutils"
	"github.com/ethereum/go-ethereum/crypto"
	"pkg.world.dev/world-engine/assert"
)

func TestCQLLoad(t *testing.T) {
	t.Logf("Starting loop for http load.")
	// Test persona
	privateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	signerAddr := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	username, deviceID, personaTag := clientutils.Triple(clientutils.RandomString())
	c := clientutils.NewNakamaClient(t)
	assert.NilError(t, c.RegisterDevice(username, deviceID))

	resp, err := c.RPC("nakama/claim-persona", map[string]any{
		"personaTag":    personaTag,
		"signerAddress": signerAddr,
	})
	assert.NilError(t, err, "claim-persona failed")
	assert.Equal(t, 200, resp.StatusCode, clientutils.CopyBody(resp))

	var finalResults []interface{}

	// hits the cql endpoint to simulate http load for profiling.
	cqlQuery := "ALL()"
	for {
		resp, err = c.RPC("query/game/cql", struct {
			CQL string `json:"CQL"`
		}{cqlQuery})
		assert.NilError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		results, err := io.ReadAll(resp.Body)
		assert.NilError(t, err)

		err = json.Unmarshal(results, &finalResults)
		if err != nil {
			t.Logf("There was an error: %s", err.Error()) // ok it's just benchmarking.
		}
		t.Logf("http cql query: \"%s\" called", cqlQuery)
		time.Sleep(2 * time.Second)
	}
}

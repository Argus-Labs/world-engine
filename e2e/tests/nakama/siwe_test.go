package nakama

import (
	"crypto/ecdsa"
	"fmt"
	"math/rand"
	"testing"

	"github.com/argus-labs/world-engine/e2e/tests/clients"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"

	"pkg.world.dev/world-engine/assert"
)

func TestAuthenticateSIWE(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(crypto.S256(), rand.New(rand.NewSource(0)))
	assert.NilError(t, err)
	username := randomString()

	signerAddress := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	client := clients.NewNakamaClient(t)
	err = client.AuthenticateSIWE(username, signerAddress, func(msg string) string {
		// Signing via go instructions found here: https://docs.login.xyz/libraries/go#signing-messages-from-go-code
		msg = fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(msg), msg)
		hash := crypto.Keccak256Hash([]byte(msg))
		sig, err := crypto.Sign(hash.Bytes(), privateKey)
		assert.NilError(t, err)
		sig[64] += 27
		return hexutil.Encode(sig)
	})
	assert.NilError(t, err)
}

package sign

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"gotest.tools/v3/assert"
)

func TestCanSignAndVerifyPayload(t *testing.T) {
	goodKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	badKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	wantBody := "this is a request body"
	wantPersonaTag := "my-tag"
	wantNamespace := "my-namespace"
	wantNonce := uint64(100)

	sp, err := NewSignedPayload(goodKey, wantPersonaTag, wantNamespace, wantNonce, wantBody)
	assert.NilError(t, err)

	buf, err := sp.Marshal()
	assert.NilError(t, err)

	toBeVerified, err := Unmarshal(buf)
	assert.NilError(t, err)

	goodAddressHex := crypto.PubkeyToAddress(goodKey.PublicKey).Hex()
	badAddressHex := crypto.PubkeyToAddress(badKey.PublicKey).Hex()

	assert.Equal(t, toBeVerified.PersonaTag, wantPersonaTag)
	assert.Equal(t, toBeVerified.Namespace, wantNamespace)
	assert.Equal(t, toBeVerified.Nonce, wantNonce)
	assert.NilError(t, toBeVerified.Verify(goodAddressHex))
	assert.Error(t, toBeVerified.Verify(badAddressHex), ErrorSignatureValidationFailed.Error())
}

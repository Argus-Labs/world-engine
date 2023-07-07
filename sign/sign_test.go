package sign

import (
	"crypto/ecdsa"
	"crypto/rand"
	"testing"

	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"gotest.tools/v3/assert"
)

func TestCanSignAndVerifyPayload(t *testing.T) {
	goodKey, err := ecdsa.GenerateKey(secp256k1.S256(), rand.Reader)
	assert.NilError(t, err)
	badKey, err := ecdsa.GenerateKey(secp256k1.S256(), rand.Reader)
	assert.NilError(t, err)
	wantBody := "this is a request body"
	wantPersonaTag := "my-tag"
	wantNamespace := "my-namespace"
	wantNonce := uint64(100)

	sp, err := NewSignedPayload([]byte(wantBody), wantPersonaTag, wantNamespace, wantNonce, goodKey)
	assert.NilError(t, err)

	buf, err := sp.Marshal()
	assert.NilError(t, err)

	toBeVerified, err := Unmarshal(buf)
	assert.NilError(t, err)

	assert.Equal(t, toBeVerified.PersonaTag, wantPersonaTag)
	assert.Equal(t, toBeVerified.Namespace, wantNamespace)
	assert.Equal(t, toBeVerified.Nonce, wantNonce)
	assert.NilError(t, toBeVerified.Verify(goodKey.PublicKey))
	assert.Error(t, toBeVerified.Verify(badKey.PublicKey), ErrorSignatureValidationFailed.Error())
}

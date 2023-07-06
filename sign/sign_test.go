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
	wantTag := "my-tag"
	wantNonce := uint64(100)

	sp, err := NewSignedPayload([]byte(wantBody), wantTag, wantNonce, goodKey)
	assert.NilError(t, err)

	buf, err := sp.Marshal()
	assert.NilError(t, err)

	toBeVerified, err := Unmarshal(buf)
	assert.NilError(t, err)

	assert.Equal(t, toBeVerified.Tag, wantTag)
	assert.Equal(t, toBeVerified.Nonce, wantNonce)
	assert.NilError(t, toBeVerified.Verify(goodKey.PublicKey))
	assert.Check(t, nil != toBeVerified.Verify(badKey.PublicKey))
}

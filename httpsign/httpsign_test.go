package httpsign

import (
	"crypto/ecdsa"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"gotest.tools/v3/assert"
)

func TestCanSignAndVerifyRequest(t *testing.T) {
	goodKey, err := ecdsa.GenerateKey(secp256k1.S256(), rand.Reader)
	assert.NilError(t, err)
	badKey, err := ecdsa.GenerateKey(secp256k1.S256(), rand.Reader)
	assert.NilError(t, err)
	wantBody := "this is a request body"
	wantTag := "my-tag"
	wantNonce := uint64(100)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		details, err := GetSigDetails(r)
		assert.NilError(t, err)
		assert.Equal(t, details.Tag, wantTag)
		assert.Equal(t, details.Nonce, wantNonce)

		assert.Check(t, nil != Verify(r, badKey.PublicKey))
		assert.NilError(t, Verify(r, goodKey.PublicKey))
		// Make sure we can still read the request body
		body, err := io.ReadAll(r.Body)
		assert.NilError(t, err)
		assert.Equal(t, string(body), wantBody)
	}))
	defer ts.Close()

	req, err := http.NewRequest("GET", ts.URL, strings.NewReader(wantBody))
	assert.NilError(t, err)
	err = Sign(req, wantTag, wantNonce, goodKey)
	assert.NilError(t, err)

	_, err = ts.Client().Do(req)
	assert.NilError(t, err)
}

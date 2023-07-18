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

	toBeVerified, err := UnmarshalSignedPayload(buf)
	assert.NilError(t, err)

	goodAddressHex := crypto.PubkeyToAddress(goodKey.PublicKey).Hex()
	badAddressHex := crypto.PubkeyToAddress(badKey.PublicKey).Hex()

	assert.Equal(t, toBeVerified.PersonaTag, wantPersonaTag)
	assert.Equal(t, toBeVerified.Namespace, wantNamespace)
	assert.Equal(t, toBeVerified.Nonce, wantNonce)
	assert.NilError(t, toBeVerified.Verify(goodAddressHex))
	assert.Error(t, toBeVerified.Verify(badAddressHex), ErrorSignatureValidationFailed.Error())
}

func TestFailsIfFieldsMissing(t *testing.T) {
	goodKey, err := crypto.GenerateKey()
	assert.NilError(t, err)

	testCases := []struct {
		name    string
		payload func() *SignedPayload
		expErr  bool
	}{
		{
			name: "valid",
			payload: func() *SignedPayload {
				sp, err := NewSignedPayload(goodKey, "tag", "namespace", 40, "body")
				assert.NilError(t, err)
				return sp
			},
			expErr: false,
		},
		{
			name: "missing persona tag",
			payload: func() *SignedPayload {
				sp, err := NewSignedPayload(goodKey, "", "ns", 20, "body")
				assert.NilError(t, err)
				return sp
			},
			expErr: true,
		},
		{
			name: "missing namespace",
			payload: func() *SignedPayload {
				sp, err := NewSignedPayload(goodKey, "fop", "", 20, "body")
				assert.NilError(t, err)
				return sp
			},
			expErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bz, err := tc.payload().Marshal()
			assert.NilError(t, err)
			_, err = UnmarshalSignedPayload(bz)
			if tc.expErr {
				assert.ErrorContains(t, err, "")
			} else {
				assert.NilError(t, err)
			}
		})
	}
}

package sign

import (
	"encoding/json"
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
		payload func() (*SignedPayload, error)
		expErr  error
	}{
		{
			name: "valid",
			payload: func() (*SignedPayload, error) {
				return NewSignedPayload(goodKey, "tag", "namespace", 40, "body")
			},
			expErr: nil,
		},
		{
			name: "missing persona tag",
			payload: func() (*SignedPayload, error) {
				return NewSignedPayload(goodKey, "", "ns", 20, "body")
			},
			expErr: ErrorInvalidPersonaTag,
		},
		{
			name: "missing namespace",
			payload: func() (*SignedPayload, error) {
				return NewSignedPayload(goodKey, "fop", "", 20, "body")
			},
			expErr: ErrorInvalidNamespace,
		},
		{
			name: "system signed payload",
			payload: func() (*SignedPayload, error) {
				return NewSystemSignedPayload(goodKey, "some-namespace", 25, "body")
			},
			expErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := tc.payload()
			if tc.expErr != nil {
				assert.ErrorIs(t, tc.expErr, err)
				return
			}
			assert.NilError(t, err, "in test case %q", tc.name)
			bz, err := payload.Marshal()
			assert.NilError(t, err)
			_, err = UnmarshalSignedPayload(bz)
			assert.NilError(t, err)
		})
	}
}

func TestStringsBytesAndStructsCanBeSigned(t *testing.T) {
	key, err := crypto.GenerateKey()
	assert.NilError(t, err)

	type SomeStruct struct {
		Str string
		Num int
	}
	testCases := []any{
		SomeStruct{Str: "a-string", Num: 99},
		`{"Str": "a-string", "Num": 99}`,
		[]byte(`{"Str": "a-string", "Num": 99}`),
	}

	for _, tc := range testCases {
		sp, err := NewSignedPayload(key, "coolmage", "world", 100, tc)
		assert.NilError(t, err)

		buf, err := sp.Marshal()
		assert.NilError(t, err)
		gotSP, err := UnmarshalSignedPayload(buf)
		assert.NilError(t, err)
		var gotStruct SomeStruct
		assert.NilError(t, json.Unmarshal(gotSP.Body, &gotStruct))
		assert.Equal(t, "a-string", gotStruct.Str)
		assert.Equal(t, 99, gotStruct.Num)
	}
}

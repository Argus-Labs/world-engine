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
	wantBody := `{"msg": "this is a request body"}`
	wantPersonaTag := "my-tag"
	wantNamespace := "my-namespace"
	wantNonce := uint64(100)

	sp, err := NewTransaction(goodKey, wantPersonaTag, wantNamespace, wantNonce, wantBody)
	assert.NilError(t, err)

	buf, err := sp.Marshal()
	assert.NilError(t, err)

	toBeVerified, err := UnmarshalTransaction(buf)
	assert.NilError(t, err)

	goodAddressHex := crypto.PubkeyToAddress(goodKey.PublicKey).Hex()
	badAddressHex := crypto.PubkeyToAddress(badKey.PublicKey).Hex()

	assert.Equal(t, toBeVerified.PersonaTag, wantPersonaTag)
	assert.Equal(t, toBeVerified.Namespace, wantNamespace)
	assert.Equal(t, toBeVerified.Nonce, wantNonce)
	assert.NilError(t, toBeVerified.Verify(goodAddressHex))
	assert.Error(t, toBeVerified.Verify(badAddressHex), ErrSignatureValidationFailed.Error())
}

func TestFailsIfFieldsMissing(t *testing.T) {
	goodKey, err := crypto.GenerateKey()
	assert.NilError(t, err)

	testCases := []struct {
		name    string
		payload func() (*Transaction, error)
		expErr  error
	}{
		{
			name: "valid",
			payload: func() (*Transaction, error) {
				return NewTransaction(goodKey, "tag", "namespace", 40, "{}")
			},
			expErr: nil,
		},
		{
			name: "missing persona tag",
			payload: func() (*Transaction, error) {
				return NewTransaction(goodKey, "", "ns", 20, "{}")
			},
			expErr: ErrInvalidPersonaTag,
		},
		{
			name: "missing namespace",
			payload: func() (*Transaction, error) {
				return NewTransaction(goodKey, "fop", "", 20, "{}")
			},
			expErr: ErrInvalidNamespace,
		},
		{
			name: "system transaction",
			payload: func() (*Transaction, error) {
				return NewSystemTransaction(goodKey, "some-namespace", 25, "{}")
			},
			expErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var payload *Transaction
			payload, err = tc.payload()
			if tc.expErr != nil {
				assert.ErrorIs(t, tc.expErr, err)
				return
			}
			assert.NilError(t, err, "in test case %q", tc.name)
			var bz []byte
			bz, err = payload.Marshal()
			assert.NilError(t, err)
			_, err = UnmarshalTransaction(bz)
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
		// This test case has different kinds of whitespace.
		`{
		"Str":      "a-string", 

"Num":    99   }`,
	}

	for _, tc := range testCases {
		var sp *Transaction
		sp, err = NewTransaction(key, "coolmage", "world", 100, tc)
		assert.NilError(t, err)
		var buf []byte
		buf, err = sp.Marshal()
		assert.NilError(t, err)
		var gotSP *Transaction
		gotSP, err = UnmarshalTransaction(buf)
		assert.NilError(t, err)
		var gotStruct SomeStruct
		assert.NilError(t, json.Unmarshal(gotSP.Body, &gotStruct))
		assert.Equal(t, "a-string", gotStruct.Str)
		assert.Equal(t, 99, gotStruct.Num)
	}
}

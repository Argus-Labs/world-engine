package sign

import (
	"encoding/json"
	"testing"

	"github.com/ethereum/go-ethereum/common"
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
	// Make sure an empty hash is regenerated
	toBeVerified.Hash = common.Hash{}
	assert.NilError(t, toBeVerified.Verify(goodAddressHex))

	// Verify signature verification can fail
	assert.ErrorIs(t, toBeVerified.Verify(badAddressHex), ErrSignatureValidationFailed)
}

func TestCanParseAMappedSignedPayload(t *testing.T) {
	goodKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	body := `{"msg": "this is a request body"}`
	personaTag := "my-tag"
	namespace := "my-namespace"
	nonce := uint64(100)

	sp, err := NewSignedPayload(goodKey, personaTag, namespace, nonce, body)
	assert.NilError(t, err)
	bz, err := json.Marshal(sp)
	assert.NilError(t, err)
	asMap := map[string]any{}
	assert.NilError(t, json.Unmarshal(bz, &asMap))

	gotSP, err := MappedSignedPayload(asMap)
	assert.NilError(t, err)

	assert.DeepEqual(t, sp, gotSP)
}

func TestCanGetHashHex(t *testing.T) {
	goodKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	wantBody := `{"msg": "this is a request body"}`
	wantPersonaTag := "my-tag"
	wantNamespace := "my-namespace"
	wantNonce := uint64(100)

	sp, err := NewSignedPayload(goodKey, wantPersonaTag, wantNamespace, wantNonce, wantBody)
	wantHash := sp.HashHex()

	sp.Hash = common.Hash{}
	// Make sure the hex is regenerated
	gotHash := sp.HashHex()
	assert.Equal(t, wantHash, gotHash)
}

func TestIsSignedSystemPayload(t *testing.T) {
	goodKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	body := `{"msg": "this is a request body"}`
	personaTag := "my-tag"
	namespace := "my-namespace"
	nonce := uint64(100)

	sp, err := NewSignedPayload(goodKey, personaTag, namespace, nonce, body)
	assert.NilError(t, err)
	assert.Check(t, !sp.IsSystemPayload())

	sp, err = NewSystemSignedPayload(goodKey, namespace, nonce, body)
	assert.NilError(t, err)
	assert.Check(t, sp.IsSystemPayload())
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
				return NewSignedPayload(goodKey, "tag", "namespace", 40, "{}")
			},
			expErr: nil,
		},
		{
			name: "missing persona tag",
			payload: func() (*SignedPayload, error) {
				return NewSignedPayload(goodKey, "", "ns", 20, "{}")
			},
			expErr: ErrInvalidPersonaTag,
		},
		{
			name: "missing namespace",
			payload: func() (*SignedPayload, error) {
				return NewSignedPayload(goodKey, "fop", "", 20, "{}")
			},
			expErr: ErrInvalidNamespace,
		},
		{
			name: "system signed payload",
			payload: func() (*SignedPayload, error) {
				return NewSystemSignedPayload(goodKey, "some-namespace", 25, "{}")
			},
			expErr: nil,
		},
		{
			name: "signed payload with SystemPersonaTag",
			payload: func() (*SignedPayload, error) {
				return NewSignedPayload(goodKey, SystemPersonaTag, "some-namespace", 25, "{}")
			},
			expErr: ErrInvalidPersonaTag,
		},
		{
			name: "empty body",
			payload: func() (*SignedPayload, error) {
				return NewSystemSignedPayload(goodKey, "some-namespace", 25, "")
			},
			expErr: ErrCannotSignEmptyBody,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var payload *SignedPayload
			payload, err = tc.payload()
			if tc.expErr != nil {
				assert.ErrorIs(t, tc.expErr, err)
				return
			}
			assert.NilError(t, err, "in test case %q", tc.name)
			var bz []byte
			bz, err = payload.Marshal()
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
		// This test case has different kinds of whitespace.
		`{
		"Str":      "a-string", 

"Num":    99   }`,
	}

	for _, tc := range testCases {
		var sp *SignedPayload
		sp, err = NewSignedPayload(key, "coolmage", "world", 100, tc)
		assert.NilError(t, err)
		var buf []byte
		buf, err = sp.Marshal()
		assert.NilError(t, err)
		var gotSP *SignedPayload
		gotSP, err = UnmarshalSignedPayload(buf)
		assert.NilError(t, err)
		var gotStruct SomeStruct
		assert.NilError(t, json.Unmarshal(gotSP.Body, &gotStruct))
		assert.Equal(t, "a-string", gotStruct.Str)
		assert.Equal(t, 99, gotStruct.Num)
	}
}

func TestRejectInvalidSignatures(t *testing.T) {
	key, err := crypto.GenerateKey()
	assert.NilError(t, err)
	type Payload struct {
		Value int
	}
	_, err = NewSignedPayload(key, "", "namespace", 100, Payload{100})
	assert.ErrorIs(t, err, ErrInvalidPersonaTag)
	_, err = NewSignedPayload(key, "persona_tag", "", 100, Payload{100})
	assert.ErrorIs(t, err, ErrInvalidNamespace)
	_, err = NewSignedPayload(key, "persona_tag", "", 100, nil)
	assert.ErrorIs(t, err, ErrCannotSignEmptyBody)
}

func TestRejectInvalidJSON(t *testing.T) {
	data := `{"personaTag":"jeff", "namespace": {{{`
	_, err := UnmarshalSignedPayload([]byte(data))
	assert.Check(t, err != nil)
}

func TestRejectSignatureWithExtraField(t *testing.T) {
	data := map[string]any{
		"personaTag": "persona-tag",
		"namespace":  "namespace",
		"nonce":      100,
		"signature":  "xyzzy",
		"body":       "bar",
	}

	bz, err := json.Marshal(data)
	assert.NilError(t, err)
	_, err = UnmarshalSignedPayload(bz)
	assert.NilError(t, err)
	data["extra_field"] = "hello"
	bz, err = json.Marshal(data)
	assert.NilError(t, err)
	_, err = UnmarshalSignedPayload(bz)
	assert.Check(t, err != nil)

	_, err = MappedSignedPayload(data)
	assert.Check(t, err != nil)
}

func TestRejectBadSerializedSignatures(t *testing.T) {
	validData := map[string]any{
		"personaTag": "persona-tag",
		"namespace":  "namespace",
		"nonce":      100,
		"signature":  "xyzzy",
		"body":       "bar",
	}

	// Make sure the valid data can actually be unmarshalled
	bz, err := json.Marshal(validData)
	assert.NilError(t, err)
	_, err = UnmarshalSignedPayload(bz)
	assert.NilError(t, err)

	fieldsToOmit := []string{"personaTag", "namespace", "signature", "body"}

	copyValidData := func() map[string]any {
		cpy := map[string]any{}
		for k, v := range validData {
			cpy[k] = v
		}
		return cpy
	}

	// Take out each field one at a time to ensure the Unmarshaling of incomplete signatures fails
	for _, field := range fieldsToOmit {
		currData := copyValidData()
		delete(currData, field)
		bz, err = json.Marshal(currData)
		assert.NilError(t, err)
		_, err = UnmarshalSignedPayload(bz)
		assert.Check(t, err != nil, "in UnmarshalSignedPayload want error when field %q is missing", field)

		_, err = MappedSignedPayload(currData)
		assert.Check(t, err != nil, "in MappedSignedPayload: want error when field %q is missing", field)
	}
}

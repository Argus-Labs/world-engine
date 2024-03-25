package sign

import (
	"encoding/json"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/rotisserie/eris"
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
	// Make sure an empty hash is regenerated
	toBeVerified.Hash = common.Hash{}
	assert.NilError(t, toBeVerified.Verify(goodAddressHex))

	// Verify signature verification can fail
	errorWithStackTrace := toBeVerified.Verify(badAddressHex)
	err = eris.Unwrap(errorWithStackTrace)
	assert.ErrorIs(t, err, ErrSignatureValidationFailed)
}

func TestCanParseAMappedTransaction(t *testing.T) {
	goodKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	body := `{"msg": "this is a request body"}`
	personaTag := "my-tag"
	namespace := "my-namespace"
	nonce := uint64(100)

	sp, err := NewTransaction(goodKey, personaTag, namespace, nonce, body)
	assert.NilError(t, err)
	bz, err := json.Marshal(sp)
	assert.NilError(t, err)
	asMap := map[string]any{}
	assert.NilError(t, json.Unmarshal(bz, &asMap))

	gotSP, err := MappedTransaction(asMap)
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

	sp, err := NewTransaction(goodKey, wantPersonaTag, wantNamespace, wantNonce, wantBody)
	assert.NilError(t, err)
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

	sp, err := NewTransaction(goodKey, personaTag, namespace, nonce, body)
	assert.NilError(t, err)
	assert.Check(t, !sp.IsSystemTransaction())

	sp, err = NewSystemTransaction(goodKey, namespace, nonce, body)
	assert.NilError(t, err)
	assert.Check(t, sp.IsSystemTransaction())
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
		{
			name: "signed payload with SystemPersonaTag",
			payload: func() (*Transaction, error) {
				return NewTransaction(goodKey, SystemPersonaTag, "some-namespace", 25, "{}")
			},
			expErr: ErrInvalidPersonaTag,
		},
		{
			name: "empty body",
			payload: func() (*Transaction, error) {
				return NewSystemTransaction(goodKey, "some-namespace", 25, "")
			},
			expErr: ErrCannotSignEmptyBody,
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
		assert.NilError(t, json.Unmarshal(gotSP.Message, &gotStruct))
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
	_, err = NewTransaction(key, "", "namespace", 100, Payload{100})
	assert.ErrorIs(t, err, ErrInvalidPersonaTag)
	_, err = NewTransaction(key, "persona_tag", "", 100, Payload{100})
	assert.ErrorIs(t, err, ErrInvalidNamespace)
	_, err = NewTransaction(key, "persona_tag", "", 100, nil)
	assert.ErrorIs(t, err, ErrCannotSignEmptyBody)
}

func TestRejectInvalidJSON(t *testing.T) {
	data := `{"personaTag":"jeff", "namespace": {{{`
	_, err := UnmarshalTransaction([]byte(data))
	assert.Check(t, err != nil)
}

func TestRejectSignatureWithExtraField(t *testing.T) {
	data := map[string]any{
		"personaTag": "persona-tag",
		"namespace":  "namespace",
		"nonce":      100,
		"signature":  "xyzzy",
		"Message":    "bar",
	}

	bz, err := json.Marshal(data)
	assert.NilError(t, err)
	_, err = UnmarshalTransaction(bz)
	assert.NilError(t, err)
	data["extra_field"] = "hello"
	bz, err = json.Marshal(data)
	assert.NilError(t, err)
	_, err = UnmarshalTransaction(bz)
	assert.Check(t, err != nil)

	_, err = MappedTransaction(data)
	assert.Check(t, err != nil)
}

func TestRejectBadSerializedSignatures(t *testing.T) {
	validData := map[string]any{
		"personaTag": "persona-tag",
		"namespace":  "namespace",
		"nonce":      100,
		"signature":  "xyzzy",
		"message":    "bar",
	}

	// Make sure the valid data can actually be unmarshalled
	bz, err := json.Marshal(validData)
	assert.NilError(t, err)
	_, err = UnmarshalTransaction(bz)
	assert.NilError(t, err)

	fieldsToOmit := []string{"personaTag", "namespace", "signature", "message"}

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
		_, err = UnmarshalTransaction(bz)
		assert.Check(t, err != nil, "in UnmarshalTransaction want error when field %q is missing", field)

		_, err = MappedTransaction(currData)
		assert.Check(t, err != nil, "in MappedTransaction: want error when field %q is missing", field)
	}
}

func TestUnsortedJSONBlobsCanBeSignedAndVerified(t *testing.T) {
	key, err := crypto.GenerateKey()
	assert.NilError(t, err)

	// This is valid JSON, however the fields are not sorted. The hash for this body will be different from a
	// hash generated from a swagger endpoint (because the body becomes a map[string]any{}). This test ensures
	// unsorted JSON bodies and the corresponding map[string]any bodies can be consistently signed.
	bodyStr := `{
					"omega":2,
					"alpha":1
				}`

	tx, err := NewTransaction(key, "persona-tag", "namespace", 100, bodyStr)
	assert.NilError(t, err)

	body := map[string]any{
		"alpha": 1,
		"omega": 2,
	}

	dataAsMap := map[string]any{
		"personaTag": "persona-tag",
		"namespace":  "namespace",
		"nonce":      100,
		"signature":  tx.Signature,
		"message":    body,
	}
	gotTx, err := MappedTransaction(dataAsMap)
	assert.NilError(t, err)
	addr := crypto.PubkeyToAddress(key.PublicKey).Hex()

	assert.NilError(t, gotTx.Verify(addr))
}

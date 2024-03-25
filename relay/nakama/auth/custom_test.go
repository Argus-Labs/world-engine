package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama-common/runtime"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/relay/nakama/siwe"
	"pkg.world.dev/world-engine/relay/nakama/testutils"
)

func TestMissingSignerAddressReturnsError(t *testing.T) {
	testCases := []struct {
		wantErr error
		account *api.AccountCustom
	}{
		{
			siwe.ErrMissingSignerAddress,
			&api.AccountCustom{
				Id: "",
				Vars: map[string]string{
					"type":      "siwe",
					"signature": "a signature",
					"message":   "a message",
				},
			},
		},
		{
			ErrBadCustomAuthType,
			&api.AccountCustom{
				Id: "some-id",
				Vars: map[string]string{
					"type":      "",
					"signature": "a signature",
					"message":   "a message",
				},
			},
		},
		{
			ErrBadCustomAuthType,
			&api.AccountCustom{
				Id: "some-id",
				Vars: map[string]string{
					"type":      "what-type-is-this",
					"signature": "a signature",
					"message":   "a message",
				},
			},
		},
		{
			siwe.ErrMissingSignature,
			&api.AccountCustom{
				Id: "some-id",
				Vars: map[string]string{
					"type":      "siwe",
					"signature": "",
					"message":   "where is my signature?",
				},
			},
		},
		{
			siwe.ErrMissingMessage,
			&api.AccountCustom{
				Id: "some-id",
				Vars: map[string]string{
					"type":      "siwe",
					"signature": "where is my message?",
					"message":   "",
				},
			},
		},
	}
	logger := testutils.MockNoopLogger(t)
	var db *sql.DB
	var nk runtime.NakamaModule
	for _, tc := range testCases {
		in := &api.AuthenticateCustomRequest{
			Account: tc.account,
		}
		_, err := handleCustomAuthentication(context.Background(), logger, db, nk, in)
		assert.ErrorContains(t, err, tc.wantErr.Error())
	}
}

func TestCanGenerateAndSignSIWEMessage(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	address := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	logger := testutils.MockNoopLogger(t)
	var db *sql.DB
	nk := testutils.NewFakeNakamaModule()
	in := &api.AuthenticateCustomRequest{
		Account: &api.AccountCustom{
			Id: address,
			Vars: map[string]string{
				"type": "siwe",
			},
		},
	}
	_, err = handleCustomAuthentication(context.Background(), logger, db, nk, in)
	assert.NotNil(t, err)
	var result siwe.GenerateResult
	err = json.Unmarshal([]byte(err.Error()), &result)
	assert.NilError(t, err)

	// The SIWE message we must sign is now in result.SIWEMessage
	// Signing via go instructions found here: https://docs.login.xyz/libraries/go#signing-messages-from-go-code
	msg := result.SIWEMessage
	msg = fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(msg), msg)
	hash := crypto.Keccak256Hash([]byte(msg))
	sig, err := crypto.Sign(hash.Bytes(), privateKey)
	assert.NilError(t, err)
	sig[64] += 27
	signature := hexutil.Encode(sig)

	// Use the message and signature to validate this user
	in.Account.Vars["message"] = result.SIWEMessage
	in.Account.Vars["signature"] = signature
	in, err = handleCustomAuthentication(context.Background(), logger, db, nk, in)
	assert.NilError(t, err)
	assert.NotNil(t, in)

	// Attempting to authenticate again should fail
	_, err = handleCustomAuthentication(context.Background(), logger, db, nk, in)
	assert.IsError(t, err)
}

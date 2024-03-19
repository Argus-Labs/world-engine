package siwe_test

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	spruceswid "github.com/spruceid/siwe-go"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/relay/nakama/siwe"
	"pkg.world.dev/world-engine/relay/nakama/testutils"
)

const anySignerAddress = "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2"

func TestSignerAddressRequired(t *testing.T) {
	_, _, err := siwe.HandleSIWE(context.Background(), nil, "", "", "")
	assert.ErrorIs(t, err, siwe.ErrMissingSignerAddress)
}

func TestMessageAndSignatureMustBothBePresentOrBothBeMissing(t *testing.T) {
	_, _, err := siwe.HandleSIWE(context.Background(), nil, "sa", "message", "")
	assert.ErrorIs(t, err, siwe.ErrMissingSignature)

	_, _, err = siwe.HandleSIWE(context.Background(), nil, "sa", "", "signature")
	assert.ErrorIs(t, err, siwe.ErrMissingMessage)
}

func TestSignerAddressInMessage(t *testing.T) {
	fakeNK := testutils.NewFakeNakamaModule()
	// No signature and no message was provided, so a new SIWE message should be generated
	_, resp, err := siwe.HandleSIWE(context.Background(), fakeNK, anySignerAddress, "", "")
	assert.NilError(t, err)
	assert.Contains(t, resp.SIWEMessage, anySignerAddress)
}

func TestDomainInMessage(t *testing.T) {
	fakeNK := testutils.NewFakeNakamaModule()
	_, resp, err := siwe.HandleSIWE(context.Background(), fakeNK, anySignerAddress, "", "")
	assert.NilError(t, err)
	assert.Contains(t, resp.SIWEMessage, siwe.DefaultDomain)
}

func TestURIInMessage(t *testing.T) {
	fakeNK := testutils.NewFakeNakamaModule()
	_, resp, err := siwe.HandleSIWE(context.Background(), fakeNK, anySignerAddress, "", "")
	assert.NilError(t, err)
	assert.Contains(t, resp.SIWEMessage, siwe.DefaultURI)
}

func signMessage(t *testing.T, msg string, pk *ecdsa.PrivateKey) string {
	// Signing via go instructions found here: https://docs.login.xyz/libraries/go#signing-messages-from-go-code
	msg = fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(msg), msg)
	hash := crypto.Keccak256Hash([]byte(msg))
	sig, err := crypto.Sign(hash.Bytes(), pk)
	assert.NilError(t, err)
	sig[64] += 27
	return hexutil.Encode(sig)
}

func TestCanSignAndValidateMessage(t *testing.T) {
	ctx := context.Background()
	fakeNK := testutils.NewFakeNakamaModule()
	privateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	address := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	// Get the message that needs to be signed
	isAuthSuccessful, resp, err := siwe.HandleSIWE(ctx, fakeNK, address, "", "")
	assert.NilError(t, err)
	assert.False(t, isAuthSuccessful)
	assert.NotNil(t, resp)

	signature := signMessage(t, resp.SIWEMessage, privateKey)
	isAuthSuccessful, resp, err = siwe.HandleSIWE(ctx, fakeNK, address, resp.SIWEMessage, signature)
	assert.NilError(t, err)
	assert.True(t, isAuthSuccessful)
	assert.Nil(t, resp)
}

func TestInvalidSignatureIsRejected(t *testing.T) {
	testCases := []struct {
		name string
		// replaceStrings will be used to modify the SIWE Message for each teat case. The "from" string
		// will be replaced with the "to" string. All such modifications should result in a failed signature.
		toReplace func(*spruceswid.Message) (from, to string)
	}{
		{
			name: "modified nonce",
			toReplace: func(message *spruceswid.Message) (string, string) {
				nonce := message.GetNonce()
				return nonce, "12345"
			},
		},
		{
			name: "modified signer address",
			toReplace: func(message *spruceswid.Message) (string, string) {
				addr := message.GetAddress().Hex()
				return addr, anySignerAddress
			},
		},
		{
			name: "modified domain",
			toReplace: func(*spruceswid.Message) (string, string) {
				return siwe.DefaultDomain, "some-other-domain"
			},
		},
		{
			name: "modified uri",
			toReplace: func(*spruceswid.Message) (string, string) {
				return "authenticate/custom", "some-other-uri"
			},
		},
		{
			name: "missing expiration time",
			toReplace: func(message *spruceswid.Message) (string, string) {
				str := *message.GetExpirationTime()
				toRemove := "\nExpiration Time: " + str
				return toRemove, ""
			},
		},
		{
			name: "old expiration time",
			toReplace: func(message *spruceswid.Message) (string, string) {
				str := *message.GetExpirationTime()
				expTime, err := time.Parse(time.RFC3339, str)
				assert.NilError(t, err)
				// Moving the expiration time an hour into the past should case it to be expired
				oldExpTime := expTime.Add(-time.Hour)
				return str, oldExpTime.Format(time.RFC3339)
			},
		},
	}

	fakeNK := testutils.NewFakeNakamaModule()
	ctx := context.Background()
	privateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	address := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	isAuthSuccessful, resp, err := siwe.HandleSIWE(ctx, fakeNK, address, "", "")
	assert.NilError(t, err)
	assert.False(t, isAuthSuccessful)
	assert.NotNil(t, resp)

	msg, err := spruceswid.ParseMessage(resp.SIWEMessage)
	assert.NilError(t, err)
	for _, tc := range testCases {
		from, to := tc.toReplace(msg)
		originalMessage := msg.String()
		newMsg := strings.Replace(originalMessage, from, to, 1)
		// To make sure this test case is actually testing something, make sure the message changes.
		assert.NotEqual(t, originalMessage, newMsg)
		signature := signMessage(t, newMsg, privateKey)
		isAuthSuccessful, _, err = siwe.HandleSIWE(ctx, fakeNK, address, newMsg, signature)
		errMsg := fmt.Sprintf("in test case %q, sig verification succeeded when it should have failed", tc.name)
		assert.Check(t, err != nil, errMsg)
		assert.False(t, isAuthSuccessful)
	}
}

func TestOnlyOneValidateMessageShouldBeSuccessful(t *testing.T) {
	ctx := context.Background()
	fakeNK := testutils.NewFakeNakamaModule()
	privateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	address := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	_, resp, err := siwe.HandleSIWE(ctx, fakeNK, address, "", "")
	assert.NilError(t, err)
	assert.NotEqual(t, resp.SIWEMessage, "")

	signature := signMessage(t, resp.SIWEMessage, privateKey)

	type result struct {
		resp *siwe.HandleSIWEResult
		err  error
	}

	results := make(chan result)
	trials := 100
	for i := 0; i < trials; i++ {
		go func() {
			_, currResp, currErr := siwe.HandleSIWE(ctx, fakeNK, address, resp.SIWEMessage, signature)
			results <- result{
				resp: currResp,
				err:  currErr,
			}
		}()
	}

	numOfSuccesses := 0
	for i := 0; i < trials; i++ {
		currResult := <-results
		if currResult.err == nil {
			numOfSuccesses++
		}
	}
	assert.Equal(t, 1, numOfSuccesses)
}

func TestCustomNonceCanBeUsed(t *testing.T) {
	ctx := context.Background()
	fakeNK := testutils.NewFakeNakamaModule()
	privateKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	address := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	// Get some random SIWE Message. The nonce will be modified
	_, resp, err := siwe.HandleSIWE(ctx, fakeNK, address, "", "")
	assert.NilError(t, err)
	assert.NotNil(t, resp)

	// Find the current nonce; we'll use the nonce string to find/replace it with some custom nonce
	msg, err := spruceswid.ParseMessage(resp.SIWEMessage)
	assert.NilError(t, err)
	nonce := msg.GetNonce()

	// Replace the nonce that was returned from the server
	newMsg := strings.Replace(resp.SIWEMessage, nonce, "SomeDifferentNonce", 1)
	signature := signMessage(t, newMsg, privateKey)

	// This verification should succeed
	isAuthSuccessful, _, err := siwe.HandleSIWE(ctx, fakeNK, address, newMsg, signature)
	assert.NilError(t, err)
	assert.Check(t, isAuthSuccessful)

	// But trying to use the same signature/message should fail
	isAuthSuccessful, _, err = siwe.HandleSIWE(ctx, fakeNK, address, newMsg, signature)
	assert.IsError(t, err)
	assert.False(t, isAuthSuccessful)
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"pkg.world.dev/world-engine/relay/nakama/allowlist"
	"pkg.world.dev/world-engine/relay/nakama/signer"
	"pkg.world.dev/world-engine/relay/nakama/testutils"
	"testing"

	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/relay/nakama/mocks"
)

type AllowListTestSuite struct {
	suite.Suite
	originalAllowListEnabled bool
}

func TestAllowList(t *testing.T) {
	suite.Run(t, &AllowListTestSuite{})
}

func (a *AllowListTestSuite) SetupTest() {
	a.originalAllowListEnabled = allowlist.Enabled
	allowlist.Enabled = true
	a.T().Setenv(allowlist.EnabledEnvVar, "")
}

func (a *AllowListTestSuite) TearDownTest() {
	allowlist.Enabled = a.originalAllowListEnabled
}

func (a *AllowListTestSuite) TestUserIDRequired() {
	t := a.T()
	// This context does not have a user ID.
	ctx := context.Background()

	_, err := handleClaimKey(ctx, testutils.MockNoopLogger(t), nil, nil, "")
	// handleClaimKey should fail because the userID cannot be found in the context
	assert.IsError(t, err)
}

func (a *AllowListTestSuite) TestErrorFromStorageIsReturnedToCaller() {
	t := a.T()
	userID := "the-user-id-is-also-the-store-key"
	ctx := testutils.CtxWithUserID(userID)
	errorMsg := "read failure"

	mockNK := mocks.NewNakamaModule(t)
	// Make sure the code asks specifically for the uerID key
	mockNK.On("StorageRead", mock.Anything, testutils.MockMatchReadKey(userID)).
		Return(nil, errors.New(errorMsg)).
		Once()

	_, err := handleClaimKey(ctx, testutils.MockNoopLogger(t), nil, mockNK, `{"Key":"beta-key"}`)
	assert.ErrorContains(t, err, errorMsg)
}

func (a *AllowListTestSuite) TestCannotClaimASecondBetaKey() {
	t := a.T()
	ctx := testutils.CtxWithUserID("foo")

	// This storage object doesn't contain any data. The mere presence of the storage object means the beta key
	// has been verified
	readResponse := []*api.StorageObject{{
		Value: "{}",
	}}

	mockNK := mocks.NewNakamaModule(t)
	mockNK.On("StorageRead", mock.Anything, mock.Anything).
		Return(readResponse, nil).
		Once()

	_, err := handleClaimKey(ctx, testutils.MockNoopLogger(t), nil, mockNK, `{"Key":"some-other-beta-key"}`)
	assert.ErrorContains(t, err, allowlist.ErrAlreadyVerified.Error())
}

func (a *AllowListTestSuite) TestBadKeyRequestsAreRejected() {
	t := a.T()
	ctx := testutils.CtxWithUserID("foo")
	mockNK := mocks.NewNakamaModule(t)
	// This storage read is checking for a valid beta key. In this test the user hasn't yet been verified, so
	// no results will be returned.
	mockNK.On("StorageRead", mock.Anything, mock.Anything).
		Return(nil, nil).
		Once()

	_, err := handleClaimKey(ctx, testutils.MockNoopLogger(t), nil, mockNK, `{"key": ""}`)
	// Nakama returns its own custom runtime error which does NOT implement the Is method, making ErrorIs not helpful.
	assert.ErrorContains(t, err, allowlist.ErrInvalidBetaKey.Error())

	badBody := `{"key": "{{{{`
	_, err = handleClaimKey(ctx, testutils.MockNoopLogger(t), nil, mockNK, badBody)
	assert.IsError(t, err)
}

func (a *AllowListTestSuite) TestCanDisableAllowList() {
	t := a.T()
	testCases := []string{
		"false",
		"F",
		"False",
	}
	for _, tc := range testCases {
		t.Setenv(allowlist.EnabledEnvVar, tc)
		assert.NilError(t, initAllowlist(nil, nil))
		assert.Equal(t, false, allowlist.Enabled)
	}
}

func (a *AllowListTestSuite) TestRejectBadAllowListFlag() {
	a.T().Setenv(allowlist.EnabledEnvVar, "unclear-boolean-value")
	assert.IsError(a.T(), initAllowlist(nil, nil))
}

func (a *AllowListTestSuite) TestCanEnableAllowList() {
	testCases := []string{
		"true",
		"True",
		"T",
	}
	for _, tc := range testCases {
		a.T().Setenv(allowlist.EnabledEnvVar, tc)
		initializer := mocks.NewInitializer(a.T())
		initializer.On("RegisterRpc", "generate-beta-keys", mock.Anything).
			Return(nil)

		initializer.On("RegisterRpc", "claim-key", mock.Anything).
			Return(nil)

		assert.NilError(a.T(), initAllowlist(nil, initializer))
		assert.Equal(a.T(), true, allowlist.Enabled)
	}
}

func (a *AllowListTestSuite) TestAllowListFailsIfRPCRegistrationFails() {
	a.T().Setenv(allowlist.EnabledEnvVar, "true")
	initializer := mocks.NewInitializer(a.T())
	initializer.On("RegisterRpc", "generate-beta-keys", mock.Anything).
		Return(errors.New("failed to register"))

	assert.IsError(a.T(), initAllowlist(nil, initializer))

	initializer = mocks.NewInitializer(a.T())
	initializer.On("RegisterRpc", "generate-beta-keys", mock.Anything).
		Return(nil)

	initializer.On("RegisterRpc", "claim-key", mock.Anything).
		Return(errors.New("failed to register"))

	assert.IsError(a.T(), initAllowlist(nil, initializer))
}

func (a *AllowListTestSuite) TestCanHandleBetaKeyGenerationFailures() {
	t := a.T()
	ctx := context.Background()
	logger := testutils.MockNoopLogger(t)

	// No user ID is defined
	_, err := handleGenerateKey(ctx, logger, nil, nil, `{"amount":10}`)
	assert.IsError(t, err)

	// Non admin user ID is defined
	ctx = testutils.CtxWithUserID("some-non-admin-user-id")
	_, err = handleGenerateKey(ctx, logger, nil, nil, `{"amount":10}`)
	assert.ErrorContains(t, err, "unauthorized")

	// The GenKeys payload is malformed
	ctx = testutils.CtxWithUserID(signer.AdminAccountID)
	_, err = handleGenerateKey(ctx, logger, nil, nil, `{"bad-payload":{{{{`)
	assert.IsError(t, err)

	nk := mocks.NewNakamaModule(t)
	errMsg := "storage write failure"
	nk.On("StorageWrite", mock.Anything, mock.Anything).
		Return(nil, errors.New(errMsg))
	_, err = handleGenerateKey(ctx, logger, nil, nk, `{"amount":10}`)
	assert.ErrorContains(t, err, errMsg)
}

func (a *AllowListTestSuite) TestCanAddBetaKeys() {
	t := a.T()
	ctx := testutils.CtxWithUserID(signer.AdminAccountID)
	numOfKeysToGenerate := 100
	nk := mocks.NewNakamaModule(t)
	keysInDB := map[string]bool{}
	nk.On("StorageWrite", mock.Anything, mock.MatchedBy(func(writes []*runtime.StorageWrite) bool {
		// Make sure all keys are unique
		seenKeys := map[string]bool{}
		for _, write := range writes {
			assert.Equal(t, false, seenKeys[write.Key])
			seenKeys[write.Key] = true
			keysInDB[write.Key] = true
		}
		assert.Equal(t, len(writes), numOfKeysToGenerate)
		assert.Equal(t, len(seenKeys), numOfKeysToGenerate)
		return true
	})).Return(nil, nil)

	payload := fmt.Sprintf(`{"amount":%d}`, numOfKeysToGenerate)
	resp, err := handleGenerateKey(ctx, testutils.MockNoopLogger(t), nil, nk, payload)
	assert.NilError(t, err)

	// Make sure the beta keys were included in the response
	result := map[string]any{}

	assert.NilError(t, json.Unmarshal([]byte(resp), &result))
	keysAsIface, ok := result["keys"]
	assert.Check(t, ok)
	keys, ok := keysAsIface.([]any)
	assert.Check(t, ok)
	assert.Equal(t, len(keys), numOfKeysToGenerate)
	// Make sure the keys in the response are unique
	seenKeys := map[string]bool{}
	for _, key := range keys {
		assert.Equal(t, false, seenKeys[key.(string)])
		seenKeys[key.(string)] = true
	}
	assert.Equal(t, numOfKeysToGenerate, len(seenKeys))
	// Make sure the returned keys and keys in DB are the same
	assert.DeepEqual(t, seenKeys, keysInDB)
}

func (a *AllowListTestSuite) TestCanClaimBetaKeyWow() {
	t := a.T()

	userID := "foobar"
	ctx := testutils.CtxWithUserID(userID)
	mockNK := mocks.NewNakamaModule(t)

	// First call is to check if the user already has a beta key
	mockNK.On("StorageRead",
		testutils.AnyContext, testutils.MockMatchStoreRead(allowlist.AllowedUsers, userID, signer.AdminAccountID)).
		// No storageObject objects signals that this user has not yet claimed a beta key
		Return(nil, nil).
		Once()

	betaKeyToUse := "abcd-efgh"
	// Make sure the beta keys are converted to upper case.
	validBetaKey := "ABCD-EFGH"

	// This single storage object indicates that the beta key was found (and is valid)
	betaKeyReadReturnVal := []*api.StorageObject{{
		Value: fmt.Sprintf(`{"Key":"%s","UsedBy":"","Used":false}`, validBetaKey),
	}}

	// Second call is to see if the beta key is valid
	mockNK.On("StorageRead", testutils.AnyContext,
		testutils.MockMatchStoreRead(allowlist.KeyCollection, validBetaKey, signer.AdminAccountID)).
		Return(betaKeyReadReturnVal, nil).
		Once()

	// Third call is to update the beta key to mark it as used
	mockNK.On("StorageWrite", testutils.AnyContext,
		testutils.MockMatchStoreWrite(allowlist.KeyCollection, validBetaKey, signer.AdminAccountID)).
		Return(nil, nil).
		Once()

	// Fourth call is to save the newly validated user into the DB
	mockNK.On("StorageWrite", testutils.AnyContext,
		testutils.MockMatchStoreWrite(allowlist.AllowedUsers, "", signer.AdminAccountID)).
		Return(nil, nil).
		Once()

	payload := fmt.Sprintf(`{"key":"%s"}`, betaKeyToUse)

	_, err := handleClaimKey(ctx, testutils.MockNoopLogger(t), nil, mockNK, payload)
	assert.NilError(t, err)
}

func (a *AllowListTestSuite) TestClaimedBetaKeyCannotBeReclaimed() {
	t := a.T()
	userID := "foobar"
	ctx := testutils.CtxWithUserID(userID)
	mockNK := mocks.NewNakamaModule(t)
	mockNK.On("StorageRead", testutils.AnyContext, mock.Anything).
		// No storageObject objects signals that this user has not yet claimed a beta key
		Return(nil, nil).
		Once()

	// This single storage object indicates that the beta key was found (and is valid)
	betaKeyReadReturnVal := []*api.StorageObject{{
		// This beta key is used by someone else
		Value: `{"Key":"xyzzy","UsedBy":"someone-else","Used":true}`,
	}}

	mockNK.On("StorageRead", testutils.AnyContext, mock.Anything).
		Return(betaKeyReadReturnVal, nil).
		Once()

	payload := `{"key": "xyzzy"}`
	_, err := handleClaimKey(ctx, testutils.MockNoopLogger(t), nil, mockNK, payload)
	assert.ErrorContains(t, err, allowlist.ErrBetaKeyAlreadyUsed.Error())
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/heroiclabs/nakama-common/runtime"

	"pkg.world.dev/world-engine/relay/nakama/allowlist"
	"pkg.world.dev/world-engine/relay/nakama/signer"
	"pkg.world.dev/world-engine/relay/nakama/testutils"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"pkg.world.dev/world-engine/assert"

	"pkg.world.dev/world-engine/relay/nakama/mocks"
)

type AllowListTestSuite struct {
	suite.Suite
	originalAllowListEnabled bool
	fakeNK                   runtime.NakamaModule
	logger                   runtime.Logger
	validBetaKeys            []string
}

func TestAllowList(t *testing.T) {
	suite.Run(t, &AllowListTestSuite{})
}

func (a *AllowListTestSuite) SetupTest() {
	a.originalAllowListEnabled = allowlist.Enabled
	allowlist.Enabled = true
	a.T().Setenv(allowlist.EnabledEnvVar, "")
	a.fakeNK = testutils.NewFakeNakamaModule()
	a.logger = testutils.MockNoopLogger(a.T())

	// Create a valid beta key using the allowlist package directly. Below, there is a more comprehensive
	// test to make sure the rpc handler to generate beta keys is working as intended. This is a courtesy for tests
	// that want a valid beta key but aren't testing the beta key creation logic.
	ctx := testutils.CtxWithUserID(signer.AdminAccountID)
	resp, err := allowlist.GenerateBetaKeys(ctx, a.fakeNK, allowlist.GenKeysMsg{Amount: 10})
	assert.NilError(a.T(), err)
	a.validBetaKeys = resp.Keys
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

	fakeNK := testutils.NewFakeNakamaModule().WithError(errors.New(errorMsg))

	_, err := handleClaimKey(ctx, a.logger, nil, fakeNK, `{"Key":"beta-key"}`)
	assert.ErrorContains(t, err, errorMsg)
}

func (a *AllowListTestSuite) TestCannotClaimASecondBetaKey() {
	t := a.T()

	// This initial beta key claim should succeed
	userCtx := testutils.CtxWithUserID("foo")
	payload := fmt.Sprintf(`{"Key":"%s"}`, a.validBetaKeys[0])
	_, err := handleClaimKey(userCtx, a.logger, nil, a.fakeNK, payload)
	assert.NilError(t, err)

	// But trying to claim the same beta key again should fail.
	_, err = handleClaimKey(userCtx, a.logger, nil, a.fakeNK, payload)
	assert.ErrorContains(t, err, allowlist.ErrAlreadyVerified.Error())

	// Trying to claim some other valid beta key should also fail
	payload = fmt.Sprintf(`{"Key":"%s"}`, a.validBetaKeys[1])
	_, err = handleClaimKey(userCtx, a.logger, nil, a.fakeNK, payload)
	assert.ErrorContains(t, err, allowlist.ErrAlreadyVerified.Error())
}

func (a *AllowListTestSuite) TestBadKeyRequestsAreRejected() {
	t := a.T()
	ctx := testutils.CtxWithUserID("foo")

	badBody := `{"key": ""}`
	_, err := handleClaimKey(ctx, a.logger, nil, a.fakeNK, badBody)
	// Nakama returns its own custom runtime error which does NOT implement the Is method, making ErrorIs not helpful.
	assert.ErrorContains(t, err, allowlist.ErrInvalidBetaKey.Error())

	badBody = `{"key": "{{{{`
	_, err = handleClaimKey(ctx, a.logger, nil, a.fakeNK, badBody)
	assert.IsError(t, err)

	validBetaKey := a.validBetaKeys[0]
	// Change the first letter of the beta key
	badBetaKey := "X" + validBetaKey[1:]
	if badBetaKey == validBetaKey {
		// Whoops. I guess the first character was already an X
		badBetaKey = "Y" + validBetaKey[1:]
	}

	badBody = fmt.Sprintf(`{"key": %q}`, badBetaKey)
	_, err = handleClaimKey(ctx, a.logger, nil, a.fakeNK, badBody)
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

	// No user ID is defined
	_, err := handleGenerateKey(ctx, a.logger, nil, nil, `{"amount":10}`)
	assert.IsError(t, err)

	// Non admin user ID is defined
	ctx = testutils.CtxWithUserID("some-non-admin-user-id")
	_, err = handleGenerateKey(ctx, a.logger, nil, nil, `{"amount":10}`)
	assert.ErrorContains(t, err, "unauthorized")

	// The GenKeys payload is malformed
	ctx = testutils.CtxWithUserID(signer.AdminAccountID)
	_, err = handleGenerateKey(ctx, a.logger, nil, nil, `{"bad-payload":{{{{`)
	assert.IsError(t, err)

	errMsg := "storage write failure"
	nk := testutils.NewFakeNakamaModule().WithError(errors.New(errMsg))
	_, err = handleGenerateKey(ctx, a.logger, nil, nk, `{"amount":10}`)
	assert.ErrorContains(t, err, errMsg)
}

func parseGenerateKeysResponse(t *testing.T, resp string) []string {
	result := map[string]any{}

	assert.NilError(t, json.Unmarshal([]byte(resp), &result))
	keysAsIface, ok := result["keys"]
	assert.Check(t, ok)
	keysSlice, ok := keysAsIface.([]any)
	assert.Check(t, ok)

	keys := make([]string, 0, len(keysSlice))
	for _, key := range keysSlice {
		keys = append(keys, key.(string))
	}
	return keys
}

func (a *AllowListTestSuite) TestCanAddAndClaimBetaKeys() {
	t := a.T()
	ctx := testutils.CtxWithUserID(signer.AdminAccountID)
	numOfKeysToGenerate := 100

	payload := fmt.Sprintf(`{"amount":%d}`, numOfKeysToGenerate)
	resp, err := handleGenerateKey(ctx, a.logger, nil, a.fakeNK, payload)
	assert.NilError(t, err)

	// Make sure the beta keys were included in the response
	keys := parseGenerateKeysResponse(t, resp)
	assert.Equal(t, len(keys), numOfKeysToGenerate)
	// Make sure the keys in the response are unique
	seenKeys := map[string]bool{}
	for _, key := range keys {
		assert.Equal(t, false, seenKeys[key])
		seenKeys[key] = true
	}
	// Make sure there are the correct number of keys
	assert.Equal(t, numOfKeysToGenerate, len(seenKeys))

	// Make sure all the generated keys can be claimed
	for _, key := range keys {
		userID := fmt.Sprintf("user-that-wants-%s", key)
		userCtx := testutils.CtxWithUserID(userID)
		payload = fmt.Sprintf(`{"key":%q}`, key)
		resp, err = handleClaimKey(userCtx, a.logger, nil, a.fakeNK, payload)
		assert.NilError(t, err)

		assert.Equal(t, resp, `{"success":true}`)
	}
}

func (a *AllowListTestSuite) TestClaimedBetaKeyCannotBeReclaimed() {
	t := a.T()

	payload := fmt.Sprintf(`{"key": %q}`, a.validBetaKeys[0])
	firstUserCtx := testutils.CtxWithUserID("first")
	_, err := handleClaimKey(firstUserCtx, a.logger, nil, a.fakeNK, payload)
	assert.NilError(t, err)

	secondUserCtx := testutils.CtxWithUserID("second")
	_, err = handleClaimKey(secondUserCtx, a.logger, nil, a.fakeNK, payload)
	assert.ErrorContains(t, err, allowlist.ErrBetaKeyAlreadyUsed.Error())
}

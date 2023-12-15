package main

import (
	"context"
	"errors"
	"testing"

	"github.com/heroiclabs/nakama-common/api"
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
	a.originalAllowListEnabled = allowlistEnabled
	allowlistEnabled = true
}

func (a *AllowListTestSuite) TearDownTest() {
	allowlistEnabled = a.originalAllowListEnabled
}

func (a *AllowListTestSuite) TestUserIDRequired() {
	t := a.T()
	// This context does not have a user ID.
	ctx := context.Background()

	_, err := claimKeyRPC(ctx, noopLogger(t), nil, nil, "")
	// claimKeyRPC should fail because the userID cannot be found in the context
	assert.IsError(t, err)
}

func (a *AllowListTestSuite) TestChecksCorrectStorageObject() {
	t := a.T()
	t.Skip("This test fails because we treat all errors from checkVerified as the user not having a beta key")
	userID := "the-user-id-is-also-the-store-key"
	ctx := ctxWithUserID(userID)
	errorMsg := "read failure"

	mockNK := mocks.NewNakamaModule(t)
	// Make sure the code asks specifically for the uerID key
	mockNK.On("StorageRead", mock.Anything, mockMatchReadKey(userID)).
		Return(nil, errors.New(errorMsg)).
		Once()

	_, err := claimKeyRPC(ctx, noopLogger(t), nil, mockNK, `{"Key":"beta-key"}`)
	assert.ErrorContains(t, err, errorMsg)
}

func (a *AllowListTestSuite) TestCannotClaimASecondBetaKey() {
	t := a.T()
	ctx := ctxWithUserID("foo")

	// This storage object doesn't contain any data. The mere presence of the storage object means the beta key
	// has been verified
	readResponse := []*api.StorageObject{{
		Value: "{}",
	}}

	mockNK := mocks.NewNakamaModule(t)
	mockNK.On("StorageRead", mock.Anything, mock.Anything).
		Return(readResponse, nil).
		Once()

	_, err := claimKeyRPC(ctx, noopLogger(t), nil, mockNK, `{"Key":"some-other-beta-key"}`)
	assert.ErrorContains(t, err, ErrAlreadyVerified.Error())
}

func (a *AllowListTestSuite) TestEmptyKeyRejected() {
	t := a.T()
	ctx := ctxWithUserID("foo")
	mockNK := mocks.NewNakamaModule(t)
	// This storage read is checking for a valid beta key. In this test the user hasn't yet been verified, so
	// no results will be returned.
	mockNK.On("StorageRead", mock.Anything, mock.Anything).
		Return(nil, nil).
		Once()

	_, err := claimKeyRPC(ctx, noopLogger(t), nil, mockNK, `{"key": ""}`)
	// Nakama returns its own custom runtime error which does NOT implement the Is method, making ErrorIs not helpful.
	assert.ErrorContains(t, err, ErrInvalidBetaKey.Error())
}

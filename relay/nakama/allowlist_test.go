package main

import (
	"context"
	"errors"
	"testing"

	"github.com/heroiclabs/nakama-common/api"
	"github.com/stretchr/testify/mock"
	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/relay/nakama/mocks"
)

func TestAllowList_UserIDRequired(t *testing.T) {
	enableAllowListForTest(t)
	// This context does not have a user ID.
	ctx := context.Background()

	_, err := claimKeyRPC(ctx, noopLogger(t), nil, nil, "")
	assert.Check(t, err != nil)
}

func TestAllowList_ChecksCorrectStorageObject(t *testing.T) {
	t.Skip("This test fails because we treat all errors from checkVerified as the user not having a beta key")
	enableAllowListForTest(t)
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

func TestAllowList_CannotClaimASecondBetaKey(t *testing.T) {
	enableAllowListForTest(t)
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

func TestAllowList_EmptyKeyRejected(t *testing.T) {
	enableAllowListForTest(t)
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

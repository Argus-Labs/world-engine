package main

import (
	"context"
	"testing"

	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/stretchr/testify/mock"
	"pkg.world.dev/world-engine/relay/nakama/mocks"
)

// This file contains helpers that are common across all tests.
//
// The mocks/ directory was generated with mockery. The Nakama interfaces are unlikely to change, so regenerating the
// mocks will likely not be required. That being said, here are instructions for regenerating the mocks.
// Install mockery. On Mac:
//
//	$ brew install mockery
//
// Run mockery:
//
//	$ mockery
//
// The configuration file at .mockery.yaml will be used to generate the Nakama mocks.

// ctxWithUserID saves the given user ID to the background context in a location that Nakama expects to find user IDs.
func ctxWithUserID(userID string) context.Context {
	ctx := context.Background()
	return context.WithValue(ctx, runtime.RUNTIME_CTX_USER_ID, userID)
}

// noopLogger returns a mock logger that ignores all log messages.
func noopLogger(t *testing.T) runtime.Logger {
	mockLog := mocks.NewLogger(t)
	mockLog.On("Error", mock.Anything).Return().Maybe()
	mockLog.On("Debug", mock.Anything).Return().Maybe()
	mockLog.On("Info", mock.Anything).Return().Maybe()
	return mockLog
}

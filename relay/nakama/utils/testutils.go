package utils

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

// AnyContext is used to make mock expectations more readable.
// someMock.On("SomeFunction", anyContext...) makes it clear that the first parameter is supposed to be a context.
// context.valueCtx is the type returned by context.Background.
var AnyContext = mock.AnythingOfType("*context.valueCtx")

// CtxWithUserID saves the given user ID to the background context in a location that Nakama expects to find user IDs.
func CtxWithUserID(userID string) context.Context {
	ctx := context.Background()
	//nolint:staticcheck // this is how Nakama reads userIDs from the context.
	return context.WithValue(ctx, runtime.RUNTIME_CTX_USER_ID, userID)
}

func MockMatchStoreWrite(collection, key, userID string) any {
	return mock.MatchedBy(func(writes []*runtime.StorageWrite) bool {
		if len(writes) != 1 {
			return false
		}
		if collection != "" && writes[0].Collection != collection {
			return false
		}
		if key != "" && writes[0].Key != key {
			return false
		}
		if userID != "" && writes[0].UserID != userID {
			return false
		}
		return true
	})
}

// MockMatchStoreRead creates a mock.Matcher (suitable for use as a variadic argument into an "On" method).
// It should be used when your test is expecting a call to "StorageRead", and it verifies that the single
// read request matches the given collection/key/userID. Use the empty string ("") to skip any of the comparisons.
func MockMatchStoreRead(collection, key, userID string) any {
	return mock.MatchedBy(func(reads []*runtime.StorageRead) bool {
		if len(reads) != 1 {
			return false
		}
		if collection != "" && reads[0].Collection != collection {
			return false
		}
		if key != "" && reads[0].Key != key {
			return false
		}
		if userID != "" && reads[0].UserID != userID {
			return false
		}
		return true
	})
}

// NoopLogger returns a mock logger that ignores all log messages.
func NoopLogger(t *testing.T) runtime.Logger {
	mockLog := mocks.NewLogger(t)
	mockLog.On("Error", mock.Anything).Return().Maybe()
	mockLog.On("Debug", mock.Anything).Return().Maybe()
	mockLog.On("Info", mock.Anything).Return().Maybe()
	return mockLog
}

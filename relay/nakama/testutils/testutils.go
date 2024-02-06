package testutils

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"pkg.world.dev/world-engine/relay/nakama/receipt"
	"sync"
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

// MockNoopLogger returns a mock logger that ignores all log messages.
func MockNoopLogger(t *testing.T) runtime.Logger {
	mockLog := mocks.NewLogger(t)
	mockLog.On("Error", mock.Anything).Return().Maybe()
	mockLog.On("Debug", mock.Anything).Return().Maybe()
	mockLog.On("Info", mock.Anything).Return().Maybe()
	return mockLog
}

func MockMatchReadKey(key string) interface{} {
	return mock.MatchedBy(func(storeRead []*runtime.StorageRead) bool {
		if len(storeRead) != 1 {
			return false
		}
		return storeRead[0].Key == key
	})
}

func MockMatchWriteKey(key string) interface{} {
	return mock.MatchedBy(func(storeWrite []*runtime.StorageWrite) bool {
		if len(storeWrite) != 1 {
			return false
		}
		return storeWrite[0].Key == key
	})
}

type NoOpLogger struct {
	mu     sync.Mutex // Ensure that the logger is goroutine-safe
	Errors []string   // Store error messages
}

func (l *NoOpLogger) Debug(string, ...interface{}) {}
func (l *NoOpLogger) Info(string, ...interface{})  {}
func (l *NoOpLogger) Warn(string, ...interface{})  {}

// Capture error messages
//
//nolint:goprintffuncname // [not important]
func (l *NoOpLogger) Error(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Errors = append(l.Errors, fmt.Sprintf(format, args...))
}

// GetErrors A method to retrieve captured errors for assertions
func (l *NoOpLogger) GetErrors() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.Errors
}

func (l *NoOpLogger) WithField(string, interface{}) runtime.Logger     { return nil }
func (l *NoOpLogger) WithFields(map[string]interface{}) runtime.Logger { return nil }
func (l *NoOpLogger) Fields() map[string]interface{}                   { return make(map[string]interface{}) }

// Ensure that NoOpLogger implements runtime.Logger (this will produce a compile-time error if it doesn't)
var _ runtime.Logger = (*NoOpLogger)(nil)

// Setup mock server in a more streamlined manner, directly using the URL provided by httptest.
func SetupMockServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := receipt.TransactionReceiptsReply{
			StartTick: 0,
			EndTick:   1,
			Receipts:  []*receipt.Receipt{{TxHash: "hash1", Result: map[string]any{"status": "success"}, Errors: []string{}}},
		}
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(response)
		if err != nil {
			return
		}
	}))
}

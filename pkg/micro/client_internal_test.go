package micro

import (
	"errors"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

// NATS invokes ErrHandler with a nil subscription for connection-level async
// errors, so this test guards against regressing to a nil-pointer panic.
func TestClient_HandleErrorWithNilSubscription(t *testing.T) {
	t.Parallel()

	client := &Client{log: zerolog.Nop()}

	assert.NotPanics(t, func() {
		client.handleError(nil, nil, errors.New("transient connection error"))
	})
}

package testutils

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/telemetry"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

var (
	// ErrServerStartTimeout indicates the NATS server failed to start within the timeout period.
	ErrServerStartTimeout = eris.New("NATS server failed to start within timeout")
	// ErrPublishTimeout indicates a publish operation timed out.
	ErrPublishTimeout = eris.New("publish operation timed out")
	// ErrSubscriberTimeout indicates waiting for subscribers timed out.
	ErrSubscriberTimeout = eris.New("timeout waiting for subscribers")
)

// NATS represents a test NATS server with its client.
type NATS struct {
	t      *testing.T
	log    zerolog.Logger
	Server *server.Server
	Client *nats.Conn
}

// NewNATS creates a new NATS test server with a connected client.
// It automatically registers cleanup with t.Cleanup().
func NewNATS(t *testing.T) *NATS {
	t.Helper()
	log := telemetry.GetGlobalLogger("router.testutils.nats")

	// Generate a unique directory path for JetStream storage
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("nats-test-%s", t.Name()))

	opts := &server.Options{
		Host:       "127.0.0.1",
		Port:       -1, // Will pick a random available port
		NoLog:      true,
		NoSigs:     true,
		MaxPayload: 2 * 1024 * 1024, // 2MB
		JetStream:  true,
		StoreDir:   tempDir, // Use unique directory for JetStream storage
	}

	ns, err := server.NewServer(opts)
	require.NoError(t, err, "failed to create NATS server")
	require.NotNil(t, ns)

	go ns.Start()

	// Wait for server to be ready
	if !ns.ReadyForConnections(4 * time.Second) {
		log.Error().Msg("NATS server failed to start")
		t.Fatal(ErrServerStartTimeout)
	}

	// Connect a client
	nc, err := nats.Connect(ns.ClientURL())
	require.NoError(t, err, "failed to connect NATS client")
	require.NotNil(t, nc)

	log.Info().Str("url", ns.ClientURL()).Str("store_dir", tempDir).Msg("Test NATS server started")

	nt := &NATS{
		t:      t,
		Server: ns,
		Client: nc,
		log:    log,
	}

	// Register cleanup to ensure resources are freed
	t.Cleanup(func() {
		nt.Shutdown()
		// Clean up the temporary directory
		if err := os.RemoveAll(tempDir); err != nil {
			log.Warn().Err(err).Str("dir", tempDir).Msg("Failed to remove temp directory")
		}
	})

	return nt
}

// Shutdown gracefully shuts down the NATS test server and client.
func (n *NATS) Shutdown() {
	if n.Client != nil {
		n.Client.Close()
		n.log.Info().Msg("NATS client closed")
	}
	if n.Server != nil {
		n.Server.Shutdown()
		n.log.Info().Msg("NATS server shutdown")
	}
}

// WaitForSubscribers waits for the specified number of subscribers on a subject.
// Returns true if the expected number of subscribers is reached within the timeout.
func (n *NATS) WaitForSubscribers(numSubscribers int, timeout time.Duration) bool {
	n.t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	n.log.Debug().
		Int("expected_subscribers", numSubscribers).
		Str("timeout", timeout.String()).
		Msg("Waiting for subscribers")

	for {
		select {
		case <-timer.C:
			n.log.Warn().
				Int("expected_subscribers", numSubscribers).
				Int("current_subscribers", int(n.Server.NumSubscriptions())).
				Msg("Timeout waiting for subscribers")
			return false
		case <-ticker.C:
			subs := int(n.Server.NumSubscriptions())
			if subs >= numSubscribers {
				n.log.Info().
					Int("subscribers", subs).
					Msg("Required number of subscribers reached")
				return true
			}
		}
	}
}

// PublishWithReply publishes a message to a subject and waits for a reply.
// Returns the reply data and any error that occurred.
func (n *NATS) PublishWithReply(subject string, data []byte, timeout time.Duration) ([]byte, error) {
	n.t.Helper()
	n.log.Debug().
		Str("subject", subject).
		Int("data_size", len(data)).
		Str("timeout", timeout.String()).
		Msg("Publishing message with reply")

	msg, err := n.Client.Request(subject, data, timeout)
	if err != nil {
		return nil, eris.Wrapf(err, "failed to publish message to subject %s", subject)
	}

	n.log.Debug().
		Str("subject", subject).
		Int("reply_size", len(msg.Data)).
		Msg("Received reply")
	return msg.Data, nil
}

// SubscribeSync creates a synchronous subscription and returns it.
// The subscription is automatically cleaned up when the test ends.
func (n *NATS) SubscribeSync(subject string) *nats.Subscription {
	n.t.Helper()
	n.log.Debug().Str("subject", subject).Msg("Creating synchronous subscription")

	sub, err := n.Client.Subscribe(subject, func(msg *nats.Msg) {
		n.log.Debug().
			Str("subject", subject).
			Int("msg_size", len(msg.Data)).
			Msg("Message received")
	})
	require.NoError(n.t, err, "failed to create subscription")

	// Register cleanup for the subscription
	n.t.Cleanup(func() {
		if err := sub.Unsubscribe(); err != nil {
			n.log.Error().Err(err).Str("subject", subject).Msg("Failed to unsubscribe")
		}
	})

	return sub
}

// WaitForMessage waits for a message on the given subscription.
// Returns the received message or fails the test if timeout is reached.
func (n *NATS) WaitForMessage(sub *nats.Subscription, timeout time.Duration) *nats.Msg {
	n.t.Helper()
	n.log.Debug().
		Str("subject", sub.Subject).
		Str("timeout", timeout.String()).
		Msg("Waiting for message")

	msg, err := sub.NextMsg(timeout)
	require.NoError(n.t, err, "failed to receive message")

	n.log.Debug().
		Str("subject", sub.Subject).
		Int("msg_size", len(msg.Data)).
		Msg("Message received")
	return msg
}

// QueueSubscribeSync creates a synchronous queue subscription and returns it.
// The subscription is automatically cleaned up when the test ends.
func (n *NATS) QueueSubscribeSync(subject, queue string) *nats.Subscription {
	n.t.Helper()
	n.log.Debug().
		Str("subject", subject).
		Str("queue", queue).
		Msg("Creating queue subscription")

	sub, err := n.Client.QueueSubscribe(subject, queue, func(msg *nats.Msg) {
		n.log.Debug().
			Str("subject", subject).
			Int("msg_size", len(msg.Data)).
			Msg("Message received")
	})
	require.NoError(n.t, err, "failed to create queue subscription")

	// Register cleanup for the subscription
	n.t.Cleanup(func() {
		if err := sub.Unsubscribe(); err != nil {
			n.log.Error().Err(err).Str("subject", subject).Msg("Failed to unsubscribe")
		}
	})

	return sub
}

// Publish publishes a message to a subject.
// Returns an error if the publish operation fails.
func (n *NATS) Publish(subject string, data []byte) error {
	n.t.Helper()
	n.log.Info().
		Str("subject", subject).
		Int("data_size", len(data)).
		Msg("Publishing message")

	if err := n.Client.Publish(subject, data); err != nil {
		return eris.Wrapf(err, "failed to publish message to subject %s", subject)
	}
	return nil
}

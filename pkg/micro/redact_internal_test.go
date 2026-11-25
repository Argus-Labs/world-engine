package micro

import (
	"bytes"
	"net"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/goccy/go-json"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats-server/v2/test"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// All credentials below are synthetic. Assertions report only booleans so a failure never prints
// a credential or the log text that carries one.
const (
	fixtureUser     = "fixture-user"
	fixturePassword = "fixture-password"
	fixtureToken    = "fixture-token"
)

func TestRedactNATSURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"no credentials", "nats://127.0.0.1:4222", "nats://127.0.0.1:4222"},
		{
			"user and password",
			"nats://" + fixtureUser + ":" + fixturePassword + "@127.0.0.1:4222",
			"nats://127.0.0.1:4222",
		},
		{"token only", "nats://" + fixtureToken + "@127.0.0.1:4222", "nats://127.0.0.1:4222"},
		{"password with at sign", "nats://" + fixtureUser + ":" + fixturePassword + "@x@nats:4222", "nats://nats:4222"},
		{"no scheme", fixtureUser + ":" + fixturePassword + "@nats:4222", "nats://nats:4222"},
		{"tls scheme", "tls://" + fixtureToken + "@nats:4222", "tls://nats:4222"},
		{"websocket path dropped", "ws://" + fixtureToken + "@nats:8080/" + fixturePassword, "ws://nats:8080"},
		{"query dropped", "nats://nats:4222?" + fixtureToken, "nats://nats:4222"},
		{
			"server list",
			"nats://" + fixtureToken + "@a:4222, nats://" + fixtureUser + ":" + fixturePassword + "@b:4222,,",
			"nats://a:4222,nats://b:4222",
		},
		{"unparseable", "nats://" + fixtureUser + ":" + fixturePassword + "@127.0.0.1:bad port", invalidNATSURL},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assertLoggedURL(t, tt.want, redactNATSURL(tt.raw))
		})
	}
}

func TestNewClient_InvalidURLErrorOmitsCredentials(t *testing.T) {
	t.Parallel()

	_, err := NewClient(
		WithLogger(zerolog.Nop()),
		WithNATSConfig(NATSConfig{
			Name: "redact-invalid",
			URL:  "nats://" + fixtureUser + ":" + fixturePassword + "@127.0.0.1:bad port",
		}),
	)
	require.Error(t, err)
	assert.False(t, containsCredential(err.Error()), "connect error contains a credential")
}

// TestClient_ConnectionLogsOmitCredentials connects to an authenticated server, then restarts it,
// and checks that the connect, disconnect, and reconnect diagnostics name the server without its
// credentials.
func TestClient_ConnectionLogsOmitCredentials(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"no credentials", "password", "token"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			opts := &server.Options{
				Host:     "127.0.0.1",
				Port:     -1,
				NoLog:    true,
				NoSigs:   true,
				StoreDir: t.TempDir(),
			}

			switch mode {
			case "password":
				opts.Username, opts.Password = fixtureUser, fixturePassword
			case "token":
				opts.Authorization = fixtureToken
			}

			srv := test.RunServer(opts)

			t.Cleanup(func() { srv.Shutdown() })

			address, err := url.Parse(srv.ClientURL())
			require.NoError(t, err)

			wantURL := address.String()

			switch mode {
			case "password":
				address.User = url.UserPassword(fixtureUser, fixturePassword)
			case "token":
				address.User = url.User(fixtureToken)
			}

			logs := &syncBuffer{}
			client, err := NewClient(
				WithLogger(zerolog.New(logs)),
				WithNATSConfig(NATSConfig{Name: "redact-" + mode, URL: address.String()}),
			)
			require.NoError(t, err)
			t.Cleanup(client.Close)

			port := srv.Addr().(*net.TCPAddr).Port
			srv.Shutdown()
			srv.WaitForShutdown()
			requireLogged(t, logs, "Disconnected from NATS")

			opts.Port = port
			srv = test.RunServer(opts)

			requireLogged(t, logs, "Reconnected to NATS")

			out := logs.String()
			assert.False(t, containsCredential(out), "log output contains a credential")
			assertLoggedURL(t, wantURL, loggedField(out, "Connected to NATS server", "url"))
			assertLoggedURL(t, wantURL, loggedField(out, "Disconnected from NATS", "nats_url"))
			assertLoggedURL(t, wantURL, loggedField(out, "Reconnected to NATS", "nats_url"))
		})
	}
}

func containsCredential(s string) bool {
	return strings.Contains(s, fixtureUser) ||
		strings.Contains(s, fixturePassword) ||
		strings.Contains(s, fixtureToken)
}

// assertLoggedURL compares a logged URL, printing it only when it holds no credential.
func assertLoggedURL(t *testing.T, want, got string) {
	t.Helper()

	if containsCredential(got) {
		t.Error("logged URL contains a credential")
		return
	}

	assert.Equal(t, want, got)
}

func requireLogged(t *testing.T, logs *syncBuffer, message string) {
	t.Helper()
	require.Eventually(t, func() bool {
		return strings.Contains(logs.String(), message)
	}, 10*time.Second, 10*time.Millisecond, "no %q log line", message)
}

// loggedField returns field from the first JSON log line whose message starts with prefix.
func loggedField(logs, prefix, field string) string {
	for line := range strings.Lines(logs) {
		var record map[string]any
		if json.Unmarshal([]byte(line), &record) != nil {
			continue
		}

		if msg, _ := record["message"].(string); strings.HasPrefix(msg, prefix) {
			value, _ := record[field].(string)
			return value
		}
	}

	return ""
}

// syncBuffer is a log sink safe for the NATS callback goroutines.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

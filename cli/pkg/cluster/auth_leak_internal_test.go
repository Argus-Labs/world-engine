package cluster

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/rotisserie/eris"
	"github.com/stretchr/testify/require"

	operatorv1 "github.com/argus-labs/world-engine/cli/proto/gen/go/cardinal/operator/v1"
	operatorv1connect "github.com/argus-labs/world-engine/cli/proto/gen/go/cardinal/operator/v1/operatorv1connect"
)

// errTokenSource simulates the expired/absent Argus login ticket state that
// TokenSource.Token reports — the path `world logs --env` hits when the user is
// signed out. The error is a real auth error (not [io.EOF]): [io.EOF] would trip
// connect's `errors.Is(err, [io.EOF])` short-circuit in CallServerStream and mask
// the leak behind a nil caller error, whereas a real token-source error keeps
// the caller-facing CodeUnauthenticated intact so the leak is observable.
type errTokenSource struct{}

func (errTokenSource) Token(context.Context) (string, error) {
	return "", eris.New("no argus login ticket")
}

// okTokenSource hands out a fixed bearer token, simulating a valid Argus login
// ticket — the path `world logs --env` hits when the user is signed in.
type okTokenSource struct{ token string }

func (s okTokenSource) Token(context.Context) (string, error) { return s.token, nil }

// TestStreamPodLogs_AuthFailure_LeaksNoRequest is the regression test for the
// auth-failure path of bearerInterceptor.WrapStreamingClient. When the token
// source fails, the returned failedConn must short-circuit every method
// connect's CallServerStream cleanup calls (CloseRequest/CloseResponse), so the
// underlying live duplexHTTPCall is never flushed. Otherwise connect fires an
// unauthenticated, empty-body POST to the operator during cleanup before the
// CodeUnauthenticated error is returned to the caller.
//
// The interceptor's whole purpose (PR #773) is to stop unauthenticated server
// streams from going out; the credential-failure path must not be the one path
// that leaks.
func TestStreamPodLogs_AuthFailure_LeaksNoRequest(t *testing.T) {
	var requests atomic.Int32
	var lastAuth string
	var lastBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		lastAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		lastBody = b
		_ = r.Body.Close()
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := operatorv1connect.NewOperatorServiceClient(
		http.DefaultClient,
		srv.URL,
		connect.WithInterceptors(bearerInterceptor{tokens: errTokenSource{}}),
	)

	_, err := client.StreamPodLogs(
		context.Background(),
		connect.NewRequest(&operatorv1.StreamPodLogsRequest{PodName: "pod-123"}),
	)
	require.Error(t, err, "auth failure must surface to the caller, not silently succeed")
	require.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err),
		"the caller-facing error must stay CodeUnauthenticated so the CLI tells the user to sign in")

	// The leaked POST is fired asynchronously by duplexHTTPCall.CloseWrite ->
	// `go makeRequest()`, so poll briefly to give the buggy path a chance to
	// betray itself rather than racing the assertion. On the fixed path nothing
	// is ever sent and this loop simply runs out the clock at zero.
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if got := requests.Load(); got != 0 {
			t.Fatalf("BUG: %d unauthenticated request(s) reached the operator when auth failed "+
				"(authorization=%q, body=%q) — failedConn must short-circuit CloseRequest/CloseResponse",
				got, lastAuth, bytes.TrimSpace(lastBody))
		}
		time.Sleep(5 * time.Millisecond)
	}
	require.Equal(t, int32(0), requests.Load(),
		"no request should be sent when auth setup fails")
}

// TestStreamPodLogs_AuthSuccess_SendsSingleAuthenticatedRequest pins the happy
// path so the leak fix can't over-correct and break the legitimate remote-logs
// flow. With a valid token the interceptor must attach the bearer header to
// the one POST connect makes for the server stream, the request message must
// arrive intact, the streamed payload must be delivered end-to-end, and exactly
// one request must reach the operator.
//
// This is the only wire-level coverage of bearerInterceptor: every other
// pkg/cluster test uses a localhost endpoint, which operatorClient skips, so
// the interceptor (and its failedConn) would otherwise be unexercised.
func TestStreamPodLogs_AuthSuccess_SendsSingleAuthenticatedRequest(t *testing.T) {
	const token = "hunter2"
	var requests atomic.Int32
	var lastAuth string

	op := &fakeOperator{
		scripts: []func(context.Context, *connect.ServerStream[operatorv1.StreamPodLogsResponse]) error{
			sendN(1), // one log line, then a clean EOF (a finite previous-container dump)
		},
	}
	procedurePath, handler := operatorv1connect.NewOperatorServiceHandler(op)
	mux := http.NewServeMux()
	mux.Handle(procedurePath, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		lastAuth = r.Header.Get("Authorization")
		handler.ServeHTTP(w, r)
	}))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := operatorv1connect.NewOperatorServiceClient(
		http.DefaultClient,
		srv.URL,
		connect.WithInterceptors(bearerInterceptor{tokens: okTokenSource{token: token}}),
	)

	stream, err := client.StreamPodLogs(
		context.Background(),
		connect.NewRequest(&operatorv1.StreamPodLogsRequest{PodName: "gameplay-abc"}),
	)
	require.NoError(t, err, "a valid token must open the stream")

	var lines []string
	for stream.Receive() {
		for _, line := range stream.Msg().GetLines() {
			lines = append(lines, line.GetLine())
		}
	}
	require.NoError(t, stream.Err(), "a clean EOF after the finite dump is not an error")
	require.NoError(t, stream.Close())

	require.Equal(t, int32(1), requests.Load(),
		"exactly one authenticated POST must reach the operator for a server-stream RPC")
	require.Equal(t, "Bearer "+token, lastAuth,
		"the interceptor must attach the bearer token before the request goes out")
	require.Equal(t, "gameplay-abc", op.snapshot()[0].PodName,
		"the request message must reach the operator intact")
	require.Len(t, lines, 1, "the one emitted log line must be delivered to the client")
	require.Equal(t, "prev", lines[0], "the streamed payload survives end-to-end")
}

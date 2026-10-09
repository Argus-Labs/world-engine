package cluster

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/rotisserie/eris"
	"github.com/stretchr/testify/require"

	operatorv1 "github.com/argus-labs/world-engine/cli/proto/gen/go/cardinal/operator/v1"
	operatorv1connect "github.com/argus-labs/world-engine/cli/proto/gen/go/cardinal/operator/v1/operatorv1connect"
)

// A proxy that cuts an idle stream reports INTERNAL, not a clean EOF. Treating
// that as terminal is what made `world logs --project` exit silently after
// ~100s behind Cloudflare.
func TestIsTransient(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		want bool
	}{
		{"cloudflare idle reset", connect.NewError(connect.CodeInternal, eris.New("stream error: INTERNAL_ERROR")), true},
		{"unavailable", connect.NewError(connect.CodeUnavailable, eris.New("no route")), true},
		{"deadline exceeded", connect.NewError(connect.CodeDeadlineExceeded, eris.New("timeout")), true},
		{"wrapped internal", eris.Wrap(connect.NewError(connect.CodeInternal, eris.New("x")), "open"), true},
		{"unauthenticated is terminal", connect.NewError(connect.CodeUnauthenticated, eris.New("no token")), false},
		{"not found is handled elsewhere", connect.NewError(connect.CodeNotFound, eris.New("gone")), false},
		{"plain error", eris.New("boom"), false},
		{"nil", nil, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isTransient(tt.err))
		})
	}
}

// isNotFound must stay distinct from isTransient: it drives pod re-resolution,
// not a re-subscribe to the same pod.
func TestIsNotFoundIsNotTransient(t *testing.T) {
	err := connect.NewError(connect.CodeNotFound, eris.New("pod gone"))
	require.True(t, isNotFound(err))
	require.False(t, isTransient(err))
}

// StreamShardLogs must close out on every path. When it returned an error
// without closing, `world logs --project` hung forever on an unreachable or
// unauthorized operator instead of reporting it.
func TestStreamShardLogsClosesOutOnError(t *testing.T) {
	c := NewClient(Config{OperatorEndpoint: "http://127.0.0.1:1"}) // nothing listening
	out := make(chan LogLine, 1)

	err := c.StreamShardLogs(context.Background(), LogsOpts{}, out)
	require.Error(t, err, "unreachable operator should error")

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range out {
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("out was not closed; a caller ranging over it would hang")
	}
}

// fakeOperator implements OperatorServiceHandler for the log tests. It records
// the (Previous, Follow, PodName) of every StreamPodLogs call and serves a
// scripted sequence of behaviors so a test can drive tailPod's re-subscribe
// loop without a real cluster.
type fakeOperator struct {
	operatorv1connect.UnimplementedOperatorServiceHandler

	mu       sync.Mutex
	recorded []logStreamCall
	// scripts[i] governs the (i+1)-th StreamPodLogs call; once exhausted the
	// handler blocks on its request context until the test cancels the client.
	scripts []func(context.Context, *connect.ServerStream[operatorv1.StreamPodLogsResponse]) error
	// pod is the pod name Status reports for the "gameplay" instance. Empty
	// defaults to "gameplay-abc" so legacy tests are unchanged; a test can flip
	// it mid-run to simulate a rolling deploy for pod-churn coverage.
	pod string
	// pools, when non-nil, overrides Status to return exactly these pools
	// (and skips the default single-instance "gameplay" pool + per-call pod
	// swapping). Lets a test express a scaled-to-zero env, a pool_size>1
	// pool, or the unknown-shard case without a new handler type.
	pools []*operatorv1.ShardPoolStatus
}

type logStreamCall struct {
	Previous bool
	Follow   bool
	PodName  string
}

func (f *fakeOperator) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.recorded)
}

func (f *fakeOperator) snapshot() []logStreamCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]logStreamCall, len(f.recorded))
	copy(out, f.recorded)
	return out
}

func (f *fakeOperator) setPod(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pod = name
}

// runRemoteLogsConsumer mirrors apps/world-cli/commands/root/logs_remote.go's
// printer loop (errgroup + fmt %-18s %s\n) against the same StreamShardLogs
// entry the real CLI uses. Returns a content getter (safe to call any time —
// the shared buffer is mutex-guarded so it can be read while the consumer is
// still draining a live tail), a done channel that closes when out is drained
// — i.e. when the command would have exited — and the stream's terminal error,
// which is what decides the command's exit status.
func runRemoteLogsConsumer(
	ctx context.Context,
	t *testing.T,
	c *Client,
	opts LogsOpts,
) (func() string, <-chan struct{}, <-chan error) {
	t.Helper()
	out := make(chan LogLine, 256)
	// StreamShardLogs blocks until every pod's stream has ended, so it runs in
	// its own goroutine with out drained concurrently — the same shape as
	// logs_remote.go's errgroup.
	errCh := make(chan error, 1)
	go func() { errCh <- c.StreamShardLogs(ctx, opts, out) }()
	var mu sync.Mutex
	var buf strings.Builder
	done := make(chan struct{})
	go func() {
		defer close(done)
		for line := range out {
			mu.Lock()
			fmt.Fprintf(&buf, "%-18s %s\n", line.InstanceName, line.Line)
			mu.Unlock()
		}
	}()
	return func() string {
		mu.Lock()
		defer mu.Unlock()
		return buf.String()
	}, done, errCh
}

func (f *fakeOperator) Status(
	_ context.Context,
	_ *connect.Request[operatorv1.StatusRequest],
) (*connect.Response[operatorv1.StatusResponse], error) {
	f.mu.Lock()
	pod := f.pod
	pools := f.pools
	f.mu.Unlock()
	if pools != nil {
		return connect.NewResponse(&operatorv1.StatusResponse{Pools: pools}), nil
	}
	if pod == "" {
		pod = "gameplay-abc"
	}
	return connect.NewResponse(&operatorv1.StatusResponse{
		Pools: []*operatorv1.ShardPoolStatus{{
			ShardId: "gameplay",
			Instances: []*operatorv1.ShardInstanceStatus{{
				Name:    "gameplay",
				PodName: pod,
			}},
		}},
	}), nil
}

func (f *fakeOperator) StreamPodLogs(
	ctx context.Context,
	req *connect.Request[operatorv1.StreamPodLogsRequest],
	stream *connect.ServerStream[operatorv1.StreamPodLogsResponse],
) error {
	f.mu.Lock()
	f.recorded = append(f.recorded, logStreamCall{
		Previous: req.Msg.GetPrevious(),
		Follow:   req.Msg.GetFollow(),
		PodName:  req.Msg.GetPodName(),
	})
	idx := len(f.recorded) - 1
	f.mu.Unlock()

	script := blockUntilCanceled
	if idx < len(f.scripts) {
		script = f.scripts[idx]
	}
	return script(ctx, stream)
}

// sendN returns a script that emits n lines with strictly increasing RFC3339
// timestamps and then returns nil — a clean EOF, exactly what the operator
// produces for a previous-container dump (server forces follow off).
func sendN(n int) func(context.Context, *connect.ServerStream[operatorv1.StreamPodLogsResponse]) error {
	return func(_ context.Context, stream *connect.ServerStream[operatorv1.StreamPodLogsResponse]) error {
		for i := range n {
			ts := time.Now().Add(time.Duration(i) * time.Millisecond).UTC().Format(time.RFC3339Nano)
			if err := stream.Send(&operatorv1.StreamPodLogsResponse{
				Lines: []*operatorv1.PodLogLine{{Timestamp: ts, Line: "prev"}},
			}); err != nil {
				return err
			}
		}
		return nil
	}
}

// blockUntilCanceled holds the stream open until the request context is done.
// Stands in for a real operator's live follow of the current container.
func blockUntilCanceled(ctx context.Context, _ *connect.ServerStream[operatorv1.StreamPodLogsResponse]) error {
	<-ctx.Done()
	return ctx.Err()
}

// TestTailPodResubscribesPreviousAsLiveFollow is a regression test: a
// previous-container query (Previous=true) closes cleanly once the finite
// buffer is drained (operator forces follow off; kubelet stream ends).
// tailPod must treat that clean EOF as terminal — the user asked for the
// previous container's logs and nothing more — NOT re-subscribe with Previous
// cleared and Follow=true, which silently opens a live follow of the current
// container.
//
// Against buggy tailPod this fails: it observes a 2nd StreamPodLogs call with
// Previous=false / Follow=true. Against the fix it passes: exactly one call
// and `out` closes promptly.
func TestTailPodResubscribesPreviousAsLiveFollow(t *testing.T) {
	op := &fakeOperator{
		scripts: []func(context.Context, *connect.ServerStream[operatorv1.StreamPodLogsResponse]) error{
			sendN(2),           // call 1: previous-container dump, clean EOF
			blockUntilCanceled, // call 2: stands in for a live follow of the current container
		},
	}
	mux := http.NewServeMux()
	path, handler := operatorv1connect.NewOperatorServiceHandler(op)
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	c := NewClient(Config{OperatorEndpoint: srv.URL})
	out := make(chan LogLine, 16)
	errCh := make(chan error, 1)
	go func() { errCh <- c.StreamShardLogs(ctx, LogsOpts{Previous: true}, out) }()

	closed := make(chan struct{})
	go func() {
		for range out {
		}
		close(closed)
	}()
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if op.callCount() >= 2 {
			t.Fatalf("BUG: tailPod re-subscribed after a Previous=true clean EOF; "+
				"2nd call has Previous=%v Follow=%v (want no 2nd call)",
				op.snapshot()[1].Previous, op.snapshot()[1].Follow)
		}
		select {
		case <-closed:
			require.Equal(t, 1, op.callCount(),
				"Previous=true clean EOF must terminate tailPod after exactly one StreamPodLogs call")
			require.True(t, op.snapshot()[0].Previous, "the one call must be the user's --previous request")
			require.NoError(t, <-errCh, "a clean previous-container dump is a success, not a failure")
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
	t.Fatal("timed out: out never closed and no re-subscribe observed")
}

// TestTailPodTransientCutKeepsPrevious covers the proxy-RST-mid-replay path
// that a naive fix (return on `wasPrevious && err == nil` only) would miss. A
// previous-container dump can be cut by a transient (Cloudflare idle RST
// arrives as CodeInternal, classified transient by isTransient) before the
// finite buffer finishes draining. tailPod must re-subscribe with Previous
// PRESERVED (to finish the dump) — NOT with Previous cleared, which would
// silently switch to a live follow of the current container.
func TestTailPodTransientCutKeepsPrevious(t *testing.T) {
	transient := connect.NewError(connect.CodeInternal, eris.New("stream error: INTERNAL_ERROR"))
	require.True(t, isTransient(transient), "precondition: CodeInternal is transient")
	op := &fakeOperator{
		scripts: []func(context.Context, *connect.ServerStream[operatorv1.StreamPodLogsResponse]) error{
			func(_ context.Context, stream *connect.ServerStream[operatorv1.StreamPodLogsResponse]) error {
				_ = stream.Send(&operatorv1.StreamPodLogsResponse{
					Lines: []*operatorv1.PodLogLine{{
						Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Line: "prev",
					}},
				})
				return transient // proxy RST mid-replay
			},
			blockUntilCanceled, // 2nd call: must re-request Previous, not a live follow
		},
	}
	mux := http.NewServeMux()
	path, handler := operatorv1connect.NewOperatorServiceHandler(op)
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	c := NewClient(Config{OperatorEndpoint: srv.URL})
	out := make(chan LogLine, 16)
	go func() { _ = c.StreamShardLogs(ctx, LogsOpts{Previous: true}, out) }()

	deadline := time.Now().Add(8 * time.Second)
	for op.callCount() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("tailPod did not re-subscribe after a transient cut; calls=%d", op.callCount())
		}
		time.Sleep(50 * time.Millisecond)
	}
	calls := op.snapshot()
	require.True(t, calls[0].Previous, "first call is the previous-container query")
	require.True(t, calls[1].Previous,
		"transient cut mid-previous-dump must re-request Previous (preserve), not switch to a live follow")
	require.True(t, calls[1].Follow, "Follow stays true (streamOne hard-codes it)")
}

// TestTailPodNoPreviousContainerTerminates covers the no-previous-container
// edge case: a --previous query against a pod that never crashed. kubelet
// rejects the stream before any line flows — surfacing as CodeNotFound on
// some k8s versions and CodeInternal (transient!) on others — so tailPod must
// terminate rather than spin forever re-requesting a stream that can never
// open. A naive fix that returns only on `wasPrevious && err == nil` still
// re-subscribes here (the error is transient or NotFound) and hangs.
func TestTailPodNoPreviousContainerTerminates(t *testing.T) {
	for _, tt := range []struct {
		name   string
		script func(context.Context, *connect.ServerStream[operatorv1.StreamPodLogsResponse]) error
	}{
		{
			name: "not found at open",
			script: func(_ context.Context, _ *connect.ServerStream[operatorv1.StreamPodLogsResponse]) error {
				return connect.NewError(connect.CodeNotFound, eris.New("previous terminated container not found"))
			},
		},
		{
			name: "internal at open (transient, zero lines)",
			script: func(_ context.Context, _ *connect.ServerStream[operatorv1.StreamPodLogsResponse]) error {
				return connect.NewError(connect.CodeInternal, eris.New("previous terminated container not found"))
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			op := &fakeOperator{
				scripts: []func(context.Context, *connect.ServerStream[operatorv1.StreamPodLogsResponse]) error{
					tt.script,
					blockUntilCanceled, // must NOT be reached: a --previous open error terminates
				},
			}
			mux := http.NewServeMux()
			path, handler := operatorv1connect.NewOperatorServiceHandler(op)
			mux.Handle(path, handler)
			srv := httptest.NewServer(mux)
			defer srv.Close()

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			c := NewClient(Config{OperatorEndpoint: srv.URL})
			out := make(chan LogLine, 16)
			errCh := make(chan error, 1)
			go func() { errCh <- c.StreamShardLogs(ctx, LogsOpts{Previous: true}, out) }()

			closed := make(chan struct{})
			go func() {
				for range out {
				}
				close(closed)
			}()
			deadline := time.Now().Add(6 * time.Second)
			for time.Now().Before(deadline) {
				if op.callCount() >= 2 {
					t.Fatalf("BUG: --previous re-subscribed after a zero-line open error")
				}
				select {
				case <-closed:
					require.Equal(t, 1, op.callCount(),
						"--previous open error must terminate after one StreamPodLogs call")
					require.True(t, op.snapshot()[0].Previous, "the one call must be the user's --previous request")
					// Terminating is not enough: without a reported error the command
					// prints "Tailing dev" and exits 0, which is exactly what an empty
					// previous buffer looks like. The user cannot tell "this pod never
					// crashed" from "the request failed" unless we say which it was.
					require.Error(t, <-errCh,
						"a --previous stream that never opened must be reported, not exit 0 with no output")
					return
				case <-time.After(50 * time.Millisecond):
				}
			}
			t.Fatal("timed out: --previous open error did not terminate and no re-subscribe observed")
		})
	}
}

// newClusterTestServer wires op behind a real Connect HTTP handler over
// httptest, exactly the transport `world logs --env` uses against a deployed
// operator. Returns the server (close with defer srv.Close()) whose URL is
// passed to NewClient.
func newClusterTestServer(t *testing.T, op *fakeOperator) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := operatorv1connect.NewOperatorServiceHandler(op)
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// sendLinesThen emits lines (with strictly increasing timestamps) and then
// runs next, letting a script both deliver body and pick the close behavior
// (clean EOF, a transient RST, or blockUntilCanceled).
func sendLinesThen(
	n int, next func(error) error,
) func(context.Context, *connect.ServerStream[operatorv1.StreamPodLogsResponse]) error {
	return func(_ context.Context, stream *connect.ServerStream[operatorv1.StreamPodLogsResponse]) error {
		for i := range n {
			ts := time.Now().Add(time.Duration(i) * time.Millisecond).UTC().Format(time.RFC3339Nano)
			if err := stream.Send(&operatorv1.StreamPodLogsResponse{
				Lines: []*operatorv1.PodLogLine{{Timestamp: ts, Line: "line"}},
			}); err != nil {
				return err
			}
		}
		return next(nil)
	}
}

func eof(error) error { return nil } // clean EOF stand-in

// TestTailPodPreviousTransientCutCompletesDump is the full E4 path: a proxy
// RSTs the stream after the first batch of previous-container lines, so
// tailPod re-subscribes with Previous PRESERVED, the operator drains the rest
// of the finite previous buffer with a clean EOF, and the command then exits.
// (U2 stops at "2nd call has Previous=true"; this proves the recovery
// actually finishes and the consumer loop exits, so the user gets the whole
// dump and no hang.) -> G1, G4.
func TestTailPodPreviousTransientCutCompletesDump(t *testing.T) {
	transient := connect.NewError(connect.CodeInternal, eris.New("stream error: INTERNAL_ERROR"))
	op := &fakeOperator{
		scripts: []func(context.Context, *connect.ServerStream[operatorv1.StreamPodLogsResponse]) error{
			func(_ context.Context, stream *connect.ServerStream[operatorv1.StreamPodLogsResponse]) error {
				_ = stream.Send(&operatorv1.StreamPodLogsResponse{
					Lines: []*operatorv1.PodLogLine{{
						Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Line: "crash-1",
					}},
				})
				return transient // proxy RST after the first line
			},
			sendLinesThen(1, eof), // 2nd call: drain the rest of the previous buffer, clean EOF
		},
	}
	srv := newClusterTestServer(t, op)
	ctx := t.Context()
	c := NewClient(Config{OperatorEndpoint: srv.URL})
	getContent, done, errCh := runRemoteLogsConsumer(ctx, t, c, LogsOpts{Previous: true})

	select {
	case <-done:
	case <-time.After(6 * time.Second):
		t.Fatal("BUG: previous dump after a mid-replay cut did not complete (hang)")
	}
	out := getContent()
	calls := op.snapshot()
	require.Equal(t, 2, op.callCount(), "previous mid-replay cut re-subscribes exactly once")
	require.True(t, calls[0].Previous && calls[1].Previous, "both calls are previous-container queries")
	require.True(t, calls[0].Follow && calls[1].Follow, "Follow stays true (streamOne hard-codes it)")
	require.Contains(t, out, "crash-1", "the pre-cut line is delivered")
	require.Contains(t, out, "line", "the post-recovery line is delivered")
	require.NoError(t, <-errCh, "a dump that recovered and finished must report success")
}

// TestTailPodPreviousStopsResubscribingWhenCutsRepeat bounds the recovery the
// test above exercises. A --previous dump is a finite read, so retrying it
// forever is both pointless and unsafe: "previous" is resolved server-side per
// request, and a pod that restarts again during the backoff makes the retry
// target a NEWER incarnation, splicing two crashes into one seamless-looking
// dump. After maxPreviousResubscribes the cut is reported instead — a
// truncated dump the user knows about beats one they cannot trust.
func TestTailPodPreviousStopsResubscribingWhenCutsRepeat(t *testing.T) {
	transient := connect.NewError(connect.CodeInternal, eris.New("stream error: INTERNAL_ERROR"))
	cut := func(error) error { return transient }
	op := &fakeOperator{
		scripts: []func(context.Context, *connect.ServerStream[operatorv1.StreamPodLogsResponse]) error{
			sendLinesThen(1, cut), // call 1: cut mid-dump
			sendLinesThen(1, cut), // call 2: the one allowed retry, cut again
			sendLinesThen(1, cut), // call 3: must NOT happen
		},
	}
	srv := newClusterTestServer(t, op)
	ctx := t.Context()
	c := NewClient(Config{OperatorEndpoint: srv.URL})
	_, done, errCh := runRemoteLogsConsumer(ctx, t, c, LogsOpts{Previous: true})

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("BUG: --previous kept re-subscribing through repeated cuts; a finite dump must not retry forever")
	}
	require.Equal(t, 1+maxPreviousResubscribes, op.callCount(),
		"--previous retries a cut dump at most maxPreviousResubscribes times")
	require.Error(t, <-errCh,
		"a dump that never finished must be reported as cut short, not returned as success")
}

// TestTailPodPreviousNotFoundAfterPartialDumpReportsTruncation separates the
// two silences a NotFound can mean for --previous. Nothing delivered means the
// pod has no previous container at all; a NotFound after lines have already
// flowed means the dump was cut off mid-flight. Reporting the second as "no
// previous-container logs" would contradict the output the user just watched
// scroll past, so the message turns on whether anything arrived.
func TestTailPodPreviousNotFoundAfterPartialDumpReportsTruncation(t *testing.T) {
	notFound := connect.NewError(connect.CodeNotFound, eris.New("pod gameplay-abc gone"))
	op := &fakeOperator{
		scripts: []func(context.Context, *connect.ServerStream[operatorv1.StreamPodLogsResponse]) error{
			sendLinesThen(1, func(error) error { return notFound }),
			blockUntilCanceled, // must NOT be reached: --previous cannot recover a NotFound
		},
	}
	srv := newClusterTestServer(t, op)
	ctx := t.Context()
	c := NewClient(Config{OperatorEndpoint: srv.URL})
	getContent, done, errCh := runRemoteLogsConsumer(ctx, t, c, LogsOpts{Previous: true})

	select {
	case <-done:
	case <-time.After(6 * time.Second):
		t.Fatal("BUG: --previous did not terminate on a mid-dump NotFound")
	}
	require.Equal(t, 1, op.callCount(), "--previous does not retry a NotFound")
	require.Contains(t, getContent(), "line", "the lines that did arrive are still delivered")
	err := <-errCh
	require.Error(t, err, "a cut-off dump must be reported")
	require.Contains(t, err.Error(), "cut short",
		"lines already reached the user, so this is a truncated dump — not an absent previous container")
	require.NotContains(t, err.Error(), "no previous-container logs",
		"saying there are no logs contradicts the ones already printed")
}

// TestTailPodLiveTailReportsNonRetryableError covers the live-tail half of the
// error plumbing. StreamShardLogs used to return nil the instant it spawned its
// goroutines, so a tail that died on a permission or auth failure closed out and
// exited 0 — the TUI's own "Error tailing shard logs" branch could never fire.
// These codes are neither transient nor NotFound, so there is nothing to retry:
// the only useful response is to say what happened.
func TestTailPodLiveTailReportsNonRetryableError(t *testing.T) {
	denied := connect.NewError(connect.CodePermissionDenied, eris.New("token lacks logs:read"))
	require.False(t, isTransient(denied), "precondition: PermissionDenied is not retried")
	require.False(t, isNotFound(denied), "precondition: PermissionDenied is not pod churn")
	op := &fakeOperator{
		scripts: []func(context.Context, *connect.ServerStream[operatorv1.StreamPodLogsResponse]) error{
			func(_ context.Context, _ *connect.ServerStream[operatorv1.StreamPodLogsResponse]) error {
				return denied
			},
			blockUntilCanceled, // must NOT be reached: retrying cannot mint permissions
		},
	}
	srv := newClusterTestServer(t, op)
	ctx := t.Context()
	c := NewClient(Config{OperatorEndpoint: srv.URL})
	_, done, errCh := runRemoteLogsConsumer(ctx, t, c, LogsOpts{Previous: false})

	select {
	case <-done:
	case <-time.After(6 * time.Second):
		t.Fatal("BUG: a live tail denied by the operator did not terminate")
	}
	require.Equal(t, 1, op.callCount(), "a permission failure is not retried")
	err := <-errCh
	require.Error(t, err, "a live tail that died on PermissionDenied must not report success")
	require.Contains(t, err.Error(), "token lacks logs:read", "the operator's reason reaches the user")
}

// TestTailPodLiveTailResubscribesOnCleanEOF pins the unchanged live-tail
// path: a clean EOF (proxy idle-cut) on a Previous=false stream re-subscribes
// to the SAME pod with Previous=false and Follow=true; `since` suppresses
// the replayed history. The --previous fix must not regress this. -> G8.
func TestTailPodLiveTailResubscribesOnCleanEOF(t *testing.T) {
	op := &fakeOperator{
		scripts: []func(context.Context, *connect.ServerStream[operatorv1.StreamPodLogsResponse]) error{
			sendLinesThen(1, eof), // call 1: live tail, proxy idles the stream (clean EOF)
			blockUntilCanceled,    // call 2: re-subscribed live follow, held open
		},
	}
	srv := newClusterTestServer(t, op)
	ctx := t.Context()
	c := NewClient(Config{OperatorEndpoint: srv.URL})
	getContent, done, _ := runRemoteLogsConsumer(ctx, t, c, LogsOpts{Previous: false})

	// Wait for the re-subscribe (call 2) to happen without closing out — live
	// tails must keep streaming, not exit on a clean EOF.
	deadline := time.Now().Add(6 * time.Second)
	for op.callCount() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("BUG: live tail did not re-subscribe after a clean EOF; calls=%d", op.callCount())
		}
		time.Sleep(20 * time.Millisecond)
	}
	calls := op.snapshot()
	require.False(t, calls[0].Previous, "live tail call 1 is Previous=false")
	require.False(t, calls[1].Previous, "live tail re-subscribe keeps Previous=false")
	require.True(t, calls[1].Follow, "live tail re-subscribe keeps Follow=true")
	require.Equal(t, calls[0].PodName, calls[1].PodName, "re-subscribe targets the SAME pod")
	require.Contains(t, getContent(), "line", "live history is delivered")
	select {
	case <-done:
		t.Fatal("BUG: live tail closed out (out closed) — live tails must keep streaming until ctx")
	case <-time.After(200 * time.Millisecond):
	}
}

// TestTailPodLiveTailReconnectsReplacementPod pins the unchanged pod-churn
// path for a live tail: a CodeNotFound mid-stream triggers
// findReplacementPod, which (after the rolling deploy lands a new pod)
// reconnects to that replacement with Previous=false, Follow=true. The
// --previous fix must not regress this (and --previous itself terminates on
// NotFound instead of churning, per G6). -> G10.
func TestTailPodLiveTailReconnectsReplacementPod(t *testing.T) {
	notFound := connect.NewError(connect.CodeNotFound, eris.New("pod gameplay-abc gone (rolled)"))
	op := &fakeOperator{pod: "gameplay-abc"}
	op.scripts = []func(context.Context, *connect.ServerStream[operatorv1.StreamPodLogsResponse]) error{
		func(_ context.Context, stream *connect.ServerStream[operatorv1.StreamPodLogsResponse]) error {
			_ = stream.Send(&operatorv1.StreamPodLogsResponse{
				Lines: []*operatorv1.PodLogLine{{
					Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Line: "before-roll",
				}},
			})
			// Pod was rolled mid-stream; surface NotFound, then flip Status
			// to the replacement pod so findReplacementPod resolves it.
			op.setPod("gameplay-xyz")
			return notFound
		},
		blockUntilCanceled, // call 2: live follow against the replacement pod
	}
	srv := newClusterTestServer(t, op)
	ctx := t.Context()
	c := NewClient(Config{OperatorEndpoint: srv.URL})
	getContent, done, _ := runRemoteLogsConsumer(ctx, t, c, LogsOpts{Previous: false})

	deadline := time.Now().Add(6 * time.Second)
	for op.callCount() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("BUG: live tail did not reconnect to the replacement pod; calls=%d", op.callCount())
		}
		time.Sleep(20 * time.Millisecond)
	}
	calls := op.snapshot()
	require.False(t, calls[0].Previous, "live tail call 1 is Previous=false")
	require.False(t, calls[1].Previous, "replacement reconnect keeps Previous=false")
	require.True(t, calls[1].Follow, "replacement reconnect keeps Follow=true")
	require.Equal(t, "gameplay-abc", calls[0].PodName, "first call targets the rolled pod")
	require.Equal(t, "gameplay-xyz", calls[1].PodName, "second call targets the REPLACEMENT pod")
	require.Contains(t, getContent(), "before-roll", "pre-roll history is delivered")
	select {
	case <-done:
		t.Fatal("BUG: live tail closed out after pod churn — it must keep streaming until ctx")
	case <-time.After(200 * time.Millisecond):
	}
}

// TestTailPodCancelsCleanly pins ctx cancellation: canceling the command's
// context (Ctrl+C / parent timeout) unblocks streamOne's out<- / Receive,
// ends tailPod, closes out, and the consumer loop exits promptly — no
// goroutine or process hang. -> G7 (covers both --previous and live tail).
func TestTailPodCancelsCleanly(t *testing.T) {
	for _, previous := range []bool{true, false} {
		t.Run(fmt.Sprintf("previous=%v", previous), func(t *testing.T) {
			op := &fakeOperator{
				scripts: []func(context.Context, *connect.ServerStream[operatorv1.StreamPodLogsResponse]) error{
					blockUntilCanceled, // call 1: held open (live follow for live; stuck previous open)
				},
			}
			srv := newClusterTestServer(t, op)
			ctx, cancel := context.WithCancel(t.Context())
			c := NewClient(Config{OperatorEndpoint: srv.URL})
			_, done, errCh := runRemoteLogsConsumer(ctx, t, c, LogsOpts{Previous: previous})

			// Give the stream a moment to open and block, then cancel — like
			// a user hitting Ctrl+C mid-tail.
			time.Sleep(100 * time.Millisecond)
			cancel()

			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("BUG: canceling ctx did not unblock the consumer (hang) — out was not closed")
			}
			require.Equal(t, 1, op.callCount(), "exactly one stream opened before the cancel")
			require.Equal(t, previous, op.snapshot()[0].Previous, "honors the requested --previous flag")
			require.NoError(t, <-errCh, "ctx cancellation is the user's own Ctrl+C, not a stream failure")
		})
	}
}

// TestStreamShardLogsShardFilterTypoErrors is the core regression: a
// `--shard` value that matches no shard pool (here a typo of "gameplay")
// used to print "Tailing <env>" and exit 0 with no output, so a bad filter
// was indistinguishable from a quiet shard — the sole `world` subcommand that
// accepts a shard selector and does NOT validate it. StreamShardLogs must
// surface the no-match as an error so the CLI exits non-zero, matching
// `world build` / `world reload` / `world debug`. out must still close so the
// consumer loop (and thus the command) terminates without a hang.
func TestStreamShardLogsShardFilterTypoErrors(t *testing.T) {
	op := &fakeOperator{} // default "gameplay" pool, instance "gameplay"
	srv := newClusterTestServer(t, op)
	ctx := t.Context()
	c := NewClient(Config{OperatorEndpoint: srv.URL})
	getContent, done, errCh := runRemoteLogsConsumer(ctx, t, c, LogsOpts{
		ShardIDs: []string{"gamepla"}, // typo: not a live pool id
	})

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("BUG: out never closed on a no-match --shard filter (consumer hang)")
	}
	require.Empty(t, getContent(), "an unmatched filter must emit no log lines")
	require.Equal(t, 0, op.callCount(),
		"a no-match filter opens zero pod streams (instances filtered out before any StreamPodLogs)")
	err := <-errCh
	require.Error(t, err, "a no-match --shard filter must error, not exit 0")
	require.Contains(t, err.Error(), "no pods matched",
		"the error must say the filter matched nothing")
	require.Contains(t, err.Error(), "gamepla",
		"the error must name the unmatched filter value the user typed")
}

// TestStreamShardLogsInstanceNameNoMatchErrors covers the InstanceNames half
// of the fix (the local picker sends valid instance names, but a direct
// StreamShardLogs caller may not). An instance name that matches no instance
// in the matching pools must error, not collapse to a silent success.
func TestStreamShardLogsInstanceNameNoMatchErrors(t *testing.T) {
	op := &fakeOperator{} // pool "gameplay" with only the "gameplay" instance
	srv := newClusterTestServer(t, op)
	ctx := t.Context()
	c := NewClient(Config{OperatorEndpoint: srv.URL})
	getContent, done, errCh := runRemoteLogsConsumer(ctx, t, c, LogsOpts{
		InstanceNames: []string{"gameplay-3"}, // no such instance in the pool
	})

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("BUG: out never closed on a no-match instance-name filter (hang)")
	}
	require.Empty(t, getContent())
	require.Equal(t, 0, op.callCount(), "a no-match instance filter opens zero pod streams")
	err := <-errCh
	require.Error(t, err, "a no-match InstanceNames filter must error, not exit 0")
	require.Contains(t, err.Error(), "gameplay-3")
}

// TestStreamShardLogsUnfilteredZeroPodsReturnsNil guards the unfiltered branch
// the fix deliberately leaves untouched. A user who runs `world logs --env`
// with NO --shard against an env that genuinely has zero running pods
// (scaled to zero, mid-rolling-deploy, fresh namespace) must NOT see an error:
// there was no filter to be "wrong". Without this guard a too-broad fix would
// make a quiet-but-healthy scaled-to-zero env look like a bad filter.
func TestStreamShardLogsUnfilteredZeroPodsReturnsNil(t *testing.T) {
	for _, tt := range []struct {
		name  string
		pools []*operatorv1.ShardPoolStatus
	}{
		{
			name:  "no pools at all",
			pools: []*operatorv1.ShardPoolStatus{},
		},
		{
			name: "pool with an instance whose pod is not scheduled",
			pools: []*operatorv1.ShardPoolStatus{{
				ShardId: "gameplay",
				Instances: []*operatorv1.ShardInstanceStatus{{
					Name:    "gameplay",
					PodName: "", // listShardInstances skips empty-podName instances
				}},
			}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			op := &fakeOperator{pools: tt.pools}
			srv := newClusterTestServer(t, op)
			ctx := t.Context()
			c := NewClient(Config{OperatorEndpoint: srv.URL})
			getContent, done, errCh := runRemoteLogsConsumer(ctx, t, c, LogsOpts{})

			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("BUG: out never closed on an unfiltered zero-pod env (hang)")
			}
			require.Empty(t, getContent())
			require.NoError(t, <-errCh,
				"an unfiltered env with zero running pods is not a bad filter; must not error")
		})
	}
}

// TestStreamShardLogsShardFilterMatchStreams pins the matching half of the
// behavior so the fix cannot regress the happy path: a `--shard` value that IS
// a live pool id streams its logs and reports success, never tripping the new
// "no pods matched" branch. Uses --previous + a clean EOF so it terminates
// deterministically (no live-tail race).
func TestStreamShardLogsShardFilterMatchStreams(t *testing.T) {
	op := &fakeOperator{
		scripts: []func(context.Context, *connect.ServerStream[operatorv1.StreamPodLogsResponse]) error{
			sendN(2), // previous-container dump: 2 lines then clean EOF (terminal for --previous)
		},
	}
	srv := newClusterTestServer(t, op)
	ctx := t.Context()
	c := NewClient(Config{OperatorEndpoint: srv.URL})
	getContent, done, errCh := runRemoteLogsConsumer(ctx, t, c, LogsOpts{
		ShardIDs: []string{"gameplay"}, // pool id that matches
		Previous: true,
	})

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("BUG: a matching --shard filter did not complete")
	}
	require.Contains(t, getContent(), "prev", "a matching --shard filter must deliver log lines")
	require.Equal(t, 1, op.callCount(), "exactly one pod stream for the matching pool")
	require.True(t, op.snapshot()[0].Previous, "the call honors the --previous flag")
	require.NoError(t, <-errCh,
		"a matching filter that completes must report success, not 'no pods matched'")
}

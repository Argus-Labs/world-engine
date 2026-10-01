package cluster

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rotisserie/eris"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

// These tests exercise StreamPlatformLogs's reconnection loop against a fake
// kube-apiserver (an httptest server speaking just enough of the pods List +
// GetLogs HTTP API). They focus on the --previous path, where a request
// against a pod that has never crashed is rejected by the kubelet and must
// terminate with an error instead of silently spinning on the next==current
// retry branch (which never clears opts.Previous). Mirrors the shard-side
// tailPod regression tests in logs_internal_test.go.

// platformPodName is the single Running pod every fake apiserver advertises,
// so pickPlatformPod always returns the same name — the precondition for the
// next==current retry branch that the bug spins on.
const platformPodName = "nats-aaaaaaaa-1111"

// logScript governs one GetLogs call's HTTP response.
type logScript func(w http.ResponseWriter, r *http.Request)

// fakeAPIServer mounts the pod List + GetLogs endpoints behind an httptest
// server. List always returns one Running pod named platformPodName so
// pickPlatformPod is deterministic. GetLogs dispatches to scripts[i] for the
// (i+1)-th call, then to holdLog. Each GetLogs call's `previous` query flag is
// recorded so a test can assert the reconnection loop's behavior.
//
// Returns the server (closed via t.Cleanup) and a rest.Config that
// kubernetes.NewForConfig turns into a clientset pointed at it — so the
// apierrors.StatusError parsed from a rejection body is a genuine client-go
// artifact, not a hand-rolled error, and isPlatformPodGone classifies it the
// same way a real apiserver rejection is.
func fakeAPIServer(t *testing.T, scripts []logScript) (*rest.Config, *fakeAPIServerState) {
	t.Helper()
	st := &fakeAPIServerState{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/namespaces/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/log") && r.Method == http.MethodGet:
			prev := r.URL.Query().Get("previous") == "true"
			st.mu.Lock()
			st.calls = append(st.calls, prev)
			idx := len(st.calls) - 1
			st.mu.Unlock()
			script := holdLog
			if idx < len(scripts) {
				script = scripts[idx]
			}
			script(w, r)
		case strings.HasSuffix(r.URL.Path, "/pods") && r.Method == http.MethodGet:
			// Label selector is ignored: the picker only needs a Running pod.
			podList := &corev1.PodList{
				TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "PodList"},
				Items: []corev1.Pod{{
					ObjectMeta: metav1.ObjectMeta{Name: platformPodName, Namespace: "ns"},
					Status:     corev1.PodStatus{Phase: corev1.PodRunning},
				}},
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(podList))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &rest.Config{Host: srv.URL}, st
}

type fakeAPIServerState struct {
	mu    sync.Mutex
	calls []bool // previous flag of each GetLogs call, in order
}

func (s *fakeAPIServerState) snapshot() []bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]bool, len(s.calls))
	copy(out, s.calls)
	return out
}

func (s *fakeAPIServerState) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

// holdLog is the default tail of the script list: a live follow that writes
// one line then holds the connection open until the request context is done —
// how a real kubelet holds a live tail open with nothing more to say.
func holdLog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "%s live-line\n", "2026-09-07T10:00:00.000000000Z")
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	<-r.Context().Done()
}

// rejectStatus writes a metav1.Status JSON body with the given HTTP code and
// reason — the exact shape client-go's rest client parses into a
// *apierrors.StatusError (via transformResponse -> Result.Error), so
// isPlatformPodGone classifies it the same way a real apiserver rejects a
// `previous=true` request against a container that never crashed.
func rejectStatus(t *testing.T, w http.ResponseWriter, code int, reason metav1.StatusReason, msg string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	require.NoError(t, json.NewEncoder(w).Encode(&metav1.Status{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"},
		Status:   metav1.StatusFailure,
		Reason:   reason,
		Message:  msg,
		Code:     int32(code),
	}))
}

// dumpThenEOF is a previous-container dump that emits `n` timestamped lines and
// then closes the response — a clean EOF exactly like the kubelet ending a
// finite previous-container buffer. The scanner finishes; streamPlatformPod
// returns nil; StreamPlatformLogs treats it as a completed dump.
func dumpThenEOF(n int) logScript {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		for i := range n {
			fmt.Fprintf(w, "2026-09-07T10:00:0%d.000000000Z prev%d\n", i, i)
		}
	}
}

// newPlatformClient returns a Client whose memoized kube client points at the
// fake apiserver, bypassing k3d entirely. StreamPlatformLogs builds its
// clientset from restCfg, so no real cluster is needed.
func newPlatformClient(restCfg *rest.Config) *Client {
	c := NewClient(Config{})
	c.cachedKube = &kubeClient{restCfg: restCfg}
	return c
}

// platformRef is the PlatformPodRef every test streams — namespace and
// selector are inert against the fake apiserver (List always returns the same
// pod regardless of selector).
func platformRef() PlatformPodRef {
	return PlatformPodRef{Name: "nats", Namespace: "ns", Selector: "app=nats"}
}

// TestStreamPlatformLogs_PreviousNoPreviousContainerTerminates is the core
// regression test for the silent-spin bug. A --previous request against a pod
// that has never crashed is rejected by the kubelet before any line flows.
// k8s reports the rejection as `BadRequest` ("previous terminated container
// not found") on some versions and `NotFound` on others; both are classified
// pod-gone by isPlatformPodGone, so pre-fix the next==current retry branch
// spins every reconnectBackoff (re-requesting previous=true) until the
// caller's context is cancelled, then returns nil — printing nothing and
// exiting 0, indistinguishable from an empty previous buffer.
//
// The fix mirrors tailPod: a --previous open that fails before delivering any
// line is terminal, so it surfaces an error after exactly one GetLogs call
// and returns promptly — no spin.
func TestStreamPlatformLogs_PreviousNoPreviousContainerTerminates(t *testing.T) {
	for _, tt := range []struct {
		name   string
		reject logScript
		isGone func(error) bool
	}{
		{
			name: "bad request",
			reject: func(w http.ResponseWriter, _ *http.Request) {
				rejectStatus(t, w, http.StatusBadRequest, metav1.StatusReasonBadRequest, "previous terminated container not found")
			},
			isGone: apierrors.IsBadRequest,
		},
		{
			name: "not found",
			reject: func(w http.ResponseWriter, _ *http.Request) {
				rejectStatus(t, w, http.StatusNotFound, metav1.StatusReasonNotFound, "previous terminated container not found")
			},
			isGone: apierrors.IsNotFound,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			restCfg, st := fakeAPIServer(t, []logScript{
				tt.reject, // call 1: rejected --previous open
				holdLog,   // call 2: must NOT be reached; --previous open error terminates
			})
			c := newPlatformClient(restCfg)

			out := make(chan LogLine, 16)
			closed := make(chan struct{})
			var got []LogLine
			go func() {
				for line := range out {
					got = append(got, line)
				}
				close(closed)
			}()

			// Generous ctx so a buggy build (which spins until ctx cancel) is
			// reliably caught by the assertions below rather than hanging the
			// test process.
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			defer cancel()

			start := time.Now()
			err := c.StreamPlatformLogs(ctx, platformRef(), LogsOpts{Previous: true}, out)
			elapsed := time.Since(start)

			require.Error(t, err, "a --previous request rejected before any line must surface an error, "+
				"not exit 0 with nothing printed (indistinguishable from an empty previous buffer)")
			require.True(t, tt.isGone(err),
				"the kubelet rejection must propagate so callers can tell 'no previous container' from other failures")
			require.Contains(t, err.Error(), "previous-container logs for ns/"+platformPodName,
				"the error must identify the namespace/pod the failed dump targeted")
			require.Equal(t, 1, st.count(),
				"--previous open error must terminate after exactly one GetLogs call (no reconnect spin)")
			require.True(t, st.snapshot()[0], "the one call must carry the user's --previous flag")
			require.Less(t, elapsed, reconnectBackoff,
				"must return promptly (before one backoff cycle), not spin until the ctx deadline")

			select {
			case <-closed:
			case <-time.After(2 * time.Second):
				t.Fatal("out was not closed; a caller ranging over it would hang")
			}
			require.Empty(t, got, "no lines should be delivered for a failed --previous open")
		})
	}
}

// TestStreamPlatformLogs_PreviousEmptyBufferSucceeds guards the documented
// success semantics for --previous: a previous container whose buffer is empty
// (exists but had nothing to log) ends with a clean EOF, which is a SUCCESS —
// the user asked for previous logs and got (zero) of them, not a failure. The
// fix only short-circuits the error path, so an empty dump must still return
// nil after exactly one call.
func TestStreamPlatformLogs_PreviousEmptyBufferSucceeds(t *testing.T) {
	restCfg, st := fakeAPIServer(t, []logScript{
		dumpThenEOF(0), // empty previous buffer -> clean EOF
		holdLog,        // must NOT be reached
	})
	c := newPlatformClient(restCfg)

	out := make(chan LogLine, 16)
	closed := make(chan struct{})
	var got []LogLine
	go func() {
		for line := range out {
			got = append(got, line)
		}
		close(closed)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	start := time.Now()
	err := c.StreamPlatformLogs(ctx, platformRef(), LogsOpts{Previous: true}, out)
	elapsed := time.Since(start)

	require.NoError(t, err, "an empty previous buffer is a successful (zero-line) dump, not a failure")
	require.Equal(t, 1, st.count(), "a completed previous dump must make exactly one GetLogs call")
	require.True(t, st.snapshot()[0], "the one call must carry --previous")
	require.Less(t, elapsed, reconnectBackoff, "a completed dump must return promptly")

	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("out was not closed")
	}
	require.Empty(t, got, "an empty previous buffer delivers no lines")
}

// TestStreamPlatformLogs_LiveFollowRetryUnchanged guards that the fix did not
// alter the live-tail (Previous=false) reconnect semantics: a pod that reports
// gone (NotFound) while still appearing Running to the picker must still
// back off and re-subscribe to the same pod. The fix only short-circuits the
// Previous=true path, so an identical-but-not-previous failure must keep
// retrying across the backoff — exactly the production live-tail behavior.
func TestStreamPlatformLogs_LiveFollowRetryUnchanged(t *testing.T) {
	restCfg, st := fakeAPIServer(t, []logScript{
		func(w http.ResponseWriter, _ *http.Request) {
			rejectStatus(t, w, http.StatusNotFound, metav1.StatusReasonNotFound, "pod not found")
		}, // call 1: transient gone while picker still sees Running->next==current
		holdLog, // call 2 onwards: live line then hold (subscription recovers)
	})
	c := newPlatformClient(restCfg)

	out := make(chan LogLine, 16)
	closed := make(chan struct{})
	firstLine := make(chan struct{})
	var once sync.Once
	var mu sync.Mutex
	var got []LogLine
	go func() {
		for line := range out {
			mu.Lock()
			got = append(got, line)
			mu.Unlock()
			once.Do(func() { close(firstLine) })
		}
		close(closed)
	}()

	// Backoff is 2s, so allow well past one cycle for the re-subscribe.
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	go func() { _ = c.StreamPlatformLogs(ctx, platformRef(), LogsOpts{}, out) }()

	// Wait for the recovered live line (channel signal avoids reading `got`
	// while the drain goroutine is still appending), then assert a
	// re-subscribe happened.
	select {
	case <-firstLine:
	case <-time.After(7 * time.Second):
		t.Fatalf("live follow never recovered after a transient gone: calls=%d lines=%d", st.count(), func() int {
			mu.Lock()
			defer mu.Unlock()
			return len(got)
		}())
	}
	// Terminate the live tail now that recovery is confirmed.
	cancel()
	<-closed

	snap := st.snapshot()
	require.GreaterOrEqual(t, len(snap), 2, "a live-tail gone-with-same-pod must re-subscribe (retry path intact)")
	for i, prev := range snap {
		require.False(t, prev, "call %d must keep Previous=false on the live-tail path", i)
	}
	mu.Lock()
	bodies := lineBodies(got)
	mu.Unlock()
	require.Contains(t, bodies, "live-line", "the recovered subscription's live line must be delivered")
}

// TestStreamPlatformLogs_PreviousCutShortErrors guards the delivered-then-error
// path: a --previous dump that opens, delivers a line, then fails mid-stream
// (here: a log line exceeding the scanner's 1MB max buffer, which the
// buffer-sizing comment in streamPlatformPod warns is a real zerolog-failure
// mode — forcing bufio.ErrTooLong, a genuine non-EOF scanner error standing in
// for any mid-stream transport/decoding failure) must surface the error after
// exactly one call rather than retry. A retry would risk silently splicing two
// crashes' previous logs (the pod's `previous` is resolved server-side per
// request and could move to a newer incarnation during the backoff), and the
// platform path has no transient classifier to bound such a retry. Reporting a
// truncated dump the user is told about beats a seamless-looking one they
// cannot trust — the same tradeoff tailPod makes for the non-transient cut.
func TestStreamPlatformLogs_PreviousCutShortErrors(t *testing.T) {
	restCfg, st := fakeAPIServer(t, []logScript{
		func(w http.ResponseWriter, _ *http.Request) {
			// Open succeeds (200), deliver one line, then emit a single
			// over-size line (>1MB, no newline) so the client's scanner hits
			// bufio.ErrTooLong after prev0 was delivered — a real mid-stream
			// error after a delivered line.
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "2026-09-07T10:00:00.000000000Z prev0\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			_, _ = w.Write(make([]byte, 2*1024*1024)) // no newline -> one over-size line
		},
		holdLog, // must NOT be reached: --previous cut mid-dump is terminal
	})
	c := newPlatformClient(restCfg)

	out := make(chan LogLine, 16)
	closed := make(chan struct{})
	var got []LogLine
	go func() {
		for line := range out {
			got = append(got, line)
		}
		close(closed)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	start := time.Now()
	err := c.StreamPlatformLogs(ctx, platformRef(), LogsOpts{Previous: true}, out)
	elapsed := time.Since(start)

	require.Error(t, err, "a --previous dump that errors mid-stream must surface the error, not retry silently")
	require.ErrorIs(t, err, bufio.ErrTooLong, "the scanner error must propagate through the eris wrap")
	require.Contains(t, err.Error(), "previous-container logs for ns/"+platformPodName,
		"the error must identify the failed dump's namespace/pod")
	require.Equal(t, 1, st.count(), "a cut-short --previous dump must not re-subscribe")
	require.True(t, st.snapshot()[0], "the one call must carry --previous")
	require.Less(t, elapsed, reconnectBackoff, "must return promptly, not spin across a backoff")
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("out was not closed")
	}
	require.Equal(t, []string{"prev0"}, lineBodies(got),
		"the line delivered before the mid-stream error must still be emitted")
}

// lineBodies returns the LogLine.Line bodies in delivery order.
func lineBodies(lines []LogLine) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.Line
	}
	return out
}

// Ensure eris-wrapped apierrors still classify via the standard library
// errors.As traversal (StreamPlatformLogs double-wraps the rejection through
// streamPlatformPod + the Previous guard). If this ever breaks, the core
// regression test's `tt.isGone(err)` assertion and isPlatformPodGone itself
// silently flip to false-positive.
func TestPreviousRejectClassificationSurvivesErisWrap(t *testing.T) {
	base := apierrors.NewBadRequest("previous terminated container not found")
	require.True(t, apierrors.IsBadRequest(base))
	wrapped := eris.Wrap(eris.Wrap(base, "open log stream for ns/pod"), "previous-container logs for ns/pod")
	require.True(t, apierrors.IsBadRequest(wrapped),
		"apierrors.IsBadRequest must traverse the eris wrap chain to the StatusError")
	require.True(t, isPlatformPodGone(wrapped),
		"isPlatformPodGone must still classify the double-wrapped rejection as gone")
}

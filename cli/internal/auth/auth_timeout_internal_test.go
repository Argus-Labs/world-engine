package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// friendlyTimeoutMsg is the actionable message an interactive developer should
// see when nobody finishes the browser sign-in within pollTimeout, instead of
// the raw "context deadline exceeded" which reads like a network failure.
const friendlyTimeoutMsg = "timed out waiting for authorization; run the command again"

// assertFriendlyTimeout verifies err is the actionable timeout message and not
// a leak of the raw wrapped deadline error.
func assertFriendlyTimeout(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if !strings.Contains(err.Error(), friendlyTimeoutMsg) {
		t.Errorf("error = %q, want the friendly timeout message %q", err, friendlyTimeoutMsg)
	}
	if strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("error = %q, must not leak the raw context deadline exceeded wrapping", err)
	}
}

// A short-lived parent context drives pollEvery's derived deadline down from
// the 11-minute pollTimeout, so the timeout path can be exercised in
// milliseconds. The derived child's ctx.Err() is DeadlineExceeded when the
// parent deadline fires (verified separately), so the friendly-message branch
// is reached regardless of which select case wins.
func shortDeadline(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

// TestPollTimeoutFriendlyMessage exercises the headline guarantee of the fix:
// when nobody finishes the browser sign-in before pollEvery's derived deadline
// expires, the CLI returns the actionable "run the command again" message
// rather than the raw wrapped "context deadline exceeded" error. The deadline
// expires here during the select wait (instant server), so this is the path
// that already worked — it guards against a regression that re-introduces the
// raw message in the easy case.
func TestPollTimeoutFriendlyMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"status":"pending"}`)
	}))
	defer srv.Close()

	c := &Client{http: srv.Client()}
	ctx, cancel := shortDeadline(40 * time.Millisecond)
	defer cancel()

	_, err := c.pollEvery(ctx, srv.URL, 13*time.Millisecond) // non-aligned: 40/13 is not an integer
	assertFriendlyTimeout(t, err)
}

// TestPollTimeoutFriendlyMessageWhenDeadlineFiresDuringStatusCall targets
// Mechanism B directly: the server blocks until the request's context is
// cancelled, so the derived deadline fires while c.status is in flight. Before
// the fix the raw "polling authorization status: ... context deadline exceeded"
// error was returned 100% of the time; after the fix it must be the friendly
// message.
func TestPollTimeoutFriendlyMessageWhenDeadlineFiresDuringStatusCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // hold the call open until the client deadline cancels it
	}))
	defer srv.Close()

	c := &Client{http: srv.Client()}
	ctx, cancel := shortDeadline(50 * time.Millisecond)
	defer cancel()

	_, err := c.pollEvery(ctx, srv.URL, 37*time.Millisecond) // non-aligned so only Mechanism B can produce the error
	assertFriendlyTimeout(t, err)
}

// TestPollTimeoutFriendlyMessageAtAlignedBoundary targets Mechanism A: with an
// aligned deadline/interval (mirroring production's 660s/3s = 220:1 ratio), the
// final ticker tick and ctx.Done() coincide and Go's select picks one at
// random. Before the fix, the runs where the ticker won returned the raw error
// on the next status call; after the fix every run must return the friendly
// message regardless of which case wins. The loop exercises the race many
// times so a regression re-introducing the ~50% raw rate fails reliably.
func TestPollTimeoutFriendlyMessageAtAlignedBoundary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"status":"pending"}`)
	}))
	defer srv.Close()

	c := &Client{http: srv.Client()}
	const iters = 40
	for i := range iters {
		// 60ms / 10ms = 6:1, the same divisibility that makes 660s/3s align.
		ctx, cancel := shortDeadline(60 * time.Millisecond)
		_, err := c.pollEvery(ctx, srv.URL, 10*time.Millisecond)
		cancel()
		if err == nil {
			t.Fatalf("iter %d: expected a timeout error, got nil", i)
		}
		if !strings.Contains(err.Error(), friendlyTimeoutMsg) {
			t.Fatalf("iter %d: error = %q, want the friendly timeout message "+
				"(the select race returned the raw error)", i, err)
		}
		if strings.Contains(err.Error(), "context deadline exceeded") {
			t.Fatalf("iter %d: error = %q, must not leak the raw deadline error", i, err)
		}
	}
}

// TestPollTimeoutFriendlyMessageWithRealisticLatency combines Mechanisms A and B
// under production-proportioned timing: the deadline/interval are aligned and
// the status call occupies a non-trivial fraction of each cycle (1ms / 15ms ~
// 6.7%, like production's ~200ms / 3s). Before the fix this produced the raw
// error on ~59% of runs; after the fix every run must return the friendly
// message. Timing is scaled down from production (660s/3s/200ms) by 4400x to
// (120ms/15ms/1ms) keeping the 8:1 alignment and 6.7% duty cycle. 30 iterations
// at a ~59% per-run raw rate detect a regression with P > 1 - 0.41^30 ~= 1.
func TestPollTimeoutFriendlyMessageWithRealisticLatency(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(1 * time.Millisecond)
		fmt.Fprint(w, `{"status":"pending"}`)
	}))
	defer srv.Close()

	c := &Client{http: srv.Client()}
	const iters = 30
	for i := range iters {
		// 120ms / 15ms = 8:1 (aligned), mirroring production's 660s/3s ratio.
		ctx, cancel := shortDeadline(120 * time.Millisecond)
		_, err := c.pollEvery(ctx, srv.URL, 15*time.Millisecond)
		cancel()
		if err == nil {
			t.Fatalf("iter %d: expected a timeout error, got nil", i)
		}
		if !strings.Contains(err.Error(), friendlyTimeoutMsg) {
			t.Fatalf("iter %d: error = %q, want the friendly timeout message", i, err)
		}
		if strings.Contains(err.Error(), "context deadline exceeded") {
			t.Fatalf("iter %d: error = %q, must not leak the raw deadline error", i, err)
		}
	}
}

// TestPollSurfacesNonDeadlineErrors guards the fix against being too eager: a
// transport failure that is NOT the poll deadline must still surface with its
// original "polling authorization status: ..." wrapping rather than being
// mapped to the friendly timeout message. Here the server is shut down so the
// first status call fails with a connection error while the context is still
// alive (ctx.Err() == nil).
func TestPollSurfacesNonDeadlineErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"status":"pending"}`)
	}))
	srv.Close() // shut down so calls fail with connection refused, not a deadline

	c := &Client{http: srv.Client()}
	_, err := c.pollEvery(context.Background(), srv.URL, time.Millisecond)
	if err == nil {
		t.Fatal("expected a non-deadline transport error, got nil")
	}
	if strings.Contains(err.Error(), friendlyTimeoutMsg) {
		t.Errorf("error = %q, a non-deadline failure must not be mapped to the friendly timeout message", err)
	}
	if !strings.Contains(err.Error(), "polling authorization status") {
		t.Errorf("error = %q, want the original wrapping preserved for non-deadline failures", err)
	}
}

// TestPollSurfacesParentCancellationDuringStatusCall verifies the fix only
// rewrites the DeadlineExceeded case: a parent context that is cancelled while
// a status call is in flight must still return the wrapped "context canceled"
// error, not the friendly timeout message. This preserves the existing
// behaviour described in the bug report and keeps the timeout message
// specific to "you didn't finish sign-in in time".
func TestPollSurfacesParentCancellationDuringStatusCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // hold the call open until the parent cancels it
	}))
	defer srv.Close()

	c := &Client{http: srv.Client()}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	_, err := c.pollEvery(ctx, srv.URL, time.Hour) // long interval: only the parent cancel can stop the loop
	if err == nil {
		t.Fatal("expected a parent-cancellation error, got nil")
	}
	if strings.Contains(err.Error(), friendlyTimeoutMsg) {
		t.Errorf("error = %q, parent cancellation must not be mapped to the friendly timeout message", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want it to wrap context.Canceled", err)
	}
}

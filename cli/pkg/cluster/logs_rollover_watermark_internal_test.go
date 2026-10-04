package cluster

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/stretchr/testify/require"

	operatorv1 "github.com/argus-labs/world-engine/cli/proto/gen/go/cardinal/operator/v1"
	"github.com/argus-labs/world-engine/cli/proto/gen/go/cardinal/operator/v1/operatorv1connect"
)

// sendPodLog emits one batched line on a StreamPodLogs server stream.
func sendPodLog(stream *connect.ServerStream[operatorv1.StreamPodLogsResponse], ts, body string) error {
	return stream.Send(&operatorv1.StreamPodLogsResponse{
		Lines: []*operatorv1.PodLogLine{{Timestamp: ts, Line: body}},
	})
}

// newOperatorServer mounts a fake OperatorServiceHandler behind an httptest
// server and returns the server. The caller must Close it.
func newOperatorServer(h operatorv1connect.OperatorServiceHandler) *httptest.Server {
	mux := http.NewServeMux()
	path, handler := operatorv1connect.NewOperatorServiceHandler(h)
	mux.Handle(path, handler)
	return httptest.NewServer(mux)
}

func gameplayPool(pod string) *operatorv1.StatusResponse {
	return &operatorv1.StatusResponse{
		Pools: []*operatorv1.ShardPoolStatus{{
			ShardId: "gameplay",
			Instances: []*operatorv1.ShardInstanceStatus{{
				Name:    "gameplay",
				PodName: pod,
			}},
		}},
	}
}

// drainLogs runs StreamShardLogs and collects `want` lines (by Line body),
// failing the test if `out` closes early or the deadline passes. Cancels the
// stream and waits for StreamShardLogs to return so no goroutine leaks.
func drainLogs(t *testing.T, c *Client, want int) ([]string, error) {
	t.Helper()
	out := make(chan LogLine, 256)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- c.StreamShardLogs(ctx, LogsOpts{ShardIDs: []string{"gameplay"}, TailLines: 200}, out) }()

	var got []string
	deadline := time.After(15 * time.Second)
	for len(got) < want {
		select {
		case line, ok := <-out:
			if !ok {
				t.Fatalf("out closed early; got=%v", got)
			}
			got = append(got, line.Line)
		case <-deadline:
			t.Fatalf("timeout waiting for %d lines; got=%v", want, got)
		}
	}
	cancel()
	return got, <-errCh
}

// rolloverHandler models the REAL production two-step rollover signal path.
// In production a pod that delivered lines and then terminates ends the open
// stream with a scanner EOF, which the operator returns as clean nil
// (forwardLogStream's EOF branch). CodeNotFound is returned only on the NEXT
// open, when the pod is already gone (logReq.Stream fails at open time).
//
// So the production rollover is a two-step sequence per pod:
//  1. streamOne(podA) ends with err == nil (clean EOF on termination) ->
//     tailPod's `case err == nil` branch -> current stays podA, since = A's
//     last timestamp.
//  2. After reconnectBackoff, streamOne(podA) re-opens; podA is now gone ->
//     operator returns CodeNotFound at open -> streamOne's open-fail path
//     returns since UNCHANGED -> tailPod's `case isNotFound` branch ->
//     findReplacementPod -> current = podB.
//  3. streamOne(podB) streams the replacement pod's history.
//
// podACalls counts StreamPodLogs opens for podA so the handler can return a
// clean EOF on the first open and CodeNotFound on the second, matching
// production. Status returns podA on the first call (listShardInstances) and
// podB on subsequent calls (findReplacementPod).
type rolloverHandler struct {
	operatorv1connect.UnimplementedOperatorServiceHandler

	mu        sync.Mutex
	statusN   int
	podACalls int
}

func (h *rolloverHandler) Status(
	context.Context,
	*connect.Request[operatorv1.StatusRequest],
) (*connect.Response[operatorv1.StatusResponse], error) {
	h.mu.Lock()
	h.statusN++
	pod := "podA"
	if h.statusN > 1 {
		pod = "podB"
	}
	h.mu.Unlock()
	return connect.NewResponse(gameplayPool(pod)), nil
}

func (h *rolloverHandler) StreamPodLogs(
	ctx context.Context,
	req *connect.Request[operatorv1.StreamPodLogsRequest],
	stream *connect.ServerStream[operatorv1.StreamPodLogsResponse],
) error {
	switch req.Msg.GetPodName() {
	case "podA":
		// First open: deliver lines then clean EOF (scanner finished, no
		// error) — how the operator signals a terminated-but-was-present pod.
		// Second open: pod is gone -> CodeNotFound at open time.
		h.mu.Lock()
		h.podACalls++
		first := h.podACalls == 1
		h.mu.Unlock()
		if !first {
			return connect.NewError(connect.CodeNotFound, nil)
		}
		for _, l := range []struct{ ts, body string }{
			{"2026-09-07T10:00:01.000000000Z", "A1"},
			{"2026-09-07T10:00:02.000000000Z", "A2"},
			{"2026-09-07T10:00:03.000000000Z", "A3"},
		} {
			if err := sendPodLog(stream, l.ts, l.body); err != nil {
				return err
			}
		}
		return nil
	case "podB":
		// B_STARTUP is timestamped BEFORE podA's last delivered line A3
		// (10:00:03). During a maxSurge>=1 rolling update the replacement
		// pod boots before the old pod terminates, so its startup lines
		// predate the old pod's last delivered line.
		for _, l := range []struct{ ts, body string }{
			{"2026-09-07T10:00:00.500000000Z", "B_STARTUP"},
			{"2026-09-07T10:00:04.000000000Z", "B4"},
			{"2026-09-07T10:00:05.000000000Z", "B5"},
		} {
			if err := sendPodLog(stream, l.ts, l.body); err != nil {
				return err
			}
		}
		<-ctx.Done() // live follow with nothing more to say right now
		return ctx.Err()
	default:
		return connect.NewError(connect.CodeNotFound, nil)
	}
}

// TestRolloverDeliversReplacementPodStartup verifies the fix: when a pod
// rolls over to a replacement whose startup lines are timestamped before the
// old pod's last delivered line, those startup lines are delivered — not
// silently dropped by a stale `since` watermark carried across the rollover.
//
// Before the fix this test reproduces the data-loss: B_STARTUP (10:00:00.5)
// is filtered out by the `!ts.After(since)` filter in streamOne because
// `since` still holds podA's last timestamp (10:00:03), so only 5 lines
// arrive and the test times out waiting for the 6th.
func TestRolloverDeliversReplacementPodStartup(t *testing.T) {
	srv := newOperatorServer(&rolloverHandler{})
	defer srv.Close()

	c := NewClient(Config{OperatorEndpoint: srv.URL})
	got, _ := drainLogs(t, c, 6)

	require.Equal(t, []string{"A1", "A2", "A3", "B_STARTUP", "B4", "B5"}, got,
		"replacement pod startup lines timestamped before the old pod's last line must be delivered after the rollover")
	require.Contains(t, got, "B_STARTUP",
		"B_STARTUP must not be dropped by a stale watermark carried across the pod change")
}

// dedupHandler models a same-pod re-subscribe (the case `since` was designed
// for): a proxy idles out a live stream (clean EOF / RST), the client
// re-subscribes to the SAME pod, and the operator replays the tail history.
// The `since` watermark must suppress the replayed duplicates while still
// delivering new lines.
type dedupHandler struct {
	operatorv1connect.UnimplementedOperatorServiceHandler

	mu    sync.Mutex
	opens int
}

func (h *dedupHandler) Status(
	context.Context,
	*connect.Request[operatorv1.StatusRequest],
) (*connect.Response[operatorv1.StatusResponse], error) {
	return connect.NewResponse(gameplayPool("podA")), nil
}

func (h *dedupHandler) StreamPodLogs(
	ctx context.Context,
	req *connect.Request[operatorv1.StreamPodLogsRequest],
	stream *connect.ServerStream[operatorv1.StreamPodLogsResponse],
) error {
	if req.Msg.GetPodName() != "podA" {
		return connect.NewError(connect.CodeNotFound, nil)
	}
	h.mu.Lock()
	h.opens++
	n := h.opens
	h.mu.Unlock()
	if n == 1 {
		// First open: deliver history, then clean EOF (proxy idled the stream).
		for _, l := range []struct{ ts, body string }{
			{"2026-09-07T10:00:01.000000000Z", "A1"},
			{"2026-09-07T10:00:02.000000000Z", "A2"},
			{"2026-09-07T10:00:03.000000000Z", "A3"},
		} {
			if err := sendPodLog(stream, l.ts, l.body); err != nil {
				return err
			}
		}
		return nil
	}
	// Second open: re-subscribe to the SAME pod -> operator replays the tail.
	// Replayed A1/A2/A3 (ts <= since) must be deduped; only A4 (new) delivered.
	for _, l := range []struct{ ts, body string }{
		{"2026-09-07T10:00:01.000000000Z", "A1"},
		{"2026-09-07T10:00:02.000000000Z", "A2"},
		{"2026-09-07T10:00:03.000000000Z", "A3"},
		{"2026-09-07T10:00:04.000000000Z", "A4"},
	} {
		if err := sendPodLog(stream, l.ts, l.body); err != nil {
			return err
		}
	}
	<-ctx.Done()
	return ctx.Err()
}

// TestSamePodResubscribeDedupsReplayedHistory is the regression guard for the
// watermark's intended purpose: deduping replayed history on a same-pod
// re-subscribe. It must keep passing after the rollover fix so the per-pod
// watermark is still applied when the pod does NOT change.
func TestSamePodResubscribeDedupsReplayedHistory(t *testing.T) {
	srv := newOperatorServer(&dedupHandler{})
	defer srv.Close()

	c := NewClient(Config{OperatorEndpoint: srv.URL})
	got, _ := drainLogs(t, c, 4)

	require.Equal(t, []string{"A1", "A2", "A3", "A4"}, got,
		"same-pod re-subscribe must dedup replayed history via the since watermark while still delivering new lines")
}

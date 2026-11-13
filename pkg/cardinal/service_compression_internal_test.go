package cardinal

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A client that accepts gzip gets small events uncompressed and large ones compressed. The test
// reads the gRPC envelopes off the wire: a set first bit in an envelope's flags byte means
// compressed.
func TestEventStream_CompressesOnlyLargeEvents(t *testing.T) {
	fixture := newServiceFixture(t, testutils.NewRand(t), false)
	fixture.world.events.RegisterHandler(event.KindDefault, fixture.svc.publishDefaultEvent)
	mux, err := fixture.svc.newMux(authenticatorDev{}.authenticate)
	require.NoError(t, err)
	server := httptest.NewUnstartedServer(mux)
	server.Config.Protocols = h2cProtocols()
	server.Start()
	t.Cleanup(server.Close)

	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	wire := &recordingTransport{next: &http.Transport{Protocols: protocols}}
	t.Cleanup(wire.next.CloseIdleConnections)
	client := cardinalv1connect.NewCardinalServiceClient(&http.Client{Transport: wire}, server.URL, connect.WithGRPC())

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req := connect.NewRequest(&cardinalv1.StartEventStreamRequest{Subscriptions: []*cardinalv1.EventSubscription{{
		Address: fixture.world.address, Events: []string{"*"},
	}}})
	req.Header().Set(devPlayerIDHeader, "player-1")
	stream, err := client.StartEventStream(ctx, req)
	require.NoError(t, err)
	require.True(t, stream.Receive(), "initial message: %v", stream.Err())

	large := strings.Repeat("a large, repetitive event payload ", 64)
	for _, payload := range []event.Payload{testutils.SimpleEvent{Value: 7}, testutils.AnotherEvent{Data: large}} {
		fixture.world.events.Enqueue(event.Event{Kind: event.KindDefault, Payload: payload, Recipient: "player-1"})
	}
	fixture.world.dispatchEvents(context.Background())
	for _, name := range []string{"simple_event", "another_event"} {
		require.True(t, stream.Receive(), "%s: %v", name, stream.Err())
		require.Equal(t, name, stream.Msg().GetEvent().GetName())
	}
	assert.Equal(t, []bool{false, false, true}, wire.compressed(t), "initial, small and large message")
}

// recordingTransport keeps every response body byte the client reads.
type recordingTransport struct {
	next *http.Transport
	mu   sync.Mutex
	body bytes.Buffer
}

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := r.next.RoundTrip(req)
	if err == nil {
		resp.Body = &recordedBody{ReadCloser: resp.Body, r: r}
	}
	return resp, err
}

type recordedBody struct {
	io.ReadCloser

	r *recordingTransport
}

func (b *recordedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.r.mu.Lock()
	b.r.body.Write(p[:n])
	b.r.mu.Unlock()
	return n, err
}

// compressed reports, for each whole gRPC envelope read so far, whether it was compressed.
func (r *recordingTransport) compressed(t *testing.T) []bool {
	t.Helper()
	r.mu.Lock()
	data := bytes.Clone(r.body.Bytes())
	r.mu.Unlock()
	var flags []bool
	for len(data) >= 5 {
		n := int(binary.BigEndian.Uint32(data[1:5]))
		if len(data) < 5+n {
			break
		}
		flags = append(flags, data[0]&1 != 0)
		data = data[5+n:]
	}
	return flags
}

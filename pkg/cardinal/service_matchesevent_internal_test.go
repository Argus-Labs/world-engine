package cardinal

import (
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/authn"
	"connectrpc.com/connect"
	"connectrpc.com/validate"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
	"github.com/rotisserie/eris"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchesEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		subscription string
		eventName    string
		want         bool
	}{
		{subscription: "combat.hit", eventName: "combat.hit", want: true},
		{subscription: "combat.hit", eventName: "combat.miss", want: false},
		{subscription: "combat.hit", eventName: "combat", want: false},
		{subscription: "combat", eventName: "combat.hit", want: false},

		{subscription: "*", eventName: "combat.hit", want: true},
		{subscription: "*", eventName: "combat", want: true},
		{subscription: "*", eventName: "combat.a.b.c", want: true},
		{subscription: "*", eventName: "player_death", want: true},

		{subscription: ">", eventName: "combat.hit", want: true},
		{subscription: ">", eventName: "combat", want: true},
		{subscription: ">", eventName: "combat.a.b.c", want: true},
		{subscription: ">", eventName: "player_death", want: true},

		{subscription: "combat.>", eventName: "combat.hit", want: true},
		{subscription: "combat.>", eventName: "combat.a.b", want: true},
		{subscription: "combat.>", eventName: "combat", want: false},
		{subscription: "combat.>", eventName: "foo.hit", want: false},
		{subscription: "combat.>", eventName: "combatx.hit", want: false},
		{subscription: "a.b.>", eventName: "a.b.c", want: true},
		{subscription: "a.b.>", eventName: "a.b.c.d", want: true},
		{subscription: "a.b.>", eventName: "a.b", want: false},
		{subscription: "a.b.>", eventName: "a.c.d", want: false},
		{subscription: "a.b.>", eventName: "axb.c", want: false},

		{subscription: "combat.*", eventName: "combat.hit", want: true},
		{subscription: "combat.*", eventName: "combat.player_hit", want: true},
		{subscription: "combat.*", eventName: "combat.a.b", want: false},
		{subscription: "combat.*", eventName: "combat", want: false},
		{subscription: "combat.*", eventName: "foo.hit", want: false},
		{subscription: "combat.*", eventName: "combatx.hit", want: false},

		{subscription: "*.bar", eventName: "foo.bar", want: true},
		{subscription: "*.bar", eventName: "foo.baz", want: false},
		{subscription: "*.bar", eventName: "foo.bar.baz", want: false},
		{subscription: "*.bar", eventName: "bar.bar", want: true},

		{subscription: "*.*", eventName: "foo.bar", want: true},
		{subscription: "*.*", eventName: "foo.bar.baz", want: false},
		{subscription: "*.*", eventName: "foo", want: false},

		{subscription: "a.*.c", eventName: "a.b.c", want: true},
		{subscription: "a.*.c", eventName: "a.b.d", want: false},
		{subscription: "a.*.c", eventName: "a.b.c.d", want: false},
		{subscription: "a.*.c", eventName: "x.b.c", want: false},

		{subscription: "*.*.*", eventName: "a.b.c", want: true},
		{subscription: "*.*.*", eventName: "a.b", want: false},
		{subscription: "*.*.*", eventName: "a.b.c.d", want: false},

		{subscription: "player_death", eventName: "player_death", want: true},
		{subscription: "player_death", eventName: "player-spawn", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.subscription+"/"+tt.eventName, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, matchesEvent(tt.subscription, tt.eventName))
		})
	}
}

type dottedEvent struct {
	name  string
	value int64
}

func (e dottedEvent) Name() string { return e.name }

func (dottedEvent) SizeWire() int { return 8 }

func (e dottedEvent) AppendWire(b []byte) []byte {
	return binary.LittleEndian.AppendUint64(b, uint64(e.value))
}

func (dottedEvent) UnmarshalWire(b []byte) (any, error) {
	if len(b) < 8 {
		return nil, eris.New("dottedEvent: malformed wire bytes")
	}
	return dottedEvent{value: int64(binary.LittleEndian.Uint64(b[:8]))}, nil
}

type eventStreamFixture struct {
	fixture *serviceFixture
	server  *httptest.Server
	client  cardinalv1connect.CardinalServiceClient
}

func newEventStreamFixture(t *testing.T) *eventStreamFixture {
	t.Helper()
	prng := testutils.NewRand(t)
	fixture := newServiceFixture(t, prng, false)
	fixture.world.events.RegisterHandler(event.KindDefault, fixture.svc.publishDefaultEvent)

	mux := http.NewServeMux()
	authMiddleware := authn.NewMiddleware(authenticatorDev{}.authenticate)
	validateInterceptor := validate.NewInterceptor()
	path, handler := cardinalv1connect.NewCardinalServiceHandler(
		fixture.svc,
		connect.WithInterceptors(validateInterceptor),
	)
	mux.Handle(path, authMiddleware.Wrap(handler))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client := cardinalv1connect.NewCardinalServiceClient(server.Client(), server.URL)
	return &eventStreamFixture{fixture: fixture, server: server, client: client}
}

func (f *eventStreamFixture) openStream(
	t *testing.T,
	userID, pattern string,
) *connect.ServerStreamForClient[cardinalv1.StartEventStreamResponse] {
	t.Helper()
	req := connect.NewRequest(&cardinalv1.StartEventStreamRequest{
		Subscriptions: []*cardinalv1.EventSubscription{{
			Address: f.fixture.world.address,
			Events:  []string{pattern},
		}},
	})
	req.Header().Set("X-Email", userID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	stream, err := f.client.StartEventStream(ctx, req)
	require.NoError(t, err)
	require.True(t, stream.Receive(), "receive initial empty event")
	require.Empty(t, stream.Msg().GetEvent().GetName(), "first message should be the empty init event")
	return stream
}

func TestEventStream_StarWildcardReceivesSingleSegmentEvent(t *testing.T) {
	t.Parallel()
	f := newEventStreamFixture(t)

	starStream := f.openStream(t, "star-user", "combat.*")
	gtStream := f.openStream(t, "gt-user", "combat.>")

	require.NoError(t, f.fixture.svc.publishDefaultEvent(context.Background(), event.Event{
		Kind:    event.KindDefault,
		Payload: dottedEvent{name: "combat.player_hit", value: 42},
	}))

	require.True(t, starStream.Receive(), "combat.* subscriber should receive the event")
	require.Equal(t, "combat.player_hit", starStream.Msg().GetEvent().GetName())

	require.True(t, gtStream.Receive(), "combat.> subscriber should receive the event (no regression)")
	require.Equal(t, "combat.player_hit", gtStream.Msg().GetEvent().GetName())
}

func TestEventStream_StarWildcardDoesNotMatchMultiSegment(t *testing.T) {
	t.Parallel()
	f := newEventStreamFixture(t)

	starStream := f.openStream(t, "star-user", "combat.*")
	gtStream := f.openStream(t, "gt-user", "combat.>")

	require.NoError(t, f.fixture.svc.publishDefaultEvent(context.Background(), event.Event{
		Kind:    event.KindDefault,
		Payload: dottedEvent{name: "combat.nested.deep", value: 7},
	}))

	require.True(t, gtStream.Receive(), "combat.> should receive the multi-segment event")
	require.Equal(t, "combat.nested.deep", gtStream.Msg().GetEvent().GetName())

	starRecv := make(chan string, 1)
	go func() {
		if starStream.Receive() {
			starRecv <- starStream.Msg().GetEvent().GetName()
		}
	}()
	select {
	case name := <-starRecv:
		t.Fatalf("combat.* should not match a multi-segment event, but received %q", name)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestEventStream_BareWildcardReceivesNamespacedEvent(t *testing.T) {
	t.Parallel()
	f := newEventStreamFixture(t)

	starStream := f.openStream(t, "bare-star-user", "*")
	gtStream := f.openStream(t, "bare-gt-user", ">")

	require.NoError(t, f.fixture.svc.publishDefaultEvent(context.Background(), event.Event{
		Kind:    event.KindDefault,
		Payload: dottedEvent{name: "combat.player_hit", value: 99},
	}))

	require.True(t, starStream.Receive(), "bare '*' subscriber should receive the event")
	require.Equal(t, "combat.player_hit", starStream.Msg().GetEvent().GetName())

	require.True(t, gtStream.Receive(), "bare '>' subscriber should receive the event")
	require.Equal(t, "combat.player_hit", gtStream.Msg().GetEvent().GetName())
}

func TestEventStream_ExactMatchReceivesEvent(t *testing.T) {
	t.Parallel()
	f := newEventStreamFixture(t)

	exactStream := f.openStream(t, "exact-user", "combat.player_hit")
	otherStream := f.openStream(t, "other-user", "combat.miss")

	require.NoError(t, f.fixture.svc.publishDefaultEvent(context.Background(), event.Event{
		Kind:    event.KindDefault,
		Payload: dottedEvent{name: "combat.player_hit", value: 5},
	}))

	require.True(t, exactStream.Receive(), "exact-match subscriber should receive the event")
	require.Equal(t, "combat.player_hit", exactStream.Msg().GetEvent().GetName())

	otherRecv := make(chan string, 1)
	go func() {
		if otherStream.Receive() {
			otherRecv <- otherStream.Msg().GetEvent().GetName()
		}
	}()
	select {
	case name := <-otherRecv:
		t.Fatalf("non-matching exact subscription should not receive the event, but got %q", name)
	case <-time.After(300 * time.Millisecond):
	}
}

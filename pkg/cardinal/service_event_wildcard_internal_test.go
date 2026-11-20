package cardinal

import (
	"strconv"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type chatMessage struct{ Text string }

func (chatMessage) Name() string { return "chat.message" }

type chatThreadReply struct{ Text string }

func (chatThreadReply) Name() string { return "chat.thread.reply" }

// Event subscriptions use NATS-style wildcards: '*' matches exactly one dot-separated token and a
// trailing '>' matches one or more. A bare '*' or '>' receives every event.
func TestEventStream_SubscriptionWildcards(t *testing.T) {
	t.Parallel()
	h := newStreamHarness(t)
	marker := testutils.SimpleEvent{}.Name()

	cases := []struct {
		subscription string
		want         []string
	}{
		{"chat.message", []string{"chat.message"}},
		{"chat.*", []string{"chat.message"}},
		{"chat.*.reply", []string{"chat.thread.reply"}},
		{"chat.>", []string{"chat.message", "chat.thread.reply"}},
		{">", []string{"chat.message", "chat.thread.reply"}},
		{"*", []string{"chat.message", "chat.thread.reply"}},
	}

	streams := make([]*clientStream, len(cases))
	for i, c := range cases {
		streams[i] = h.open(t, "player-"+strconv.Itoa(i), c.subscription, marker)
		streams[i].read()
	}

	h.publish(t, chatMessage{Text: "hi"}, "")
	h.publish(t, chatThreadReply{Text: "re"}, "")
	h.publish(t, testutils.SimpleEvent{Value: 1}, "") // A stream delivers in order, so this marks the end.

	for i, c := range cases {
		t.Run(c.subscription, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.want, eventsBefore(t, streams[i], marker))
		})
	}
}

// eventsBefore returns the names of the events a stream delivers before the marker event.
func eventsBefore(t *testing.T, c *clientStream, marker string) []string {
	t.Helper()

	var names []string

	for {
		select {
		case msg := <-c.messages:
			switch name := msg.GetEvent().GetName(); name {
			case "": // Keepalive.
			case marker:
				return names
			default:
				names = append(names, name)
			}
		case <-c.done:
			require.FailNow(t, "stream ended early", "after %v: %v", names, c.err)
		case <-time.After(5 * time.Second):
			require.FailNow(t, "timed out waiting for the marker", "got %v", names)
		}
	}
}

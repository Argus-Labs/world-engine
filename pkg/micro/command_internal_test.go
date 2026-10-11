package micro

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestParseCommandSubject(t *testing.T) {
	t.Parallel()

	gameplay := GetAddress("us-west1", "argus", "rampage", "gameplay-3")
	platform := GetAddress("us-west1", "argus", "rampage", "platform")
	const subject = "us-west1.argus.rampage.gameplay-3.isc.plat_result.us-west1.argus.rampage.platform"

	require.Equal(t, subject, CommandRoute{To: gameplay, Name: "plat_result", From: platform}.Subject())

	route, err := ParseCommandSubject(subject)
	require.NoError(t, err)
	assert.True(t, proto.Equal(gameplay, route.To), "to = %v", route.To)
	assert.Equal(t, "plat_result", route.Name)
	assert.True(t, proto.Equal(platform, route.From), "from = %v", route.From)

	for _, bad := range []string{
		// The sender has three tokens.
		"us-west1.argus.rampage.gameplay-3.isc.plat_result.us-west1.argus.rampage",
		// The old "command" endpoint.
		"us-west1.argus.rampage.gameplay-3.command.plat_result.us-west1.argus.rampage.platform",
		// A sender with five tokens (the old realm-bearing address).
		"us-west1.argus.rampage.gameplay-3.isc.plat_result.us-west1.world.argus.rampage.platform",
		// A name that is not one subject token's worth of [a-zA-Z0-9_-].
		"us-west1.argus.rampage.gameplay-3.isc.plat$result.us-west1.argus.rampage.platform",
		// An uppercase service ID.
		"us-west1.argus.rampage.gameplay-3.isc.plat_result.us-west1.argus.rampage.Platform",
	} {
		_, err := ParseCommandSubject(bad)
		assert.Error(t, err, bad)
	}
}

func TestCommand_RoundTrip(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	svc, client := newTestService(t, prng)
	from := GetAddress("us-west1", "argus", "rampage", "sender-"+strconv.FormatUint(prng.Uint64(), 36))
	received := make(chan Command, 8)
	require.NoError(t, svc.AddCommand("greet", func(_ context.Context, cmd Command) error {
		received <- cmd
		switch string(cmd.Payload) {
		case "busy":
			return status.Error(codes.FailedPrecondition, "not now")
		case "boom":
			return errors.New("plain error")
		}
		return nil
	}))

	// Watch the raw subject to pin the wire format: the body is the payload and nothing else.
	wire, err := client.SubscribeSync(CommandRoute{To: svc.Address, Name: "greet", From: from}.Subject())
	require.NoError(t, err)
	require.NoError(t, client.Flush())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Accepted: the receiver sees the sender from the subject and the payload byte for byte.
	{
		require.NoError(t, client.SendCommand(ctx, from, svc.Address, "greet", []byte{0x08, 0x2a}))

		raw, err := wire.NextMsgWithContext(ctx)
		require.NoError(t, err)
		assert.Equal(t, String(svc.Address)+".isc.greet.us-west1.argus.rampage."+from.GetServiceId(), raw.Subject)
		assert.Equal(t, []byte{0x08, 0x2a}, raw.Data)

		cmd := <-received
		assert.Equal(t, "greet", cmd.Name)
		assert.True(t, proto.Equal(from, cmd.From), "from = %v", cmd.From)
		assert.Equal(t, []byte{0x08, 0x2a}, cmd.Payload)
	}

	// Rejected with a status code: the sender gets the code and message back.
	{
		err := client.SendCommand(ctx, from, svc.Address, "greet", []byte("busy"))
		assert.Equal(t, codes.FailedPrecondition, status.Code(err))
		assert.Equal(t, "not now", status.Convert(err).Message())
		<-received
	}

	// Rejected with a plain error: codes.Unknown.
	{
		err := client.SendCommand(ctx, from, svc.Address, "greet", []byte("boom"))
		assert.Equal(t, codes.Unknown, status.Code(err))
		assert.Equal(t, "plain error", status.Convert(err).Message())
		<-received
	}

	// A malformed sender is rejected before the handler, which still serves the next good command.
	{
		reply, err := client.RequestWithContext(
			ctx,
			String(svc.Address)+".isc.greet.us-west1.argus.rampage",
			[]byte("x"),
		)
		require.NoError(t, err)
		assert.Equal(t, "3", reply.Header.Get("Nats-Service-Error-Code")) // codes.InvalidArgument

		require.NoError(t, client.SendCommand(ctx, from, svc.Address, "greet", []byte("next")))
		assert.Equal(t, []byte("next"), (<-received).Payload, "handler saw the malformed command")
	}

	// A reply subject outside _INBOX. could turn the receiver's reply into a command it never sent, so
	// the command is dropped unhandled and nothing is published there.
	{
		victim := CommandRoute{To: from, Name: "privileged", From: svc.Address}.Subject()
		spoofed, err := client.SubscribeSync(victim)
		require.NoError(t, err)
		require.NoError(t, client.PublishMsg(&nats.Msg{
			Subject: CommandRoute{To: svc.Address, Name: "greet", From: from}.Subject(),
			Reply:   victim,
			Data:    []byte("spoof"),
		}))

		require.NoError(t, client.SendCommand(ctx, from, svc.Address, "greet", []byte("after-spoof")))
		assert.Equal(t, []byte("after-spoof"), (<-received).Payload, "handler saw the spoofing command")
		_, err = spoofed.NextMsg(100 * time.Millisecond)
		require.ErrorIs(t, err, nats.ErrTimeout, "receiver replied onto another service's command subject")
	}

	// Names must be one subject token, or one subscription would overlap another.
	{
		require.Error(t, svc.AddCommand("greet.us-west1", func(context.Context, Command) error { return nil }))
		require.Error(t, client.SendCommand(ctx, from, svc.Address, "greet.us-west1", nil))
	}

	// A name nobody serves has no responders.
	{
		err := client.SendCommand(ctx, from, svc.Address, "nobody-serves-this", nil)
		assert.ErrorIs(t, err, nats.ErrNoResponders)
	}
}

// TestCommand_InvalidRejectionCode checks that a reply claiming rejection with code 0 (OK) or an unknown
// code is still an error, not a silent success.
func TestCommand_InvalidRejectionCode(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	client := NewTestClient(t)
	to := RandServiceAddress(t, prng)
	from := RandServiceAddress(t, prng)
	for _, code := range []string{"0", "17", "x"} {
		name := "reject-" + code
		sub, err := client.Subscribe(CommandRoute{To: to, Name: name, From: from}.Subject(), func(msg *nats.Msg) {
			reply := nats.NewMsg(msg.Reply)
			reply.Header.Set("Nats-Service-Error-Code", code)
			reply.Header.Set("Nats-Service-Error", "rejected")
			_ = msg.RespondMsg(reply)
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })
	}
	require.NoError(t, client.Flush())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, code := range []string{"0", "17", "x"} {
		err := client.SendCommand(ctx, from, to, "reject-"+code, nil)
		assert.ErrorContains(t, err, `command rejected with invalid code "`+code+`": rejected`)
	}
}

package micro

import (
	"context"
	"math/rand/v2"
	"testing"
	"time"

	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
	"github.com/stretchr/testify/require"
)

const testCommandName = "test_command"

// iscFixture is a service serving one inter-shard command, plus a record of what its handler saw.
type iscFixture struct {
	service  *Service
	client   *Client
	sender   *ServiceAddress
	received chan *iscv1.Command
	handlErr error
}

func newISCFixture(t *testing.T, prng *rand.Rand) *iscFixture {
	t.Helper()

	service, client := newTestService(t, prng)
	fixture := &iscFixture{
		service:  service,
		client:   client,
		sender:   RandServiceAddress(t, prng),
		received: make(chan *iscv1.Command, 1),
	}

	require.NoError(t, service.ServeCommands([]string{testCommandName},
		func(_ context.Context, cmd *iscv1.Command) error {
			fixture.received <- cmd
			return fixture.handlErr
		}))
	require.NoError(t, client.Flush())

	return fixture
}

// handled returns the command the handler received, or fails if none arrived.
func (f *iscFixture) handled(t *testing.T) *iscv1.Command {
	t.Helper()

	select {
	case cmd := <-f.received:
		return cmd
	case <-time.After(2 * time.Second):
		t.Fatal("handler was never called")
		return nil
	}
}

// refused fails if the handler ran. Callers use it to prove a rejected command never reached the
// runtime behind the transport.
func (f *iscFixture) refused(t *testing.T) {
	t.Helper()

	select {
	case cmd := <-f.received:
		t.Fatalf("handler ran for a command it should have rejected: %v", cmd)
	case <-time.After(200 * time.Millisecond):
	}
}

func iscTestContext(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// TestServeCommands_DeliversValidCommand covers the happy path end to end: a command sent with
// Client.SendCommand reaches the handler intact, which also pins the subject convention — if the
// send and serve sides ever disagree on it, this never arrives.
func TestServeCommands_DeliversValidCommand(t *testing.T) {
	prng := rand.New(rand.NewPCG(1, 2))
	fixture := newISCFixture(t, prng)

	payload := []byte("payload-bytes")
	require.NoError(t, fixture.client.SendCommand(
		iscTestContext(t), fixture.service.Address, testCommandName, String(fixture.sender), payload))

	got := fixture.handled(t)
	require.Equal(t, testCommandName, got.GetName())
	require.Equal(t, String(fixture.sender), got.GetPersona().GetId())
	require.Equal(t, String(fixture.service.Address), String(got.GetAddress()))
	require.Equal(t, payload, got.GetPayload())
}

// TestServeCommands_RejectsNonShardPersona is the security boundary: inter-shard endpoints must be
// reachable only by shards. A caller presenting a player identity is refused before the handler
// runs, so a runtime behind this transport cannot mistake a player for a peer shard.
func TestServeCommands_RejectsNonShardPersona(t *testing.T) {
	prng := rand.New(rand.NewPCG(3, 4))
	fixture := newISCFixture(t, prng)

	for _, persona := range []string{"player@example.com", "not-a-shard", "1234"} {
		err := fixture.client.SendCommand(
			iscTestContext(t), fixture.service.Address, testCommandName, persona, nil)
		require.Error(t, err, "persona %q must be refused", persona)
		fixture.refused(t)
	}
}

// TestServeCommands_RejectsMismatchedAddress verifies a command addressed to a different shard is
// refused even when it is delivered to this one's subject.
func TestServeCommands_RejectsMismatchedAddress(t *testing.T) {
	prng := rand.New(rand.NewPCG(5, 6))
	fixture := newISCFixture(t, prng)
	elsewhere := RandServiceAddress(t, prng)

	cmd := &iscv1.Command{
		Name:    testCommandName,
		Address: elsewhere,
		Persona: &iscv1.Persona{Id: String(fixture.sender)},
	}

	_, err := fixture.client.Request(
		iscTestContext(t), fixture.service.Address, commandGroup+"."+testCommandName, cmd)
	require.Error(t, err)
	fixture.refused(t)
}

// TestServeCommands_HandlerErrorFailsSender verifies a runtime that cannot accept a command — a
// rejected transaction, a full queue — reports that back to the sending shard rather than
// acknowledging work it never did.
func TestServeCommands_HandlerErrorFailsSender(t *testing.T) {
	prng := rand.New(rand.NewPCG(7, 8))
	fixture := newISCFixture(t, prng)
	fixture.handlErr = eris.New("runtime refused the command")

	err := fixture.client.SendCommand(
		iscTestContext(t), fixture.service.Address, testCommandName, String(fixture.sender), nil)
	require.Error(t, err)
	fixture.handled(t) // the handler did run; it was its error that failed the send
}

// TestServeCommands_ServesPing verifies the liveness endpoint every shard is expected to answer.
func TestServeCommands_ServesPing(t *testing.T) {
	prng := rand.New(rand.NewPCG(9, 10))
	fixture := newISCFixture(t, prng)

	_, err := fixture.client.Request(iscTestContext(t), fixture.service.Address, pingEndpoint, nil)
	require.NoError(t, err)
}

// TestServeCommands_NilHandlerRejected verifies a wiring mistake fails at startup rather than
// producing a service that accepts commands and silently drops them.
func TestServeCommands_NilHandlerRejected(t *testing.T) {
	prng := rand.New(rand.NewPCG(11, 12))
	service, _ := newTestService(t, prng)

	require.Error(t, service.ServeCommands([]string{testCommandName}, nil))
}

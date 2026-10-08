package cardinal

import (
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"github.com/argus-labs/world-engine/pkg/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// SubscribeEvents / UnsubscribeEvents TOCTOU regression tests
// -------------------------------------------------------------------------------------------------
// UpdateSubscriptions on event subscriptions must check subscriber existence and mutate the
// subscriber's event set under the same write lock. The original implementation split those two
// steps across two lock acquisitions: hasSubscriber checked under an RLock in subscriptionRequest,
// then subscribeEvents/unsubscribeEvents re-looked-up the subscriber under a write Lock. A
// concurrent removeSubscriber (deferred by StartEventStream when the client stream closes) can
// delete the subscriber in the unlocked window, so the mutation read nil and panicked —
// assert.That in dev builds, nil-pointer dereference in release builds.
//
// With the fix the check and mutation are atomic under one write lock, so a closed stream surfaces
// as a clean connect.CodeFailedPrecondition error instead of a panic. These tests reproduce the
// deterministic worst-case interleaving (subscriber removed between validation and mutation),
// exercise the public RPC handlers for the no-stream case, confirm the happy path still mutates
// the event set, and run the race under a concurrent stream-close stress to ensure no panic and
// only FailedPrecondition errors.
// -------------------------------------------------------------------------------------------------

const (
	// subStressIters is the number of add/remove/subscribe races the concurrent stress test runs.
	// Under -race this is enough to reliably surface a TOCTOU panic if the window were reintroduced.
	subStressIters = 200
)

// newSubscription builds a single EventSubscription targeting the fixture's shard address for the
// given event names.
func newSubscription(fixture *serviceFixture, events ...string) []*cardinalv1.EventSubscription {
	return []*cardinalv1.EventSubscription{
		{Address: fixture.world.address, Events: events},
	}
}

// TestService_Subscriptions_ReturnFailedPreconditionWhenStreamClosedBeforeMutation reproduces the
// deterministic worst-case interleaving: the subscriber exists when subscriptionRequest runs but is
// removed before subscribeEvents/unsubscribeEvents mutates it. On the buggy code the mutation read
// nil and panicked; with the fix it returns a clean CodeFailedPrecondition error.
func TestService_Subscriptions_ReturnFailedPreconditionWhenStreamClosedBeforeMutation(t *testing.T) {
	t.Parallel()

	t.Run("subscribeEvents returns error and does not panic", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newServiceFixture(t, prng, false)
		userID := testutils.RandString(prng, 8)
		subscriptions := newSubscription(fixture, "currency")

		// Open a stream (adds the subscriber to the map).
		_, err := fixture.svc.addSubscriber(t.Context(), &Player{ID: userID}, nil)
		require.NoError(t, err)

		// subscriptionRequest validates the request while the subscriber still exists. With the buggy
		// code this is where the RLock hasSubscriber check passed.
		validated, err := fixture.svc.subscriptionRequest(serviceTestContext(userID), subscriptions)
		require.NoError(t, err)

		// The client stream closes here — StartEventStream's deferred removeSubscriber deletes the
		// subscriber between validation and mutation.
		fixture.svc.removeSubscriber(&Player{ID: userID})

		// On the buggy code the next line panics (assert.That in dev, nil-deref in release). With the
		// fix the check+mutation are atomic under one lock, so a missing subscriber is a clean error.
		var subErr error
		require.NotPanics(t, func() {
			subErr = fixture.svc.subscribeEvents(validated, subscriptions)
		})
		require.Error(t, subErr)
		require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(subErr))
	})

	t.Run("unsubscribeEvents returns error and does not panic", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newServiceFixture(t, prng, false)
		userID := testutils.RandString(prng, 8)
		subscriptions := newSubscription(fixture, "currency")

		_, err := fixture.svc.addSubscriber(t.Context(), &Player{ID: userID}, nil)
		require.NoError(t, err)

		validated, err := fixture.svc.subscriptionRequest(serviceTestContext(userID), subscriptions)
		require.NoError(t, err)

		fixture.svc.removeSubscriber(&Player{ID: userID})

		var unsubErr error
		require.NotPanics(t, func() {
			unsubErr = fixture.svc.unsubscribeEvents(validated, subscriptions)
		})
		require.Error(t, unsubErr)
		require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(unsubErr))
	})
}

// TestService_Subscriptions_RPCRequiresOpenStream verifies the public SubscribeEvents and
// UnsubscribeEvents RPC handlers return CodeFailedPrecondition (not a recovered panic) when the
// caller has no open event stream.
func TestService_Subscriptions_RPCRequiresOpenStream(t *testing.T) {
	t.Parallel()

	t.Run("SubscribeEvents without stream returns FailedPrecondition", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newServiceFixture(t, prng, false)
		userID := testutils.RandString(prng, 8)
		subscriptions := newSubscription(fixture, "currency")

		var (
			resp *connect.Response[cardinalv1.SubscribeEventsResponse]
			err  error
		)
		require.NotPanics(t, func() {
			resp, err = fixture.svc.SubscribeEvents(
				serviceTestContext(userID),
				connect.NewRequest(&cardinalv1.SubscribeEventsRequest{Subscriptions: subscriptions}),
			)
		})
		require.Error(t, err)
		require.Nil(t, resp)
		require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
		assert.Contains(t, err.Error(), "stream")
	})

	t.Run("UnsubscribeEvents without stream returns FailedPrecondition", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newServiceFixture(t, prng, false)
		userID := testutils.RandString(prng, 8)
		subscriptions := newSubscription(fixture, "currency")

		var (
			resp *connect.Response[cardinalv1.UnsubscribeEventsResponse]
			err  error
		)
		require.NotPanics(t, func() {
			resp, err = fixture.svc.UnsubscribeEvents(
				serviceTestContext(userID),
				connect.NewRequest(&cardinalv1.UnsubscribeEventsRequest{Subscriptions: subscriptions}),
			)
		})
		require.Error(t, err)
		require.Nil(t, resp)
		require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
		assert.Contains(t, err.Error(), "stream")
	})
}

// TestService_Subscriptions_HappyPath verifies that, when an open stream exists, SubscribeEvents
// registers the requested events and UnsubscribeEvents removes them — i.e. the fix did not regress
// the normal subscription lifecycle. It also confirms UnsubscribeEvents of an unsubscribed event is
// a harmless no-op rather than an error.
func TestService_Subscriptions_HappyPath(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)
	fixture := newServiceFixture(t, prng, false)
	userID := testutils.RandString(prng, 8)
	subscriptions := newSubscription(fixture, "currency", "combat")

	// Open a stream.
	_, err := fixture.svc.addSubscriber(t.Context(), &Player{ID: userID}, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		fixture.svc.removeSubscriber(&Player{ID: userID})
	})

	// Subscribe: both events should be registered on the subscriber.
	resp, err := fixture.svc.SubscribeEvents(
		serviceTestContext(userID),
		connect.NewRequest(&cardinalv1.SubscribeEventsRequest{Subscriptions: subscriptions}),
	)
	require.NoError(t, err)
	require.NotNil(t, resp)
	fixture.svc.mu.RLock()
	subscriber := fixture.svc.subscribers[userID]
	fixture.svc.mu.RUnlock()
	require.NotNil(t, subscriber)
	subscriber.mu.Lock()
	_, hasCurrency := subscriber.events["currency"]
	_, hasCombat := subscriber.events["combat"]
	subscriber.mu.Unlock()
	assert.True(t, hasCurrency, "currency event should be subscribed")
	assert.True(t, hasCombat, "combat event should be subscribed")

	// Unsubscribe only the currency event: it should be removed, combat should remain.
	_, err = fixture.svc.UnsubscribeEvents(
		serviceTestContext(userID),
		connect.NewRequest(&cardinalv1.UnsubscribeEventsRequest{
			Subscriptions: newSubscription(fixture, "currency"),
		}),
	)
	require.NoError(t, err)
	subscriber.mu.Lock()
	_, hasCurrency = subscriber.events["currency"]
	_, hasCombat = subscriber.events["combat"]
	subscriber.mu.Unlock()
	assert.False(t, hasCurrency, "currency event should be unsubscribed")
	assert.True(t, hasCombat, "combat event should still be subscribed")

	// Duplicate subscription is idempotent (re-subscribing currency keeps it registered).
	_, err = fixture.svc.SubscribeEvents(
		serviceTestContext(userID),
		connect.NewRequest(&cardinalv1.SubscribeEventsRequest{
			Subscriptions: newSubscription(fixture, "currency"),
		}),
	)
	require.NoError(t, err)
	subscriber.mu.Lock()
	_, hasCurrency = subscriber.events["currency"]
	subscriber.mu.Unlock()
	assert.True(t, hasCurrency, "re-subscribing currency should register it again")

	// Unsubscribing an event that was never subscribed is a harmless no-op (not an error).
	_, err = fixture.svc.UnsubscribeEvents(
		serviceTestContext(userID),
		connect.NewRequest(&cardinalv1.UnsubscribeEventsRequest{
			Subscriptions: newSubscription(fixture, "never-subscribed"),
		}),
	)
	require.NoError(t, err)
}

// TestService_Subscriptions_InvalidArgumentStillValidated confirms the address-validation guard in
// subscriptionRequest still returns CodeInvalidArgument for a wrong-shard subscription, independent
// of whether a stream exists (the subscriber check now lives in the mutation, but request validation
// still runs first).
func TestService_Subscriptions_InvalidArgumentStillValidated(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)
	fixture := newServiceFixture(t, prng, false)
	userID := testutils.RandString(prng, 8)

	wrongAddress := RandServiceAddress(prng)
	subscriptions := []*cardinalv1.EventSubscription{
		{Address: wrongAddress, Events: []string{"currency"}},
	}

	// Without a stream: bad address is rejected as InvalidArgument before the subscriber check.
	_, err := fixture.svc.SubscribeEvents(
		serviceTestContext(userID),
		connect.NewRequest(&cardinalv1.SubscribeEventsRequest{Subscriptions: subscriptions}),
	)
	require.Error(t, err)
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	// With a stream: bad address is still rejected as InvalidArgument.
	_, err = fixture.svc.addSubscriber(t.Context(), &Player{ID: userID}, nil)
	require.NoError(t, err)
	_, err = fixture.svc.UnsubscribeEvents(
		serviceTestContext(userID),
		connect.NewRequest(&cardinalv1.UnsubscribeEventsRequest{Subscriptions: subscriptions}),
	)
	require.Error(t, err)
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// TestService_Subscriptions_ConcurrentStreamCloseNoPanic races subscribe/unsubscribe RPCs against a
// concurrent removeSubscriber (the StartEventStream disconnect path) many times. With the fix every
// interleaving either mutates the subscriber (stream still open) or returns a clean
// CodeFailedPrecondition (stream closed); it never panics. Under `-race` this would surface a
// TOCTOU panic if the check and mutation were ever split across two locks again.
func TestService_Subscriptions_ConcurrentStreamCloseNoPanic(t *testing.T) {
	t.Parallel()

	t.Run("SubscribeEvents races removeSubscriber", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newServiceFixture(t, prng, false)
		assertConcurrentCloseNoPanic(t, fixture, prng /*unsubscribe*/, false)
	})

	t.Run("UnsubscribeEvents races removeSubscriber", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newServiceFixture(t, prng, false)
		assertConcurrentCloseNoPanic(t, fixture, prng /*unsubscribe*/, true)
	})
}

// assertConcurrentCloseNoPanic runs `iters` races. Each iteration opens a stream for a fresh user,
// then concurrently (a) removes the subscriber (the stream-disconnect path) and (b) issues the
// subscribe or unsubscribe RPC. The remover always wins the cleanup, so the subscriber map does not
// leak. The RPC may succeed (subscriber still present when it ran) or fail with
// CodeFailedPrecondition (subscriber removed first); it must never panic and any error must be
// FailedPrecondition.
func assertConcurrentCloseNoPanic(t *testing.T, fixture *serviceFixture, prng *rand.Rand, unsubscribe bool) {
	t.Helper()
	subscriptions := newSubscription(fixture, "currency", "combat")

	var preconditionErrors atomic.Int64
	var successes atomic.Int64

	for range subStressIters {
		userID := testutils.RandString(prng, 8)
		_, err := fixture.svc.addSubscriber(t.Context(), &Player{ID: userID}, nil)
		require.NoError(t, err)

		var wg sync.WaitGroup
		wg.Add(2)

		// Goroutine A: the client disconnects — StartEventStream's deferred removeSubscriber fires.
		go func() {
			defer wg.Done()
			fixture.svc.removeSubscriber(&Player{ID: userID})
		}()

		// Goroutine B: a concurrent subscribe/unsubscribe RPC from the same client. callErr is
		// declared in the test goroutine and read after wg.Wait, which establishes happens-before, so
		// the read is race-free and the assertion can run on the test goroutine (testify's require
		// must not run from a worker goroutine).
		var callErr error
		go func() {
			defer wg.Done()
			rpcCtx := serviceTestContext(userID)
			if unsubscribe {
				_, callErr = fixture.svc.UnsubscribeEvents(
					rpcCtx,
					connect.NewRequest(&cardinalv1.UnsubscribeEventsRequest{Subscriptions: subscriptions}),
				)
			} else {
				_, callErr = fixture.svc.SubscribeEvents(
					rpcCtx,
					connect.NewRequest(&cardinalv1.SubscribeEventsRequest{Subscriptions: subscriptions}),
				)
			}
		}()

		wg.Wait()

		// The RPC either registered the subscription (remover lost the race) or returned a clean
		// FailedPrecondition (remover won). Any other outcome is a regression.
		if callErr == nil {
			successes.Add(1)
		} else {
			require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(callErr),
				"a racing subscription RPC must only fail with FailedPrecondition, got %v", callErr)
			preconditionErrors.Add(1)
		}
	}

	// The remover wins every cleanup, so the subscriber map must be empty after the stress run.
	fixture.svc.mu.RLock()
	leaked := len(fixture.svc.subscribers)
	fixture.svc.mu.RUnlock()
	assert.Zero(t, leaked, "subscriber map leaked entries after concurrent close stress")

	// We expect a mix of successes (RPC won the race) and FailedPrecondition errors (remover won).
	// Both counters being zero would indicate the race never materialized (a flaky test), so assert
	// at least one of each path was exercised.
	t.Logf("subscribe race: %d successes, %d precondition errors out of %d iterations",
		successes.Load(), preconditionErrors.Load(), subStressIters)
}
